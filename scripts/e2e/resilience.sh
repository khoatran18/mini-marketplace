#!/usr/bin/env bash
# Tiêm lỗi trên stack Docker Swarm đang chạy: dừng/khôi phục từng thành phần bằng `docker service scale`, rồi kiểm tra
# hệ thống suy giảm đúng cách và tự hồi phục. Cần deploy trước (scripts/deploy.sh), image marketplace/frontend:latest (có node)
# và ADMIN_BOOTSTRAP_* (để kiểm /admin/system/health; không có thì bỏ qua phần đó).
#
# Usage: scripts/e2e/resilience.sh [A B C ... M]      (mặc định chạy tất cả; mỗi chữ là một kịch bản)
#   A ClickHouse down   B Kafka down (cả 3 broker)   C payment-service down   D product-service down
#   E Redis down        F Postgres down (/ready 503)           G order-service down sau khi nhận đơn
#   H rolling restart   I Postgres restart   J một broker Kafka down (cụm 3 broker)
#   K Loki down (hạ tầng quan sát)   L hai service down cùng lúc (payment + order)   M khoá bảng products trong Postgres (hệ thống chậm)
# Biến: PUBLIC_HOST, ADMIN_USERNAME/ADMIN_PASSWORD, STACK_INFRA (marketplace-infra), STACK_APP (marketplace), STACK_OBS (marketplace-obs)
set -uo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
host="${PUBLIC_HOST:-marketplace.swarm.localhost}"
scheme="${PUBLIC_SCHEME:-$(grep -E '^PUBLIC_SCHEME=' "$root/deploy/.env" 2>/dev/null | tail -1 | cut -d= -f2 | sed 's/[[:space:]#].*//')}"; scheme="${scheme:-http}"
ca=(); [ "$scheme" = https ] && ca=(-v "$root/deploy/traefik/certs/local.crt:/ca.crt:ro" -e NODE_EXTRA_CA_CERTS=/ca.crt)
INFRA="${STACK_INFRA:-marketplace-infra}"
APP="${STACK_APP:-marketplace}"
OBS="${STACK_OBS:-marketplace-obs}"
state="$(mktemp -d)"
fails=0
want=" ${*:-A B C D E F G H I J K L M} "

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
stop()  { gt_log fault_start "$1"; scale "$1" 0; wait_down "$1"; sleep "${2:-5}"; [ -n "${GT_HOLD:-}" ] && sleep "$GT_HOLD"; return 0; }
# hold_traffic <giây>: giữ lỗi thêm một lúc VÀ vẫn gửi request đặt hàng đều đặn (cảnh báo như CircuitBreakerOpen chỉ bắn khi có lưu lượng)
hold_traffic() { local end=$((SECONDS+$1)); while [ $SECONDS -lt $end ]; do step order >/dev/null; sleep 4; done; }
start() { scale "$1" 1; wait_up "$1" 120; gt_log fault_end "$1"; sleep "${2:-8}"; }

# --- ground truth (đáp án đúng cho agent chẩn đoán): mỗi lần tiêm lỗi ghi 1 dòng JSON "fault_start" / "fault_end" vào
# $GROUND_TRUTH_FILE (mặc định ~/mm-ground-truth/resilience.jsonl). Đây là NHÃN do script biết chắc (lỗi do chính nó gây ra), không phải
# dữ liệu ngoài. `expected_alerts` là phân tích: chỉ bắn nếu lỗi kéo dài hơn `for:` của luật; dùng GT_HOLD=<giây> để giữ lỗi đủ lâu.
# Kiểm đáp án thật so với cảnh báo thực tế: scripts/e2e/ground_truth_check.sh
GT_FILE="${GROUND_TRUTH_FILE:-$HOME/mm-ground-truth/resilience.jsonl}"
GT_RUN="$(date +%Y%m%d-%H%M%S)-$$"
SCEN=""
gt_meta() { # gt_meta <scenario> -> "fault|playbook|alerts(,)|signals"
  case "$1" in
    A) echo "clickhouse_down|analytics-data,service-unavailable|ServiceNotReady:analytics-service|analytics /ready 503 (check clickhouse); /admin/analytics 503; POST /events vẫn 202" ;;
    B) echo "kafka_all_brokers_down|outbox-stuck|OutboxBacklogOld|mm_outbox_pending tăng; /ready degraded (check kafka); POST /orders vẫn 201" ;;
    C) echo "payment_service_down|service-unavailable|ServiceDown/ServiceMissing:payment-service|đơn online chờ payment; COD không ảnh hưởng; CircuitBreakerOpen tới PaymentClient nếu có lời gọi" ;;
    D) echo "product_service_down|service-unavailable|CircuitBreakerOpen:ProductClient|POST /orders 503 Product service is unavailable; order-service degraded; log i/o timeout tới IP lạ nếu DNS nhà mạng hijack" ;;
    E) echo "redis_down|service-unavailable||/ready degraded (check redis); rate limit fail-open; không cảnh báo" ;;
    F) echo "postgres_down|service-unavailable|ServiceNotReady:auth+user+product+order+payment|/ready 503 not_ready (check postgres), /healthz vẫn 200" ;;
    G) echo "order_service_down|service-unavailable|ServiceDown/ServiceMissing:order-service|đơn đã nhận vẫn được xử lý khi order-service lên lại" ;;
    H) echo "rolling_restart|service-unavailable||thoáng qua; không cảnh báo" ;;
    I) echo "postgres_restart|service-unavailable||thoáng qua; không cảnh báo" ;;
    J) echo "one_kafka_broker_down|kafka-consumer-dlq||RF=3 min ISR=2 vẫn ghi được; không cảnh báo" ;;
    K) echo "loki_down|observability-down|InfraTargetDown:loki|không có log mới trong Loki; cửa hàng vẫn chạy bình thường (InfraTargetDown cần lỗi > 2 phút: dùng GT_HOLD>=120)" ;;
    L) echo "two_services_down|multiple-faults,service-unavailable|ServiceDown/ServiceMissing:payment-service+order-service|hai service cùng down; /admin/system/health liệt kê cả hai; đặt đơn 503 (order-service down)" ;;
    M) echo "postgres_table_lock|slow-database|GatewayLatencyHigh:api-gateway|truy vấn tới bảng products bị chặn bởi khoá ACCESS EXCLUSIVE: catalog/đặt đơn chậm hoặc 5xx tới khi nhả khoá (cần giữ > 5 phút và có lưu lượng: GT_HOLD>=300). DBPoolWaiting có thể bắn nhưng không bảo đảm" ;;
  esac
}
gt_log() { # gt_log <phase> <component>
  [ -n "$SCEN" ] || return 0
  local m fault pb al sig; m="$(gt_meta "$SCEN")"; IFS='|' read -r fault pb al sig <<<"$m"
  mkdir -p "$(dirname "$GT_FILE")" 2>/dev/null || return 0
  python3 -c "
import json,sys,datetime
a=sys.argv
print(json.dumps({'run':a[1],'ts':datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ'),'phase':a[2],'scenario':a[3],'component':a[4],'fault':a[5],
 'playbooks':[x for x in a[6].split(',') if x],'expected_alerts':[x for x in a[7].split(',') if x],'expected_signals':a[8]},ensure_ascii=False))" \
    "$GT_RUN" "$1" "$SCEN" "$2" "$fault" "$pb" "$al" "$sig" >> "$GT_FILE" 2>/dev/null || true
}

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
SCEN=A
echo "[A] ClickHouse down: analytics 503, nhưng đặt hàng + POST /events vẫn OK; hồi phục sau khi bật lại"
  stop "${INFRA}_clickhouse"
  r=$(step analytics); echo "  analytics: $r"
  expect "báo cáo analytics trả 503/504 khi ClickHouse down (504 = hết deadline gRPC)" "$([[ "$(j "$r" http)" == 503 || "$(j "$r" http)" == 504 ]]; echo $?)"
  r=$(step events); echo "  events: $r"
  expect "POST /events vẫn 202 (best-effort)" "$(eq "$(j "$r" http)" 202; echo $?)"
  r=$(step order); echo "  order: $r"
  expect "đặt đơn COD vẫn 201" "$(eq "$(j "$r" http)" 201; echo $?)"
  r=$(step catalog); expect "duyệt catalog vẫn 200" "$(eq "$(j "$r" http)" 200; echo $?)"
  start "${INFRA}_clickhouse" 15
  wait_step analytics http 200 step analytics; expect "analytics trở lại 200 sau khi ClickHouse bật lại" "$?"
fi

if [[ "$want" == *" B "* ]]; then
SCEN=B
echo "[B] Kafka down (dừng CẢ 3 broker): đơn vẫn được nhận (outbox), xử lý xong khi Kafka quay lại"
  for b in 1 2 3; do scale "${INFRA}_broker$b" 0; done
  for b in 1 2 3; do wait_down "${INFRA}_broker$b"; done; gt_log fault_start kafka_brokers; sleep 5
  r=$(step order); echo "  order: $r"
  expect "đặt đơn COD nhận 201 khi Kafka down" "$(eq "$(j "$r" http)" 201; echo $?)"
  sleep 5; st=$(step status); echo "  $st"
  expect "đơn còn PENDING khi Kafka down" "$([ "$(count "$st" PENDING)" -ge 1 ] 2>/dev/null; echo $?)"
  # OutboxBacklogOld cần hàng cũ > 300s kéo dài 5 phút: giữ GT_HOLD + 300s
  [ -n "${GT_HOLD:-}" ] && sleep $((GT_HOLD+300))
  for b in 1 2 3; do scale "${INFRA}_broker$b" 1; done
  for b in 1 2 3; do wait_up "${INFRA}_broker$b" 120; done; gt_log fault_end kafka_brokers; sleep 15
  st=$(step wait -e WAIT_MS=180000); echo "  $st"
  expect "không còn đơn PENDING sau khi Kafka trở lại" "$(eq "$(j "$st" timed_out)" False; echo $?)"
fi

if [[ "$want" == *" C "* ]]; then
SCEN=C
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
SCEN=D
echo "[D] product-service down: đặt hàng bị từ chối nhanh (503), không treo"
  GT_HOLD= stop "${APP}_product-service"
  r=$(step order); echo "  order: $r"
  expect "POST /orders trả 503" "$(eq "$(j "$r" http)" 503; echo $?)"
  [ -n "${GT_HOLD:-}" ] && hold_traffic 120   # breaker phải mở liên tục > 30s: cần lưu lượng trong lúc lỗi
  start "${APP}_product-service" 12
  # đo thời gian hồi phục: lỗi kéo dài thì kênh gRPC (backoff) có thể cần vài chục giây đến vài phút mới nối lại
  t0=$SECONDS; wait_step order http 201 step order; rc=$?
  echo "  hồi phục đặt hàng sau $((SECONDS-t0))s kể từ lúc product-service sẵn sàng"
  expect "đặt hàng lại bình thường sau khi hồi phục (trong ~2 phút)" "$rc"
fi

if [[ "$want" == *" E "* ]]; then
SCEN=E
echo "[E] Redis down: gateway không sập (rate limit fail-open), hồi phục sau khi bật lại"
  stop "${INFRA}_redis"
  r=$(step catalog); echo "  catalog: $r"
  expect "catalog công khai vẫn trả 200" "$(eq "$(j "$r" http)" 200; echo $?)"
  r=$(step events); echo "  events: $r"
  expect "POST /events vẫn 202" "$(eq "$(j "$r" http)" 202; echo $?)"
  r=$(probe "http://api-gateway:8081/healthz"); expect "gateway /healthz (liveness) vẫn 200" "$(eq "$(j "$r" http)" 200; echo $?)"
  # Redis chỉ là đường nhanh kiểm phiên bản mật khẩu của JWT; Redis chết thì gateway hỏi DB (auth-service), nên request có đăng nhập vẫn chạy
  r=$(step order); echo "  đặt đơn khi Redis down: $r"
  expect "đặt đơn có đăng nhập vẫn 201 khi Redis down (gateway hỏi DB)" "$(eq "$(j "$r" http)" 201; echo $?)"
  start "${INFRA}_redis" 10
  wait_code "http://api-gateway:8081/ready" 200 60; expect "gateway /ready trở lại 200" "$?"
  r=$(step order); expect "đặt đơn bình thường sau khi Redis trở lại" "$(eq "$(j "$r" http)" 201; echo $?)"
fi

if [[ "$want" == *" F "* ]]; then
SCEN=F
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
SCEN=G
echo "[G] order-service down sau khi nhận đơn: kết quả xử lý được áp dụng khi khởi động lại"
  r=$(step order); echo "  $r"
  stop "${APP}_order-service" 12
  start "${APP}_order-service" 10
  st=$(step wait -e WAIT_MS=120000); echo "  $st"
  expect "không còn đơn PENDING sau khi order-service chạy lại" "$(eq "$(j "$st" timed_out)" False; echo $?)"
fi

if [[ "$want" == *" H "* ]]; then
SCEN=H
echo "[H] Rolling restart mọi service ứng dụng: dữ liệu và token không mất"
  before=$(step status)
  gt_log fault_start "${APP}_all"
  for s in api-gateway auth-service user-service product-service order-service payment-service analytics-service; do
    docker service update --force --detach=true "${APP}_$s" >/dev/null 2>&1
  done
  sleep 30; for s in api-gateway auth-service user-service product-service order-service payment-service analytics-service; do wait_up "${APP}_$s" 120; done
  gt_log fault_end "${APP}_all"; sleep 10
  after=$(step status); echo "  trước: $before"; echo "  sau:   $after"
  expect "số đơn & tồn kho giữ nguyên qua rolling restart" "$(eq "$(j "$before" counts)" "$(j "$after" counts)"; echo $?)"
  r=$(step order); expect "đặt đơn bình thường sau restart" "$(eq "$(j "$r" http)" 201; echo $?)"
fi

if [[ "$want" == *" I "* ]]; then
SCEN=I
echo "[I] Postgres restart (force update): service tự kết nối lại"
  gt_log fault_start "${INFRA}_postgres"
  docker service update --force --detach=true "${INFRA}_postgres" >/dev/null 2>&1
  sleep 25; wait_up "${INFRA}_postgres" 120; gt_log fault_end "${INFRA}_postgres"; sleep 15
  wait_step order http 201 step order; expect "đặt đơn lại được sau khi Postgres restart" "$?"
  st=$(step wait -e WAIT_MS=120000); echo "  $st"
  expect "không còn đơn PENDING" "$(eq "$(j "$st" timed_out)" False; echo $?)"
fi

if [[ "$want" == *" J "* ]]; then
SCEN=J
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

if [[ "$want" == *" K "* ]]; then
SCEN=K
echo "[K] Loki down (hạ tầng quan sát): cửa hàng vẫn chạy, hồi phục khi bật lại"
  stop "${OBS}_loki" 5
  r=$(step order); echo "  order: $r"
  expect "đặt đơn COD vẫn 201 khi Loki down" "$(eq "$(j "$r" http)" 201; echo $?)"
  r=$(step catalog); expect "duyệt catalog vẫn 200" "$(eq "$(j "$r" http)" 200; echo $?)"
  start "${OBS}_loki" 15
  r=$(step order); expect "đặt đơn bình thường sau khi Loki trở lại" "$(eq "$(j "$r" http)" 201; echo $?)"
fi

if [[ "$want" == *" L "* ]]; then
SCEN=L
echo "[L] Hai service down cùng lúc (payment-service + order-service): hồi phục cả hai, không còn đơn PENDING"
  gt_log fault_start "${APP}_payment-service"; gt_log fault_start "${APP}_order-service"
  scale "${APP}_payment-service" 0; scale "${APP}_order-service" 0
  wait_down "${APP}_payment-service"; wait_down "${APP}_order-service"; sleep 5
  [ -n "${GT_HOLD:-}" ] && hold_traffic "$GT_HOLD"
  r=$(step order); echo "  order: $r"
  expect "đặt đơn không còn 201 khi order-service down (503 từ gateway)" "$([[ "$(j "$r" http)" == 5* ]]; echo $?)"
  r=$(step catalog); expect "duyệt catalog vẫn 200 (không phụ thuộc hai service này)" "$(eq "$(j "$r" http)" 200; echo $?)"
  scale "${APP}_payment-service" 1; scale "${APP}_order-service" 1
  wait_up "${APP}_payment-service" 120; wait_up "${APP}_order-service" 120
  gt_log fault_end "${APP}_payment-service"; gt_log fault_end "${APP}_order-service"; sleep 10
  wait_step order http 201 step order; expect "đặt đơn lại được sau khi cả hai trở lại" "$?"
  st=$(step wait -e WAIT_MS=120000); echo "  $st"
  expect "không còn đơn PENDING" "$(eq "$(j "$st" timed_out)" False; echo $?)"
fi

if [[ "$want" == *" M "* ]]; then
SCEN=M
echo "[M] Khoá bảng products trong Postgres (ACCESS EXCLUSIVE): hệ thống chậm/lỗi trong lúc khoá, tự hết khi nhả"
  pgc=$(docker ps -q -f name="${INFRA}_postgres" | head -1)
  pgdb="$(envval POSTGRES_DB)"; pguser="$(envval POSTGRES_USER)"
  if [ -z "$pgc" ]; then echo "  bỏ qua: không tìm thấy container postgres"; else
    dur=$((45 + ${GT_HOLD:-0}))
    gt_log fault_start "${INFRA}_postgres"
    docker exec -d "$pgc" psql -U "${pguser:-postgres}" -d "${pgdb:-postgres}" -c "BEGIN; LOCK TABLE products IN ACCESS EXCLUSIVE MODE; SELECT pg_sleep($dur); COMMIT;" >/dev/null 2>&1
    sleep 5
    r=$(step catalog); echo "  catalog khi bị khoá: $r (chỉ ghi nhận, kết quả có thể là 5xx/504 hoặc 200 nếu đã có cache)"
    end=$((SECONDS+dur-8)); while [ $SECONDS -lt $end ]; do step catalog >/dev/null; sleep 3; done
    sleep 12
    gt_log fault_end "${INFRA}_postgres"
    wait_step catalog http 200 step catalog; expect "catalog trở lại 200 sau khi nhả khoá" "$?"
    wait_step order http 201 step order; expect "đặt đơn lại được sau khi nhả khoá" "$?"
  fi
fi

if [ "$has_admin" = 1 ]; then
  echo "[cuối] /admin/system/health sau toàn bộ kịch bản"
  wait_step health status ready step health; expect "toàn bộ service ready" "$?"
fi

rm -rf "$state"
echo; [ "$fails" = 0 ] && echo "resilience: tất cả kiểm tra đạt" || { echo "resilience: $fails kiểm tra thất bại"; exit 1; }
