#!/usr/bin/env bash
# So đáp án (ground truth) do resilience.sh ghi với cảnh báo Prometheus THỰC SỰ đã bắn trong từng cửa sổ lỗi.
# Usage: scripts/e2e/ground_truth_check.sh [file.jsonl]   (mặc định ~/mm-ground-truth/resilience.jsonl; lấy lần chạy cuối)
# Mục đích: biết cảnh báo nào chưa bắn dù lỗi có thật (khoảng trống phát hiện) trước khi cho agent dựa vào chúng.
set -euo pipefail
f="${1:-${GROUND_TRUTH_FILE:-$HOME/mm-ground-truth/resilience.jsonl}}"
[ -f "$f" ] || { echo "không có $f (chạy scripts/e2e/resilience.sh trước)"; exit 1; }
python3 - "$f" <<'PY'
import json, sys, subprocess, urllib.parse, datetime as dt
rows = [json.loads(l) for l in open(sys.argv[1]) if l.strip()]
run = rows[-1]["run"]; rows = [r for r in rows if r["run"] == run]
def t(s): return dt.datetime.strptime(s, "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=dt.timezone.utc)
def prom(path):
    return json.loads(subprocess.run(["docker","run","--rm","--network","marketplace-net","alpine","wget","-qO-","http://prometheus:9090"+path],capture_output=True,text=True).stdout or "{}")
starts = {}
bad = 0
print(f"lần chạy {run}")
for r in rows:
    key = (r["scenario"], r["component"])
    if r["phase"] == "fault_start": starts[key] = r; continue
    s = starts.pop(key, None)
    if not s: continue
    a, b = t(s["ts"]), t(r["ts"]) + dt.timedelta(seconds=120)
    q = urllib.parse.quote('ALERTS{alertstate="firing"}')
    d = prom(f"/api/v1/query_range?query={q}&start={int(a.timestamp())}&end={int(b.timestamp())}&step=15")
    fired = sorted({(x["metric"]["alertname"], x["metric"].get("swarm_service") or x["metric"].get("service") or "") for x in d.get("data",{}).get("result",[])})
    names = {n for n,_ in fired}
    exp = [e.split(":")[0] for e in r["expected_alerts"]]
    # "A/B" = một trong hai (vd ServiceDown khi DNS trả IP giả, ServiceMissing khi target biến mất)
    miss = [e for e in exp if not any(x in names for x in e.split("/"))]
    dur = int((t(r["ts"]) - a).total_seconds())
    flag = "OK " if not miss else "THIẾU"
    if miss: bad += 1
    print(f"[{r['scenario']}] {r['fault']:<24} lỗi {dur:>4}s  mong đợi={exp or '-'}  thực tế={sorted(names) or '-'}  {flag}{' '+str(miss) if miss else ''}")
print("\nTHIẾU = cảnh báo mong đợi không bắn trong cửa sổ lỗi (lỗi quá ngắn so với `for:` của luật, hoặc khoảng trống phát hiện thật). Dùng GT_HOLD=<giây> khi chạy resilience.sh để giữ lỗi lâu hơn.")
sys.exit(1 if bad else 0)
PY
