package template

import (
	"fmt"
	"os"
	"text/template"
)

const hcloudCSITmpl = `---
apiVersion: v1
kind: Secret
metadata:
  name: hcloud-csi
stringData:
  token: {{ .Token }}
`

// GenerateHCloudCSIConfig generates the config for hcloud-csi
func GenerateHCloudCSIConfig(path string, token string) error {
	var (
		f   *os.File
		err error
	)
	t := template.Must(template.New("hcloudCSIConfig").Parse(hcloudCSITmpl))
	if f, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644); err != nil {
		return fmt.Errorf("error opening output file at \"%s\": %w", path, err)
	}
	defer f.Close()
	if err = t.Execute(f, struct {
		Token string
	}{token}); err != nil {
		return fmt.Errorf("error executing template: %w", err)
	}
	return nil
}
