#!/usr/bin/env bash
# Dựng ClickHouse 24.8 tạm thời bằng docker (cổng 18125), chạy unittest, rồi dọn.
# Dùng: scripts/export/tests/run_tests.sh [python]   (PY_PARQUET=/path/python để thử nhánh parquet trong venv có pyarrow)
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
PORT="${CH_PORT:-18125}"
NAME="export-test-ch-$$"
PASS="test-pass-$$"
PY="${1:-python3}"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT
docker run -d --rm --name "$NAME" -p "127.0.0.1:${PORT}:8123" \
  -e CLICKHOUSE_USER=tester -e CLICKHOUSE_PASSWORD="$PASS" -e CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT=1 \
  clickhouse/clickhouse-server:24.8 >/dev/null
for i in $(seq 1 60); do
  if curl -fs -u "tester:$PASS" "http://127.0.0.1:${PORT}/ping" >/dev/null 2>&1; then break; fi
  sleep 1
done
export TEST_CLICKHOUSE_URL="http://127.0.0.1:${PORT}" TEST_CLICKHOUSE_USER=tester TEST_CLICKHOUSE_PASSWORD="$PASS"
cd "$HERE/.."
"$PY" -m unittest discover -s tests -v
