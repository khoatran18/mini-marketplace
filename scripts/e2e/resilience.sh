#!/usr/bin/env bash
# Tiêm lỗi trên stack Docker Swarm đang chạy: dừng/khôi phục từng thành phần bằng `docker service scale`, rồi kiểm tra
# hệ thống suy giảm đúng cách và tự hồi phục. Cần deploy trước (scripts/deploy.sh), image marketplace/frontend:latest (có node)
# và ADMIN_BOOTSTRAP_* (để kiểm /admin/system/health; không có thì bỏ qua phần đó).
#
# Usage: scripts/e2e/resilience.sh [A B C ... J]      (mặc định chạy tất cả; mỗi chữ là một kịch bản)
#   A ClickHouse down   B Kafka down (cả 3 broker)   C payment-service down   D product-service down
#   E Redis down        F Postgres down (/ready 503)           G order-service down sau khi nhận đơn
#   H rolling restart   I Postgres restart   J một broker Kafka down (cụm 3 broker)
# Biến: PUBLIC_HOST, ADMIN_USERNAME/ADMIN_PASSWORD, STACK_INFRA (marketplace-infra), STACK_APP (marketplace)
set -uo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
host="${PUBLIC_HOST:-marketplace.swarm.localhost}"
scheme="${PUBLIC_SCHEME:-$(grep -E '^PUBLIC_SCHEME=' "$root/deploy/.env" 2>/dev/null | tail -1 | cut -d= -f2 | sed 's/[[:space:]#].*//')}"; scheme="${scheme:-http}"
ca=(); [ "$scheme" = https ] && ca=(-v "$root/deploy/traefik/certs/local.crt:/ca.crt:ro" -e NODE_EXTRA_CA_CERTS=/ca.crt)
INFRA="${STACK_INFRA:-marketplace-infra}"
APP="${STACK_APP:-marketplace}"
state="$(mktemp -d)"
fails=0
want=" ${*:-A B C D E F G H I J} "

# Tài khoản admin (cho /admin/system/health): lấy từ biến môi trường, không có thì đọc ADMIN_BOOTSTRAP_* trong deploy/.env
envval() { [ -f "$root/deploy/.env" ] && grep -E "^$1=" "$root/deploy/.env" | tail -1 | cut -d= -f2- | sed 's/[[:space:]]*#.*$//' || true; }
ADMIN_USERNAME="${ADMIN_USERNAME:-${ADMIN_BOOTSTRAP_USERNAME:-$(envval ADMIN_BOOTSTRAP_USERNAME)}}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-${ADMIN_BOOTSTRAP_PASSWORD:-$(envval ADMIN_BOOTSTRAP_PASSWORD)}}"

cid=$(docker ps -q -f name="${APP}_traefik" | head -1)
[ -n "$cid" ] || { echo "không tìm thấy container traefik của stack $APP (đã deploy chưa?)"; exit 1; }
vip=$(docker inspect "$cid" --format '{{(index .NetworkSettings.Networks "marketplace-net").IPAddress}}')

# step <STEP> [-e K=V ...]: chạy resilience.mjs trong container tạm gắn vào mạng swarm, in 1 dòng JSON
step() {
  local s="$1"; shift
  docker run --rm --user root --network marketplace-net --add-host "api.$host:$vip" \
    -v "$root/scripts/e2e:/e2e:ro" "${ca[@]+"${ca[@]}"}" -v "$state:/state" \
    -e API_URL="$scheme://api.$host" -e STEP="$s" -e STATE_FILE=/state/state.json \
    -e ADMIN_USERNAME="${ADMIN_USERNAME:-${ADMIN_BOOTSTRAP_USERNAME:-}}" -e ADMIN_PASSWORD="${ADMIN_PASSWORD:-${ADMIN_BOOTSTRAP_PASSWORD:-}}" "$@" \
    --entrypoint node marketplace/frontend:latest /e2e/resilience.mjs 2>&1 | tail -1
}
# j '<json>' key  -> giá trị (hỗ trợ khoá lồng nhau a.b.c); rỗng nếu không có
j() { python3 -c "
import json,sys
try:
    d=json.loads(sys.argv[1])
    for k in sys.argv[2].split('.'): d=d.get(k) if isinstance(d,dict) else None
    print('' if d is None else (json.dumps(d) if isinstance(d,(dict,list)) else d))
except Exception: print('')" "$1" "$2"; }
expect() { # expect <mô tả> <mã thoát của điều kiện>
  if [ "$2" = "0" ]; then echo "  ok   $1"; else echo "  FAIL $1"; fails=$((fails+1)); fi
}
eq() { [ "$1" = "$2" ]; }

# Dừng/khởi động service: scale + chờ ổn định
scale() { docker service scale --detach=true "$1=$2" >/dev/null 2>&1; }
running() { # running <service> -> số replica đang chạy
  docker service ls --filter "name=$1" --format '{{.Name}} {{.Replicas}}' | awk -v n="$1" '$1==n{split($2,a,"/"); print a[1]}'
}
wait_up() { # wait_up <service> [giây]: đợi đủ replica
  local want_n; want_n=$(docker service ls --filter "name=$1" --format '{{.Name}} {{.Replicas}}' | awk -v n="$1" '$1==n{split($2,a,"/"); print a[2]}')
  for _ in $(seq 1 "${2:-90}"); do
    r=$(running "$1"); [ -n "$r" ] && [ "$r" != "0" ] && [ "$r" = "${want_n:-1}" ] && return 0; sleep 2
  done; return 1
}
wait_down() { for _ in $(seq 1 60); do [ "$(running "$1")" = "0" ] && return 0; sleep 1; done; return 1; }
stop()  { scale "$1" 0; wait_down "$1"; sleep "${2:-5}"; }
start() { scale "$1" 1; wait_up "$1" 120; sleep "${2:-8}"; }

probe() { step probe -e PROBE_URL="$1"; }
# wait_code <url> <mã mong đợi> [giây]: chờ endpoint nội bộ trả đúng mã
wait_code() { for _ in $(seq 1 "${3:-60}"); do [ "$(j "$(probe "$1")" http)" = "$2" ] && return 0; sleep 2; done; return 1; }
# Chờ tới khi kết quả của lệnh $2 (một step) có key $3 == $4
wait_step() { local name="$1" key="$2" val="$3"; shift 3; for _ in $(seq 1 40); do r=$("$@"); [ "$(j "$r" "$key")" = "$val" ] && return 0; sleep 3; done; return 1; }
count() { j "$1" "counts.$2"; }

echo "[setup]"; s=$(step setup); echo "  $s"
[ "$(j "$s" ok)" = "True" ] || { echo "setup thất bại (stack chưa sẵn sàng / rate limit?)"; rm -rf "$state"; exit 1; }
has_admin=0; [ -n "${ADMIN_USERNAME:-${ADMIN_BOOTSTRAP_USERNAME:-}}" ] && has_admin=1

if [[ "$want" == *" A "* ]]; then
echo "[A] ClickHouse down: analytics 503, nhưng đặt hàng + POST /events vẫn OK; hồi phục sau khi bật lại"
  stop "${INFRA}_clickhouse"
  r=$(step analytics); echo "  analytics: $r"
  expect "báo cáo analytics trả 503 khi ClickHouse down" "$(eq "$(j "$r" http)" 503; echo $?)"
  r=$(step events); echo "  events: $r"
  expect "POST /events vẫn 202 (best-effort)" "$(eq "$(j "$r" http)" 202; echo $?)"
  r=$(step order); echo "  order: $r"
  expect "đặt đơn COD vẫn 201" "$(eq "$(j "$r" http)" 201; echo $?)"
  r=$(step catalog); expect "duyệt catalog vẫn 200" "$(eq "$(j "$r" http)" 200; echo $?)"
  start "${INFRA}_clickhouse" 15
  wait_step analytics http 200 step analytics; expect "analytics trở lại 200 sau khi ClickHouse bật lại" "$?"
fi

if [[ "$want" == *" B "* ]]; then
echo "[B] Kafka down (dừng CẢ 3 broker): đơn vẫn được nhận (outbox), xử lý xong khi Kafka quay lại"
  for b in 1 2 3; do scale "${INFRA}_broker$b" 0; done
  for b in 1 2 3; do wait_down "${INFRA}_broker$b"; done; sleep 5
  r=$(step order); echo "  order: $r"
  expect "đặt đơn COD nhận 201 khi Kafka down" "$(eq "$(j "$r" http)" 201; echo $?)"
  sleep 5; st=$(step status); echo "  $st"
  expect "đơn còn PENDING khi Kafka down" "$([ "$(count "$st" PENDING)" -ge 1 ] 2>/dev/null; echo $?)"
  for b in 1 2 3; do scale "${INFRA}_broker$b" 1; done
  for b in 1 2 3; do wait_up "${INFRA}_broker$b" 120; done; sleep 15
  st=$(step wait -e WAIT_MS=180000); echo "  $st"
  expect "không còn đơn PENDING sau khi Kafka trở lại" "$(eq "$(j "$st" timed_out)" False; echo $?)"
fi

if [[ "$want" == *" C "* ]]; then
echo "[C] payment-service down: COD không ảnh hưởng; đơn online chờ payment, thanh toán được sau khi hồi phục"
  stop "${APP}_payment-service"
  r=$(step order); expect "COD vẫn đặt được (201)" "$(eq "$(j "$r" http)" 201; echo $?)"
  r=$(step order -e METHOD=MOCK_CARD); echo "  online: $r"
  expect "đơn MOCK_CARD vẫn được nhận (payment tạo bất đồng bộ)" "$([[ "$(j "$r" http)" == 201 || "$(j "$r" http)" == 503 ]]; echo $?)"
  start "${APP}_payment-service" 15
  if [ "$(j "$r" http)" = "201" ]; then
    r=$(step pay); echo "  pay: $r"
    expect "payment được tạo bù và thanh toán → đơn PAID" "$(eq "$(j "$r" order_status)" PAID; echo $?)"
  fi
fi

if [[ "$want" == *" D "* ]]; then
echo "[D] product-service down: đặt hàng bị từ chối nhanh (503), không treo"
  stop "${APP}_product-service"
  r=$(step order); echo "  order: $r"
  expect "POST /orders trả 503" "$(eq "$(j "$r" http)" 503; echo $?)"
  start "${APP}_product-service" 12
  r=$(step order); expect "đặt hàng lại bình thường sau khi hồi phục" "$(eq "$(j "$r" http)" 201; echo $?)"
fi

if [[ "$want" == *" E "* ]]; then
echo "[E] Redis down: gateway không sập (rate limit fail-open), hồi phục sau khi bật lại"
  stop "${INFRA}_redis"
  r=$(step catalog); echo "  catalog: $r"
  expect "catalog công khai vẫn trả 200" "$(eq "$(j "$r" http)" 200; echo $?)"
  r=$(step events); echo "  events: $r"
  expect "POST /events vẫn 202" "$(eq "$(j "$r" http)" 202; echo $?)"
  r=$(probe "http://api-gateway:8081/healthz"); expect "gateway /healthz (liveness) vẫn 200" "$(eq "$(j "$r" http)" 200; echo $?)"
  # token / cache dùng Redis: ghi nhận hành vi (không khẳng định cứng)
  r=$(step order); echo "  (thông tin) đặt đơn khi Redis down -> $r"
  start "${INFRA}_redis" 10
  wait_code "http://api-gateway:8081/ready" 200 60; expect "gateway /ready trở lại 200" "$?"
  r=$(step order); expect "đặt đơn bình thường sau khi Redis trở lại" "$(eq "$(j "$r" http)" 201; echo $?)"
fi

if [[ "$want" == *" F "* ]]; then
echo "[F] Postgres down: các service giữ dữ liệu báo /ready 503, hồi phục tự động"
  stop "${INFRA}_postgres" 10
  for svc in order-service product-service auth-service user-service payment-service; do
    wait_code "http://$svc:8081/ready" 503 45; expect "$svc /ready = 503 khi Postgres down" "$?"
    r=$(probe "http://$svc:8081/healthz"); expect "$svc /healthz (liveness) vẫn 200 (không bị kill oan)" "$(eq "$(j "$r" http)" 200; echo $?)"
  done
  if [ "$has_admin" = 1 ]; then r=$(step health); echo "  health: $r"; fi
  start "${INFRA}_postgres" 20
  for svc in order-service product-service auth-service user-service payment-service; do
    wait_code "http://$svc:8081/ready" 200 90; expect "$svc /ready = 200 sau khi Postgres trở lại" "$?"
  done
  r=$(step order); expect "đặt đơn lại bình thường" "$(eq "$(j "$r" http)" 201; echo $?)"
fi

if [[ "$want" == *" G "* ]]; then
echo "[G] order-service down sau khi nhận đơn: kết quả xử lý được áp dụng khi khởi động lại"
  r=$(step order); echo "  $r"
  stop "${APP}_order-service" 12
  start "${APP}_order-service" 10
  st=$(step wait -e WAIT_MS=120000); echo "  $st"
  expect "không còn đơn PENDING sau khi order-service chạy lại" "$(eq "$(j "$st" timed_out)" False; echo $?)"
fi

if [[ "$want" == *" H "* ]]; then
echo "[H] Rolling restart mọi service ứng dụng: dữ liệu và token không mất"
  before=$(step status)
  for s in api-gateway auth-service user-service product-service order-service payment-service analytics-service; do
    docker service update --force --detach=true "${APP}_$s" >/dev/null 2>&1
  done
  sleep 30; for s in api-gateway auth-service user-service product-service order-service payment-service analytics-service; do wait_up "${APP}_$s" 120; done
  sleep 10
  after=$(step status); echo "  trước: $before"; echo "  sau:   $after"
  expect "số đơn & tồn kho giữ nguyên qua rolling restart" "$(eq "$(j "$before" counts)" "$(j "$after" counts)"; echo $?)"
  r=$(step order); expect "đặt đơn bình thường sau restart" "$(eq "$(j "$r" http)" 201; echo $?)"
fi

if [[ "$want" == *" I "* ]]; then
echo "[I] Postgres restart (force update): service tự kết nối lại"
  docker service update --force --detach=true "${INFRA}_postgres" >/dev/null 2>&1
  sleep 25; wait_up "${INFRA}_postgres" 120; sleep 15
  wait_step order http 201 step order; expect "đặt đơn lại được sau khi Postgres restart" "$?"
  st=$(step wait -e WAIT_MS=120000); echo "  $st"
  expect "không còn đơn PENDING" "$(eq "$(j "$st" timed_out)" False; echo $?)"
fi

if [[ "$want" == *" J "* ]]; then
echo "[J] Một broker Kafka down (cụm 3 broker, RF=3, min ISR=2): đơn vẫn 201 và được xử lý, không còn PENDING"
  stop "${INFRA}_broker3" 10
  r=$(step order); echo "  order: $r"
  expect "đặt đơn COD vẫn 201 khi 1/3 broker down" "$(eq "$(j "$r" http)" 201; echo $?)"
  r=$(step order); expect "đặt thêm đơn vẫn 201" "$(eq "$(j "$r" http)" 201; echo $?)"
  st=$(step wait -e WAIT_MS=120000); echo "  $st"
  expect "không còn đơn PENDING khi 1 broker down (xử lý qua 2 broker còn lại)" "$(eq "$(j "$st" timed_out)" False; echo $?)"
  start "${INFRA}_broker3" 15
  r=$(step order); expect "đặt đơn bình thường sau khi broker3 trở lại" "$(eq "$(j "$r" http)" 201; echo $?)"
  st=$(step wait -e WAIT_MS=120000); echo "  $st"
  expect "không còn đơn PENDING sau khi broker3 trở lại" "$(eq "$(j "$st" timed_out)" False; echo $?)"
fi

if [ "$has_admin" = 1 ]; then
  echo "[cuối] /admin/system/health sau toàn bộ kịch bản"
  wait_step health status ready step health; expect "toàn bộ service ready" "$?"
fi

rm -rf "$state"
echo; [ "$fails" = 0 ] && echo "resilience: tất cả kiểm tra đạt" || { echo "resilience: $fails kiểm tra thất bại"; exit 1; }
