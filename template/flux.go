package template

import (
	"fmt"
	"os"
	"text/template"

	"github.com/shark/hcloud-k3os-configurator/model"
)

const fluxTmpl = `---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: flux
spec:
  template:
    spec:
      containers:
      - name: flux
        args:
        - --manifest-generation=true
        - --memcached-hostname=memcached.flux
        - --memcached-service=
        - --ssh-keygen-dir=/var/fluxd/keygen
        - --git-branch=master
        - --git-user=hcloud-k3os
        - --git-email=hcloud-k3os@sh4rk.pw
        - --git-url={{ .Config.GitURL }}

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: flux-helm-operator
spec:
  template:
    spec:
      volumes:
      - name: repositories-yaml
        secret:
          secretName: helm-repositories
      - name: repositories-cache
        emptyDir: {}
      containers:
      - name: flux-helm-operator
        args:
          - --enabled-helm-versions=v3
        volumeMounts:
        - name: repositories-yaml
          mountPath: /var/fluxd/helm/repository
        - name: repositories-cache
          mountPath: /var/fluxd/helm/repository/cache

---
apiVersion: v1
kind: Secret
metadata:
  name: flux-git-deploy
  namespace: flux
type: Opaque
data:
  identity: {{ .Config.GitPrivateKey }}
`

// GenerateFluxConfig generates the config for flux CD
func GenerateFluxConfig(path string, cfg *model.FluxConfig) error {
	var (
		f   *os.File
		err error
	)
	t := template.Must(template.New("fluxConfig").Parse(fluxTmpl))
	if f, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644); err != nil {
		return fmt.Errorf("error opening output file at \"%s\": %w", path, err)
	}
	defer f.Close()
	if err = t.Execute(f, struct {
		Config *model.FluxConfig
	}{cfg}); err != nil {
		return fmt.Errorf("error executing template: %w", err)
	}
	return nil
}
