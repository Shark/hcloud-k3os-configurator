package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/shark/hcloud-k3os-configurator/cmd"
	"github.com/shark/hcloud-k3os-configurator/model"
)

const bootstrappedFile = "/var/lib/rancher/.bootstrapped"
const backupDir = "/var/lib/rancher"
const cacheDir = "/var/lib/hcloud-k3os/cache"

// IsBootstrapped returns true if the node is bootstrapped
func IsBootstrapped() bool {
	var err error
	if _, err = os.Stat(bootstrappedFile); err == nil {
		return true
	}
	return false
}

// MarkBootstrapped creates the bootstrap file
func MarkBootstrapped(log *logrus.Logger, dry bool) error {
	var (
		tcmd = &cmd.Command{
			Name: "touch",
			Arg:  []string{bootstrappedFile},
		}
		err error
	)
	if _, err = cmd.Run(tcmd, log, dry); err != nil {
		return fmt.Errorf("error touching bootstrappedFile: %v", err)
	}
	return nil
}

// Init initializes the restic repository
func Init(bcfg *model.BackupConfig, log *logrus.Logger, dry bool) error {
	var (
		icmd = &cmd.Command{
			Name: "restic",
			Arg: []string{
				"init",
				"--cache-dir",
				cacheDir,
			},
			Env: resticEnv(bcfg),
		}
		out string
		err error
	)

	if out, err = cmd.Run(icmd, log, dry); err != nil {
		if strings.Contains(out, "already initialized") {
			return nil
		}
		return fmt.Errorf("error initializing restic repository: %v", err)
	}

	return nil
}

// Snapshot is a restic snapshot
type Snapshot struct {
	Time     string   `json:"time"`
	Tree     string   `json:"tree"`
	Paths    []string `json:"paths"`
	Hostname string   `json:"hostname"`
	Username string   `json:"username"`
	Excludes []string `json:"excludes"`
	ID       string   `json:"id"`
	ShortID  string   `json:"short_id"`
}

// ListSnapshots lists the snapshots in a restic repository
func ListSnapshots(bcfg *model.BackupConfig, log *logrus.Logger, dry bool) ([]*Snapshot, error) {
	var (
		lcmd = &cmd.Command{
			Name: "restic",
			Arg: []string{
				"--json",
				"snapshots",
			},
			Env: resticEnv(bcfg),
		}
		out       string
		snapshots []*Snapshot
		err       error
	)

	if out, err = cmd.Run(lcmd, log, dry); err != nil {
		return nil, fmt.Errorf("error running list command: %v", err)
	}

	if err = json.Unmarshal([]byte(out), &snapshots); err != nil {
		return nil, fmt.Errorf("error unmarshalling list output: %v", err)
	}

	return snapshots, nil
}

// Restore restores the latest restic backup
func Restore(bcfg *model.BackupConfig, log *logrus.Logger, dry bool) error {
	var (
		rcmd = &cmd.Command{
			Name: "restic",
			Arg: []string{
				"restore",
				"latest",
				"--cache-dir",
				cacheDir,
				"--target",
				"/",
				"--path",
				backupDir,
			},
			Env: resticEnv(bcfg),
		}
		err error
	)

	if _, err = cmd.Run(rcmd, log, dry); err != nil {
		return fmt.Errorf("error running restore command: %v", err)
	}

	return nil
}

// Backup runs a restic backup
func Backup(bcfg *model.BackupConfig, log *logrus.Logger, dry bool) error {
	var (
		bcmd = &cmd.Command{
			Name: "restic",
			Arg: []string{
				"backup",
				"--cache-dir",
				cacheDir,
				"--exclude",
				"/var/lib/rancher/k3s/agent/containerd",
				"--exclude",
				"/var/lib/rancher/k3s/data",
				backupDir,
			},
			Env: resticEnv(bcfg),
		}
		err error
	)

	if _, err = cmd.Run(bcmd, log, dry); err != nil {
		return fmt.Errorf("error running backup command: %v", err)
	}

	return nil
}

func resticEnv(bcfg *model.BackupConfig) map[string]string {
	return map[string]string{
		"RESTIC_PASSWORD":       bcfg.Password,
		"RESTIC_REPOSITORY":     bcfg.RepositoryURL,
		"AWS_ACCESS_KEY_ID":     bcfg.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY": bcfg.SecretAccessKey,
	}
}
