//go:build linux

package manager

import (
	"net"
	"strings"

	log "log/slog"

	"github.com/vishvananda/netlink"
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

		if preserve.contains(ip) {
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
