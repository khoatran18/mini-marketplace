#!/usr/bin/env bash
# Build toàn bộ image dùng trong deploy/services.yml.
# URL API được nướng vào frontend lúc build: API_BASE_URL=https://api.example.com scripts/build-images.sh
# Biến tuỳ chọn:
#   REGISTRY   vd. ghcr.io/<owner>  -> tên image: $REGISTRY/<name>:$IMAGE_TAG (không set: marketplace/<name>:latest như cũ)
#   IMAGE_TAG  mặc định = git short sha khi có REGISTRY, ngược lại "latest"
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
version="${SERVICE_VERSION:-dev}"
commit="$(git -C "$root" rev-parse --short HEAD 2>/dev/null || echo unknown)"
built_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

registry="${REGISTRY:-}"
registry="${registry%/}"
if [ -n "$registry" ]; then
  prefix="$registry"
  tag="${IMAGE_TAG:-$commit}"
else
  prefix="marketplace"
  tag="${IMAGE_TAG:-latest}"
fi

for s in api-gateway auth-service user-service product-service order-service payment-service analytics-service; do
  docker build -t "$prefix/$s:$tag" \
    --build-arg SERVICE_VERSION="$version" --build-arg GIT_COMMIT="$commit" --build-arg BUILD_TIME="$built_at" \
    "$root/services/$s"
done
docker build -t "$prefix/frontend:$tag" \
  --build-arg NEXT_PUBLIC_API_BASE_URL="${API_BASE_URL:-https://api.marketplace.swarm.localhost}" \
  "$root/apps/frontend"
