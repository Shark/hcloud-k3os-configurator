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
)

const instanceMetadataBaseURL = "http://169.254.169.254"
const hetznerAPIBaseURL = "https://api.hetzner.cloud/v1"

// RetryableError represents a temporary error which tells the client that the operation may succeed when retried.
type RetryableError struct {
	Message string
	Err     error
}

// Error is the error message
func (e *RetryableError) Error() string {
	return fmt.Sprintf("retryable error: %s: %s", e.Message, e.Err)
}

// Unwrap makes this error conformant with Go 1.13 errors
func (e *RetryableError) Unwrap() error {
	return e.Err
}

// GetInstanceID retrieves the server's instance ID from the server metadata service
func GetInstanceID() (string, error) {
	hc := &http.Client{Timeout: 3 * time.Second}
	resp, err := hc.Get(instanceMetadataBaseURL + "/hetzner/v1/metadata/instance-id")
	if err != nil {
		var neterr net.Error
		if errors.As(err, &neterr) && (neterr.Timeout() || neterr.Temporary()) {
			return "", &RetryableError{Message: "timeout or temporary error in HTTP request", Err: neterr}
		}
		return "", fmt.Errorf("error in http request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		err = fmt.Errorf("unexpected status code %d != 200", resp.StatusCode)
		if resp.StatusCode >= 500 && resp.StatusCode < 600 {
			return "", &RetryableError{Message: "retryable HTTP error", Err: err}
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
func GetUserData() (string, error) {
	hc := &http.Client{Timeout: 3 * time.Second}
	resp, err := hc.Get(instanceMetadataBaseURL + "/latest/user-data")
	if err != nil {
		var neterr net.Error
		// TODO handle "dial tcp 169.254.169.254:80: connect: host is down" as a RetryableError
		if errors.As(err, &neterr) && (neterr.Timeout() || neterr.Temporary()) {
			return "", &RetryableError{Message: "timeout or temporary error in HTTP request", Err: neterr}
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
	FluxEnable        bool
	FluxGitURL        *string
	FluxGitPrivateKey *string

	SealedSecretsEnable  bool
	SealedSecretsTLSCert *string
	SealedSecretsTLSKey  *string
}

// GetUserConfigFromUserData reads a user data string in YAML format and returns the flux config
func GetUserConfigFromUserData() (f *UserConfig, err error) {
	var userData string
	if userData, err = GetUserData(); err != nil {
		return nil, fmt.Errorf("error getting user data: %w", err)
	}
	var rawUserData struct {
		Flux struct {
			Enable        bool    `yaml:"enable"`
			GitURL        *string `yaml:"git_url"`
			GitPrivateKey *string `yaml:"git_private_key"`
		} `yaml:"flux"`
		SealedSecrets struct {
			Enable  bool    `yaml:"enable"`
			TLSCert *string `yaml:"tls_cert"`
			TLSKey  *string `yaml:"tls_key"`
		} `yaml:"sealed_secrets"`
	}
	if err = yaml.Unmarshal([]byte(userData), &rawUserData); err != nil {
		return nil, fmt.Errorf("error unmarshalling YAML: %w", err)
	}
	f = &UserConfig{FluxEnable: false, SealedSecretsEnable: false}
	if rawUserData.Flux.Enable {
		f.FluxEnable = true
		f.FluxGitPrivateKey = rawUserData.Flux.GitPrivateKey
		if rawUserData.Flux.GitURL == nil {
			return nil, errors.New("invalid: got nil git_url though flux is enabled")
		}
		f.FluxGitURL = rawUserData.Flux.GitURL
	}
	if rawUserData.SealedSecrets.Enable {
		f.SealedSecretsEnable = true
		if rawUserData.SealedSecrets.TLSCert == nil {
			return nil, errors.New("invalid: got nil tls_cert though Sealed Secrets is enabled")
		}
		f.SealedSecretsTLSCert = rawUserData.SealedSecrets.TLSCert
		if rawUserData.SealedSecrets.TLSKey == nil {
			return nil, errors.New("invalid: got nil tls_key though Sealed Secrets is enabled")
		}
		f.SealedSecretsTLSKey = rawUserData.SealedSecrets.TLSKey
	}
	return f, nil
}

// Server represents a Hetzner Cloud Server
type Server struct {
	Name            string
	IPv4Address     string
	IPv6Subnet      string
	PrivateNetworks []*NetworkAssociation
	Labels          map[string]string
}

// NetworkAssociation represents an association of a server to a network
type NetworkAssociation struct {
	ID         string
	ServerIP   string
	MACAddress string
}

// GetServer fetches a server resource from the Hetzner API
func GetServer(id string, token string) (*Server, error) {
	req, err := http.NewRequest("GET", hetznerAPIBaseURL+"/servers/"+id, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Add("Authorization", "Bearer "+token)
	hc := http.Client{Timeout: 3 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		var neterr net.Error
		if errors.As(err, &neterr) && (neterr.Timeout() || neterr.Temporary()) {
			return nil, &RetryableError{Message: "timeout or temporary error in HTTP request", Err: neterr}
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

// GetServerWithRole performs a search for a server with the label role and the given value and calls GetServer(id)
func GetServerWithRole(role string, token string) (*Server, error) {
	req, err := http.NewRequest("GET", hetznerAPIBaseURL+"/servers", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	q := req.URL.Query()
	q.Add("label_selector", "role=="+role)
	req.URL.RawQuery = q.Encode()
	req.Header.Add("Authorization", "Bearer "+token)
	hc := http.Client{Timeout: 3 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		var neterr net.Error
		if errors.As(err, &neterr) && (neterr.Timeout() || neterr.Temporary()) {
			return nil, &RetryableError{Message: "timeout or temporary error in HTTP request", Err: neterr}
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
	server, err := GetServer(strconv.FormatUint(rawServers.Servers[0].ID, 10), token)
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
func GetNetwork(id string, token string) (*Network, error) {
	req, err := http.NewRequest("GET", hetznerAPIBaseURL+"/networks/"+id, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Add("Authorization", "Bearer "+token)
	hc := http.Client{Timeout: 3 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		var neterr net.Error
		if errors.As(err, &neterr) && (neterr.Timeout() || neterr.Temporary()) {
			return nil, &RetryableError{Message: "timeout or temporary error in HTTP request", Err: neterr}
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

// GetFloatingIPsForCluster finds floating IPs that have a label with key 'cluster' and the given name
func GetFloatingIPsForCluster(name string, token string) ([]*FloatingIP, error) {
	req, err := http.NewRequest("GET", hetznerAPIBaseURL+"/floating_ips", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	q := req.URL.Query()
	q.Add("label_selector", "cluster=="+name)
	req.URL.RawQuery = q.Encode()
	req.Header.Add("Authorization", "Bearer "+token)
	hc := http.Client{Timeout: 3 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		var neterr net.Error
		if errors.As(err, &neterr) && (neterr.Timeout() || neterr.Temporary()) {
			return nil, &RetryableError{Message: "timeout or temporary error in HTTP request", Err: neterr}
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
