#!/usr/bin/env bash
set -euo pipefail; [[ "${TRACE-}" ]] && set -x
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

mkdir -p /var/lib/rancher/k3os/config.d

config_file='/var/lib/rancher/k3os/config.d/configurator.yaml'
config_hash_before=''

if [[ -f $config_file ]]; then
  config_hash_before="$(sha256sum "$config_file" | cut -d' ' -f1)"
fi

/opt/hcloud-k3os-configurator

config_hash_after="$(sha256sum "$config_file" | cut -d' ' -f1)"

# if config changed, reboot
if [[ $config_hash_before != $config_hash_after ]]; then
  >&2 echo "config changed, rebooting"
  reboot
fi
