#!/usr/bin/env bash
# Gỡ stack Swarm của dự án. Mặc định chỉ gỡ stack (GIỮ dữ liệu: volume Postgres/Kafka/ClickHouse/...).
# Usage: scripts/teardown.sh [--volumes] [--images] [--leave-swarm] [-y]
#   (không cờ)     docker stack rm marketplace, marketplace-obs, marketplace-infra (+ mạng marketplace-net)
#   --volumes      XÓA luôn dữ liệu: các volume deploy_* của dự án (mất sạch DB, Kafka, ClickHouse, Grafana, log)
#   --images       xóa image marketplace/* (build lại bằng scripts/build-images.sh)
#   --leave-swarm  docker swarm leave --force (thoát hẳn chế độ Swarm; kéo theo configs/secrets của swarm)
#   -y             không hỏi xác nhận
set -euo pipefail
volumes=0; images=0; leave=0; yes=0
for a in "$@"; do
  case "$a" in
    --volumes) volumes=1 ;; --images) images=1 ;; --leave-swarm) leave=1 ;; -y|--yes) yes=1 ;;
    *) echo "usage: $0 [--volumes] [--images] [--leave-swarm] [-y]"; exit 1 ;;
  esac
done

echo "Sẽ gỡ stack: marketplace, marketplace-obs, marketplace-infra"
[ $volumes = 1 ] && echo "  + XÓA volume dữ liệu deploy_* (không khôi phục được)"
[ $images = 1 ] && echo "  + xóa image marketplace/*"
[ $leave = 1 ] && echo "  + rời Swarm"
if [ $yes = 0 ]; then
  read -r -p "Tiếp tục? [y/N] " ans; [ "$ans" = y ] || [ "$ans" = Y ] || { echo "Hủy."; exit 0; }
fi

# thứ tự ngược với deploy: app -> observability -> infra
for s in marketplace marketplace-obs marketplace-infra; do
  docker stack rm "$s" 2>/dev/null || true
done

# chờ container và mạng được gỡ hết (volume không xóa được khi còn container dùng)
echo "Chờ stack gỡ xong..."
for _ in $(seq 1 60); do
  left=$(for s in marketplace marketplace-obs marketplace-infra; do docker ps -q --filter "label=com.docker.stack.namespace=$s"; done | wc -l)
  net=$(docker network ls -q --filter name=marketplace-net | wc -l)
  [ "$left" = 0 ] && [ "$net" = 0 ] && break
  sleep 2
done

if [ $volumes = 1 ]; then
  docker volume ls -q | grep -E '^deploy_(clickhouse-data|kafka-data-[123]|postgres-data|redis-data|grafana-data|loki-data|prometheus-data|vector-data|analytics-spool)$' \
    | xargs -r docker volume rm
fi
if [ $images = 1 ]; then
  docker images -q 'marketplace/*' | sort -u | xargs -r docker rmi -f
fi
if [ $leave = 1 ]; then
  docker swarm leave --force
fi
echo "Xong."
