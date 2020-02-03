package node

import (
	"encoding/base64"
	"fmt"
	"net"
	"strings"

	"github.com/shark/hcloud-k3os-configurator/api"
)

// Role is the node role, i.e. server or agent
type Role string

const (
	// RoleServer means this is a server
	RoleServer = "server"

	// RoleAgent means this is an agent
	RoleAgent = "agent"
)

// Config is the static node configuration
type Config struct {
	Name                 string
	ShortName            string
	Role                 Role
	JoinURL              string
	PublicMAC            string
	IPv4Address          string
	IPv6Subnet           string
	IPv6Address          string
	PrivateIPv4Address   string
	FluxEnable           bool
	FluxGitURL           *string
	FluxGitPrivateKey    *string
	SealedSecretsEnable  bool
	SealedSecretsTLSCert *string
	SealedSecretsTLSKey  *string
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
	Type       string // ipv4 or ipv6
	DeviceName string
}

// GenerateConfig resolves the MAC address for the given public IPv4 and returns a Config struct for the node
func GenerateConfig(ipv4Address string, ipv6Subnet string, userConfig *api.UserConfig) (*Config, error) {
	mac, err := findMACForInterfaceWithIPV4Address(ipv4Address)
	if err != nil {
		return nil, fmt.Errorf("error finding MAC for interface with IP '%s': %w", ipv4Address, err)
	}
	ipv6Elems := strings.Split(ipv6Subnet, "/")
	if len(ipv6Elems) != 2 {
		return nil, fmt.Errorf("invalid IPv6 subnet: %s, expected CIDR notation", ipv6Subnet)
	}
	var fluxGitPrivateKeyB64 *string
	if userConfig.FluxGitPrivateKey != nil {
		inStr := *userConfig.FluxGitPrivateKey
		// append a newline to the private key if it does not have one (to make the SSH private key syntactically valid)
		if inStr[len(inStr)-1] != '\n' {
			inStr += "\n"
		}
		s := base64.StdEncoding.EncodeToString([]byte(inStr))
		fluxGitPrivateKeyB64 = &s
	}
	return &Config{
		PublicMAC:            mac,
		IPv4Address:          ipv4Address,
		IPv6Subnet:           ipv6Subnet,
		IPv6Address:          ipv6Elems[0],
		FluxEnable:           userConfig.FluxEnable,
		FluxGitURL:           userConfig.FluxGitURL,
		FluxGitPrivateKey:    fluxGitPrivateKeyB64,
		SealedSecretsEnable:  userConfig.SealedSecretsEnable,
		SealedSecretsTLSCert: userConfig.SealedSecretsTLSCert,
		SealedSecretsTLSKey:  userConfig.SealedSecretsTLSKey,
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
