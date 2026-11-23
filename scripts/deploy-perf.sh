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
case "${1:-all}" in
  infra)    render infra | docker stack deploy --detach=true -c - marketplace-infra ;;
  services) render services | docker stack deploy --detach=true -c - marketplace ;;
  all)      render infra | docker stack deploy --detach=true -c - marketplace-infra
            render services | docker stack deploy --detach=true -c - marketplace ;;
  *) echo "usage: $0 [infra|services|all]"; exit 1 ;;
esac
