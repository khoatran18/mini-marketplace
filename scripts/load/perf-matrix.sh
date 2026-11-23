#!/usr/bin/env bash
# Dải đo đóng + mở, mỗi mức lặp REPEAT lần (mặc định 3) để thấy độ dao động. Cùng một cấu hình cho mọi mức (không chỉnh giữa chừng).
#   scripts/load/perf-matrix.sh                       # đóng 100 200 300 500 800 1000 rồi mở 5 10 20 30 40
#   CLOSED="300 500" OPEN="" REPEAT=1 scripts/load/perf-matrix.sh
# Ước lượng: đóng ~23 phút/vòng, mở ~15 phút/vòng (mô hình AI phải là bản giả: chạy scripts/deploy-perf.sh trước).
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
CLOSED="${CLOSED-100 200 300 500 800 1000}"; OPEN="${OPEN-5 10 20 30 40}"; REPEAT="${REPEAT:-3}"; DURATION="${DURATION:-90}"
for r in $(seq 1 "$REPEAT"); do
  for v in $CLOSED; do "$here/perf-run.sh" "closed-$v-r$r" --vus="$v" --duration="$DURATION" || true; sleep 30; done
  for a in $OPEN; do "$here/perf-run.sh" "open-$a-r$r" --arrival-rate="$a" --duration="$DURATION" || true; sleep 30; done
done
