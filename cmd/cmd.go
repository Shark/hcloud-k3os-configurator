package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/sirupsen/logrus"
)

// Error represents a failed cmd run error
type Error struct {
	err      error
	exitCode int
}

func (c *Error) Error() string {
	return c.err.Error()
}

// ExitCode is the command exit code
func (c *Error) ExitCode() int {
	return c.exitCode
}

// Unwrap makes this error conformant with Go 1.13 errors
func (c *Error) Unwrap() error {
	return c.err
}

// Run executes a command
func Run(cmd *Command, log *logrus.Logger, dry bool) (string, error) {
	var (
		out []byte
		err error
	)
	log.Debugf("Running %s %#v", cmd.Name, cmd.Arg)
	if dry {
		log.Debugf("Dry run, not executing %s %#v", cmd.Name, cmd.Arg)
		return "", nil
	}
	ecmd := exec.Command(cmd.Name, cmd.Arg...)
	if cmd.Env != nil {
		ecmd.Env = os.Environ()
		for k, v := range cmd.Env {
			ecmd.Env = append(ecmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	if out, err = ecmd.CombinedOutput(); err != nil {
		var exiterr *exec.ExitError
		if errors.As(err, &exiterr) {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				return string(out), &Error{
					err:      err,
					exitCode: status.ExitStatus(),
				}
			}
		}
		log.WithError(err).Errorf("Running command \"%s %#v\" failed, output: %v", cmd.Name, cmd.Arg, string(out))
		return string(out), fmt.Errorf("running command \"%s %#v\" failed: %w", cmd.Name, cmd.Arg, err)
	}
	return string(out), nil
}

// Command specifies a command to run
type Command struct {
	Name string
	Arg  []string
	Env  map[string]string
}

// RunMultiple runs a pipeline of commands and fails early if any command fails
func RunMultiple(log *logrus.Logger, dry bool, commands []*Command) error {
	var err error
	for _, cmd := range commands {
		if _, err = Run(cmd, log, dry); err != nil {
			log.WithError(err).Errorf("Command %#v of pipeline %#v failed", cmd, commands)
			return fmt.Errorf("running command %#v failed: %w", cmd, err)
		}
	}
	return nil
}
