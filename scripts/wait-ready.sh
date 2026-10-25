#!/usr/bin/env bash
# Chờ các stack Swarm đạt N/N replica và /ready của từng service Go trả 200.
# Usage: scripts/wait-ready.sh [infra|observability|services|all]    (mặc định lấy WAIT_TARGET hoặc all)
# Biến: WAIT_TIMEOUT (giây, mặc định 300), WAIT_INTERVAL (mặc định 5),
#       WAIT_PROBE_IMAGE (image có wget để probe, mặc định marketplace/api-gateway:latest, không cần pull),
#       WAIT_SKIP_READY=1 (chỉ kiểm N/N, bỏ probe /ready).
# Thoát 0 khi tất cả sẵn sàng; thoát 1 khi quá hạn (in ra thứ chưa sẵn sàng).
set -euo pipefail
target="${1:-${WAIT_TARGET:-all}}"
timeout="${WAIT_TIMEOUT:-300}"
interval="${WAIT_INTERVAL:-5}"
probe_image="${WAIT_PROBE_IMAGE:-marketplace/api-gateway:latest}"

stacks=()
case "$target" in
  infra)         stacks=(marketplace-infra) ;;
  observability) stacks=(marketplace-obs) ;;
  services)      stacks=(marketplace) ;;
  all)           stacks=(marketplace-infra marketplace-obs marketplace) ;;
  *) echo "usage: $0 [infra|observability|services|all]"; exit 2 ;;
esac

# in ra các service chưa đạt N/N (mỗi dòng một service)
not_converged() {
  local s
  for s in "${stacks[@]}"; do
    docker stack services --format '{{.Name}} {{.Replicas}}' "$s" 2>/dev/null | awk '
      { split($2, r, "[/ ]"); if (r[1] != r[2] || r[2] == 0) print $1 " (" $2 ")" }'
  done
}

# service Go có admin port :8081 và /ready (đặt trong stack marketplace)
go_services=(api-gateway auth-service user-service product-service order-service payment-service analytics-service)
not_ready() {
  local svc
  for svc in "${go_services[@]}"; do
    docker run --rm --network marketplace-net --entrypoint wget "$probe_image" \
      -q -T 3 -O /dev/null "http://tasks.$svc:8081/ready" >/dev/null 2>&1 || echo "$svc"
  done
}

deadline=$((SECONDS + timeout))
echo "Chờ sẵn sàng (${stacks[*]}), tối đa ${timeout}s..."
while :; do
  pending="$(not_converged || true)"
  if [ -z "$pending" ] && { [ "${WAIT_SKIP_READY:-0}" = "1" ] || [[ ! " ${stacks[*]} " =~ " marketplace " ]]; }; then
    echo "OK: mọi service đạt N/N."; exit 0
  fi
  if [ -z "$pending" ]; then
    pending="$(not_ready || true)"
    if [ -z "$pending" ]; then echo "OK: mọi service N/N và /ready = 200."; exit 0; fi
    pending="chưa /ready: $(echo $pending)"
  fi
  if [ "$SECONDS" -ge "$deadline" ]; then
    echo "THẤT BẠI sau ${timeout}s, chưa sẵn sàng:"; echo "$pending"
    echo "Gợi ý: docker service ps --no-trunc <service>; docker service logs <service>"
    exit 1
  fi
  echo "  đang chờ: $(echo $pending | tr '\n' ' ')"
  sleep "$interval"
done
