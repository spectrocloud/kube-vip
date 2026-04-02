//go:build linux

package manager

import (
	"context"
	"net"
	"strings"
	"time"

	log "log/slog"

	"github.com/kube-vip/kube-vip/pkg/instance"
	"github.com/vishvananda/netlink"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// cleanupStaleKubeVipHostRoutes removes secondary IPv4 /32 and IPv6 /128 addresses from the
// kube-vip service interface. Primary prefixes (e.g. DHCP /18) are left intact.
// Configured control-plane/static VIPs (Address, VIP) are not removed.
// Service LB IPs from annotations are intentionally not preserved here—they should not
// stay on non-leaders once cleanup runs after lease loss.
// Duplicate /32 of the node IP are removed; Config has no separate NodeAddress field.
func (sm *Manager) cleanupStaleKubeVipHostRoutes() {
	iface := sm.config.Interface
	if sm.config.ServicesInterface != "" {
		iface = sm.config.ServicesInterface
	}
	if iface == "" {
		log.Debug("cleanup service host routes: no interface configured, skipping")
		return
	}

	link, err := netlink.LinkByName(iface)
	if err != nil {
		log.Error("cleanup: could not find interface", "iface", iface, "err", err)
		return
	}

	addrs, err := netlink.AddrList(link, netlink.FAMILY_ALL)
	if err != nil {
		log.Error("cleanup: failed to list addresses", "iface", iface, "err", err)
		return
	}

	preserve := sm.addrsToPreserveDuringInterfaceCleanup()
	primaryKeys := nonHostPrefixIPKeys(addrs)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	otherNodeIPs, selfIPs := sm.nodeInternalIPMaps(ctx)

	for i := range addrs {
		a := &addrs[i]
		ip := a.IP
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}

		ones, bits := a.Mask.Size()
		if bits == 0 || ones != bits {
			continue
		}

		// e.g. 10.x/18 + kube-vip added 10.x/32 — drop the host route (no K8s API needed).
		if hostRouteShadowsPrimaryPrefix(ip, primaryKeys) {
			log.Info("cleanup: removing host route shadowing primary prefix on same interface", "ip", ip.String(), "iface", iface)
			if err := netlink.AddrDel(link, a); err != nil {
				log.Warn("cleanup: failed to remove address", "ip", ip.String(), "iface", iface, "err", err)
			}
			continue
		}

		if preserve.contains(ip) {
			continue
		}

		if remove, why := sm.hostRouteRemovedForNodeIPRules(ip, otherNodeIPs, selfIPs); remove {
			log.Info("cleanup: removing host route", "ip", ip.String(), "iface", iface, "reason", why)
			if err := netlink.AddrDel(link, a); err != nil {
				log.Warn("cleanup: failed to remove address", "ip", ip.String(), "iface", iface, "err", err)
			}
			continue
		}

		log.Info("cleanup: removing kube-vip-style host route from interface", "ip", ip.String(), "iface", iface)
		if err := netlink.AddrDel(link, a); err != nil {
			log.Warn("cleanup: failed to remove address", "ip", ip.String(), "iface", iface, "err", err)
		}
	}
}

type ipPreserveSet []net.IP

func (s ipPreserveSet) contains(ip net.IP) bool {
	for _, p := range s {
		if p.Equal(ip) {
			return true
		}
	}
	return false
}

func (sm *Manager) addrsToPreserveDuringInterfaceCleanup() ipPreserveSet {
	var out ipPreserveSet
	if ip := net.ParseIP(sm.config.Address); ip != nil {
		out = append(out, ip)
	}
	if sm.config.VIP != "" {
		// Same semantics as pkg/vip/util.go Split: comma-separated, trim spaces.
		for _, part := range strings.Split(sm.config.VIP, ",") {
			part = strings.TrimSpace(trimHostRouteCIDR(part))
			if part == "" {
				continue
			}
			if ip := net.ParseIP(part); ip != nil {
				out = append(out, ip)
			}
		}
	}
	return out
}

func trimHostRouteCIDR(s string) string {
	for i, c := range s {
		if c == '/' {
			return s[:i]
		}
	}
	return s
}

// reconcileLeaderServiceHostRoutes removes IPv4/32 and IPv6/128 on the service interface that
// are not listed in configured preserves and not owned by any active ServiceInstance (annotations +
// per-VIPConfig addresses). Catches stragglers missed by event-only cleanup.
func (sm *Manager) reconcileLeaderServiceHostRoutes() {
	iface := sm.config.Interface
	if sm.config.ServicesInterface != "" {
		iface = sm.config.ServicesInterface
	}
	if iface == "" {
		return
	}

	link, err := netlink.LinkByName(iface)
	if err != nil {
		log.Error("reconcile: could not find interface", "iface", iface, "err", err)
		return
	}

	addrs, err := netlink.AddrList(link, netlink.FAMILY_ALL)
	if err != nil {
		log.Error("reconcile: failed to list addresses", "iface", iface, "err", err)
		return
	}

	expected := make(map[string]struct{})
	for _, ip := range sm.addrsToPreserveDuringInterfaceCleanup() {
		expected[ip.String()] = struct{}{}
	}

	insts := sm.svcProcessor.ServiceInstancesSnapshot()
	for _, inst := range insts {
		if inst == nil {
			continue
		}
		if inst.ServiceSnapshot != nil {
			addrsStr, _ := instance.FetchServiceAddresses(inst.ServiceSnapshot)
			for _, vs := range addrsStr {
				if vs == "0.0.0.0" || vs == "::" {
					continue
				}
				if ip := net.ParseIP(vs); ip != nil {
					expected[ip.String()] = struct{}{}
				}
			}
		}
		for _, cfg := range inst.VIPConfigs {
			if cfg == nil || cfg.VIP == "" {
				continue
			}
			if ip := net.ParseIP(cfg.VIP); ip != nil {
				expected[ip.String()] = struct{}{}
			}
		}
		if inst.DHCPInterfaceIPv4 != "" {
			if ip := net.ParseIP(inst.DHCPInterfaceIPv4); ip != nil {
				expected[ip.String()] = struct{}{}
			}
		}
		if inst.DHCPInterfaceIPv6 != "" {
			if ip := net.ParseIP(inst.DHCPInterfaceIPv6); ip != nil {
				expected[ip.String()] = struct{}{}
			}
		}
	}

	primaryKeys := nonHostPrefixIPKeys(addrs)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	otherNodeIPs, selfIPs := sm.nodeInternalIPMaps(ctx)

	for i := range addrs {
		a := &addrs[i]
		ip := a.IP
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		ones, bits := a.Mask.Size()
		if bits == 0 || ones != bits {
			continue
		}
		if hostRouteShadowsPrimaryPrefix(ip, primaryKeys) {
			log.Info("reconcile: removing host route shadowing primary prefix on same interface", "ip", ip.String(), "iface", iface)
			if err := netlink.AddrDel(link, a); err != nil {
				log.Warn("reconcile: failed to remove address", "ip", ip.String(), "iface", iface, "err", err)
			}
			continue
		}
		if remove, why := sm.hostRouteRemovedForNodeIPRules(ip, otherNodeIPs, selfIPs); remove {
			log.Info("reconcile: removing host route", "ip", ip.String(), "iface", iface, "reason", why)
			if err := netlink.AddrDel(link, a); err != nil {
				log.Warn("reconcile: failed to remove address", "ip", ip.String(), "iface", iface, "err", err)
			}
			continue
		}
		if _, ok := expected[ip.String()]; ok {
			continue
		}
		log.Info("reconcile: removing stray host route not in active service VIPs", "ip", ip.String(), "iface", iface)
		if err := netlink.AddrDel(link, a); err != nil {
			log.Warn("reconcile: failed to remove address", "ip", ip.String(), "iface", iface, "err", err)
		}
	}
}

func (sm *Manager) nodeInternalIPMaps(ctx context.Context) (other map[string]struct{}, self map[string]struct{}) {
	other = make(map[string]struct{})
	self = make(map[string]struct{})
	if sm.clientSet == nil || sm.config.NodeName == "" {
		return other, self
	}
	nodes, err := sm.clientSet.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Warn("node IP maps: list nodes failed; node-based cleanup rules skipped", "err", err)
		return other, self
	}
	for i := range nodes.Items {
		name := nodes.Items[i].Name
		dest := other
		if name == sm.config.NodeName {
			dest = self
		}
		for _, a := range nodes.Items[i].Status.Addresses {
			if a.Type != v1.NodeInternalIP {
				continue
			}
			if ip := net.ParseIP(a.Address); ip != nil {
				dest[ip.String()] = struct{}{}
			}
		}
	}
	return other, self
}

func (sm *Manager) hostRouteRemovedForNodeIPRules(ip net.IP, otherNodeIPs, selfIPs map[string]struct{}) (bool, string) {
	if _, ok := otherNodeIPs[ip.String()]; ok {
		return true, "matches_another_node_internal_ip"
	}
	if _, ok := selfIPs[ip.String()]; ok {
		return true, "duplicate_host_route_of_this_nodes_internal_ip"
	}
	return false, ""
}

// nonHostPrefixIPKeys returns IPs that have at least one non-host-route address on the link
// (e.g. /18). Used to find bogus secondary /32|/128 aliases of the same IP.
func nonHostPrefixIPKeys(addrs []netlink.Addr) map[string]struct{} {
	m := make(map[string]struct{})
	for i := range addrs {
		a := &addrs[i]
		ip := a.IP
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		ones, bits := a.Mask.Size()
		if bits == 0 || ones == bits {
			continue
		}
		m[ip.String()] = struct{}{}
	}
	return m
}

func hostRouteShadowsPrimaryPrefix(ip net.IP, primaryKeys map[string]struct{}) bool {
	_, ok := primaryKeys[ip.String()]
	return ok
}
