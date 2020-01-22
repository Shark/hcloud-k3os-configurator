package node

import (
	"fmt"
	"net"
)

// Config is the static node configuration
type Config struct {
	Name        string
	PublicMAC   string
	IPv4Address string
	IPv6Subnet  string
}

// PrivateNetwork is a network that the node is attached to
type PrivateNetwork struct {
	ID           string
	MAC          string
	IP           string
	NetworkIP    string
	PrefixLength string
}

// FloatingIP is a floating IP that the server may be attached to
type FloatingIP struct {
	IP string
}

// GenerateConfig resolves the MAC address for the given public IPv4 and returns a Config struct for the node
func GenerateConfig(name string, ipv4Address string, ipv6Subnet string) (*Config, error) {
	mac, err := findMACForInterfaceWithIPV4Address(ipv4Address)
	if err != nil {
		return nil, fmt.Errorf("error finding MAC for interface with IP '%s': %w", ipv4Address, err)
	}
	return &Config{
		Name:        name,
		PublicMAC:   mac,
		IPv4Address: ipv4Address,
		IPv6Subnet:  ipv6Subnet,
	}, nil
}

func findMACForInterfaceWithIPV4Address(ip string) (string, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("error listing interfaces: %w", err)
	}
	for _, ifc := range ifs {
		addrs, err := ifc.Addrs()
		if err != nil {
			return "", fmt.Errorf("error getting addrs for interface '%s': %w", ifc.Name, err)
		}
		for _, addr := range addrs {
			if addr.String() == ip+"/32" {
				return ifc.HardwareAddr.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no interface with IP '%s'", ip)
}
