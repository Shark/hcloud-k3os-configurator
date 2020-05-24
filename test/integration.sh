#!/usr/bin/env bash
set -euo pipefail; [[ "${TRACE-}" ]] && set -x
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

main() {
  #docker-compose build
  docker-compose up -d hcloud
  docker-compose run --rm api-mock 4406144

  docker-compose up -d app
  sleep 5
  local rc=0
  docker-compose exec app test -f /var/lib/hcloud-k3os/.running || rc=$?
  if [[ $rc -ne 0 ]]; then
    docker-compose logs app
    exit 1
  fi

  docker-compose rm -f -s -v app
  docker-compose run --rm api-mock 4406228

  docker-compose up -d app
  sleep 5
  local rc=0
  docker-compose exec app test -f /var/lib/hcloud-k3os/.running || rc=$?
  if [[ $rc -ne 0 ]]; then
    docker-compose logs app
    exit 1
  fi

  docker-compose down -v --rmi all
}

main "$@"