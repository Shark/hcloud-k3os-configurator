package store

import (
	"fmt"
	"io/ioutil"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"

	"github.com/shark/hcloud-k3os-configurator/cmd"
	"github.com/shark/hcloud-k3os-configurator/model"
	"github.com/shark/hcloud-k3os-configurator/store/fetch"
)

const cachedConfigPath = "/var/lib/hcloud-k3os/config.yaml"

// Load tries to fetch & generate the config from scratch and falls back to the cached config if that's not possible
func Load(log *logrus.Logger, dry bool) (cfg *model.HCloudK3OSConfig, err error) {
	defer func() {
		if _, err2 := cmd.Run(&cmd.Command{Name: "dhcpcd", Arg: []string{"--ipv4only", "--noarp", "--release", "eth0"}}, log, dry); err2 != nil {
			log.WithError(err2).Error("error releasing eth0 from DHCP")
			err = fmt.Errorf("error releasing eth0 from DHCP: %w", err2)
		}
	}()

	if _, err = cmd.Run(&cmd.Command{Name: "dhcpcd", Arg: []string{"--ipv4only", "--noarp", "eth0"}}, log, dry); err != nil {
		return nil, fmt.Errorf("error configuring eth0 with DHCP: %w", err)
	}

	if cfg, err = fetch.Run(); err == nil {
		return storeConfig(cfg)
	}

	log.WithError(err).Warn("Fetch failed")

	return loadCachedConfig()
}

func loadCachedConfig() (*model.HCloudK3OSConfig, error) {
	var (
		buf []byte
		cfg model.HCloudK3OSConfig
		err error
	)

	if buf, err = ioutil.ReadFile(cachedConfigPath); err != nil {
		return nil, fmt.Errorf("error reading cache file at \"%s\": %v", cachedConfigPath, err)
	}

	if err = yaml.Unmarshal(buf, &cfg); err != nil {
		return nil, fmt.Errorf("error parsing cache file at \"%s\": %v", cachedConfigPath, err)
	}

	return &cfg, nil
}

func storeConfig(cfg *model.HCloudK3OSConfig) (*model.HCloudK3OSConfig, error) {
	var (
		buf []byte
		err error
	)

	if buf, err = yaml.Marshal(cfg); err != nil {
		return nil, fmt.Errorf("error marshalling config to YAML: %v", err)
	}

	if err = ioutil.WriteFile(cachedConfigPath, buf, 0600); err != nil {
		return nil, fmt.Errorf("error writing info to YAML at \"%s\": %w", cachedConfigPath, err)
	}

	return cfg, nil
}
