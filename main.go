package main

import (
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/shark/hcloud-k3os-configurator/cli"
	"github.com/shark/hcloud-k3os-configurator/model"
)

func main() {
	var err error
	cfg := &model.RuntimeConfig{
		Logger: logrus.New(),
	}
	rootCmd := &cobra.Command{
		Use: "hcloud-k3os-configurator",
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			if cfg.Debug {
				cfg.Logger.SetLevel(logrus.DebugLevel)
			}
			return nil
		},
	}
	rootCmd.PersistentFlags().BoolVar(&cfg.Dry, "dry", false, "Dry run")
	rootCmd.PersistentFlags().BoolVar(&cfg.Debug, "debug", false, "Enable debug logging")
	rootCmd.AddCommand(cli.Daemon(cfg))
	rootCmd.AddCommand(cli.Backup(cfg))

	if err = os.MkdirAll("/var/lib/hcloud-k3os/cache", 0755); err != nil {
		cfg.Logger.WithError(err).Fatal("error creating /var/lib/hcloud-k3os/cache")
	}

	if err = rootCmd.Execute(); err != nil {
		cfg.Logger.Errorf("Command returned an error: %v", err)
	}
}
