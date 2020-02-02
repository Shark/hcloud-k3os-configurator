package config

import (
	"fmt"
	"io"
	"strings"
	"text/template"

	"github.com/shark/hcloud-k3os-configurator/node"
)

const tmpl = `---
hostname: {{ .Node.Name }}

k3os:
  {{ if .Node.JoinURL }}server_url: "{{ .Node.JoinURL }}"{{ end }}
  k3s_args:
    {{ if eq .Node.Role "server" }}- "server"
    - "--advertise-address"
    - "{{ .Node.PrivateIPv4Address }}"{{ end }}
    {{ if eq .Node.Role "agent" }}- "agent"{{ end }}
    - "--node-name"
    - "{{ .Node.ShortName }}"
    - "--node-ip"
    - "{{ .Node.PrivateIPv4Address }}"
    - "--node-external-ip"
    - "{{ .Node.IPv4Address }}"
    - "--flannel-iface"
    - "eth1"

write_files:
  - path: /opt/configure_networking.sh
    permissions: '0755'
    content: |
      #!/usr/bin/env bash
      set -euo pipefail; [[ "${TRACE-}" ]] && set -x
      DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

      main() {
        stop_connman
        configure_dns
        configure_private_networks || true
        configure_public_ipv4 || true
        configure_floating_ips || true
        configure_public_ipv6 || true
      }

      stop_connman() {
        >&2 echo "Stopping connman"
        set -x
        /etc/init.d/connman stop
        [[ "${TRACE-}" ]] || set +x
      }

      configure_dns() {
        >&2 echo "Configuring DNS"
        set -x
        rm -f /etc/resolv.conf
        cat > /etc/resolv.conf <<EOF
      nameserver 213.133.98.98
      nameserver 213.133.99.99
      nameserver 213.133.100.100
      EOF
        [[ "${TRACE-}" ]] || set +x
      }

      configure_private_networks() {
        >&2 echo "Configuring private networks"
        set -x
      {{ range .PrivateNetworks }}  ip link set up dev {{ .DeviceName }}
        ip addr add {{ .IP }}/32 dev {{ .DeviceName }}
        ip route add {{ .GatewayIP }} dev {{ .DeviceName }}
        ip route add {{ .NetworkIP }}/{{ .PrefixLengthBits }} via {{ .GatewayIP }}
      {{ end }}  [[ "${TRACE-}" ]] || set +x
      }

      configure_public_ipv4() {
        >&2 echo "Configuring public IPv4"
        set -x
        ip link set up dev eth0
        ip addr add {{ .Node.IPv4Address }}/32 dev eth0
        ip route add 172.31.1.1 dev eth0 src {{ .Node.IPv4Address }}
        ip route del default || true
        ip route add default via 172.31.1.1
        [[ "${TRACE-}" ]] || set +x
      }

      configure_floating_ips() {
        >&2 echo "Configuring floating IPs"
        set -x
      {{ range .FloatingIPs }}  ip addr add {{ .IP }} dev {{ .DeviceName }}
      {{ end }}  [[ "${TRACE-}" ]] || set +x
      }

      configure_public_ipv6() {
        >&2 echo "Configuring public IPv6"
        set -x
        ip -6 addr add {{ .Node.IPv6Subnet }} dev eth0
        ip -6 route del default || true
        sleep 10 # not entirely sure why this is needed, presumably route to fe80::1 is only valid after autoconfiguration
        ip -6 route add default via fe80::1 dev eth0 src {{ .Node.IPv6Address }}
        [[ "${TRACE-}" ]] || set +x
      }

      main "$@"
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
      -A INPUT -i cni0 -s 10.42.0.0/16 -j ACCEPT
      -A INPUT -m limit --limit 5/min -j LOG --log-prefix "iptables-rejected: "
      -A INPUT -p udp -j REJECT --reject-with icmp-port-unreachable
      -A INPUT -p tcp -j REJECT --reject-with tcp-reset
      -A INPUT -j REJECT --reject-with icmp-proto-unreachable

      -A TCP -p tcp --dport 22 -j ACCEPT
      -A TCP -p tcp -m multiport --dports 80,443 -j ACCEPT
      -A TCP -p tcp --dport 6443 -j ACCEPT

{{ range .PrivateNetworks }}
      # k3s
      -A TCP -s {{ .NetworkIP }}/{{ .PrefixLengthBits }} -j ACCEPT
      -A UDP -s {{ .NetworkIP }}/{{ .PrefixLengthBits }} -j ACCEPT
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
      -A TCP -p tcp --dport 6443 -j ACCEPT

      COMMIT

      *raw
      :PREROUTING ACCEPT [0:0]
      :OUTPUT ACCEPT [0:0]
      -A PREROUTING -p ipv6-icmp -j ACCEPT
      -A PREROUTING -m rpfilter -j ACCEPT
      -A PREROUTING -j DROP

      COMMIT

  - path: /opt/hcloud-csi/secret.yaml
    content: |
      ---
      apiVersion: v1
      kind: Secret
      metadata:
        name: hcloud-csi
      stringData:
        token: {{ .Token }}

  - path: /opt/hcloud-fip/config.yaml
    content: |
      ---
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

{{ if .Node.FluxEnable }}
  - path: /opt/flux/patch.yaml
    content: |
      ---
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
              - --git-url={{ .Node.FluxGitURL }}

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

      {{ if .Node.FluxGitPrivateKey }}
      ---
      apiVersion: v1
      kind: Secret
      metadata:
        name: flux-git-deploy
        namespace: flux
      type: Opaque
      data:
        identity: {{ .Node.FluxGitPrivateKey }}

      {{ end }}
{{ end }}

boot_cmd:
  - "iptables-restore < /etc/iptables/rules.v4"
  - "ip6tables-restore < /etc/iptables/rules.v6"

run_cmd:
  - "/opt/configure_networking.sh"
  - "kubectl kustomize /opt/hcloud-csi > /var/lib/rancher/k3s/server/manifests/hcloud-csi.yaml"
  - "kubectl kustomize /opt/hcloud-fip > /var/lib/rancher/k3s/server/manifests/hcloud-fip.yaml"
  - "[[ -f /opt/flux/patch.yaml ]] && kubectl kustomize /opt/flux > /var/lib/rancher/k3s/server/manifests/flux.yaml"
`

// Generate outputs a k3os YAML config file to the specified io.Writer
func Generate(out io.Writer, config *node.Config, privateNetworks []*node.PrivateNetwork, floatingIPs []*node.FloatingIP, token string) error {
	var quotedFloatingIPs []string
	for _, fip := range floatingIPs {
		quotedFloatingIPs = append(quotedFloatingIPs, fmt.Sprintf("\"%s\"", fip.IP))
	}
	t := template.Must(template.New("config").Parse(tmpl))
	err := t.Execute(out, struct {
		Node            *node.Config
		PrivateNetworks []*node.PrivateNetwork
		FloatingIPs     []*node.FloatingIP
		FloatingIPsJSON string
		Token           string
	}{config, privateNetworks, floatingIPs, strings.Join(quotedFloatingIPs, ","), token})
	if err != nil {
		return fmt.Errorf("error executing template: %w", err)
	}
	return nil
}
