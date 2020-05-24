package model

import "net"

// Role is the node role, i.e. master or agent
type Role string

const (
	// RoleMaster means this is a master
	RoleMaster = "master"

	// RoleAgent means this is an agent
	RoleAgent = "agent"
)

// HCloudK3OSConfig holds the node + cluster config
type HCloudK3OSConfig struct {
	NodeConfig    *NodeConfig    `yaml:"node_config"`
	ClusterConfig *ClusterConfig `yaml:"cluster_config"`
}

// NodeConfig is the config for this node
type NodeConfig struct {
	Name              string
	Role              Role
	PublicNetwork     *Network     `yaml:"public_network"`
	PrivateNetwork    *Network     `yaml:"private_networks"`
	FloatingIPs       []*IPAddress `yaml:"floating_ips"`
	SSHAuthorizedKeys []string     `yaml:"ssh_authorized_keys"`
}

// Network represents configuration of a network interface
type Network struct {
	NetDeviceName string       `yaml:"net_device_name"`
	GatewayIPv4   net.IP       `yaml:"gateway_ipv4"`
	GatewayIPv6   net.IP       `yaml:"gateway_ipv6"`
	IPv4Addresses []*IPAddress `yaml:"ipv4_addresses"`
	IPv6Addresses []*IPAddress `yaml:"ipv6_addresses"`
}

// IPAddress is an IP address on a network interface
type IPAddress struct {
	// Net contains the actual IP, not (only) the IP net
	Net       *net.IPNet
	IsPrimary bool `yaml:"is_primary"`
}

// CanonicalNet returns the actual network (if Net is 10.1.0.2/24, CanonicalNet is 10.1.0.0/24)
func (ip *IPAddress) CanonicalNet() (*net.IPNet, error) {
	var (
		n   *net.IPNet
		err error
	)
	if _, n, err = net.ParseCIDR(ip.Net.String()); err != nil {
		return nil, err
	}
	return n, nil
}

// ClusterConfig is the config for the whole cluster
type ClusterConfig struct {
	Bootstrap           bool                 `yaml:"bootstrap"`
	ClusterName         string               `yaml:"cluster_name"`
	HCloudToken         string               `yaml:"hcloud_token"`
	K3OSToken           string               `yaml:"k3os_token"`
	K3OSMasterJoinURL   string               `yaml:"k3os_master_join_url"`
	BackupConfig        *BackupConfig        `yaml:"backup_config"`
	FluxConfig          *FluxConfig          `yaml:"flux_config"`
	SealedSecretsConfig *SealedSecretsConfig `yaml:"sealed_secrets_config"`
}

// BackupConfig is the restic config
type BackupConfig struct {
	Password        string `yaml:"password"`
	AccessKeyID     string `yaml:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key"`
	RepositoryURL   string `yaml:"repository_url"`
}

// FluxConfig is the flux CD config
type FluxConfig struct {
	GitURL        string `yaml:"git_url"`
	GitPrivateKey string `yaml:"git_private_key"`
}

// SealedSecretsConfig is the Sealed Secrets config
type SealedSecretsConfig struct {
	TLSCert string `yaml:"tls_cert"`
	TLSKey  string `yaml:"tls_key"`
}
