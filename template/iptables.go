package template

import (
	"fmt"
	"os"
	"text/template"

	"github.com/shark/hcloud-k3os-configurator/model"
)

const iptablesTmpl = `*filter
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
-A INPUT -i cni0 -s 10.42.0.0/16 -j ACCEPT
-A INPUT -m limit --limit 5/min -j LOG --log-prefix "iptables-rejected: "
-A INPUT -p udp -j REJECT --reject-with icmp-port-unreachable
-A INPUT -p tcp -j REJECT --reject-with tcp-reset
-A INPUT -j REJECT --reject-with icmp-proto-unreachable

-A TCP -p tcp --dport 22 -j ACCEPT
-A TCP -p tcp -m multiport --dports 80,443 -j ACCEPT
-A TCP -p tcp --dport 6443 -j ACCEPT

# k3s
-A TCP -s {{ .PrivateNetwork }} -j ACCEPT
-A UDP -s {{ .PrivateNetwork }} -j ACCEPT

COMMIT
`

const ip6tablesTmpl = `*filter
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
-A TCP -p tcp --dport 6443 -j ACCEPT

COMMIT

*raw
:PREROUTING ACCEPT [0:0]
:OUTPUT ACCEPT [0:0]
-A PREROUTING -p ipv6-icmp -j ACCEPT
-A PREROUTING -m rpfilter -j ACCEPT
-A PREROUTING -j DROP

COMMIT
`

// GenerateIptablesConfig generates the iptables config (IPv4)
func GenerateIptablesConfig(path string, privnet *model.Network) error {
	var (
		f   *os.File
		err error
	)
	t := template.Must(template.New("iptablesConfig").Parse(iptablesTmpl))
	if f, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644); err != nil {
		return fmt.Errorf("error opening output file at \"%s\": %w", path, err)
	}
	defer f.Close()
	if err = t.Execute(f, struct {
		PrivateNetwork string
	}{privnet.IPv4Addresses[0].Net.String()}); err != nil {
		return fmt.Errorf("error executing template: %w", err)
	}
	return nil
}

// GenerateIP6tablesConfig generates the ip6tables config (IPv6)
func GenerateIP6tablesConfig(path string) error {
	var (
		f   *os.File
		err error
	)
	t := template.Must(template.New("ip6tablesConfig").Parse(ip6tablesTmpl))
	if f, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644); err != nil {
		return fmt.Errorf("error opening output file at \"%s\": %w", path, err)
	}
	defer f.Close()
	if err = t.Execute(f, nil); err != nil {
		return fmt.Errorf("error executing template: %w", err)
	}
	return nil
}
