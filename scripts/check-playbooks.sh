#!/usr/bin/env bash
# Kiểm tính nhất quán của docs/playbooks/*.yml với deploy/prometheus/alerts.yml và RUNBOOK. Cần python3 + PyYAML.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
python3 - "$root" <<'PY'
import sys, glob, re, os, yaml
root = sys.argv[1]
alerts = {r["alert"]: r for g in yaml.safe_load(open(f"{root}/deploy/prometheus/alerts.yml"))["groups"] for r in g["rules"]}
runbook = open(f"{root}/docs/instruction/03_RUNBOOK.md").read()
sections = set(re.findall(r"^## (\d+)\.", runbook, re.M))
errs, covered = [], set()
for f in sorted(glob.glob(f"{root}/docs/playbooks/*.yml")):
    n = os.path.basename(f); p = yaml.safe_load(open(f))
    for k in ("id", "title", "triggers", "runbook", "diagnose", "causes", "actions", "escalate", "ground_truth"):
        if k not in p: errs.append(f"{n}: thiếu khoá {k}")
    acts = {a["id"]: a for a in p.get("actions", [])}
    for a in acts.values():
        if a.get("risk") not in ("observe", "low", "high"): errs.append(f"{n}: action {a['id']} risk không hợp lệ")
        want = {"observe": "none", "low": "confirm", "high": "required"}.get(a.get("risk"))
        if a.get("approval") != want: errs.append(f"{n}: action {a['id']} risk={a.get('risk')} phải approval={want}")
    for d in p.get("diagnose", []):
        if d.get("read_only") is not True: errs.append(f"{n}: diagnose {d.get('id')} phải read_only: true")
    for c in p.get("causes", []):
        for a in c.get("fix", []):
            if a not in acts: errs.append(f"{n}: cause {c['id']} trỏ action không có: {a}")
    for al in p["triggers"]["alerts"]:
        covered.add(al)
        if al not in alerts: errs.append(f"{n}: cảnh báo {al} không có trong alerts.yml")
        else:
            mx = max((acts[a]["risk"] for c in p["causes"] for a in c["fix"] if a in acts), key=["observe","low","high"].index, default="observe")
            # mức cảnh báo là mức hành động xử lý đề xuất: playbook phải có hành động cùng hoặc nặng hơn
            order = ["observe", "low", "high"]
            if order.index(mx) < order.index(str(alerts[al]["annotations"]["action"]).strip()):
                errs.append(f"{n}: cảnh báo {al} action={alerts[al]['annotations']['action']} nhưng playbook chỉ có hành động tối đa {mx}")
    for s in re.findall(r"\d+", str(p["runbook"])):
        if s not in sections: errs.append(f"{n}: runbook §{s} không có trong RUNBOOK")
for al in alerts:
    if al not in covered: errs.append(f"cảnh báo {al} chưa có playbook")
if errs:
    print("\n".join(errs)); sys.exit(1)
print(f"OK: {len(glob.glob(root + '/docs/playbooks/*.yml'))} playbook, {len(alerts)} cảnh báo đều được phủ")
PY
