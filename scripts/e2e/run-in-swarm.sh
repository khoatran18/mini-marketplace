#!/usr/bin/env bash
# Chạy bộ e2e (run.mjs) từ container tạm gắn vào mạng swarm - dùng khi cổng ingress không truy cập được từ host.
# Usage: scripts/e2e/run-in-swarm.sh [tham số run.mjs, vd --only 03,04 | --skip 09]
# Biến môi trường (đọc thêm từ deploy/.env nếu có):
#   PUBLIC_HOST                 mặc định marketplace.swarm.localhost
#   ADMIN_USERNAME/PASSWORD     mặc định ADMIN_BOOTSTRAP_USERNAME/PASSWORD trong deploy/.env (không có -> bỏ qua bước admin)
#   PAYMENT_WEBHOOK_SECRET      bật bước webhook ký HMAC (gọi thẳng payment-service:8081 trong mạng swarm)
#   E2E_DEV_MODE=1              stack chạy ENV=dev: kiểm thêm force-payment
#   E2E_EXPIRY_TTL_MIN=<n>      stack chạy ORDER_PAYMENT_TTL_MIN=<n> (nhỏ, vd 1): kiểm hết hạn giữ hàng & trả kho
#   RATE_LIMIT_TEST=1           kiểm 429 của /auth (AUTH_RATE_LIMIT_PER_MINUTE=20); các giới hạn khác nên nâng khi chạy cả bộ
# Lưu ý: bộ chính gọi ~15-20 lần /auth/*; với AUTH_RATE_LIMIT_PER_MINUTE=20 client sẽ tự chờ khi gặp 429 (chậm hơn).
set -euo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
envfile="$root/deploy/.env"
envval() { [ -f "$envfile" ] && grep -E "^$1=" "$envfile" | tail -1 | cut -d= -f2- | sed 's/[[:space:]]*#.*$//' || true; }

host="${PUBLIC_HOST:-$(envval PUBLIC_HOST)}"; host="${host:-marketplace.swarm.localhost}"
admin_user="${ADMIN_USERNAME:-$(envval ADMIN_BOOTSTRAP_USERNAME)}"
admin_pass="${ADMIN_PASSWORD:-$(envval ADMIN_BOOTSTRAP_PASSWORD)}"
wh_secret="${PAYMENT_WEBHOOK_SECRET:-$(envval PAYMENT_WEBHOOK_SECRET)}"

# IP của container Traefik trên mạng swarm (VIP của service cần IPVS, không phải host nào cũng có)
cid=$(docker ps -q -f name=marketplace_traefik | head -1)
[ -n "$cid" ] || { echo "không tìm thấy container traefik (chạy scripts/deploy.sh trước)"; exit 1; }
vip=$(docker inspect "$cid" --format '{{(index .NetworkSettings.Networks "marketplace-net").IPAddress}}')
[ -n "$vip" ] || { echo "không lấy được IP traefik trên marketplace-net"; exit 1; }

docker run --rm --network marketplace-net \
  --add-host "$host:$vip" --add-host "api.$host:$vip" \
  -v "$root/scripts/e2e:/e2e:ro" -v "$root/deploy/traefik/certs/local.crt:/ca.crt:ro" \
  -e NODE_EXTRA_CA_CERTS=/ca.crt -e API_URL="https://api.$host" \
  -e ADMIN_USERNAME="$admin_user" -e ADMIN_PASSWORD="$admin_pass" \
  -e PAYMENT_WEBHOOK_URL="${wh_secret:+http://payment-service:8081/internal/payments/webhook}" -e PAYMENT_WEBHOOK_SECRET="$wh_secret" \
  -e E2E_DEV_MODE="${E2E_DEV_MODE:-0}" -e E2E_EXPIRY_TTL_MIN="${E2E_EXPIRY_TTL_MIN:-0}" -e RATE_LIMIT_TEST="${RATE_LIMIT_TEST:-0}" \
  -e E2E_ANALYTICS_WAIT_MS="${E2E_ANALYTICS_WAIT_MS:-60000}" \
  --entrypoint node marketplace/frontend:latest /e2e/run.mjs "$@"
