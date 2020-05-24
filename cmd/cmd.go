package cmd

import (
	"errors"
	"fmt"
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
func Run(log *logrus.Logger, dry bool, name string, arg ...string) error {
	var (
		out []byte
		err error
	)
	log.Debugf("Running %s %#v", name, arg)
	if dry {
		log.Debugf("Dry run, not executing %s %#v", name, arg)
		return nil
	}
	cmd := exec.Command(name, arg...)
	if out, err = cmd.CombinedOutput(); err != nil {
		var exiterr *exec.ExitError
		if errors.As(err, &exiterr) {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				return &Error{
					err:      err,
					exitCode: status.ExitStatus(),
				}
			}
		}
		log.WithError(err).Errorf("Running command \"%s %#v\" failed, output: %v", name, arg, string(out))
		return fmt.Errorf("running command \"%s %#v\" failed: %w", name, arg, err)
	}
	return nil
}

// Command specifies a command to run
type Command struct {
	Name string
	Arg  []string
}

// RunMultiple runs a pipeline of commands and fails early if any command fails
func RunMultiple(log *logrus.Logger, dry bool, commands []*Command) error {
	var err error
	for _, cmd := range commands {
		if err = Run(log, dry, cmd.Name, cmd.Arg...); err != nil {
			log.WithError(err).Errorf("Command %#v of pipeline %#v failed", cmd, commands)
			return fmt.Errorf("running command %#v failed: %w", cmd, err)
		}
	}
	return nil
}
