package kustomize

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/markbates/pkger"
	"github.com/markbates/pkger/pkging"
)

// Extract extracts the kustomization resources to a (temporary) directory
func Extract(dest string) error {
	return pkger.Walk("/kustomize/static", func(rawPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		paths := strings.Split(rawPath, ":")
		if len(paths) != 2 {
			return fmt.Errorf("path '%s' has invalid format, expected one colon got %d", rawPath, len(paths))
		}
		path := strings.TrimPrefix(paths[1], "/kustomize/static")
		if len(path) == 0 {
			return nil
		}

		if info.IsDir() {
			return os.Mkdir(dest+path, 0755)
		}

		var (
			from pkging.File
			to   *os.File
		)
		if from, err = pkger.Open(rawPath); err != nil {
			return fmt.Errorf("error opening pkged file at '%s': %w", rawPath, err)
		}
		defer from.Close()

		if to, err = os.OpenFile(dest+path, os.O_RDWR|os.O_CREATE, 0644); err != nil {
			return fmt.Errorf("error opening destination file for '%s' at '%s': %w", rawPath, dest+path, err)
		}
		defer to.Close()

		if _, err := io.Copy(to, from); err != nil {
			return fmt.Errorf("error copying file for '%s' at '%s': %w", rawPath, dest+path, err)
		}

		return nil
	})
}
