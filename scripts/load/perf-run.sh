#!/usr/bin/env bash
# Một lượt đo: chạy ccu.mjs (gắn vào 2 core riêng), ghi tài nguyên từng container, chụp cấu hình Swarm, tổng hợp ra JSON.
#   scripts/load/perf-run.sh NHÃN [tham số ccu.mjs...]     vd: perf-run.sh closed-500-r1 --vus=500 --duration=90
#                                                           vd: perf-run.sh open-20-r1 --arrival-rate=20 --duration=90
# Biến: LOAD_SCRIPT (mặc định ccu.mjs; svc.mjs = đo từng service), OUT_DIR (mặc định docs/perf-results/<ngày>), LOADGEN_CPUS (mặc định 14,15; rỗng = không gắn core).
# Kết quả trong OUT_DIR: NHÃN.json (ccu.mjs), NHÃN.resources.json, NHÃN.config.json, NHÃN.txt (nhật ký), NHÃN.stats.txt (docker stats thô).
set -euo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"; cd "$root"
label="${1:?thiếu NHÃN}"; shift
out="${OUT_DIR:-docs/perf-results/$(date +%F)}"; mkdir -p "$out"
cpus="${LOADGEN_CPUS-14,15}"
envval() { grep -E "^$1=" deploy/.env | tail -1 | cut -d= -f2- | sed 's/[[:space:]]*#.*$//'; }
export ADMIN_USERNAME="${ADMIN_USERNAME:-$(envval ADMIN_BOOTSTRAP_USERNAME)}" ADMIN_PASSWORD="${ADMIN_PASSWORD:-$(envval ADMIN_BOOTSTRAP_PASSWORD)}"

# cấu hình đang chạy (số bản, giới hạn CPU/RAM) + phiên bản
python3 - "$out/$label.config.json" <<'PY'
import json, subprocess, sys
def rd(p):
    try: return open(p).read().strip()
    except OSError: return None
cfg = {}
for name in subprocess.check_output(["docker", "service", "ls", "--format", "{{.Name}}"], text=True).split():
    i = json.loads(subprocess.check_output(["docker", "service", "inspect", name], text=True))[0]["Spec"]
    lim = (i["TaskTemplate"].get("Resources") or {}).get("Limits") or {}
    cfg[name] = {"replicas": (i.get("Mode", {}).get("Replicated") or {}).get("Replicas"),
                 "cpu_limit": (lim.get("NanoCPUs") or 0) / 1e9 or None,
                 "ram_limit_mib": round(lim["MemoryBytes"] / 1048576) if lim.get("MemoryBytes") else None,
                 "image": i["TaskTemplate"]["ContainerSpec"]["Image"].split("@")[0]}
cfg["_meta"] = {"git": subprocess.run(["git", "rev-parse", "--short", "HEAD"], capture_output=True, text=True).stdout.strip(),
                "dirty": bool(subprocess.run(["git", "status", "--porcelain"], capture_output=True, text=True).stdout.strip()),
                "cores": int(subprocess.check_output(["nproc"], text=True)), "loadgen_note": "máy tải và hệ thống dùng chung một máy",
                "platform_profile": rd("/sys/firmware/acpi/platform_profile"), "cpu_governor": rd("/sys/devices/system/cpu/cpu0/cpufreq/scaling_governor"),
                "on_ac": rd("/sys/class/power_supply/ACAD/online"), "max_temp_c_at_start": max([int(rd(f"/sys/class/thermal/thermal_zone{i}/temp") or 0) // 1000 for i in range(20)] or [0])}
json.dump(cfg, open(sys.argv[1], "w"), indent=1)
PY

( while true; do date +%s; docker stats --no-stream --format '{{.Name}} {{.CPUPerc}} {{.MemUsage}}' | grep -E '^marketplace' || true; sleep 3; done > "$out/$label.stats.txt" 2>&1 ) & sampler=$!
trap 'kill $sampler 2>/dev/null || true' EXIT
pin=(); [ -n "$cpus" ] && pin=(taskset -c "$cpus")
rc=0; "${pin[@]}" node "scripts/load/${LOAD_SCRIPT:-ccu.mjs}" "$@" --out="$out/$label.json" > "$out/$label.txt" 2>&1 || rc=$?
kill $sampler 2>/dev/null || true; wait $sampler 2>/dev/null || true
# Số lần container bị throttle (chạm trần CPU) trong lượt đo: chính xác hơn docker stats vì docker stats lấy mẫu cách quãng nên che các đợt chạm trần ngắn.
prom=$(docker ps -q -f name=marketplace-obs_prometheus | head -1)
if [ -n "$prom" ]; then
  win=$(( $(date +%s) - $(head -1 "$out/$label.stats.txt") + 5 ))
  q="sum by (container_label_com_docker_swarm_service_name)(increase(container_cpu_cfs_throttled_periods_total{container_label_com_docker_swarm_service_name!=\"\"}[${win}s])) / sum by (container_label_com_docker_swarm_service_name)(increase(container_cpu_cfs_periods_total{container_label_com_docker_swarm_service_name!=\"\"}[${win}s]))"
  docker exec "$prom" wget -qO- "http://localhost:9090/api/v1/query?query=$(python3 -c 'import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))' "$q")" > "$out/$label.throttle.json" 2>/dev/null || true
  python3 - "$out/$label.throttle.json" <<'PY' | tee -a "$out/$label.txt" || true
import json, sys
try:
    r = json.load(open(sys.argv[1]))["data"]["result"]
except Exception:
    sys.exit(0)
rows = sorted(((float(x["value"][1]), x["metric"].get("container_label_com_docker_swarm_service_name", "?")) for x in r), reverse=True)
hit = [f"{n.split('_',1)[-1]} {v*100:.1f}%" for v, n in rows if v >= 0.01]
print("THROTTLE (% chu kỳ CPU bị chặn vì chạm trần, >=1%): " + (", ".join(hit) if hit else "không service nào"))
PY
fi
if [ -f "$out/$label.json" ]; then python3 scripts/load/perf_summary.py "$out/$label.stats.txt" "$out/$label.json" "$out/$label.config.json" "$out/$label.resources.json"; fi
grep -E "^CHẾ ĐỘ MỞ|^MÁY TẢI|^TOÀN MÁY|ổn định [0-9]+ s|^AI:|^Thanh toán|PHÉP ĐO|THIẾU ĐỘ PHỦ" "$out/$label.txt" || true
exit $rc
