#!/usr/bin/env python3
"""Tổng hợp tài nguyên từng service trong pha đo từ nhật ký `docker stats` do perf-run.sh ghi.
Dùng: perf_summary.py STATS_FILE CCU_JSON CONFIG_JSON OUT_JSON
Ghi số THÔ (đỉnh, trung bình, giới hạn, số mẫu) để tính % lại được; không làm tròn thành chữ."""
import collections, json, re, sys

stats_f, ccu_f, cfg_f, out_f = sys.argv[1:5]
ccu = json.load(open(ccu_f))
cfg = json.load(open(cfg_f))
t0, t1 = ccu.get("steadyStartMs", 0) / 1000, ccu.get("steadyEndMs", 1e18) / 1000

def mib(s):
    v = float(re.sub(r"[A-Za-z]+", "", s))
    return v * 1024 if "GiB" in s else v * 1 if "MiB" in s else v / 1024 if "KiB" in s else v

per = collections.defaultdict(lambda: collections.defaultdict(list))   # service -> ts -> [cpu của từng bản]
ram = collections.defaultdict(float)
ram_lim = {}
ts = None
for l in open(stats_f):
    p = l.split()
    if len(p) == 1 and p[0].isdigit():
        ts = int(p[0]); continue
    if ts is None or not (t0 <= ts <= t1) or len(p) < 5 or not p[0].startswith("marketplace"):
        continue
    svc = p[0].split(".")[0]
    per[svc][ts].append(float(p[1].rstrip("%")))
    ram[svc] = max(ram[svc], mib(p[2])); ram_lim[svc] = mib(p[4])

out = {"window_s": round(t1 - t0), "services": {}}
host = collections.defaultdict(float)
for svc, byts in per.items():
    one = [c for v in byts.values() for c in v]                  # từng container, từng mẫu
    total = [sum(v) for v in byts.values()]                      # cộng các bản cùng thời điểm
    for k, v in byts.items(): host[k] += sum(v)
    c = cfg.get(svc, {}); lim_cpu = c.get("cpu_limit"); rep = c.get("replicas")
    out["services"][svc] = {
        "replicas": rep, "cpu_limit_cores_per_replica": lim_cpu, "ram_limit_mib": c.get("ram_limit_mib") or ram_lim.get(svc),
        "cpu_pct_of_core": {"avg_per_container": round(sum(one) / len(one), 1), "max_per_container": round(max(one), 1),
                            "avg_all_replicas": round(sum(total) / len(total), 1), "max_all_replicas": round(max(total), 1)},
        "cpu_pct_of_limit_max": round(max(one) / (lim_cpu * 100) * 100, 1) if lim_cpu else None,
        "ram_max_mib": round(ram[svc], 1),
        "ram_pct_of_limit_max": round(ram[svc] / (c.get("ram_limit_mib") or ram_lim.get(svc)) * 100, 1) if (c.get("ram_limit_mib") or ram_lim.get(svc)) else None,
        "samples": len(byts),
    }
h = list(host.values())
out["containers_total_cpu_cores"] = {"avg": round(sum(h) / len(h) / 100, 2), "max": round(max(h) / 100, 2)} if h else None
json.dump(out, open(out_f, "w"), indent=1, ensure_ascii=False)
print(f"{'service':32s}{'bản':>4s}{'CPU tb':>8s}{'CPU max':>8s}{'%lim CPU':>9s}{'RAM max':>9s}{'%lim RAM':>9s}")
for svc, v in sorted(out["services"].items(), key=lambda kv: -kv[1]["cpu_pct_of_core"]["max_per_container"])[:12]:
    c = v["cpu_pct_of_core"]
    print(f"{svc:32s}{str(v['replicas'] or '-'):>4s}{c['avg_per_container']:8.1f}{c['max_per_container']:8.1f}{(v['cpu_pct_of_limit_max'] or 0):8.0f}%{v['ram_max_mib']:8.0f}M{(v['ram_pct_of_limit_max'] or 0):8.0f}%")
if out["containers_total_cpu_cores"]: print("tổng CPU các container (core): tb %(avg)s, đỉnh %(max)s" % out["containers_total_cpu_cores"])
