#!/usr/bin/env bash
# Build all images used by deploy/services.yml.
# API base URL is baked into the frontend at build time: API_BASE_URL=https://api.example.com scripts/build-images.sh
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
for s in api-gateway auth-service user-service product-service order-service; do
  docker build -t "marketplace/$s:latest" "$root/services/$s"
done
docker build -t marketplace/frontend:latest \
  --build-arg NEXT_PUBLIC_API_BASE_URL="${API_BASE_URL:-https://api.marketplace.swarm.localhost}" \
  "$root/apps/frontend"
