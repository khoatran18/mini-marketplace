#!/usr/bin/env bash
# CHỈ CHO THỬ TẢI: giống scripts/deploy.sh nhưng gộp thêm deploy/<stack>.perf.yml (giới hạn tài nguyên, replica, giới hạn tốc độ nâng cao, IP giả).
# Usage: scripts/deploy-perf.sh [infra|services|all]    Trở lại bình thường: scripts/deploy.sh services (và infra).
# `docker compose config` in `cpus: 1.5` dạng số còn `docker stack deploy` đòi chuỗi, nên bọc lại bằng sed.
# Chỉ hỗ trợ PUBLIC_SCHEME=http (services.perf.yml chép lệnh Traefik bản HTTP).
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
env_file="$root/deploy/.env"
render() {
  docker compose --env-file "$env_file" -f "$root/deploy/$1.yml" -f "$root/deploy/$1.perf.yml" config \
    | sed -e '/^name:/d' -E -e 's/^( *published: )"([0-9]+)"$/\1\2/' -E -e 's/^( *cpus: )([0-9.]+)$/\1"\2"/'
}
# AN TOÀN: thử tải KHÔNG được gọi model thật (tốn tiền). Sau khi deploy services, mọi replica ai-agent phải phân giải 4 role (buyer, seller, admin, operator)
# sang profile provider "fake"; nếu không thì in lỗi và thoát 1. (Swarm chạy AI_AGENT_ENV=dev nên cả config.dev.yaml lẫn config.prod.yaml phải có profile fake_perf_*.)
verify_fake_models() {
  local deadline=$((SECONDS + 240)) ok c out n
  while [ $SECONDS -lt $deadline ]; do
    ok=1; n=0
    for c in $(docker ps -q -f name=marketplace_ai-agent); do
      n=$((n+1))
      out=$(docker exec "$c" python -c "
from ai_agent.config import load_config
c = load_config(); r = c.llm.roles
bad = [k for k in ('buyer', 'seller', 'admin', 'operator') if c.llm.profiles[r[k]].get('provider') != 'fake']
print('FAKE_OK' if not bad else 'REAL:' + ','.join(bad))" 2>&1 | tail -1) || true
      [ "$out" = "FAKE_OK" ] || ok=0
    done
    [ "$ok" = 1 ] && [ "$n" -ge 1 ] && { echo "OK: $n replica ai-agent chỉ dùng mô hình giả"; return 0; }
    sleep 5
  done
  echo "LỖI: có replica ai-agent KHÔNG dùng mô hình giả (${out:-không đọc được cấu hình}). KHÔNG chạy thử tải: chat AI sẽ gọi model thật. Nếu image/config đổi mà tag giữ nguyên, Swarm không tạo lại task: chạy `docker service update --force marketplace_ai-agent` rồi kiểm lại." >&2
  return 1
}

case "${1:-all}" in
  infra)    render infra | docker stack deploy --detach=true -c - marketplace-infra ;;
  services) render services | docker stack deploy --detach=true -c - marketplace; verify_fake_models ;;
  all)      render infra | docker stack deploy --detach=true -c - marketplace-infra
            render services | docker stack deploy --detach=true -c - marketplace; verify_fake_models ;;
  *) echo "usage: $0 [infra|services|all]"; exit 1 ;;
esac
