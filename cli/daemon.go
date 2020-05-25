package cli

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/avast/retry-go"
	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/shark/hcloud-k3os-configurator/backup"
	"github.com/shark/hcloud-k3os-configurator/cmd"
	"github.com/shark/hcloud-k3os-configurator/kustomize"
	"github.com/shark/hcloud-k3os-configurator/model"
	"github.com/shark/hcloud-k3os-configurator/store"
	"github.com/shark/hcloud-k3os-configurator/template"
)

// Daemon implements the daemon command
func Daemon(rcfg *model.RuntimeConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "daemon",
		Short: "Runs the background daemon for configuration and backup",
		RunE: func(_ *cobra.Command, _ []string) error {
			var (
				err    error
				tmpdir string
				log    = rcfg.Logger
				cfg    *model.HCloudK3OSConfig
			)

			if rcfg.Debug {
				log.SetLevel(logrus.DebugLevel)
			}

			if err = os.MkdirAll("/var/lib/hcloud-k3os/cache", 0755); err != nil {
				log.WithError(err).Fatal("error creating /var/lib/hcloud-k3os/cache")
			}

			if _, err = cmd.Run(&cmd.Command{Name: "rm", Arg: []string{"-f", "/var/lib/hcloud-k3os/.running"}}, log, rcfg.Dry); err != nil {
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

			if cfg, err = store.Load(log, rcfg.Dry); err != nil {
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
					if _, err = cmd.Run(&cmd.Command{Name: "sh", Arg: []string{"-c", fmt.Sprintf("kubectl kustomize %s > /var/lib/rancher/k3s/server/manifests/flux.yaml", path.Join(tmpdir, "flux"))}}, log, false); err != nil {
						log.WithError(err).Error("Error running kustomize for Flux")
					}
				}
			} else {
				log.Debug("Flux is disabled")
			}

			if err = template.GenerateHCloudCSIConfig(path.Join(tmpdir, "hcloud-csi", "secret.yaml"), cfg.ClusterConfig.HCloudToken); err != nil {
				log.WithError(err).Error("Error generating HCloud CSI config")
			} else {
				if _, err = cmd.Run(&cmd.Command{Name: "sh", Arg: []string{"-c", fmt.Sprintf("kubectl kustomize %s > /var/lib/rancher/k3s/server/manifests/hcloud-csi.yaml", path.Join(tmpdir, "hcloud-csi"))}}, log, false); err != nil {
					log.WithError(err).Error("Error running kustomize for HCloud CSI")
				}
			}

			if err = template.GenerateHCloudFIPConfig(path.Join(tmpdir, "hcloud-fip", "config.yaml"), cfg.ClusterConfig.HCloudToken, cfg.NodeConfig.FloatingIPs); err != nil {
				log.WithError(err).Error("Error generating HCloud FIP config")
			} else {
				if _, err = cmd.Run(&cmd.Command{Name: "sh", Arg: []string{"-c", fmt.Sprintf("kubectl kustomize %s > /var/lib/rancher/k3s/server/manifests/hcloud-fip.yaml", path.Join(tmpdir, "hcloud-fip"))}}, log, false); err != nil {
					log.WithError(err).Error("Error running kustomize for HCloud FIP")
				}
			}

			if cfg.NodeConfig.Role == model.RoleMaster && cfg.ClusterConfig.SealedSecretsConfig != nil {
				if err = template.GenerateSealedSecretsConfig(path.Join(tmpdir, "sealed-secrets", "secret.yaml"), cfg.ClusterConfig.SealedSecretsConfig); err != nil {
					log.WithError(err).Error("Error generating SealedSecrets config")
				} else {
					if _, err = cmd.Run(&cmd.Command{Name: "sh", Arg: []string{"-c", fmt.Sprintf("kubectl kustomize %s > /var/lib/rancher/k3s/server/manifests/sealed-secrets.yaml", path.Join(tmpdir, "sealed-secrets"))}}, log, false); err != nil {
						log.WithError(err).Error("Error running kustomize for SealedSecrets")
					}
				}
			} else {
				log.Debug("SealedSecrets is disabled")
			}

			log.Info("Resetting network interfaces")
			if _, err = cmd.Run(&cmd.Command{Name: "sh", Arg: []string{"-c", "for i in /sys/class/net/eth*; do ip link set down $(basename $i) && ip addr flush $(basename $i) && ip link set up $(basename $i); done"}}, log, rcfg.Dry); err != nil {
				log.WithError(err).Error("Error resetting network interfaces")
			}

			log.Info("Configuring public IPv4")
			cmds := []*cmd.Command{}
			for _, ip := range cfg.NodeConfig.PublicNetwork.IPv4Addresses {
				cmds = append(cmds, &cmd.Command{Name: "ip", Arg: []string{"-4", "addr", "add", ip.Net.String(), "dev", cfg.NodeConfig.PublicNetwork.NetDeviceName}})
			}
			cmds = append(
				cmds,
				&cmd.Command{Name: "ip", Arg: []string{"-4", "route", "add", cfg.NodeConfig.PublicNetwork.GatewayIPv4.String(), "dev", "eth0", "src", cfg.NodeConfig.PublicNetwork.IPv4Addresses[0].Net.IP.String()}},
				&cmd.Command{Name: "ip", Arg: []string{"-4", "route", "add", "default", "via", cfg.NodeConfig.PublicNetwork.GatewayIPv4.String()}},
			)
			if err = cmd.RunMultiple(log, rcfg.Dry, cmds); err != nil {
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
			if err = cmd.RunMultiple(log, rcfg.Dry, cmds); err != nil {
				log.WithError(err).Error("Error configuring public IPv6")
			}

			// Sleep is required for IPv6 default route to be added reliably
			// TODO debug why we need a sleep for this
			time.Sleep(3 * time.Second)

			log.Info("Configuring IPv6 default route")
			if err = retry.Do(func() error {
				var runErr error
				if _, runErr = cmd.Run(&cmd.Command{Name: "ip", Arg: []string{"-6", "route", "add", "default", "via", "fe80::1", "src", cfg.NodeConfig.PublicNetwork.IPv6Addresses[0].Net.IP.String(), "dev", "eth0"}}, log, rcfg.Dry); err != nil {
					log.WithError(err).Error("error set route")
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
			cmds = []*cmd.Command{}
			for _, ip := range cfg.NodeConfig.PrivateNetwork.IPv4Addresses {
				cmds = append(
					cmds,
					&cmd.Command{Name: "ip", Arg: []string{"-4", "addr", "add", fmt.Sprintf("%s/32", ip.Net.IP.String()), "dev", cfg.NodeConfig.PrivateNetwork.NetDeviceName}},
				)
			}
			var cnet *net.IPNet
			if cnet, err = cfg.NodeConfig.PrivateNetwork.IPv4Addresses[0].CanonicalNet(); err == nil {
				cmds = append(
					cmds,
					&cmd.Command{Name: "ip", Arg: []string{"-4", "route", "add", cfg.NodeConfig.PrivateNetwork.GatewayIPv4.String(), "dev", cfg.NodeConfig.PrivateNetwork.NetDeviceName}},
					&cmd.Command{Name: "ip", Arg: []string{"-4", "route", "add", cnet.String(), "via", cfg.NodeConfig.PrivateNetwork.GatewayIPv4.String()}},
				)
			}
			if err = cmd.RunMultiple(log, rcfg.Dry, cmds); err != nil {
				log.WithError(err).Error("Error configuring private network")
			}

			log.Info("Configuration successful!")

			if cfg.NodeConfig.Role == model.RoleMaster && !backup.IsBootstrapped() {
				if err = backup.Init(cfg.ClusterConfig.BackupConfig, log, false); err != nil {
					log.WithError(err).Fatal("Unable to initialize backup")
				}

				if !cfg.ClusterConfig.Bootstrap {
					var snapshots []*backup.Snapshot
					if snapshots, err = backup.ListSnapshots(cfg.ClusterConfig.BackupConfig, log, false); err != nil {
						log.WithError(err).Fatal("Unable to list snapshots")
					}
					if len(snapshots) == 0 {
						log.Fatal("Backup does not have any snapshots")
					}
					log.Info("Restoring latest snapshot from backup")
					if err = backup.Restore(cfg.ClusterConfig.BackupConfig, log, false); err != nil {
						log.WithError(err).Fatal("Unable to bootstrap node")
					}
					log.Info("Snapshot restored")
				} else {
					if err = backup.MarkBootstrapped(log, false); err != nil {
						log.WithError(err).Fatal("Unable to mark node as bootstrapped")
					}
					log.Info("Bootstrap mode, marked node as bootstrapped")
				}
			}

			c := cron.New()
			if _, err = c.AddFunc("@every 8h", func() {
				var err error
				if err = backup.Backup(cfg.ClusterConfig.BackupConfig, log, rcfg.Dry); err != nil {
					log.WithError(err).Error("Error running periodic backup")
				}
			}); err != nil {
				log.WithError(err).Error("Error creating job for periodic backup")
			}
			c.Start()

			if _, err = cmd.Run(&cmd.Command{Name: "touch", Arg: []string{"/var/lib/hcloud-k3os/.running"}}, log, false); err != nil {
				log.WithError(err).Error("Error creating .running file")
			}

			termChan := make(chan os.Signal, 4)
			signal.Notify(termChan, syscall.SIGINT, syscall.SIGTERM)

			<-termChan

			log.Info("Shutdown signal received")

			if _, err = cmd.Run(&cmd.Command{Name: "rm", Arg: []string{"-f", "/var/lib/hcloud-k3os/.running"}}, log, false); err != nil {
				log.WithError(err).Error("Error deleting .running file")
			}

			return nil
		},
	}
}
