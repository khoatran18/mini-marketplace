#!/usr/bin/env bash
# Một lượt đo ĐÚNG QUY TẮC CLAUDE.md: dựng sạch -> đo -> lưu mọi số liệu và log -> GIỮ NGUYÊN stack để phân tích. KHÔNG tự dọn.
#   scripts/load/perf-one.sh NHÃN [tham số ccu.mjs...]       vd: perf-one.sh spike-800-r2 --vus=800 --ramp=10
# Biến: OUT_DIR (mặc định docs/perf-results/<ngày>), DUR (giây ổn định, 90), CAP_IV (giây giữa hai mẫu capture, 1), SER_STEP (bước chuỗi Prometheus, 15),
#       DEPLOY=0 để đo trên stack đang chạy (không dọn, không dựng lại), LOG_TIMEOUT (giây tối đa cho mỗi service khi dump log, 120).
# Kết quả trong OUT_DIR: NHÃN.* (perf-run), NHÃN.capture.jsonl, NHÃN.series.json, NHÃN.diag.json, logs-NHÃN/<service>.log.
# Chịu lỗi: FAULT=<hành động của fault.sh> (kill:auth-service, redis-flush, kafka-kill:1, postgres-kill), FAULT_AT=giây sau khi bắt đầu đo (60), FAULT_DRAIN=giây chờ outbox bắt kịp trước khi kiểm dữ liệu (60).
# Dọn sau khi phân tích xong: scripts/teardown.sh --volumes -y   (người dùng được báo trước).
set -uo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"; cd "$root"
label="${1:?thiếu NHÃN}"; shift
export OUT_DIR="${OUT_DIR:-docs/perf-results/$(date +%F)}"; mkdir -p "$OUT_DIR"
API=http://api.marketplace.swarm.localhost
ready() {
  for _ in $(seq 1 90); do
    bad=$(docker service ls --format '{{.Replicas}}' | awk -F'[/ ]' '$1!=$2' | wc -l); n=$(docker service ls -q | wc -l)
    [ "$n" -ge 27 ] && [ "$bad" = 0 ] && [ "$(curl -s -o /dev/null -w '%{http_code}' $API/health)" = 200 ] && return 0
    sleep 5
  done; return 1
}
if [ "${DEPLOY:-1}" = 1 ]; then
  echo "=== $(date +%T) $label: dọn stack cũ (nếu có) + dựng"; ./scripts/teardown.sh -y --volumes >/dev/null 2>&1
  docker volume ls -q | grep '^deploy_' | xargs -r docker volume rm >/dev/null 2>&1
  sleep 20
  ./scripts/deploy-perf.sh all || { echo "DEPLOY LỖI ở $label"; exit 1; }
  ready || { echo "KHÔNG SẴN SÀNG ở $label"; docker service ls; exit 1; }
  sleep 60
fi
SINCE=$(date -u +%Y-%m-%dT%H:%M:%SZ); T0=$(date +%s)
python3 scripts/load/perf-capture.py "$OUT_DIR/$label.capture.jsonl" "${CAP_IV:-1}" & cp=$!
echo "=== $(date +%T) $label: đo"
if [ -n "${FAULT:-}" ]; then scripts/load/fault.sh "$FAULT" "${FAULT_AT:-60}" "$OUT_DIR/$label.fault.log" "$OUT_DIR/$label.txt" & fp=$!; fi
./scripts/load/perf-run.sh "$label" "$@" --duration="${DUR:-90}" || echo "perf-run $label trả mã lỗi (xem phép đo hợp lệ trong $OUT_DIR/$label.txt)"
kill $cp 2>/dev/null; wait $cp 2>/dev/null
if [ -n "${FAULT:-}" ]; then
  wait $fp 2>/dev/null; sleep "${FAULT_DRAIN:-60}"
  scripts/load/fault-check.sh "$OUT_DIR/$label.datacheck.txt" | tail -n 30
  docker service ps --no-trunc --format '{{.Name}} {{.DesiredState}} {{.CurrentState}} {{.Error}}' $(docker service ls -q) > "$OUT_DIR/$label.tasks.txt" 2>&1
fi
python3 scripts/load/perf-series.py "$OUT_DIR/$label.series.json" "$T0" "$(date +%s)" "${SER_STEP:-15}" 2>&1 | tail -1
python3 scripts/load/perf-diag.py "$OUT_DIR" "$label" 2>&1 | tee -a "$OUT_DIR/$label.txt" | tail -3
# log từng service từ lúc bắt đầu đo (song song, có timeout: `docker service logs` có thể treo và không có --until)
ld="$OUT_DIR/logs-$label"; mkdir -p "$ld"
for svc in marketplace_api-gateway marketplace_auth-service marketplace_user-service marketplace_product-service marketplace_order-service \
           marketplace_payment-service marketplace_analytics-service marketplace_ops-service marketplace_ai-agent marketplace_frontend marketplace_traefik marketplace-infra_postgres; do
  timeout "${LOG_TIMEOUT:-120}" docker service logs --since "$SINCE" --timestamps "$svc" > "$ld/${svc#*_}.log" 2>&1 &
done
wait
echo "=== $(date +%T) $label: XONG ĐO. STACK GIỮ NGUYÊN để phân tích; log ở $ld. Dọn sau khi phân tích: scripts/teardown.sh --volumes -y"
