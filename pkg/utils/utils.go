package utils

import (
	"fmt"
	"net"
	"os"
	"strings"
)

func FileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// FormatIPWithSubnetMask takes a raw IP address and a subnet mask, and returns a formatted string in CIDR notation.
func FormatIPWithSubnetMask(rawIP string, subnetMask string) (string, error) {

	addr := fmt.Sprintf("%s/%s", rawIP, subnetMask)
	// Check if the input is valid
	_, _, err := net.ParseCIDR(addr)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR: %q, %w", addr, err)
	}
	return addr, nil
}

func GenerateCidrRange(address string) (string, error) {
	var cidrs []string

	addresses := strings.Split(address, ",")
	for _, a := range addresses {
		ip := net.ParseIP(a)
		if ip == nil {
			fmt.Println("JAYESH TEST: attempting DNS resolution for: ", a)
			ips, err := net.LookupIP(a)
			if len(ips) == 0 || err != nil {
				fmt.Println("JAYESH TEST: DNS resolution failed for: ", a, " error: ", err)
				return "", fmt.Errorf("invalid IP address: %s from [%s], %v", a, address, err)
			}
			ip = ips[0]
			fmt.Println("JAYESH TEST: DNS resolution successful for: ", a, " resolved to: ", ip.String())
		}

		if ip.To4() != nil {
			cidrs = append(cidrs, "32")
		} else {
			cidrs = append(cidrs, "128")
		}
	}

	fmt.Println("JAYESH TEST: CIDRs generated: ", cidrs, " from ", address)
	return strings.Join(cidrs, ","), nil
}

func GetSubnet(address string) string {
	ip := net.ParseIP(address)

	if ip.To4() != nil {
		return "32"
	}

	return "128"
}
