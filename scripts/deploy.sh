#!/usr/bin/env bash
# Deploy (or update) the infra and application stacks on the local Docker Swarm.
# Usage: scripts/deploy.sh [infra|observability|services|all]   (default: all = infra + observability + services)
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
env_file="${ENV_FILE:-$root/deploy/.env}"
[ -f "$env_file" ] || { echo "Missing $env_file (copy deploy/.env.example)"; exit 1; }

# `docker stack deploy` does not understand variable interpolation defaults, the top-level
# `name:` that `docker compose config` adds, or quoted port numbers, so render and fix first.
render() {
  docker compose --env-file "$env_file" -f "$1" config 2>/dev/null \
    | sed -e '/^name:/d' -E -e 's/^( *published: )"([0-9]+)"$/\1\2/'
}

deploy() { render "$root/deploy/$1.yml" | docker stack deploy --detach=true -c - "$2"; }

case "${1:-all}" in
  infra)    deploy infra marketplace-infra ;;
  observability) deploy observability marketplace-obs ;;
  services) deploy services marketplace ;;
  all)      deploy infra marketplace-infra; deploy observability marketplace-obs; deploy services marketplace ;;
  *) echo "usage: $0 [infra|observability|services|all]"; exit 1 ;;
esac
