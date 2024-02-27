package vip

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"strings"
	"syscall"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

// LookupHost resolves dnsName and return an IP or an error
func lookupHost(dnsName string) (string, error) {
	addrs, err := net.LookupHost(dnsName)
	if err != nil {
		return "", err
	}
	if len(addrs) == 0 {
		return "", errors.Errorf("empty address for %s", dnsName)
	}
	return addrs[0], nil
}

// IsIP returns if address is an IP or not
func IsIP(address string) bool {
	ip := net.ParseIP(address)
	return ip != nil
}

// getHostName return the hostname from the fqdn
func getHostName(dnsName string) string {
	if dnsName == "" {
		return ""
	}

	fields := strings.Split(dnsName, ".")
	return fields[0]
}

// IsIPv4 returns true only if address is a valid IPv4 address
func IsIPv4(address string) bool {
	ip := net.ParseIP(address)
	if ip == nil {
		return false
	}
	return ip.To4() != nil
}

// IsIPv6 returns true only if address is a valid IPv6 address
func IsIPv6(address string) bool {
	ip := net.ParseIP(address)
	if ip == nil {
		return false
	}
	return ip.To4() == nil
}

// GetFullMask returns /32 for an IPv4 address and /128 for an IPv6 address
func GetFullMask(address string) (string, error) {
	if IsIPv4(address) {
		return "/32", nil
	}
	if IsIPv6(address) {
		return "/128", nil
	}
	return "", fmt.Errorf("failed to parse %s as either IPv4 or IPv6", address)
}

// GetDefaultGatewayInterface return default gateway interface link
func GetDefaultGatewayInterface() (*net.Interface, error) {
	routes, err := netlink.RouteList(nil, syscall.AF_INET)
	if err != nil {
		return nil, err
	}

	for _, route := range routes {
		if route.Dst == nil || route.Dst.String() == "0.0.0.0/0" {
			if route.LinkIndex <= 0 {
				return nil, errors.New("Found default route but could not determine interface")
			}
			return net.InterfaceByIndex(route.LinkIndex)
		}
	}

	return nil, errors.New("Unable to find default route")
}

// MonitorDefaultInterface monitor the default interface and catch the event of the default route
func MonitorDefaultInterface(ctx context.Context, defaultIF *net.Interface) error {
	routeCh := make(chan netlink.RouteUpdate)
	if err := netlink.RouteSubscribe(routeCh, ctx.Done()); err != nil {
		return fmt.Errorf("subscribe route failed, error: %w", err)
	}

	for {
		select {
		case r := <-routeCh:
			log.Debugf("type: %d, route: %+v", r.Type, r.Route)
			if r.Type == syscall.RTM_DELROUTE && (r.Dst == nil || r.Dst.String() == "0.0.0.0/0") && r.LinkIndex == defaultIF.Index {
				return fmt.Errorf("default route deleted and the default interface may be invalid")
			}
		case <-ctx.Done():
			return nil
		}
	}
}

func GenerateMac() (mac string) {
	buf := make([]byte, 3)
	_, err := rand.Read(buf)
	if err != nil {
		return
	}

	/**
	 * The first 3 bytes need to match a real manufacturer
	 * you can refer to the following lists for examples:
	 * - https://gist.github.com/aallan/b4bb86db86079509e6159810ae9bd3e4
	 * - https://macaddress.io/database-download
	 */
	mac = fmt.Sprintf("%s:%s:%s:%02x:%02x:%02x", "00", "00", "6C", buf[0], buf[1], buf[2])
	log.Infof("Generated mac: %s", mac)
	return mac
}
