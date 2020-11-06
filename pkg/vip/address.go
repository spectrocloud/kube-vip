package vip

import (
	"sync"

	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"
)

const (
	defaultValidLft = 60
)

// Network is an interface that enable managing operations for a given IP
type Network interface {
	AddIP() error
	DeleteIP() error
	IsSet() (bool, error)
	IP() string
	SetIP(ip string) error
	Interface() string
	IsDNS() bool
	IsDDNS() bool
	DDNSHostName() string
	DNSName() string
}

// network - This allows network configuration
type network struct {
	mu sync.Mutex

	address *netlink.Addr
	link    netlink.Link

	dnsName string
	isDDNS  bool
}

// NewConfig will attempt to provide an interface to the kernel network configuration
func NewConfig(address string, iface string, isDDNS bool) (Network, error) {
	result := &network{}

	link, err := netlink.LinkByName(iface)
	if err != nil {
		return result, errors.Wrapf(err, "could not get link for interface '%s'", iface)
	}
	result.link = link

	if IsIP(address) {
		result.address, err = netlink.ParseAddr(address + "/32")
		if err != nil {
			return result, errors.Wrapf(err, "could not parse address '%s'", address)
		}
		return result, nil
	}

	// address is DNS
	result.isDDNS = isDDNS
	result.dnsName = address

	// try to resolve the address
	ip, err := lookupHost(address)
	if err != nil {
		// return early for ddns if no IP is allocated for the domain
		// when leader starts, should do get IP from DHCP for the domain
		if isDDNS {
			return result, nil
		}
		return nil, err
	}

	// we're able to resolve store this as the initial IP
	if result.address, err = netlink.ParseAddr(ip + "/32"); err != nil {
		return result, err
	}
	// set ValidLft so that the VIP expires if the DNS entry is updated, otherwise it'll be refreshed by the DNS prober
	result.address.ValidLft = defaultValidLft

	return result, err
}

//AddIP - Add an IP address to the interface
func (configurator *network) AddIP() error {
	if err := netlink.AddrReplace(configurator.link, configurator.address); err != nil {
		return errors.Wrap(err, "could not add ip")
	}
	return nil
}

//DeleteIP - Remove an IP address from the interface
func (configurator *network) DeleteIP() error {
	result, err := configurator.IsSet()
	if err != nil {
		return errors.Wrap(err, "ip check in DeleteIP failed")
	}

	// Nothing to delete
	if !result {
		return nil
	}

	if err = netlink.AddrDel(configurator.link, configurator.address); err != nil {
		return errors.Wrap(err, "could not delete ip")
	}

	return nil
}

// IsSet - Check to see if VIP is set
func (configurator *network) IsSet() (result bool, err error) {
	var addresses []netlink.Addr

	if configurator.address == nil {
		return false, nil
	}

	addresses, err = netlink.AddrList(configurator.link, 0)
	if err != nil {
		err = errors.Wrap(err, "could not list addresses")

		return
	}

	for _, address := range addresses {
		if address.Equal(*configurator.address) {
			return true, nil
		}
	}

	return false, nil
}

// SetIP updates the IP that is used
func (configurator *network) SetIP(ip string) error {
	configurator.mu.Lock()
	defer configurator.mu.Unlock()

	addr, err := netlink.ParseAddr(ip + "/32")
	if err != nil {
		return err
	}
	if configurator.address != nil && configurator.IsDNS() {
		addr.ValidLft = defaultValidLft
	}
	configurator.address = addr
	return nil
}

// IP - return the IP Address
func (configurator *network) IP() string {
	configurator.mu.Lock()
	defer configurator.mu.Unlock()

	return configurator.address.IP.String()
}

// DNSName return the configured dnsName when use DNS
func (configurator *network) DNSName() string {
	return configurator.dnsName
}

// IsDNS - when dnsName is configured
func (configurator *network) IsDNS() bool {
	return configurator.dnsName != ""
}

// IsDDNS - return true if use dynamic dns
func (configurator *network) IsDDNS() bool {
	return configurator.isDDNS
}

// DDNSHostName - return the hostname for dynamic dns
// when dDNSHostName is not empty, use DHCP to get IP for hostname: dDNSHostName
// it's expected that dynamic DNS should be configured so
// the fqdn for apiserver endpoint is dDNSHostName.{LocalDomain}
func (configurator *network) DDNSHostName() string {
	return getHostName(configurator.dnsName)
}

// Interface - return the Interface name
func (configurator *network) Interface() string {
	return configurator.link.Attrs().Name
}
