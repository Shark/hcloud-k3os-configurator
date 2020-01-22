package config

import (
	"fmt"
	"io"
	"text/template"

	"github.com/shark/hcloud-k3os-configurator/node"
)

const tmpl = `---
hostname: {{ .Node.Name }}

write_files:
  - path: /var/lib/connman/public.config
    content: |
      [service_eth]
      Type=ethernet
      MAC={{ .Node.PublicMAC }}
      IPv4={{ .Node.IPv4Address }}/32/172.31.1.1
      IPv6={{ .Node.IPv6Subnet }}/fe80::1
      Nameservers=1.1.1.1,1.0.0.1
      TimeServers=ntp1.hetzner.de,ntp2.hetzner.com,ntp3.hetzner.net
{{ range .PrivateNetworks }}
  - path: /var/lib/connman/private_{{ .ID }}.config
    content: |
      [service_eth]
      Type=ethernet
      MAC={{ .MAC }}
      IPv4={{ .IP }}/{{ .PrefixLength }}
      IPv6=off
{{ end }}
  - path: /etc/iptables/rules.v4
    content: |
      *filter
      :INPUT DROP [0:0]
      :FORWARD DROP [0:0]
      :OUTPUT ACCEPT [0:0]
      :TCP - [0:0]
      :UDP - [0:0]

      # Input
      -A INPUT -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
      -A INPUT -i lo -j ACCEPT
      -A INPUT -m conntrack --ctstate INVALID -j DROP
      -A INPUT -p icmp -m icmp --icmp-type 8 -m conntrack --ctstate NEW -j ACCEPT
      -A INPUT -p udp -m conntrack --ctstate NEW -j UDP
      -A INPUT -p tcp --tcp-flags FIN,SYN,RST,ACK SYN -m conntrack --ctstate NEW -j TCP
      -A INPUT -m limit --limit 5/min -j LOG --log-prefix "iptables-rejected: "
      -A INPUT -p udp -j REJECT --reject-with icmp-port-unreachable
      -A INPUT -p tcp -j REJECT --reject-with tcp-reset
      -A INPUT -j REJECT --reject-with icmp-proto-unreachable

      -A TCP -p tcp --dport 22 -j ACCEPT
      -A TCP -p tcp -m multiport --dports 80,443 -j ACCEPT

{{ range .PrivateNetworks }}
      # k3s
      -A TCP -s {{ .NetworkIP }}/{{ .PrefixLength }} -j ACCEPT
      -A UDP -s {{ .NetworkIP }}/{{ .PrefixLength }} -j ACCEPT
{{ end }}

      COMMIT

  - path: /etc/iptables/rules.v6
    content: |
      *filter
      :INPUT DROP [0:0]
      :FORWARD DROP [0:0]
      :OUTPUT ACCEPT [0:0]
      :TCP - [0:0]
      :UDP - [0:0]

      # Input
      -A INPUT -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
      -A INPUT -i lo -j ACCEPT
      -A INPUT -m conntrack --ctstate INVALID -j DROP
      -A INPUT -m rt --rt-type 0 -j DROP
      -A INPUT -p icmpv6 -j ACCEPT
      -A INPUT -p udp -m conntrack --ctstate NEW -j UDP
      -A INPUT -p tcp --tcp-flags FIN,SYN,RST,ACK SYN -m conntrack --ctstate NEW -j TCP
      -A INPUT -m limit --limit 5/min -j LOG --log-prefix "iptables-rejected: "
      -A INPUT -p udp -j REJECT --reject-with icmp6-port-unreachable
      -A INPUT -p tcp -j REJECT --reject-with tcp-reset
      -A INPUT -j REJECT

      # Output
      -A OUTPUT -m rt --rt-type 0 -j DROP

      -A TCP -p tcp --dport 22 -j ACCEPT
      -A TCP -p tcp -m multiport --dports 80,443 -j ACCEPT

      COMMIT

      *raw
      :PREROUTING ACCEPT [0:0]
      :OUTPUT ACCEPT [0:0]
      -A PREROUTING -p ipv6-icmp -j ACCEPT
      -A PREROUTING -m rpfilter -j ACCEPT
      -A PREROUTING -j DROP

      COMMIT

boot_cmd:
  - "iptables-restore < /etc/iptables/rules.v4"
  - "ip6tables-restore < /etc/iptables/rules.v6"

{{ if .FloatingIPs }}run_cmd:
{{ range .FloatingIPs }}  - "ip addr add {{ .IP }} dev eth0"
{{ end }}{{ end }}`

// Generate outputs a k3os YAML config file to the specified io.Writer
func Generate(out io.Writer, config *node.Config, privateNetworks []*node.PrivateNetwork, floatingIPs []*node.FloatingIP) error {
	t := template.Must(template.New("config").Parse(tmpl))
	err := t.Execute(out, struct {
		Node            *node.Config
		PrivateNetworks []*node.PrivateNetwork
		FloatingIPs     []*node.FloatingIP
	}{config, privateNetworks, floatingIPs})
	if err != nil {
		return fmt.Errorf("error executing template: %w", err)
	}
	return nil
}
