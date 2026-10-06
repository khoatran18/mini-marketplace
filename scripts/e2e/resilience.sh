#!/usr/bin/env bash
# Failure-injection tests against the deployed Swarm stacks (Kafka outage, consumer outage,
# service restarts). Requires scripts/e2e/run-in-swarm.sh prerequisites.
set -uo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
host="${PUBLIC_HOST:-marketplace.swarm.localhost}"
state="$(mktemp -d)"
cid=$(docker ps -q -f name=marketplace_traefik | head -1)
vip=$(docker inspect "$cid" --format '{{(index .NetworkSettings.Networks "marketplace-net").IPAddress}}')
fails=0

step() { # step <STEP> [extra docker env...]
  local s="$1"; shift
  docker run --rm --user root --network marketplace-net --add-host "api.$host:$vip" \
    -v "$root/scripts/e2e:/e2e:ro" -v "$root/deploy/traefik/certs/local.crt:/ca.crt:ro" -v "$state:/state" \
    -e NODE_EXTRA_CA_CERTS=/ca.crt -e API_URL="https://api.$host" -e STEP="$s" "$@" \
    --entrypoint node marketplace/frontend:latest /e2e/resilience.mjs
}
expect() { # expect <name> <condition-result>
  if [ "$2" = "0" ]; then echo "  ok   $1"; else echo "  FAIL $1"; fails=$((fails+1)); fi
}
scale() { docker service scale --detach=false "$1=$2" >/dev/null 2>&1; }
ready() { # wait until a service has all replicas running
  for _ in $(seq 1 60); do
    r=$(docker service ls --filter "name=$1" --format '{{.Replicas}}' | head -1)
    [ "${r%/*}" = "${r#*/}" ] && [ "${r%/*}" != "0" ] && return 0; sleep 2
  done; return 1
}
json() { python3 -c "import json,sys; d=json.loads(sys.argv[1]); print(d.get(sys.argv[2]))" "$1" "$2"; }

echo "[setup]"; step setup | tail -1

echo "[A] Kafka broker down: orders are accepted and processed once it is back"
scale marketplace-infra_broker1 0
sleep 5
out=$(step order); echo "  $out"
expect "order accepted while Kafka is down (outbox)" "$([[ "$out" == *"http=200"* ]]; echo $?)"
sleep 5
st=$(step status); echo "  $st"
expect "order is still PENDING while Kafka is down" "$([ "$(json "$st" PENDING)" = "1" ]; echo $?)"
scale marketplace-infra_broker1 1; ready marketplace-infra_broker1
st=$(step wait -e WAIT_MS=120000); echo "  $st"
expect "order reaches SUCCESS after Kafka returns" "$([ "$(json "$st" SUCCESS)" = "1" ]; echo $?)"
expect "inventory reserved exactly once (10 → 9)" "$([ "$(json "$st" inventory)" = "9" ]; echo $?)"

echo "[B] product-service down: orders wait in Kafka and are processed when it returns"
scale marketplace_product-service 0
sleep 3
out=$(step order); echo "  $out"
# the order service re-prices via product-service, so creation is refused with 503 instead of hanging
expect "order creation fails fast (503) when the catalog is unavailable" "$([[ "$out" == *"http=503"* ]]; echo $?)"
scale marketplace_product-service 1; ready marketplace_product-service; sleep 10

echo "[C] order-service down after accepting: validation result waits and is applied on restart"
out=$(step order); echo "  $out"
scale marketplace_order-service 0
sleep 12
scale marketplace_order-service 1; ready marketplace_order-service
st=$(step wait -e WAIT_MS=90000); echo "  $st"
expect "no order stays PENDING after order-service restart" "$([ "$(json "$st" PENDING)" = "0" ]; echo $?)"
expect "two successful orders, inventory 8" "$([ "$(json "$st" SUCCESS)" = "2" ] && [ "$(json "$st" inventory)" = "8" ]; echo $?)"

echo "[D] Rolling restart of every service keeps data and tokens"
for s in api-gateway auth-service user-service product-service order-service; do
  docker service update --force --detach=true "marketplace_$s" >/dev/null 2>&1
done
sleep 30; for s in api-gateway auth-service user-service product-service order-service; do ready "marketplace_$s"; done
sleep 10
st=$(step status); echo "  $st"
expect "orders and inventory survive the restart" "$([ "$(json "$st" SUCCESS)" = "2" ] && [ "$(json "$st" inventory)" = "8" ]; echo $?)"

echo "[E] Postgres restart: services reconnect"
docker service update --force --detach=true marketplace-infra_postgres >/dev/null 2>&1
sleep 25; ready marketplace-infra_postgres; sleep 15
out=$(step order); echo "  $out"
expect "orders work again after the database restarted" "$([[ "$out" == *"http=200"* ]]; echo $?)"
st=$(step wait -e WAIT_MS=90000); echo "  $st"
expect "order after Postgres restart is processed (3 successes, inventory 7)" "$([ "$(json "$st" SUCCESS)" = "3" ] && [ "$(json "$st" inventory)" = "7" ]; echo $?)"

rm -rf "$state"
echo; [ "$fails" = 0 ] && echo "resilience: all checks passed" || { echo "resilience: $fails check(s) failed"; exit 1; }
