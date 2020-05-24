package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/avast/retry-go"
	"github.com/sirupsen/logrus"

	"github.com/shark/hcloud-k3os-configurator/cmd"
	"github.com/shark/hcloud-k3os-configurator/kustomize"
	"github.com/shark/hcloud-k3os-configurator/model"
	"github.com/shark/hcloud-k3os-configurator/store"
	"github.com/shark/hcloud-k3os-configurator/template"
)

func main() {
	var (
		err    error
		tmpdir string
		cfg    *model.HCloudK3OSConfig
	)
	log := logrus.New()

	debug := flag.Bool("debug", false, "enable debug logging")
	dry := flag.Bool("dry", false, "dry run (do not run commands or write configuration)")
	flag.Parse()

	if *debug {
		log.SetLevel(logrus.DebugLevel)
	}

	if err = os.MkdirAll("/var/lib/hcloud-k3os", 0755); err != nil {
		log.WithError(err).Fatal("error creating /var/lib/hcloud-k3os")
	}

	if err = cmd.Run(log, *dry, "rm", "-f", "/var/lib/hcloud-k3os/.running"); err != nil {
		log.WithError(err).Error("Error deleting .running file")
	}

	if err = os.MkdirAll("/var/lib/rancher/k3os/config.d", 0755); err != nil {
		log.WithError(err).Fatal("error creating /var/lib/rancher/k3os/config.d")
	}

	if tmpdir, err = ioutil.TempDir("", "*-hcloud-k3os"); err != nil {
		log.WithError(err).Fatal("Error creating temp dir")
	} else {
		defer os.RemoveAll(tmpdir)
		if err = kustomize.Extract(tmpdir); err != nil {
			log.WithError(err).Fatal("Error extracting kustomize files")
		}
	}

	if cfg, err = store.Load(log, *dry); err != nil {
		log.WithError(err).Fatal("Loading config failed")
	}

	log.Info("Generating and writing configuration files")
	if err = template.GenerateK3OSConfig("/var/lib/rancher/k3os/config.d/hcloud-k3os.yaml", cfg); err != nil {
		log.WithError(err).Error("Error generating k3os config")
	}

	if err = template.GenerateDNSConfig("/etc/resolv.conf"); err != nil {
		log.WithError(err).Error("Error generating DNS config")
	}

	if err = template.GenerateIptablesConfig("/etc/iptables/rules-save", cfg.NodeConfig.PrivateNetwork); err != nil {
		log.WithError(err).Error("Error generating iptables config")
	}

	if err = template.GenerateIP6tablesConfig("/etc/iptables/rules6-save"); err != nil {
		log.WithError(err).Error("Error generating iptables config")
	}

	if cfg.NodeConfig.Role == model.RoleMaster && cfg.ClusterConfig.FluxConfig != nil {
		if err = template.GenerateFluxConfig(path.Join(tmpdir, "flux", "patch.yaml"), cfg.ClusterConfig.FluxConfig); err != nil {
			log.WithError(err).Error("Error generating Flux config")
		} else {
			if err = cmd.Run(log, false, "sh", "-c", fmt.Sprintf("kubectl kustomize %s > /var/lib/rancher/k3s/server/manifests/flux.yaml", path.Join(tmpdir, "flux"))); err != nil {
				log.WithError(err).Error("Error running kustomize for Flux")
			}
		}
	} else {
		log.Debug("Flux is disabled")
	}

	if err = template.GenerateHCloudCSIConfig(path.Join(tmpdir, "hcloud-csi", "secret.yaml"), cfg.ClusterConfig.HCloudToken); err != nil {
		log.WithError(err).Error("Error generating HCloud CSI config")
	} else {
		if err = cmd.Run(log, false, "sh", "-c", fmt.Sprintf("kubectl kustomize %s > /var/lib/rancher/k3s/server/manifests/hcloud-csi.yaml", path.Join(tmpdir, "hcloud-csi"))); err != nil {
			log.WithError(err).Error("Error running kustomize for HCloud CSI")
		}
	}

	if err = template.GenerateHCloudFIPConfig(path.Join(tmpdir, "hcloud-fip", "config.yaml"), cfg.ClusterConfig.HCloudToken, cfg.NodeConfig.FloatingIPs); err != nil {
		log.WithError(err).Error("Error generating HCloud FIP config")
	} else {
		if err = cmd.Run(log, false, "sh", "-c", fmt.Sprintf("kubectl kustomize %s > /var/lib/rancher/k3s/server/manifests/hcloud-fip.yaml", path.Join(tmpdir, "hcloud-fip"))); err != nil {
			log.WithError(err).Error("Error running kustomize for HCloud FIP")
		}
	}

	if cfg.NodeConfig.Role == model.RoleMaster && cfg.ClusterConfig.SealedSecretsConfig != nil {
		if err = template.GenerateSealedSecretsConfig(path.Join(tmpdir, "sealed-secrets", "secret.yaml"), cfg.ClusterConfig.SealedSecretsConfig); err != nil {
			log.WithError(err).Error("Error generating SealedSecrets config")
		} else {
			if err = cmd.Run(log, false, "sh", "-c", fmt.Sprintf("kubectl kustomize %s > /var/lib/rancher/k3s/server/manifests/sealed-secrets.yaml", path.Join(tmpdir, "sealed-secrets"))); err != nil {
				log.WithError(err).Error("Error running kustomize for SealedSecrets")
			}
		}
	} else {
		log.Debug("SealedSecrets is disabled")
	}

	log.Info("Resetting network interfaces")
	if err = cmd.Run(log, *dry, "sh", "-c", "for i in $(ls /sys/class/net/); do [ $i != lo ] && /usr/sbin/ip addr flush $i; done"); err != nil {
		log.WithError(err).Error("Error resetting network interfaces")
	}

	log.Info("Configuring public IPv4")
	cmds := []*cmd.Command{{Name: "ip", Arg: []string{"-4", "link", "set", "up", "dev", cfg.NodeConfig.PublicNetwork.NetDeviceName}}}
	for _, ip := range cfg.NodeConfig.PublicNetwork.IPv4Addresses {
		cmds = append(cmds, &cmd.Command{Name: "ip", Arg: []string{"-4", "addr", "add", ip.Net.String(), "dev", "eth0"}})
	}
	cmds = append(
		cmds,
		&cmd.Command{Name: "ip", Arg: []string{"-4", "route", "add", cfg.NodeConfig.PublicNetwork.GatewayIPv4.String(), "dev", "eth0", "src", cfg.NodeConfig.PublicNetwork.IPv4Addresses[0].Net.IP.String()}},
		&cmd.Command{Name: "ip", Arg: []string{"-4", "route", "add", "default", "via", cfg.NodeConfig.PublicNetwork.GatewayIPv4.String()}},
	)
	if err = cmd.RunMultiple(log, *dry, cmds); err != nil {
		log.WithError(err).Error("Error configuring public IPv4")
	}

	log.Info("Configuring public IPv6")
	cmds = []*cmd.Command{}
	for _, ip := range cfg.NodeConfig.PublicNetwork.IPv6Addresses {
		cmds = append(
			cmds,
			&cmd.Command{Name: "ip", Arg: []string{"-6", "addr", "add", ip.Net.String(), "dev", "eth0"}},
		)
	}
	if err = cmd.RunMultiple(log, *dry, cmds); err != nil {
		log.WithError(err).Error("Error configuring public IPv6")
	}

	log.Info("Configuring IPv6 default route")
	if err = retry.Do(func() error {
		var runErr error
		if runErr = cmd.Run(log, *dry, "ip", "-6", "route", "add", "default", "via", "fe80::1", "src", cfg.NodeConfig.PublicNetwork.IPv6Addresses[0].Net.IP.String(), "dev", "eth0"); err != nil {
			var cmdErr *cmd.Error
			if errors.As(runErr, &cmdErr) {
				// IPv6 not ready yet, retry
				if cmdErr.ExitCode() == 2 {
					return runErr
				}
			}
			return retry.Unrecoverable(runErr)
		}
		return nil
	}, retry.Delay(1*time.Second)); err != nil {
		log.WithError(err).Error("Error adding IPv6 default route")
	}

	log.Info("Configuring private network")
	cmds = []*cmd.Command{
		{Name: "ip", Arg: []string{"-4", "link", "set", "up", "dev", cfg.NodeConfig.PrivateNetwork.NetDeviceName}},
	}
	for _, ip := range cfg.NodeConfig.PrivateNetwork.IPv4Addresses {
		cmds = append(
			cmds,
			&cmd.Command{Name: "ip", Arg: []string{"-4", "addr", "add", ip.Net.String(), "dev", cfg.NodeConfig.PrivateNetwork.NetDeviceName}},
		)
	}
	cmds = append(
		cmds,
		&cmd.Command{Name: "ip", Arg: []string{"-4", "route", "add", cfg.NodeConfig.PrivateNetwork.GatewayIPv4.String(), "dev", cfg.NodeConfig.PrivateNetwork.NetDeviceName}},
		&cmd.Command{Name: "ip", Arg: []string{"-4", "route", "add", cfg.NodeConfig.PrivateNetwork.IPv4Addresses[0].Net.String(), "via", cfg.NodeConfig.PrivateNetwork.GatewayIPv4.String()}},
	)
	if err = cmd.RunMultiple(log, *dry, cmds); err != nil {
		log.WithError(err).Error("Error configuring private network")
	}

	log.Info("Configuration successful!")

	if err = cmd.Run(log, false, "touch", "/var/lib/hcloud-k3os/.running"); err != nil {
		log.WithError(err).Error("Error creating .running file")
	}

	termChan := make(chan os.Signal, 4)
	signal.Notify(termChan, syscall.SIGINT, syscall.SIGTERM)

	<-termChan

	log.Info("Shutdown signal received")

	if err = cmd.Run(log, false, "rm", "-f", "/var/lib/hcloud-k3os/.running"); err != nil {
		log.WithError(err).Error("Error deleting .running file")
	}
}
