package cli

import (
	"fmt"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/shark/hcloud-k3os-configurator/backup"
	"github.com/shark/hcloud-k3os-configurator/model"
	"github.com/shark/hcloud-k3os-configurator/store"
)

// Backup implements the backup commands
func Backup(rcfg *model.RuntimeConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Run backup tasks",
	}

	cmd.AddCommand(backupList(rcfg))
	cmd.AddCommand(backupRun(rcfg))
	cmd.AddCommand(backupRestore(rcfg))

	return cmd
}

func backupList(rcfg *model.RuntimeConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List snapshots",
		RunE: func(_ *cobra.Command, _ []string) error {
			var (
				cfg       *model.HCloudK3OSConfig
				snapshots []*backup.Snapshot
				err       error
			)

			if cfg, err = store.LoadAndCache(); err != nil {
				return fmt.Errorf("error loading config: %v", err)
			}

			if snapshots, err = backup.ListSnapshots(cfg.ClusterConfig.BackupConfig, rcfg.Logger, false); err != nil {
				return fmt.Errorf("error listing snapshots: %v", err)
			}

			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"ShortID", "Time"})

			for _, s := range snapshots {
				table.Append([]string{s.ShortID, s.Time})
			}

			table.Render()
			return nil
		},
	}
}

func backupRun(rcfg *model.RuntimeConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run backup",
		RunE: func(_ *cobra.Command, _ []string) error {
			var (
				cfg *model.HCloudK3OSConfig
				err error
			)

			if cfg, err = store.LoadAndCache(); err != nil {
				return fmt.Errorf("error loading config: %v", err)
			}

			if err = backup.Backup(cfg.ClusterConfig.BackupConfig, rcfg.Logger, false); err != nil {
				return fmt.Errorf("error running backup: %v", err)
			}

			rcfg.Logger.Info("Backup completed successfully")
			return nil
		},
	}
}

func backupRestore(rcfg *model.RuntimeConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "restore",
		Short: "Restore backup",
		RunE: func(_ *cobra.Command, _ []string) error {
			var (
				cfg *model.HCloudK3OSConfig
				err error
			)

			if cfg, err = store.LoadAndCache(); err != nil {
				return fmt.Errorf("error loading config: %v", err)
			}

			if err = backup.Restore(cfg.ClusterConfig.BackupConfig, rcfg.Logger, false); err != nil {
				return fmt.Errorf("error restoring backup: %v", err)
			}

			rcfg.Logger.Info("Backup restored successfully")
			return nil
		},
	}
}
