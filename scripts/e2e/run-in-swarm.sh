#!/usr/bin/env bash
# Run the e2e suite from a throw-away container attached to the swarm network.
# Useful on hosts where the swarm ingress ports are not reachable from the host itself.
# Usage: scripts/e2e/run-in-swarm.sh [extra env, e.g. RATE_LIMIT_TEST=1]
set -euo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
host="${PUBLIC_HOST:-marketplace.swarm.localhost}"
# Address of the running Traefik container on the swarm network (the service VIP needs IPVS,
# which is not available on every host).
cid=$(docker ps -q -f name=marketplace_traefik | head -1)
vip=$(docker inspect "$cid" --format '{{(index .NetworkSettings.Networks "marketplace-net").IPAddress}}')
[ -n "$vip" ] || { echo "traefik container IP on marketplace-net not found"; exit 1; }
dash_auth="${DASH_AUTH:-admin:admin}"
docker run --rm --network marketplace-net \
  --add-host "$host:$vip" --add-host "api.$host:$vip" --add-host "dashboard.$host:$vip" \
  -v "$root/scripts/e2e:/e2e:ro" -v "$root/deploy/traefik/certs/local.crt:/ca.crt:ro" \
  -e NODE_EXTRA_CA_CERTS=/ca.crt -e API_URL="https://api.$host" -e UI_URL="https://$host" \
  -e DASH_URL="https://dashboard.$host" -e DASH_AUTH="$dash_auth" -e RATE_LIMIT_TEST="${RATE_LIMIT_TEST:-0}" \
  --entrypoint node marketplace/frontend:latest /e2e/e2e.mjs
