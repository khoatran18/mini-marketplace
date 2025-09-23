#!/bin/sh
set -e

# Chạy Redis server foreground
redis-server \
      --port 6379 \
      --cluster-enabled yes \
      --cluster-config-file nodes.conf \
      --cluster-node-timeout 5000 \
      --cluster-announce-ip redis-node-1 \
      --appendonly yes

# Sau 30 giây thì tạo cluster (background task)
(
  sleep 30
    redis-cli --cluster create \
      redis-node-1:6379 \
      redis-node-2:6379 \
      redis-node-3:6379 \
      redis-node-4:6379 \
      redis-node-5:6379 \
      redis-node-6:6379 \
      --cluster-replicas 1 \
      --cluster-yes
) &

# Giữ tiến trình Redis chạy foreground
wait -n
