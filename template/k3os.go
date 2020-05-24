package template

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"

	"github.com/shark/hcloud-k3os-configurator/model"
)

// GenerateK3OSConfig generates the config for k3os
func GenerateK3OSConfig(path string, cfg *model.HCloudK3OSConfig) (err error) {
	type k3osCfg struct {
		SSHAuthorizedKeys []string `yaml:"ssh_authorized_keys"`
		K3OS              struct {
			ServerURL string   `yaml:"server_url"`
			Token     string   `yaml:"token"`
			K3SArgs   []string `yaml:"k3s_args"`
		} `yaml:"k3os"`
	}
	var (
		f     *os.File
		k3cfg = &k3osCfg{}
		buf   []byte
	)
	k3cfg.SSHAuthorizedKeys = cfg.NodeConfig.SSHAuthorizedKeys
	k3cfg.K3OS.ServerURL = cfg.ClusterConfig.K3OSMasterJoinURL
	k3cfg.K3OS.Token = cfg.ClusterConfig.K3OSToken
	k3cfg.K3OS.K3SArgs = []string{}
	if cfg.NodeConfig.Role == model.RoleMaster {
		k3cfg.K3OS.K3SArgs = append(
			k3cfg.K3OS.K3SArgs,
			"server",
			"--advertise-address",
			cfg.NodeConfig.PrivateNetwork.IPv4Addresses[0].Net.String(),
		)
	} else if cfg.NodeConfig.Role == model.RoleAgent {
		k3cfg.K3OS.K3SArgs = append(
			k3cfg.K3OS.K3SArgs,
			"agent",
		)
	}
	k3cfg.K3OS.K3SArgs = append(
		k3cfg.K3OS.K3SArgs,
		"--node-name",
		cfg.NodeConfig.Name,
		"--node-ip",
		cfg.NodeConfig.PrivateNetwork.IPv4Addresses[0].Net.IP.String(),
		"--node-external-ip",
		cfg.NodeConfig.PublicNetwork.IPv4Addresses[0].Net.IP.String(),
		"--flannel-iface",
		cfg.NodeConfig.PrivateNetwork.NetDeviceName,
	)
	if buf, err = yaml.Marshal(k3cfg); err != nil {
		return fmt.Errorf("error marshalling k3os config: %v", err)
	}
	if f, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644); err != nil {
		return fmt.Errorf("error opening output file at \"%s\": %w", path, err)
	}
	defer func() {
		err = f.Close()
	}()
	_, err = f.Write(buf)
	return err
}
