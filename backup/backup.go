package backup

import (
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
			Env: map[string]string{
				"RESTIC_PASSWORD":       bcfg.Password,
				"RESTIC_REPOSITORY":     bcfg.RepositoryURL,
				"AWS_ACCESS_KEY_ID":     bcfg.AccessKeyID,
				"AWS_SECRET_ACCESS_KEY": bcfg.SecretAccessKey,
			},
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
			Env: map[string]string{
				"RESTIC_PASSWORD":       bcfg.Password,
				"RESTIC_REPOSITORY":     bcfg.RepositoryURL,
				"AWS_ACCESS_KEY_ID":     bcfg.AccessKeyID,
				"AWS_SECRET_ACCESS_KEY": bcfg.SecretAccessKey,
			},
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
				backupDir,
			},
			Env: map[string]string{
				"RESTIC_PASSWORD":       bcfg.Password,
				"RESTIC_REPOSITORY":     bcfg.RepositoryURL,
				"AWS_ACCESS_KEY_ID":     bcfg.AccessKeyID,
				"AWS_SECRET_ACCESS_KEY": bcfg.SecretAccessKey,
			},
		}
		err error
	)

	if _, err = cmd.Run(bcmd, log, dry); err != nil {
		return fmt.Errorf("error running backup command: %v", err)
	}

	return nil
}
