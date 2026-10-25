#!/usr/bin/env bash
# Đẩy 8 image lên registry. Cần đăng nhập trước (docker login ghcr.io ...).
# Dùng cùng REGISTRY / IMAGE_TAG với build-images.sh:
#   REGISTRY=ghcr.io/<owner> scripts/build-images.sh && REGISTRY=ghcr.io/<owner> scripts/push-images.sh
# Biến tuỳ chọn: EXTRA_TAGS="latest v1.2.3" (tag phụ, cách nhau bằng khoảng trắng)
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
registry="${REGISTRY:-}"
registry="${registry%/}"
if [ -z "$registry" ]; then
  echo "REGISTRY chưa set (vd. REGISTRY=ghcr.io/<owner>)" >&2
  exit 1
fi
tag="${IMAGE_TAG:-$(git -C "$root" rev-parse --short HEAD 2>/dev/null || echo unknown)}"

for s in api-gateway auth-service user-service product-service order-service payment-service analytics-service frontend; do
  docker push "$registry/$s:$tag"
  for extra in ${EXTRA_TAGS:-}; do
    docker tag "$registry/$s:$tag" "$registry/$s:$extra"
    docker push "$registry/$s:$extra"
  done
done
