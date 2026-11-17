"""Chạy bộ eval RCA (evals/rca/cases.yaml). Mặc định chỉ in kế hoạch và KHÔNG gọi gì.
  python evals/run_rca_evals.py                       # kế hoạch + kiểm tĩnh bộ case (không gọi mô hình)
  python evals/run_rca_evals.py --oracle              # chạy đồ thị với Brain "đáp án" (kiểm bộ eval, không LLM)
  python evals/run_rca_evals.py --live --ids C-payment-down --out evals/results/rca.json   # gọi mô hình THẬT (tốn tiền; phải hỏi người dùng trước)
--live dùng llm.roles.rca của config, gateway GIẢ (dữ liệu trong case), người được giả lập (luôn xác nhận/duyệt, hành động chạy xong ngay)."""
from __future__ import annotations

import argparse
import asyncio
import json
import sys
import time
from pathlib import Path

import rcakit

MAX_CALLS_PER_CASE = 12  # trần lượt mô hình mỗi case (rca.max_model_calls); khi ước tính chi phí lấy trần này


async def run_all(args) -> list[dict]:
    cases = [c for c in rcakit.load_cases() if (not args.ids or c["id"] in args.ids)]
    rows = []
    for c in [c for _ in range(max(1, args.repeat)) for c in cases]:  # lặp k lần cả bộ (ca nào cũng chạy k lần)
        world = rcakit.World(c)
        started = time.monotonic()
        if args.oracle:
            brain = rcakit.OracleBrain(c, world)
        elif args.live:
            brain = await live_brain(world, c)
        else:
            raise SystemExit("cần --oracle hoặc --live")
        try:
            run = await rcakit.run_case(c, world, brain)
            res = rcakit.score(c, run)
            row = {"id": c["id"], "ok": res.ok, "reasons": res.reasons, "checks": res.checks, "model_calls": run.values.get("model_calls"),
                   "input_tokens": run.values.get("input_tokens", 0), "output_tokens": run.values.get("output_tokens", 0),
                   "queries": run.values.get("queries"), "outcome": run.values.get("outcome"), "drafts": [(d["action_id"], d["params"]) for d in run.drafts],
                   "hypotheses": [h["cause"] for h in run.values.get("hypotheses", [])], "seconds": round(time.monotonic() - started, 1)}
        except Exception as exc:  # noqa: BLE001
            row = {"id": c["id"], "ok": False, "reasons": [f"ngoại lệ: {type(exc).__name__}: {exc}"], "checks": {}}
        rows.append(row)
        print(f"{'PASS' if row['ok'] else 'FAIL'} {c['id']}" + ("" if row["ok"] else f"  <- {'; '.join(row['reasons'])[:200]}"), flush=True)
    return rows


async def live_brain(world, case):
    from ai_agent.config import load_config
    from ai_agent.personas import load_personas
    from ai_agent.rca.brain import LlmBrain
    from ai_agent.rca.budget import Budget
    from ai_agent.runtime.agent_factory import AgentRuntime
    from ai_agent.runtime.context import RunContext
    from helpers import agent_token

    cfg = load_config("dev", rcakit.HERE.parent / "configs")
    cfg._data["store"]["backend"] = "memory"
    rt = AgentRuntime(cfg, rcakit.stub_gateway(world))
    ctx = RunContext(token=agent_token(9001, "operator", f"eval-{case['id']}", ttl=900), run_id=f"eval-{case['id']}", request_id="eval", user_id=9001, role="operator")
    return LlmBrain(rt, load_personas()["rca"], ctx, Budget.from_config(cfg))


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--live", action="store_true", help="gọi mô hình thật (tốn tiền)")
    ap.add_argument("--oracle", action="store_true", help="Brain đáp án, không LLM (kiểm bộ eval)")
    ap.add_argument("--ids", nargs="+")
    ap.add_argument("--repeat", type=int, default=1, help="chạy mỗi ca k lần để thấy độ dao động (--live: tốn k lần chi phí)")
    ap.add_argument("--out")
    args = ap.parse_args()
    cases = rcakit.load_cases()
    bad = rcakit.validate_cases(cases)
    if bad:
        print("bộ case không hợp lệ:\n  " + "\n  ".join(bad))
        return 2
    if not (args.live or args.oracle):
        n = len(args.ids or cases)
        k = max(1, args.repeat)
        print(f"{len(cases)} case RCA hợp lệ (9 kịch bản A-M có cảnh báo của resilience.sh + biến thể; đủ 10 playbook). Ước tính --live: {n} case x {k} lần x tối đa {MAX_CALLS_PER_CASE} lượt mô hình = tối đa {n * k * MAX_CALLS_PER_CASE} lượt gọi.")
        for c in cases:
            print(f"  {c['id']:34} playbook={c['expect']['playbook']}  hành động={(c['expect'].get('action') or c['expect']['action_any'][0])['id']}")
        print("Chưa gọi gì. Thêm --oracle (không LLM) hoặc --live (gọi mô hình thật).")
        return 0
    rows = asyncio.run(run_all(args))
    ok = sum(1 for r in rows if r["ok"])
    print(f"\ntổng: {ok}/{len(rows)} đạt")
    for k in ("cause", "playbook", "action", "forbid", "outcome", "budget", "read_only", "grounded"):
        have = [r["checks"][k] for r in rows if r.get("checks")]
        print(f"  {k:10} {sum(have)}/{len(have)}")
    if args.repeat > 1:
        sm = rcakit.summarize_repeats(rows)
        print(f"lặp {args.repeat} lần: đạt {sm['ok']}/{sm['runs']} = {sm['rate']:.0%}, khoảng tin cậy 95% {sm['ci95'][0]:.0%}-{sm['ci95'][1]:.0%}; "
              f"luôn đạt {sm['always_ok']}/{sm['cases']}, không bao giờ đạt {sm['never_ok']}; ca dao động: {', '.join(sm['flaky']) or 'không'}")
    if args.out:
        from datetime import datetime, timezone

        import evalkit
        from ai_agent.config import load_config
        cfg = load_config("dev", rcakit.HERE.parent / "configs")
        meta = {**evalkit.fingerprint(cfg._data["llm"]), "run": {"mode": "live" if args.live else "oracle", "repeat": args.repeat, "ids": args.ids,
                "at": datetime.now(timezone.utc).isoformat(timespec="seconds")}}
        evalkit.write_report(args.out, rows, meta)
        print(f"đã ghi {args.out} (dấu vân tay prompt/tool/mô hình nằm ở trường meta)")
    return 0 if ok == len(rows) else 1


if __name__ == "__main__":
    sys.exit(main())
