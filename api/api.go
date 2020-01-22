package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"
)

const instanceMetadataBaseURL = "http://169.254.169.254"
const hetznerAPIBaseURL = "https://api.hetzner.cloud/v1"

// GetInstanceID retrieves the server's instance ID from the server metadata service
func GetInstanceID() (string, error) {
	hc := &http.Client{Timeout: 10 * time.Second}
	resp, err := hc.Get(instanceMetadataBaseURL + "/hetzner/v1/metadata/instance-id")
	if err != nil {
		return "", fmt.Errorf("error in http request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("unexpected status code %d != 200", resp.StatusCode)
	}
	instanceIDB, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %w", err)
	}
	return string(instanceIDB), nil
}

// GetUserData retrieves the user data field from the server metadata service
func GetUserData() (string, error) {
	hc := &http.Client{Timeout: 10 * time.Second}
	resp, err := hc.Get(instanceMetadataBaseURL + "/latest/user-data")
	if err != nil {
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
	hc := http.Client{Timeout: 10 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
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

// Network represents a Hetzner Cloud network
type Network struct {
	IPRange string
}

// GetNetwork fetches a network resource from the Hetzner API
func GetNetwork(id string, token string) (*Network, error) {
	req, err := http.NewRequest("GET", hetznerAPIBaseURL+"/networks/"+id, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Add("Authorization", "Bearer "+token)
	hc := http.Client{Timeout: 10 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error in http request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code %d != 200", resp.StatusCode)
	}
	var rawNetwork struct {
		Network struct {
			IPRange string `json:"ip_range"`
		} `json:"network"`
	}
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&rawNetwork)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling JSON: %w", err)
	}
	network := Network{IPRange: rawNetwork.Network.IPRange}
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
	hc := http.Client{Timeout: 10 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
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
