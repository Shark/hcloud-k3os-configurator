package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/shark/hcloud-k3os-configurator/errorx"
)

const instanceMetadataBaseURL = "http://169.254.169.254"
const hetznerAPIBaseURL = "https://api.hetzner.cloud/v1"

// Client is the hcloud API client
type Client struct {
	hCloudToken string
	httpClient  *http.Client
}

// NewClient returns a new client with the given hcloud token
func NewClient(hCloudToken string) *Client {
	return &Client{
		hCloudToken: hCloudToken,
		httpClient:  &http.Client{Timeout: 3 * time.Second},
	}
}

// GetInstanceID retrieves the server's instance ID from the server metadata service
func (c *Client) GetInstanceID() (string, error) {
	resp, err := c.httpClient.Get(instanceMetadataBaseURL + "/hetzner/v1/metadata/instance-id")
	if err != nil {
		var neterr net.Error
		if errors.As(err, &neterr) && (neterr.Timeout() || neterr.Temporary()) {
			return "", &errorx.RetryableError{Message: "timeout or temporary error in HTTP request", Err: neterr}
		}
		return "", fmt.Errorf("error in http request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		err = fmt.Errorf("unexpected status code %d != 200", resp.StatusCode)
		if resp.StatusCode >= 500 && resp.StatusCode < 600 {
			return "", &errorx.RetryableError{Message: "retryable HTTP error", Err: err}
		}
		return "", err
	}
	instanceIDB, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %w", err)
	}
	return string(instanceIDB), nil
}

// GetUserData retrieves the user data field from the server metadata service
func (c *Client) GetUserData() (string, error) {
	resp, err := c.httpClient.Get(instanceMetadataBaseURL + "/latest/user-data")
	if err != nil {
		var neterr net.Error
		// TODO handle "dial tcp 169.254.169.254:80: connect: host is down" as a RetryableError
		if errors.As(err, &neterr) && (neterr.Timeout() || neterr.Temporary()) {
			return "", &errorx.RetryableError{Message: "timeout or temporary error in HTTP request", Err: neterr}
		}
		return "", fmt.Errorf("error in http request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("unexpected status code %d != 200", resp.StatusCode)
	}
	userDataB, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %w", err)
	}
	return string(userDataB), nil
}

// UserConfig represents the flux config in the user data
type UserConfig struct {
	Bootstrap bool `yaml:"bootstrap"`

	HCloudToken       string   `yaml:"hcloud_token"`
	K3OSToken         string   `yaml:"k3os_token"`
	SSHAuthorizedKeys []string `yaml:"ssh_authorized_keys"`

	BackupPassword        string `yaml:"backup_password"`
	BackupAccessKeyID     string `yaml:"backup_access_key_id"`
	BackupSecretAccessKey string `yaml:"backup_secret_access_key"`
	BackupRepositoryURL   string `yaml:"backup_repository_url"`

	FluxGitURL        *string `yaml:"flux_git_url"`
	FluxGitPrivateKey *string `yaml:"flux_git_private_key"`

	SealedSecretsTLSCert *string `yaml:"sealed_secrets_tls_cert"`
	SealedSecretsTLSKey  *string `yaml:"sealed_secrets_tls_key"`
}

// GetUserConfigFromUserData reads a user data string in YAML format and returns the flux config
func (c *Client) GetUserConfigFromUserData() (*UserConfig, error) {
	var (
		userDataStr string
		err         error
	)
	if userDataStr, err = c.GetUserData(); err != nil {
		return nil, fmt.Errorf("error getting user data: %w", err)
	}
	var userData UserConfig
	if err = yaml.Unmarshal([]byte(userDataStr), &userData); err != nil {
		return nil, fmt.Errorf("error unmarshalling YAML: %w", err)
	}
	if len(userData.HCloudToken) == 0 {
		return nil, fmt.Errorf("invalid: got zero-length HCloud token")
	}
	if len(userData.K3OSToken) == 0 {
		return nil, fmt.Errorf("invalid: got zero-length K3OS token")
	}
	if len(userData.BackupPassword) == 0 {
		return nil, fmt.Errorf("invalid: got empty BackupPassword")
	}
	if len(userData.BackupAccessKeyID) == 0 {
		return nil, fmt.Errorf("invalid: got empty BackupAccessKeyID")
	}
	if len(userData.BackupSecretAccessKey) == 0 {
		return nil, fmt.Errorf("invalid: got empty BackupSecretAccessKey")
	}
	if len(userData.BackupRepositoryURL) == 0 {
		return nil, fmt.Errorf("invalid: got empty BackupRepositoryURL")
	}
	if (userData.FluxGitURL != nil && userData.FluxGitPrivateKey == nil) || (userData.FluxGitPrivateKey != nil && userData.FluxGitURL == nil) {
		return nil, fmt.Errorf("invalid: flux_git_url and flux_git_private_key must be both set or both be null")
	}
	if (userData.SealedSecretsTLSCert != nil && userData.SealedSecretsTLSKey == nil) || (userData.SealedSecretsTLSKey != nil && userData.SealedSecretsTLSCert == nil) {
		return nil, fmt.Errorf("invalid: sealed_secrets_tls_cert and sealed_secrets_tls_key must be both set or both be null")
	}
	return &userData, nil
}

// Server represents a Hetzner Cloud Server
type Server struct {
	Name            string
	IPv4Address     string
	IPv6Subnet      string
	PrivateNetworks []*NetworkAssociation
	Labels          map[string]string
}

// IPv4Net returns the IPv4 network of this server
func (s *Server) IPv4Net() (*net.IPNet, error) {
	var ip net.IP
	if ip = net.ParseIP(s.IPv4Address); ip == nil {
		return nil, fmt.Errorf("error parsing IPv4Address '%s'", s.IPv4Address)
	}
	return &net.IPNet{
		IP:   ip,
		Mask: net.CIDRMask(32, 32),
	}, nil
}

// IPv6Net returns the IPv6 network of this server
func (s *Server) IPv6Net() (*net.IPNet, error) {
	var (
		ipnet *net.IPNet
		err   error
	)
	if _, ipnet, err = net.ParseCIDR(s.IPv6Subnet); err != nil {
		return nil, fmt.Errorf("error parsing IPv6Subnet '%s'", s.IPv6Subnet)
	}

	ipnet.IP[len(ipnet.IP)-1] = 1

	return ipnet, nil
}

// NetworkAssociation represents an association of a server to a network
type NetworkAssociation struct {
	ID         string
	ServerIP   string
	MACAddress string
}

// IPv4Net returns the IPv4 network of this network association
func (n *NetworkAssociation) IPv4Net() (*net.IPNet, error) {
	var ip net.IP
	if ip = net.ParseIP(n.ServerIP); ip == nil {
		return nil, fmt.Errorf("error parsing ServerIP '%s'", ip)
	}
	return &net.IPNet{
		IP:   ip,
		Mask: net.CIDRMask(24, 32),
	}, nil
}

// GetServer fetches a server resource from the Hetzner API
func (c *Client) GetServer(id string) (*Server, error) {
	req, err := http.NewRequest("GET", hetznerAPIBaseURL+"/servers/"+id, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Add("Authorization", "Bearer "+c.hCloudToken)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		var neterr net.Error
		if errors.As(err, &neterr) && (neterr.Timeout() || neterr.Temporary()) {
			return nil, &errorx.RetryableError{Message: "timeout or temporary error in HTTP request", Err: neterr}
		}
		return nil, fmt.Errorf("error in http request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code %d != 200", resp.StatusCode)
	}
	var rawServer struct {
		Server struct {
			Name      string `json:"name"`
			PublicNet struct {
				IPv4 struct {
					IP string `json:"ip"`
				} `json:"ipv4"`
				IPv6 struct {
					IP string `json:"ip"`
				} `json:"ipv6"`
			} `json:"public_net"`
			PrivateNet []struct {
				ID         uint64 `json:"network"`
				ServerIP   string `json:"ip"`
				MACAddress string `json:"mac_address"`
			} `json:"private_net"`
			Labels map[string]string `json:"labels"`
		} `json:"server"`
	}
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&rawServer)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling JSON: %w", err)
	}
	server := Server{
		Name:        rawServer.Server.Name,
		IPv4Address: rawServer.Server.PublicNet.IPv4.IP,
		IPv6Subnet:  rawServer.Server.PublicNet.IPv6.IP,
		Labels:      map[string]string{},
	}
	for _, net := range rawServer.Server.PrivateNet {
		server.PrivateNetworks = append(server.PrivateNetworks, &NetworkAssociation{
			ID:         strconv.FormatUint(net.ID, 10),
			ServerIP:   net.ServerIP,
			MACAddress: net.MACAddress,
		})
	}
	for k, v := range rawServer.Server.Labels {
		server.Labels[k] = v
	}
	return &server, nil
}

// GetServerWithRoleInCluster performs a search for a server with the label role and the given value and calls GetServer(id)
func (c *Client) GetServerWithRoleInCluster(role string, cluster string) (*Server, error) {
	req, err := http.NewRequest("GET", hetznerAPIBaseURL+"/servers", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	q := req.URL.Query()
	q.Add("label_selector", fmt.Sprintf("cluster==%s,role==%s", cluster, role))
	req.URL.RawQuery = q.Encode()
	req.Header.Add("Authorization", "Bearer "+c.hCloudToken)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		var neterr net.Error
		if errors.As(err, &neterr) && (neterr.Timeout() || neterr.Temporary()) {
			return nil, &errorx.RetryableError{Message: "timeout or temporary error in HTTP request", Err: neterr}
		}
		return nil, fmt.Errorf("error in http request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code %d != 200", resp.StatusCode)
	}
	var rawServers struct {
		Servers []struct {
			ID uint64 `json:"id"`
		}
	}
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&rawServers)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling JSON: %w", err)
	}
	if len(rawServers.Servers) != 1 {
		return nil, fmt.Errorf("could not find a server with role %s", role)
	}
	server, err := c.GetServer(strconv.FormatUint(rawServers.Servers[0].ID, 10))
	if err != nil {
		return nil, fmt.Errorf("error finding server with role %s (ID %d): %w", role, rawServers.Servers[0].ID, err)
	}
	return server, nil
}

// Network represents a Hetzner Cloud network
type Network struct {
	IPRange   string
	GatewayIP string
}

// GetNetwork fetches a network resource from the Hetzner API
func (c *Client) GetNetwork(id string) (*Network, error) {
	req, err := http.NewRequest("GET", hetznerAPIBaseURL+"/networks/"+id, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Add("Authorization", "Bearer "+c.hCloudToken)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		var neterr net.Error
		if errors.As(err, &neterr) && (neterr.Timeout() || neterr.Temporary()) {
			return nil, &errorx.RetryableError{Message: "timeout or temporary error in HTTP request", Err: neterr}
		}
		return nil, fmt.Errorf("error in http request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code %d != 200", resp.StatusCode)
	}
	var rawNetwork struct {
		Network struct {
			IPRange string `json:"ip_range"`
			Subnets []struct {
				Gateway string `json:"gateway"`
			} `json:"subnets"`
		} `json:"network"`
	}
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&rawNetwork)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling JSON: %w", err)
	}
	if len(rawNetwork.Network.Subnets) != 1 {
		return nil, fmt.Errorf("invalid number of subnets for network %s: %d != 1", id, len(rawNetwork.Network.Subnets))
	}
	network := Network{IPRange: rawNetwork.Network.IPRange, GatewayIP: rawNetwork.Network.Subnets[0].Gateway}
	return &network, nil
}

// FloatingIPType is the type of a floating IP, i.e. v4 or v6
type FloatingIPType int

const (
	// FloatingIPv4 means this is an IPv4 address
	FloatingIPv4 = iota

	// FloatingIPv6 means this is an IPv6 address
	FloatingIPv6
)

// FloatingIP represents a Hetzner Cloud floating ip
type FloatingIP struct {
	Type FloatingIPType
	IP   string // in case of IPv4, this is just the IP; with IPv6, this is in CIDR notation
}

// IPv4Net returns the IPv4 network of this floating IP
func (f *FloatingIP) IPv4Net() (*net.IPNet, error) {
	var ip net.IP
	if ip = net.ParseIP(f.IP); ip == nil {
		return nil, fmt.Errorf("error parsing IPv4Address '%s'", f.IP)
	}
	return &net.IPNet{
		IP:   ip,
		Mask: net.CIDRMask(32, 32),
	}, nil
}

// IPv6Net returns the IPv6 network of this floating IP
func (f *FloatingIP) IPv6Net() (*net.IPNet, error) {
	var (
		ipnet *net.IPNet
		err   error
	)
	if _, ipnet, err = net.ParseCIDR(f.IP); err != nil {
		return nil, fmt.Errorf("error parsing IPv6Subnet '%s'", f.IP)
	}

	ipnet.IP[len(ipnet.IP)-1] = 1

	return ipnet, nil
}

// GetFloatingIPsForCluster finds floating IPs that have a label with key 'cluster' and the given name
func (c *Client) GetFloatingIPsForCluster(name string) ([]*FloatingIP, error) {
	req, err := http.NewRequest("GET", hetznerAPIBaseURL+"/floating_ips", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	q := req.URL.Query()
	q.Add("label_selector", "cluster=="+name)
	req.URL.RawQuery = q.Encode()
	req.Header.Add("Authorization", "Bearer "+c.hCloudToken)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		var neterr net.Error
		if errors.As(err, &neterr) && (neterr.Timeout() || neterr.Temporary()) {
			return nil, &errorx.RetryableError{Message: "timeout or temporary error in HTTP request", Err: neterr}
		}
		return nil, fmt.Errorf("error in http request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code %d != 200", resp.StatusCode)
	}
	var rawFloatingIPs struct {
		FloatingIPs []struct {
			Type string `json:"type"`
			IP   string `json:"ip"`
		} `json:"floating_ips"`
	}
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&rawFloatingIPs)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling JSON: %w", err)
	}
	var floatingIPs []*FloatingIP
	for _, rawIP := range rawFloatingIPs.FloatingIPs {
		ip := &FloatingIP{IP: rawIP.IP}
		switch rawIP.Type {
		case "ipv4":
			ip.Type = FloatingIPv4
		case "ipv6":
			ip.Type = FloatingIPv6
		default:
			return nil, fmt.Errorf("unexpected IP type '%s'", rawIP.Type)
		}
		floatingIPs = append(floatingIPs, ip)
	}
	return floatingIPs, nil
}
