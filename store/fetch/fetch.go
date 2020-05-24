package fetch

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/avast/retry-go"

	"github.com/shark/hcloud-k3os-configurator/api"
	"github.com/shark/hcloud-k3os-configurator/errorx"
	"github.com/shark/hcloud-k3os-configurator/model"
)

// Run fetches all necessary resources from the HCloud API and generates the HCloudK3OSConfig
func Run() (*model.HCloudK3OSConfig, error) {
	// *****
	// FETCH
	// *****

	// Fetch UserData
	var (
		userConfig *api.UserConfig
		instanceID string
		val        string
		ok         bool
		err        error
	)
	if err = retry.Do(func() error {
		if userConfig, err = api.NewClient("").GetUserConfigFromUserData(); err != nil {
			if !errors.Is(err, &errorx.RetryableError{}) {
				return retry.Unrecoverable(err)
			}
			return err
		}
		return nil
	}, retry.Delay(1*time.Second)); err != nil {
		return nil, fmt.Errorf("error reading user config from user data: %v", err)
	}

	// Fetch InstanceID
	if err = retry.Do(func() error {
		if instanceID, err = api.NewClient("").GetInstanceID(); err != nil {
			if !errors.Is(err, &errorx.RetryableError{}) {
				return retry.Unrecoverable(err)
			}
			return err
		}
		return nil
	}, retry.Delay(1*time.Second)); err != nil {
		return nil, fmt.Errorf("error getting instance ID: %v", err)
	}

	// Fetch Server
	var (
		apiClient   *api.Client
		server      *api.Server
		clusterName string
	)
	{
		apiClient = api.NewClient(userConfig.HCloudToken)
	}
	if err = retry.Do(func() error {
		if server, err = apiClient.GetServer(instanceID); err != nil {
			if !errors.Is(err, &errorx.RetryableError{}) {
				return retry.Unrecoverable(err)
			}
			return err
		}
		return nil
	}, retry.Delay(1*time.Second)); err != nil {
		return nil, fmt.Errorf("error getting server: %v", err)
	}
	if clusterName, ok = server.Labels["cluster"]; !ok {
		return nil, fmt.Errorf("this server does not have a 'cluster' label: %#v", *server)
	}

	// Fetch PrivateNetworks
	networks := make(map[string]*api.Network)
	for _, network := range server.PrivateNetworks {
		var thisNetwork *api.Network
		if err = retry.Do(func() error {
			if thisNetwork, err = apiClient.GetNetwork(network.ID); err != nil {
				if !errors.Is(err, &errorx.RetryableError{}) {
					return retry.Unrecoverable(err)
				}
				return err
			}
			return nil
		}, retry.Delay(1*time.Second)); err != nil {
			return nil, fmt.Errorf("error getting network ID %s: %v", network.ID, err)
		}
		networks[network.ID] = thisNetwork
	}

	// Fetch FloatingIPs
	var floatingIPs []*api.FloatingIP
	if err = retry.Do(func() error {
		if floatingIPs, err = apiClient.GetFloatingIPsForCluster(clusterName); err != nil {
			if !errors.Is(err, &errorx.RetryableError{}) {
				return retry.Unrecoverable(err)
			}
			return err
		}
		return nil
	}, retry.Delay(1*time.Second)); err != nil {
		return nil, fmt.Errorf("error getting floating IPs for cluster '%s': %v", clusterName, err)
	}

	// Fetch MasterServer
	var masterServer *api.Server
	if err = retry.Do(func() error {
		if masterServer, err = apiClient.GetServerWithRoleInCluster("master", clusterName); err != nil {
			if !errors.Is(err, &errorx.RetryableError{}) {
				return retry.Unrecoverable(err)
			}
			return err
		}
		return nil
	}, retry.Delay(1*time.Second)); err != nil {
		return nil, fmt.Errorf("error getting master server for cluster '%s': %v", clusterName, err)
	}

	// ********
	// GENERATE
	// ********
	var (
		cfg = &model.HCloudK3OSConfig{
			NodeConfig: &model.NodeConfig{
				PublicNetwork:  &model.Network{},
				PrivateNetwork: &model.Network{},
				FloatingIPs:    []*model.IPAddress{},
			},
			ClusterConfig: &model.ClusterConfig{},
		}
		pubipv4net  *net.IPNet
		pubipv6net  *net.IPNet
		privipv4net *net.IPNet
		ipv6gw      net.IP
	)

	// NodeConfig
	if val, ok = server.Labels["node_name"]; ok {
		cfg.NodeConfig.Name = val
	} else {
		return nil, fmt.Errorf("server %v needs a label 'node_name'", server)
	}

	if val, ok = server.Labels["role"]; ok {
		switch val {
		case model.RoleMaster:
			cfg.NodeConfig.Role = model.RoleMaster
		case model.RoleAgent:
			cfg.NodeConfig.Role = model.RoleAgent
		default:
			return nil, fmt.Errorf("unexpected role '%s'", val)
		}
	}

	if pubipv4net, err = server.IPv4Net(); err != nil {
		return nil, fmt.Errorf("error getting public IPv4Net: %v", err)
	}

	if pubipv6net, err = server.IPv6Net(); err != nil {
		return nil, fmt.Errorf("error getting public IPv6Net: %v", err)
	}

	if ipv6gw = net.ParseIP("fe80::1"); ipv6gw == nil {
		return nil, fmt.Errorf("error parsing ipv6gw: %v", err)
	}

	cfg.NodeConfig.PublicNetwork.IPv4Addresses = []*model.IPAddress{{
		Net:       pubipv4net,
		IsPrimary: true,
	}}
	cfg.NodeConfig.PublicNetwork.IPv6Addresses = []*model.IPAddress{{
		Net:       pubipv6net,
		IsPrimary: true,
	}}
	for _, fip := range floatingIPs {
		var fipnet *net.IPNet

		if fip.Type == api.FloatingIPv4 {
			if fipnet, err = fip.IPv4Net(); err != nil {
				return nil, fmt.Errorf("error getting IPv4Net for floating IP %v", fip)
			}

			addr := &model.IPAddress{
				Net:       fipnet,
				IsPrimary: false,
			}
			cfg.NodeConfig.PublicNetwork.IPv4Addresses = append(
				cfg.NodeConfig.PublicNetwork.IPv4Addresses,
				addr,
			)
			cfg.NodeConfig.FloatingIPs = append(
				cfg.NodeConfig.FloatingIPs,
				addr,
			)
		} else if fip.Type == api.FloatingIPv6 {
			if fipnet, err = fip.IPv6Net(); err != nil {
				return nil, fmt.Errorf("error getting IPv6Net for floating IP %v", fip)
			}

			addr := &model.IPAddress{
				Net:       fipnet,
				IsPrimary: false,
			}
			cfg.NodeConfig.PublicNetwork.IPv6Addresses = append(
				cfg.NodeConfig.PublicNetwork.IPv6Addresses,
				addr,
			)
			cfg.NodeConfig.FloatingIPs = append(
				cfg.NodeConfig.FloatingIPs,
				addr,
			)
		} else {
			return nil, fmt.Errorf("unexpected floating IP type '%d'", fip.Type)
		}
	}

	cfg.NodeConfig.PublicNetwork.GatewayIPv4 = net.IPv4(172, 31, 1, 1)
	cfg.NodeConfig.PublicNetwork.GatewayIPv6 = ipv6gw
	cfg.NodeConfig.PublicNetwork.NetDeviceName = "eth0"

	if len(server.PrivateNetworks) != 1 {
		return nil, fmt.Errorf("server doesn't have exactly one private network")
	}

	var (
		thisPrivNet     *api.Network
		thisPrivNetGwv4 net.IP
	)
	if thisPrivNet, ok = networks[server.PrivateNetworks[0].ID]; !ok {
		return nil, fmt.Errorf("did not find cached private network ID '%s'", server.PrivateNetworks[0].ID)
	}
	if thisPrivNetGwv4 = net.ParseIP(thisPrivNet.GatewayIP); thisPrivNetGwv4 == nil {
		return nil, fmt.Errorf("unable to parse gateway IP of private network '%s'", thisPrivNet.GatewayIP)
	}
	if privipv4net, err = server.PrivateNetworks[0].IPv4Net(); err != nil {
		return nil, fmt.Errorf("error getting public IPv4Net: %v", err)
	}

	cfg.NodeConfig.PrivateNetwork.IPv4Addresses = []*model.IPAddress{{
		Net:       privipv4net,
		IsPrimary: false,
	}}
	cfg.NodeConfig.PrivateNetwork.NetDeviceName = "eth1"
	cfg.NodeConfig.PrivateNetwork.GatewayIPv4 = thisPrivNetGwv4

	cfg.NodeConfig.SSHAuthorizedKeys = userConfig.SSHAuthorizedKeys

	// ClusterConfig
	if val, ok = server.Labels["cluster"]; ok {
		cfg.ClusterConfig.ClusterName = val
	}
	cfg.ClusterConfig.HCloudToken = userConfig.HCloudToken
	cfg.ClusterConfig.K3OSToken = userConfig.K3OSToken

	if len(masterServer.PrivateNetworks) != 1 {
		return nil, fmt.Errorf("master server doesn't have exactly one private network")
	}
	cfg.ClusterConfig.K3OSMasterJoinURL = fmt.Sprintf("https://%s:6443", masterServer.PrivateNetworks[0].ServerIP)

	if userConfig.FluxGitURL != nil && userConfig.FluxGitPrivateKey != nil {
		cfg.ClusterConfig.FluxConfig = &model.FluxConfig{
			GitURL:        *userConfig.FluxGitURL,
			GitPrivateKey: *userConfig.FluxGitPrivateKey,
		}
		// append a newline to the private key if it does not have one (to make the SSH private key syntactically valid)
		if cfg.ClusterConfig.FluxConfig.GitPrivateKey[len(cfg.ClusterConfig.FluxConfig.GitPrivateKey)-1] != '\n' {
			cfg.ClusterConfig.FluxConfig.GitPrivateKey += "\n"
		}
	}

	if userConfig.SealedSecretsTLSCert != nil && userConfig.SealedSecretsTLSKey != nil {
		cfg.ClusterConfig.SealedSecretsConfig = &model.SealedSecretsConfig{
			TLSCert: *userConfig.SealedSecretsTLSCert,
			TLSKey:  *userConfig.SealedSecretsTLSKey,
		}
	}

	return cfg, nil
}
