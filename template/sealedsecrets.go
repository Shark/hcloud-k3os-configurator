package template

import (
	"fmt"
	"os"
	"text/template"

	"github.com/shark/hcloud-k3os-configurator/model"
)

const sealedSecretsTmpl = `---
apiVersion: v1
kind: Secret
type: kubernetes.io/tls
metadata:
  name: sealed-secrets-key
labels:
  sealedsecrets.bitnami.com/sealed-secrets-key: active
data:
  tls.crt: {{ .Config.TLSCert }}
  tls.key: {{ .Config.TLSKey }}
`

// GenerateSealedSecretsConfig generates the Sealed Secrets config
func GenerateSealedSecretsConfig(path string, cfg *model.SealedSecretsConfig) error {
	var (
		f   *os.File
		err error
	)
	t := template.Must(template.New("sealedSecretsConfig").Parse(sealedSecretsTmpl))
	if f, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644); err != nil {
		return fmt.Errorf("error opening output file at \"%s\": %w", path, err)
	}
	defer f.Close()
	if err = t.Execute(f, struct {
		Config *model.SealedSecretsConfig
	}{cfg}); err != nil {
		return fmt.Errorf("error executing template: %w", err)
	}
	return nil
}
