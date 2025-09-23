#!/bin/sh
set -e

# Start Redis foreground
redis-server \
  --port 6379 \
  --cluster-enabled yes \
  --cluster-config-file nodes.conf \
  --cluster-node-timeout 5000 \
  --appendonly yes \
  --protected-mode no &

# Background task: chờ tất cả node sẵn sàng rồi tạo cluster
(
  echo "[init] waiting for all redis nodes..."
  while true; do
    IPS=$(getent hosts redis-node-1 redis-node-2 redis-node-3 redis-node-4 redis-node-5 redis-node-6 | awk '{print $1}')
    count=$(echo "$IPS" | wc -l)
    echo "[init] found $count IP(s)"
    if [ "$count" -eq 6 ]; then
      break
    fi
    sleep 2
  done

  echo "[init] waiting redis ports..."
  for ip in $IPS; do
    until nc -z "$ip" 6379; do
      echo "[init] $ip:6379 not ready, retry..."
      sleep 1
    done
    echo "[init] $ip:6379 ready"
  done

  echo "[init] creating cluster..."
  yes yes | redis-cli --cluster create \
    redis-node-1:6379 \
    redis-node-2:6379 \
    redis-node-3:6379 \
    redis-node-4:6379 \
    redis-node-5:6379 \
    redis-node-6:6379 \
    --cluster-replicas 1
) &

# Keep Redis foreground
wait -n
