package main

import (
	"flag"
	"os"
	"strings"

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

	instanceID, err := api.GetInstanceID()
	if err != nil {
		log.WithError(err).Fatal("error getting instance ID")
	}
	log.Debugf("Instance ID: %s", instanceID)

	server, err := api.GetServer(instanceID, token)
	if err != nil {
		log.WithError(err).Fatal("error getting server")
	}
	log.Debugf("Server: %#v", server)

	networks := make(map[string]*api.Network)
	for _, n := range server.PrivateNetworks {
		fullN, err := api.GetNetwork(n.ID, token)
		if err != nil {
			log.WithError(err).Fatalf("error getting network with ID %s", n.ID)
		}
		networks[n.ID] = fullN
	}

	var serverFloatingIPs []*api.FloatingIP
	if cluster, ok := server.Labels["cluster"]; ok {
		serverFloatingIPs, err = api.GetFloatingIPsForCluster(cluster, token)
		if err != nil {
			log.WithError(err).Fatalf("error getting floating IPs for cluster '%s'", cluster)
		}
	} else {
		log.Infof("server without cluster= label, skipping floating IPs")
	}

	cfg, err := node.GenerateConfig(server.Name, server.IPv4Address, server.IPv6Subnet)
	if err != nil {
		log.WithError(err).Fatal("error creating node config")
	}

	var privateNetworks []*node.PrivateNetwork
	for _, n := range server.PrivateNetworks {
		if fullN, ok := networks[n.ID]; ok {
			elems := strings.Split(fullN.IPRange, "/")
			if len(elems) != 2 {
				log.WithError(err).Fatalf("network with ID %s has a malformed IPRange: %s", n.ID, fullN.IPRange)
			}
			privateNetworks = append(privateNetworks, &node.PrivateNetwork{
				ID:           n.ID,
				MAC:          n.MACAddress,
				IP:           n.ServerIP,
				NetworkIP:    elems[0],
				PrefixLength: elems[1],
			})
		} else {
			log.Fatalf("could not find network with ID %s", n.ID)
		}
	}

	var floatingIPs []*node.FloatingIP
	for _, fip := range serverFloatingIPs {
		floatingIPs = append(floatingIPs, &node.FloatingIP{IP: fip.IP})
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
