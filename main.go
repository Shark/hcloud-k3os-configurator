package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/avast/retry-go"

	"github.com/shark/hcloud-k3os-configurator/api"
	"github.com/shark/hcloud-k3os-configurator/config"
	"github.com/shark/hcloud-k3os-configurator/node"
	"github.com/sirupsen/logrus"
)

func main() {
	log := logrus.New()

	debug := flag.Bool("debug", false, "enable debug logging")
	out := flag.String("out", "/var/lib/rancher/k3os/config.d/configurator.yaml", "config output path")
	flag.Parse()

	if *debug {
		log.SetLevel(logrus.DebugLevel)
	}

	var (
		token string
		ok    bool
	)
	if token, ok = os.LookupEnv("HCLOUD_TOKEN"); !ok {
		log.Fatal("HCLOUD_TOKEN must be set")
	}

	var (
		instanceID string
		err        error
	)
	err = retry.Do(func() error {
		instanceID, err = api.GetInstanceID()
		if err != nil && !errors.Is(err, &api.RetryableError{}) {
			return retry.Unrecoverable(err)
		}
		return err
	})
	if err != nil {
		log.WithError(err).Fatal("error getting instance ID")
	}
	log.Debugf("Instance ID: %s", instanceID)

	var server *api.Server
	err = retry.Do(func() error {
		server, err = api.GetServer(instanceID, token)
		if err != nil && !errors.Is(err, &api.RetryableError{}) {
			return retry.Unrecoverable(err)
		}
		return err
	})
	if err != nil {
		log.WithError(err).Fatal("error getting server")
	}
	log.Debugf("Server: %#v", server)

	networks := make(map[string]*api.Network)
	for _, n := range server.PrivateNetworks {
		var fullN *api.Network
		err = retry.Do(func() error {
			fullN, err = api.GetNetwork(n.ID, token)
			if err != nil && !errors.Is(err, &api.RetryableError{}) {
				return retry.Unrecoverable(err)
			}
			return err
		})
		if err != nil {
			log.WithError(err).Fatalf("error getting network with ID %s", n.ID)
		}
		networks[n.ID] = fullN
	}

	var serverFloatingIPs []*api.FloatingIP
	if cluster, ok := server.Labels["cluster"]; ok {
		err = retry.Do(func() error {
			serverFloatingIPs, err = api.GetFloatingIPsForCluster(cluster, token)
			if err != nil && !errors.Is(err, &api.RetryableError{}) {
				return retry.Unrecoverable(err)
			}
			return err
		})
		if err != nil {
			log.WithError(err).Fatalf("error getting floating IPs for cluster '%s'", cluster)
		}
	} else {
		log.Infof("server without cluster= label, skipping floating IPs")
	}

	cfg, err := node.GenerateConfig(server.IPv4Address, server.IPv6Subnet)
	if err != nil {
		log.WithError(err).Fatal("error creating node config")
	}
	cfg.Name = server.Name
	if sName, ok := server.Labels["short_name"]; ok {
		cfg.ShortName = sName
	} else {
		cfg.ShortName = cfg.Name
	}
	if role, ok := server.Labels["role"]; ok {
		switch role {
		case "server":
			cfg.Role = node.RoleServer
		case "agent":
			cfg.Role = node.RoleAgent
		default:
			log.Warnf("can not process label role=%s, falling back to agent", role)
			cfg.Role = node.RoleAgent
		}
	} else {
		log.Warnf("server does not have a role label, falling back to agent")
		cfg.Role = node.RoleAgent
	}
	if cfg.Role == node.RoleAgent {
		serverServer, err := api.GetServerWithRole("server", token)
		if err != nil {
			log.WithError(err).Warnf("unable to find k3s server for this agent node")
		} else {
			// TODO add network labelling
			switch len(serverServer.PrivateNetworks) {
			case 0:
				log.Warn("k3s server does not have any private networks, can not use it")
			case 1:
				cfg.JoinURL = fmt.Sprintf("https://%s:6443", serverServer.PrivateNetworks[0].ServerIP)
			default:
				log.Warn("k3s server has multiple private networks, this is currently not supported; joining in the first one")
				cfg.JoinURL = fmt.Sprintf("https://%s:6443", serverServer.PrivateNetworks[0].ServerIP)
			}
		}
	}
	// TODO add network labelling
	switch len(server.PrivateNetworks) {
	case 0:
		log.Warn("server does not have any private networks, this is discouraged!")
	case 1:
		cfg.PrivateIPv4Address = server.PrivateNetworks[0].ServerIP
	default:
		log.Warn("server has multiple private networks, this is currently not supported; advertising the first one")
		cfg.PrivateIPv4Address = server.PrivateNetworks[0].ServerIP
	}

	var privateNetworks []*node.PrivateNetwork
	for _, n := range server.PrivateNetworks {
		if fullN, ok := networks[n.ID]; ok {
			elems := strings.Split(fullN.IPRange, "/")
			if len(elems) != 2 {
				log.WithError(err).Fatalf("network with ID %s has a malformed IPRange: %s", n.ID, fullN.IPRange)
			}
			deviceName, err := node.FindDeviceNameForMAC(n.MACAddress)
			if err != nil {
				log.WithError(err).Warnf("unable to find device for MAC %s, skipping network", n.MACAddress)
				continue
			}
			privateNetworks = append(privateNetworks, &node.PrivateNetwork{
				ID:               n.ID,
				MAC:              n.MACAddress,
				IP:               n.ServerIP,
				NetworkIP:        elems[0],
				PrefixLengthBits: elems[1],
				GatewayIP:        fullN.GatewayIP,
				DeviceName:       deviceName,
			})
		} else {
			log.Fatalf("could not find network with ID %s", n.ID)
		}
	}

	var floatingIPs []*node.FloatingIP
	for _, fip := range serverFloatingIPs {
		floatingIPs = append(floatingIPs, &node.FloatingIP{IP: fip.IP, DeviceName: "eth0"})
	}

	f, err := os.Create(*out)
	if err != nil {
		log.WithError(err).Fatalf("error creating output file at %s", *out)
	}
	defer f.Close()

	err = config.Generate(f, cfg, privateNetworks, floatingIPs)
	if err != nil {
		log.WithError(err).Fatal("error generating config")
	}

	log.Infof("wrote config at %s", *out)
}
