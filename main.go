package main

import (
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
	}
	rootCmd.PersistentFlags().BoolVar(&cfg.Dry, "dry", false, "Dry run")
	rootCmd.PersistentFlags().BoolVar(&cfg.Debug, "debug", false, "Enable debug logging")
	rootCmd.AddCommand(cli.Daemon(cfg))
	if err = rootCmd.Execute(); err != nil {
		cfg.Logger.Errorf("Command returned an error: %v", err)
	}
}
