#!/usr/bin/env bash
# Build image dùng trong deploy/services.yml. Không tham số = build tất cả; hoặc chỉ định: scripts/build-images.sh auth-service frontend
# URL API được nướng vào frontend lúc build: API_BASE_URL=https://api.example.com scripts/build-images.sh
# Mặc định: ${PUBLIC_SCHEME}://api.${PUBLIC_HOST} lấy từ biến môi trường hoặc deploy/.env (mặc định http://api.marketplace.swarm.localhost).
# Biến tuỳ chọn:
#   REGISTRY   vd. ghcr.io/<owner>  -> tên image: $REGISTRY/<name>:$IMAGE_TAG (không set: marketplace/<name>:latest như cũ)
#   IMAGE_TAG  mặc định = git short sha khi có REGISTRY, ngược lại "latest"
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
envval() { [ -f "$root/deploy/.env" ] && grep -E "^$1=" "$root/deploy/.env" | tail -1 | cut -d= -f2- | sed 's/[[:space:]]*#.*$//' || true; }
scheme="${PUBLIC_SCHEME:-$(envval PUBLIC_SCHEME)}"; scheme="${scheme:-http}"
host="${PUBLIC_HOST:-$(envval PUBLIC_HOST)}"; host="${host:-marketplace.swarm.localhost}"
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

all_services=(api-gateway auth-service user-service product-service order-service payment-service analytics-service)
want=("$@"); [ ${#want[@]} -gt 0 ] || want=("${all_services[@]}" frontend)
for w in "${want[@]}"; do
  case " ${all_services[*]} frontend " in *" $w "*) ;; *) echo "service không hợp lệ: $w (hợp lệ: ${all_services[*]} frontend)"; exit 1 ;; esac
done
has() { for w in "${want[@]}"; do [ "$w" = "$1" ] && return 0; done; return 1; }

# docker build có thử lại: lỗi mạng chập chờn tới Docker Hub (i/o timeout khi tải metadata image gốc) không nên làm hỏng cả lượt build.
# BUILD_RETRIES (mặc định 3) và BUILD_RETRY_WAIT giây giữa các lần (mặc định 10).
build() {
  local n=1 max="${BUILD_RETRIES:-3}"
  until docker build "$@"; do
    [ "$n" -ge "$max" ] && { echo "docker build thất bại sau $n lần" >&2; return 1; }
    echo "docker build lỗi (lần $n/$max), thử lại sau ${BUILD_RETRY_WAIT:-10}s..." >&2
    n=$((n+1)); sleep "${BUILD_RETRY_WAIT:-10}"
  done
}

for s in "${all_services[@]}"; do
  has "$s" || continue
  build -t "$prefix/$s:$tag" \
    --build-arg SERVICE_VERSION="$version" --build-arg GIT_COMMIT="$commit" --build-arg BUILD_TIME="$built_at" \
    -f "$root/services/$s/Dockerfile" "$root"
done
if has frontend; then
build -t "$prefix/frontend:$tag" \
  --build-arg NEXT_PUBLIC_API_BASE_URL="${API_BASE_URL:-$scheme://api.$host}" \
  "$root/apps/frontend"
fi
