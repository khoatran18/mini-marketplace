#!/usr/bin/env python3
"""Bảng CPU/throttle/RAM từng service THEO TỪNG BẬC của một lượt svc.mjs (đọc NHÃN.json + NHÃN.capture.jsonl + NHÃN.config.json).
Dùng:  python3 scripts/load/perf-svc-report.py <thư mục> <nhãn>      -> in và ghi <thư mục>/<nhãn>.svcreport.txt
Mỗi bậc: các service dùng >= MIN_CORES (mặc định 0.1) nhân, dạng  tên=nhân(%trần)/throttle%  (trần = số bản x CPU limit của một bản, lấy từ config.json). Nhân = Δusage_usec / Δgiây giữa mẫu
đầu và cuối nằm trong [t0,t1] của bậc; throttle% = Δnr_throttled / Δnr_periods. Chỉ là CPU của container (cgroup), không gồm máy tải."""
import json, sys
import os
THR = float(os.environ.get("MIN_CORES", "0.1"))
d, label = sys.argv[1], sys.argv[2]
run = json.load(open(f"{d}/{label}.json"))
cfg = json.load(open(f"{d}/{label}.config.json"))
cap = [json.loads(l) for l in open(f"{d}/{label}.capture.jsonl")][1:]
cap = [r for r in cap if "cg" in r]
lim = {}
for n, c in cfg.items():
    if n.startswith("_") or not c.get("cpu_limit"): continue
    lim[n] = (c.get("replicas") or 1) * c["cpu_limit"]
out = []
def P(s=""): print(s); out.append(s)
for tgt, res in run["results"].items():
    P(f"\n=== {tgt}: CPU container theo bậc (nhân, %trần, throttle%) ===")
    for st in res["steps"]:
        rows = [r for r in cap if st["t0"] <= r["t"] <= st["t1"]]
        if len(rows) < 2:
            P(f"bậc {st['step']} {st['rate']}/s: không đủ mẫu capture"); continue
        a, b = rows[0], rows[-1]; dt = b["t"] - a["t"]
        items = []
        for svc, cb in b["cg"].items():
            ca = a["cg"].get(svc)
            if not ca: continue
            cores = (cb["cpu"]["usage_usec"] - ca["cpu"]["usage_usec"]) / 1e6 / dt
            dp = cb["cpu"]["nr_periods"] - ca["cpu"]["nr_periods"]; thr = 100 * (cb["cpu"]["nr_throttled"] - ca["cpu"]["nr_throttled"]) / dp if dp > 0 else 0
            if cores >= THR: items.append((cores, svc, thr, max(r["cg"][svc]["mem"] for r in rows if svc in r["cg"]) / 1048576))
        items.sort(reverse=True)
        txt = "  ".join(f"{s.split('_',1)[-1]}={c:.2f}({100*c/lim[s]:.0f}%)/{t:.0f}%" if s in lim else f"{s.split('_',1)[-1]}={c:.2f}/{t:.0f}%" for c, s, t, m in items[:7])
        P(f"bậc {st['step']} {st['rate']}/s ok/s={st['okPerSec']} lỗi={st['errPct']}% p95={st['p95']}: {txt}")
open(f"{d}/{label}.svcreport.txt", "w").write("\n".join(out) + "\n")
