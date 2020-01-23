package node

import (
	"fmt"
	"net"
	"strings"
)

// Config is the static node configuration
type Config struct {
	Name        string
	PublicMAC   string
	IPv4Address string
	IPv6Subnet  string
	IPv6Address string
}

// PrivateNetwork is a network that the node is attached to
type PrivateNetwork struct {
	ID               string
	MAC              string
	IP               string
	NetworkIP        string
	PrefixLengthBits string
	GatewayIP        string
	DeviceName       string
}

// FloatingIP is a floating IP that the server may be attached to
type FloatingIP struct {
	IP         string
	DeviceName string
}

// GenerateConfig resolves the MAC address for the given public IPv4 and returns a Config struct for the node
func GenerateConfig(name string, ipv4Address string, ipv6Subnet string) (*Config, error) {
	mac, err := findMACForInterfaceWithIPV4Address(ipv4Address)
	if err != nil {
		return nil, fmt.Errorf("error finding MAC for interface with IP '%s': %w", ipv4Address, err)
	}
	ipv6Elems := strings.Split(ipv6Subnet, "/")
	if len(ipv6Elems) != 2 {
		return nil, fmt.Errorf("invalid IPv6 subnet: %s, expected CIDR notation", ipv6Subnet)
	}
	return &Config{
		Name:        name,
		PublicMAC:   mac,
		IPv4Address: ipv4Address,
		IPv6Subnet:  ipv6Subnet,
		IPv6Address: ipv6Elems[0],
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

// FindDeviceNameForMAC returns the device name (e.g. eth1) for a given MAC address
func FindDeviceNameForMAC(mac string) (string, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("error listing interfaces: %w", err)
	}
	for _, ifc := range ifs {
		if ifc.HardwareAddr.String() == mac {
			return ifc.Name, nil
		}
	}
	return "", fmt.Errorf("did not find interface with MAC %s", mac)
}
