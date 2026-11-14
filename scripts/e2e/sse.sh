#!/usr/bin/env bash
# Kiểm luồng SSE POST /ai/chat qua Traefik -> gateway trên Swarm đang chạy, dùng một upstream GIẢ (không gọi LLM nào).
# Tạm thời: bật FEATURE_AI_CHAT + AI_SERVICE_URL trên api-gateway, chạy stub, đo, rồi TRẢ LẠI cấu hình cũ và xoá stub.
# Cần: stack đã deploy, image marketplace/frontend:latest (có node), ADMIN_BOOTSTRAP_* trong deploy/.env.
# Usage: scripts/e2e/sse.sh        Biến: SSE_SECONDS (70), SSE_EVERY (5), PUBLIC_HOST, STACK_APP (marketplace)
set -uo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
host="${PUBLIC_HOST:-marketplace.swarm.localhost}"
scheme="${PUBLIC_SCHEME:-$(grep -E '^PUBLIC_SCHEME=' "$root/deploy/.env" 2>/dev/null | tail -1 | cut -d= -f2 | sed 's/[[:space:]#].*//')}"; scheme="${scheme:-http}"
ca=(); [ "$scheme" = https ] && ca=(-v "$root/deploy/traefik/certs/local.crt:/ca.crt:ro" -e NODE_EXTRA_CA_CERTS=/ca.crt)
APP="${STACK_APP:-marketplace}"
envval() { [ -f "$root/deploy/.env" ] && grep -E "^$1=" "$root/deploy/.env" | tail -1 | cut -d= -f2- | sed 's/[[:space:]]*#.*$//' || true; }
au="${ADMIN_USERNAME:-$(envval ADMIN_BOOTSTRAP_USERNAME)}"; ap="${ADMIN_PASSWORD:-$(envval ADMIN_BOOTSTRAP_PASSWORD)}"
[ -n "$au" ] && [ -n "$ap" ] || { echo "cần ADMIN_USERNAME/ADMIN_PASSWORD (hoặc ADMIN_BOOTSTRAP_* trong deploy/.env)"; exit 1; }
cid=$(docker ps -q -f name="${APP}_traefik" | head -1); [ -n "$cid" ] || { echo "không thấy traefik của stack $APP"; exit 1; }
vip=$(docker inspect "$cid" --format '{{(index .NetworkSettings.Networks "marketplace-net").IPAddress}}')
gw="${APP}_api-gateway"; stub=mm-ai-stub

cleanup() {
  docker service rm "$stub" >/dev/null 2>&1
  docker service update --detach=false --env-add FEATURE_AI_CHAT=false --env-add AI_SERVICE_URL= "$gw" >/dev/null 2>&1 && echo "[dọn] đã trả cấu hình gateway về mặc định (AI tắt) và xoá stub"
}
trap cleanup EXIT

# upstream giả: POST /chat -> SSE mỗi `every` giây; sự kiện đầu có role/user nhận được; ghi log khi bị huỷ
read -r -d '' js <<'JS'
const http = require('http');
http.createServer((req, res) => {
  if (req.method !== 'POST' || req.url !== '/chat') { res.writeHead(404).end(); return; }
  let b = ''; req.on('data', (c) => (b += c)); req.on('end', () => {
    let o = {}; try { o = JSON.parse(b); } catch {}
    const n = Math.floor((o.seconds ?? 5) / (o.every ?? 1)); let i = 0;
    res.writeHead(200, { 'Content-Type': 'text/event-stream' });
    res.write(`data: ${JSON.stringify({ i: 0, role: req.headers['x-user-role'], user: req.headers['x-user-id'], hasAuth: !!req.headers.authorization })}\n\n`);
    const t = setInterval(() => { i++; res.write(`data: ${JSON.stringify({ i })}\n\n`); if (i >= n - 1) { clearInterval(t); res.end(); } }, (o.every ?? 1) * 1000);
    res.on('close', () => { clearInterval(t); if (i < n - 1) console.log('UPSTREAM_ABORTED at', i); });
  });
}).listen(9000);
JS
docker service rm "$stub" >/dev/null 2>&1; sleep 2
docker service create --detach --name "$stub" --network marketplace-net --restart-condition none marketplace/frontend:latest node -e "$js" >/dev/null || { echo "không tạo được stub"; exit 1; }
echo "[1] bật cờ AI trên gateway (cuốn chiếu, chờ ổn định)"
docker service update --detach=false --env-add FEATURE_AI_CHAT=true --env-add "AI_SERVICE_URL=http://$stub:9000" "$gw" >/dev/null 2>&1 || { echo "không cập nhật được gateway"; exit 1; }
for _ in $(seq 1 30); do docker run --rm --network marketplace-net --add-host "api.$host:$vip" "${ca[@]+"${ca[@]}"}" --entrypoint node marketplace/frontend:latest -e "fetch('$scheme://api.$host/config/features').then(r=>r.json()).then(j=>process.exit(j.features.ai_chat?0:1)).catch(()=>process.exit(1))" && break; sleep 3; done
echo "[2] đo luồng qua Traefik -> gateway -> stub"
docker run --rm --user root --network marketplace-net --add-host "api.$host:$vip" -v "$root/scripts/e2e:/e2e:ro" "${ca[@]+"${ca[@]}"}" \
  -e API_URL="$scheme://api.$host" -e ADMIN_USERNAME="$au" -e ADMIN_PASSWORD="$ap" -e SSE_SECONDS="${SSE_SECONDS:-70}" -e SSE_EVERY="${SSE_EVERY:-5}" \
  --entrypoint node marketplace/frontend:latest /e2e/sse.mjs
rc=$?
sleep 3
if docker service logs "$stub" 2>&1 | grep -q UPSTREAM_ABORTED; then echo "  ok   ngắt giữa chừng -> upstream bị huỷ"; else echo "  FAIL ngắt giữa chừng nhưng upstream không bị huỷ"; rc=1; fi
[ $rc = 0 ] && echo "sse: tất cả kiểm tra đạt" || echo "sse: có kiểm tra thất bại"
exit $rc
