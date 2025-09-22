#!/bin/sh
sleep 45

if redis-cli -h redis-node-1 cluster info | grep -q "cluster_state:ok"; then :
  echo "Cluster already exist"
else :
  echo "Creating Redis cluster"
  yes yes | redis-cli --cluster create \
    redis-node-1:6379 \
    redis-node-2:6379 \
    redis-node-3:6379 \
    redis-node-4:6379 \
    redis-node-5:6379 \
    redis-node-6:6379 \
    --cluster-replicas 1 \
    --cluster-yes
fi

exec redis-server /usr/local/etc/redis/redis.conf --appendonly yes