#!/usr/bin/env bash
# Deploy (or update) the infra and application stacks on the local Docker Swarm.
# Usage: scripts/deploy.sh [infra|observability|services|all]   (default: all = infra + observability + services)
#
# Chọn file cấu hình (thứ tự ưu tiên):
#   ENV_FILE=/đường/dẫn/file.env          dùng đúng file này
#   DEPLOY_ENV=dev|staging|prod           dùng deploy/env/.env.<môi trường> (copy từ deploy/env/<môi trường>.env.example)
#   (mặc định)                            deploy/.env như trước đây
# HTTPS: PUBLIC_SCHEME=https (biến môi trường hoặc trong file env) => gộp services.https.yml / observability.https.yml. Mặc định http.
# Profile máy yếu: LITE=true (biến môi trường hoặc trong file env) => gộp deploy/services.lite.yml + deploy/infra.lite.yml (Kafka 1 broker, RF=1).
# Chờ sẵn sàng sau deploy: scripts/wait-ready.sh; bỏ qua bằng SKIP_WAIT=1 hoặc --no-wait.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"

args=()
for a in "$@"; do
  case "$a" in
    --no-wait) SKIP_WAIT=1 ;;
    *) args+=("$a") ;;
  esac
done
set -- "${args[@]+"${args[@]}"}"

if [ -n "${ENV_FILE:-}" ]; then
  env_file="$ENV_FILE"
elif [ -n "${DEPLOY_ENV:-}" ]; then
  case "$DEPLOY_ENV" in
    dev|staging|prod) env_file="$root/deploy/env/.env.$DEPLOY_ENV" ;;
    *) echo "DEPLOY_ENV phải là dev|staging|prod (đang là '$DEPLOY_ENV')"; exit 1 ;;
  esac
else
  env_file="$root/deploy/.env"
fi
[ -f "$env_file" ] || { echo "Thiếu $env_file (copy từ deploy/.env.example hoặc deploy/env/<môi trường>.env.example)"; exit 1; }
echo "Dùng file cấu hình: $env_file"

# LITE: ưu tiên biến môi trường, sau đó giá trị trong file env.
lite="${LITE:-$(grep -E '^LITE=' "$env_file" | tail -1 | cut -d= -f2 | sed 's/[[:space:]#].*//' || true)}"

scheme="${PUBLIC_SCHEME:-$(grep -E '^PUBLIC_SCHEME=' "$env_file" | tail -1 | cut -d= -f2 | sed 's/[[:space:]#].*//' || true)}"

# `docker stack deploy` does not understand variable interpolation defaults, the top-level
# `name:` that `docker compose config` adds, or quoted port numbers, so render and fix first.
# $@ = một hoặc nhiều file compose (override đứng sau).
render() {
  local files=() f
  for f in "$@"; do files+=(-f "$f"); done
  docker compose --env-file "$env_file" "${files[@]}" config \
    | sed -e '/^name:/d' -E -e 's/^( *published: )"([0-9]+)"$/\1\2/'
}

# LITE: Kafka chỉ có broker1 => services chỉ trỏ tới broker1 (ghi đè mặc định 3 broker trong services.yml).
[ "$lite" = "true" ] && export KAFKA_BROKERS_ADDR=broker1:9092

deploy() {
  local name="$1" stack="$2" files=("$root/deploy/$1.yml")
  if [ "$name" = infra ] && [ "$lite" = "true" ]; then
    echo "Profile LITE: gộp infra.lite.yml (Kafka 1 broker, RF=1)"
    files+=("$root/deploy/infra.lite.yml")
  fi
  if [ "$name" = services ] && [ "$lite" = "true" ]; then
    echo "Profile LITE: gộp services.lite.yml"
    files+=("$root/deploy/services.lite.yml")
  fi
  if [ "$scheme" = "https" ] && [ -f "$root/deploy/$name.https.yml" ]; then
    echo "PUBLIC_SCHEME=https: gộp $name.https.yml"
    files+=("$root/deploy/$name.https.yml")
  fi
  render "${files[@]}" | docker stack deploy --detach=true -c - "$stack"
}

target="${1:-all}"
case "$target" in
  infra)    deploy infra marketplace-infra ;;
  observability) deploy observability marketplace-obs ;;
  services) deploy services marketplace ;;
  all)      deploy infra marketplace-infra; deploy observability marketplace-obs; deploy services marketplace ;;
  *) echo "usage: $0 [--no-wait] [infra|observability|services|all]"; exit 1 ;;
esac

if [ "${SKIP_WAIT:-0}" = "1" ]; then
  echo "Bỏ qua bước chờ sẵn sàng (SKIP_WAIT=1)."
else
  WAIT_TARGET="$target" "$root/scripts/wait-ready.sh"
fi
