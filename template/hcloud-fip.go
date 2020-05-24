package template

import (
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/shark/hcloud-k3os-configurator/model"
)

const hcloudFIPTmpl = `---
apiVersion: v1
kind: Secret
metadata:
  name: fip-controller-secrets
stringData:
  HCLOUD_API_TOKEN: {{ .Token }}

---
apiVersion: v1
kind: ConfigMap
metadata:
  name: fip-controller-config
data:
  config.json: |
    {
      "hcloud_floating_ips": [{{ .FloatingIPsJSON }}],
      "lease_name": "hcloud-fip"
    }
`

// GenerateHCloudFIPConfig generates the config for hcloud-fip
func GenerateHCloudFIPConfig(path string, token string, floatingIPs []*model.IPAddress) error {
	var (
		quotedFloatingIPs []string
		f                 *os.File
		err               error
	)
	for _, fip := range floatingIPs {
		quotedFloatingIPs = append(quotedFloatingIPs, fmt.Sprintf("\"%s\"", fip.Net.IP.String()))
	}
	t := template.Must(template.New("hcloudFIPConfig").Parse(hcloudFIPTmpl))
	if f, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644); err != nil {
		return fmt.Errorf("error opening output file at \"%s\": %w", path, err)
	}
	defer f.Close()
	if err = t.Execute(f, struct {
		Token           string
		FloatingIPsJSON string
	}{token, strings.Join(quotedFloatingIPs, ",")}); err != nil {
		return fmt.Errorf("error executing template: %w", err)
	}
	return nil
}
