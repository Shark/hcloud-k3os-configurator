package template

import (
	"fmt"
	"os"
	"text/template"
)

const dnsTmpl = `nameserver 213.133.98.98
nameserver 213.133.99.99
nameserver 213.133.100.100
`

// GenerateDNSConfig generates the resolver config
func GenerateDNSConfig(path string) error {
	var (
		f   *os.File
		err error
	)
	t := template.Must(template.New("dnsConfig").Parse(dnsTmpl))
	if f, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644); err != nil {
		return fmt.Errorf("error opening output file at \"%s\": %w", path, err)
	}
	defer f.Close()
	if err = t.Execute(f, nil); err != nil {
		return fmt.Errorf("error executing template: %w", err)
	}
	return nil
}
