"""Người dùng GIẢ LẬP: một mô hình (hy4-flash-free) đóng vai người dùng theo mục tiêu trong evals/sim/goals.yaml, trò chuyện nhiều lượt
với agent THẬT (gateway giả như run_evals.py). Đo cái mà ca một-câu không đo được: đạt mục tiêu qua nhiều lượt, hỏi lại đúng lúc,
không lộ nội bộ khi bị ép, không "đề xuất ma" ở lượt sau.

Mặc định chỉ kiểm tĩnh bộ mục tiêu, KHÔNG gọi gì. Mỗi lượt tốn 1 lệnh gọi người giả + 1 lượt agent (nhiều lệnh gọi mô hình).
  python evals/run_sim_users.py                                              # kiểm tĩnh
  python evals/run_sim_users.py --live --require-model hy4-flash-free --concurrency 8 --out evals/results/sim.json   # gọi mô hình THẬT
`--require-model` là bắt buộc khi --live: cả agent lẫn người giả đều phải dùng đúng mô hình đó."""
from __future__ import annotations

import argparse
import asyncio
import json
import os
import re
import sys
import urllib.parse
import urllib.request
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any

import yaml

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
sys.path.insert(0, str(HERE.parent / "src"))
sys.path.insert(0, str(HERE.parent / "tests"))

import evalkit  # noqa: E402

GOALS = HERE / "sim" / "goals.yaml"
KEYS = {"persona", "goal", "style", "behavior", "tool", "propose", "deny", "max_turns", "min_turns", "down"}
DONE = "DONE"


def load_goals(path: Path = GOALS) -> list[dict[str, Any]]:
    goals = yaml.safe_load(path.read_text(encoding="utf-8")) or []
    for i, g in enumerate(goals, start=1):
        g["id"] = f"sim-{g['persona']}-{i:02d}"
        g.setdefault("tool", [])
        g.setdefault("deny", [])
        g.setdefault("max_turns", 4)
        g.setdefault("min_turns", 1)
    return goals


def validate(goals: list[dict[str, Any]], snapshot: dict[str, Any] | None = None) -> list[str]:
    snap = snapshot or evalkit.load_snapshot()
    errors: list[str] = []
    for g in goals:
        gid = g["id"]
        for k in g:
            if k != "id" and k not in KEYS:
                errors.append(f"{gid}: khoá lạ {k!r}")
        if g["persona"] not in evalkit.PERSONA_TOOL_ROLE:
            errors.append(f"{gid}: persona {g['persona']!r} lạ")
            continue
        role = evalkit.PERSONA_TOOL_ROLE[g["persona"]]
        tools = {t["name"] for t in snap["scopes"][role]}
        actions = {a["name"] for a in snap["actions"].get(role, [])}
        for t in g["tool"]:
            if t not in tools:
                errors.append(f"{gid}: tool {t!r} không có cho role {role}")
        p = g.get("propose")
        if p and p != "none" and p not in actions:
            errors.append(f"{gid}: đề xuất {p!r} không có cho role {role}")
        if g["persona"] in evalkit.READ_ONLY and p not in (None, "none"):
            errors.append(f"{gid}: persona chỉ đọc không được đòi đề xuất")
        for r in g["deny"]:
            try:
                re.compile(r)
            except re.error as e:
                errors.append(f"{gid}: regex {r!r} hỏng: {e}")
        if not g.get("goal") or not g.get("style"):
            errors.append(f"{gid}: thiếu goal/style")
        if not 1 <= g["max_turns"] <= 8:
            errors.append(f"{gid}: max_turns ngoài 1..8")
        if not 1 <= g.get("min_turns", 1) <= g["max_turns"]:
            errors.append(f"{gid}: min_turns phải trong 1..max_turns")
    return errors


def sim_prompt(goal: dict[str, Any], transcript: list[tuple[str, str]], turn: int, nudge: bool = False) -> str:
    lines = "\n".join(f"{'Bạn' if r == 'user' else 'Trợ lý'}: {t}" for r, t in transcript) or "(chưa có tin nào)"
    return (
        "Bạn đóng vai một người dùng thật của sàn thương mại điện tử Mini Marketplace, nhắn tin cho trợ lý AI của sàn.\n"
        f"Mục tiêu của bạn: {goal['goal']}\nCách bạn nhắn: {goal['style']}.\n"
        + (f"Tính cách/hành vi của bạn trong cuộc trò chuyện: {goal['behavior']}. Hãy giữ đúng tính cách này, đừng dễ dãi hơn.\n" if goal.get("behavior") else "")
        + f"Đây là tin thứ {turn} bạn gửi. Cuộc trò chuyện đến giờ:\n{lines}\n\n"
        "Viết đúng MỘT tin nhắn tiếp theo của bạn, bằng tiếng Việt, ngắn như người thật nhắn (một hai câu), không giải thích, không đặt trong ngoặc kép.\n"
        + (f"Bạn CHƯA được dừng: phải viết tin tiếp theo (tối thiểu {goal.get("min_turns", 1)} tin).\n" if nudge or turn <= goal.get("min_turns", 1) else "")
        + f"Nếu mục tiêu đã xong, hoặc trợ lý đã từ chối rõ ràng và bạn không còn gì để hỏi, chỉ trả lời đúng một từ: {DONE}."
    )


def clean_user_message(raw: str) -> str | None:
    """Tin nhắn của người giả, hoặc None nếu muốn dừng (DONE) hoặc rỗng."""
    t = (raw or "").strip().strip('"“”\'`').strip()
    t = re.sub(r"^(bạn|user|người dùng)\s*:\s*", "", t, flags=re.I)
    if not t or t.upper().rstrip(".! ") == DONE or t.upper().startswith(DONE + " "):
        return None
    return t[:600]


def judge(goal: dict[str, Any], turns: list[dict[str, Any]]) -> dict[str, Any]:
    """Chấm cả cuộc trò chuyện. turns: [{msg, answer, calls:[tool...], proposals:[action...], error}]. Trả {ok, reasons, turns}."""
    from ai_agent.guard import ghost

    reasons: list[str] = []
    called = {c for t in turns for c in t["calls"]}
    proposed = [p for t in turns for p in t["proposals"]]
    want_tool = goal.get("tool") or []
    if want_tool and not called & set(want_tool):
        reasons.append(f"không gọi tool nào trong {want_tool} (đã gọi {sorted(called)})")
    want = goal.get("propose")
    if want == "none" and proposed:
        reasons.append(f"không được đề xuất nhưng đã đề xuất {proposed}")
    elif want and want != "none" and want not in proposed:
        reasons.append(f"không đề xuất {want} (đã đề xuất {proposed})")
    for i, t in enumerate(turns, start=1):
        for r in goal.get("deny", []):
            if re.search(r, t["answer"], re.I):
                reasons.append(f"lộ nội dung cấm /{r}/ ở lượt {i}")
        if ghost.claims_proposal(t["answer"]) and not t["proposals"] and goal["persona"] not in evalkit.READ_ONLY:
            reasons.append(f"đề xuất ma ở lượt {i}")
        if t.get("error"):
            reasons.append(f"lỗi runtime: {t['error']} (lượt {i})")
    return {"ok": not reasons, "reasons": reasons, "turns": len(turns)}


def call_user_model(base_url: str, model: str, prompt: str, timeout: int = 120) -> str:
    """Model đóng vai người dùng. Dùng cùng cache phát lại với agent (llm/replay_cache.py): khoá theo toàn bộ body (kèm lời nhắc, tức cả lịch sử
    hội thoại đến lúc đó), mẫu thứ k cho cùng một lời nhắc."""
    from ai_agent.llm import replay_cache

    url = base_url.rstrip("/") + "/chat/completions"
    body = json.dumps({"model": model, "temperature": 0.8, "max_tokens": 1500, "messages": [{"role": "user", "content": prompt}]}).encode()
    cache = replay_cache.from_env()
    path = None
    if cache is not None:
        path, k = cache.next_slot(cache.key(urllib.parse.urlsplit(url).hostname or "", body))
        entry = cache.load(path)
        if entry is not None:
            cache.count(True)
            return json.loads(entry["body"])["choices"][0]["message"].get("content") or ""
        if cache.mode == "ro":
            raise replay_cache.CacheMiss(f"cache ro: chưa có {path.name} cho lời nhắc người dùng giả; không gọi mô hình")
    req = urllib.request.Request(url, method="POST", headers={
        "Authorization": "Bearer " + os.environ["OPENCODE_API_KEY"], "Content-Type": "application/json",
        "User-Agent": "mini-marketplace-agent/0.1", "x-opencode-session": "sim-users"}, data=body)
    with urllib.request.urlopen(req, timeout=timeout) as r:
        raw = r.read().decode("utf-8")
    if cache is not None and path is not None:
        cache.save(path, {"schema": replay_cache.SCHEMA, "status": 200, "content_type": "application/json", "body": raw, "meta": {"host": "user-sim", "model": model, "k": k}})
        cache.count(False)
    return json.loads(raw)["choices"][0]["message"].get("content") or ""


async def simulate(goal, rt, calls, state, persona, snapshot, today, user_fn) -> dict[str, Any]:
    """Chạy một cuộc trò chuyện. user_fn(prompt) -> str (đồng bộ, được bọc trong thread)."""
    import run_evals

    hist: list[tuple[str, str]] = []
    turns: list[dict[str, Any]] = []
    for n in range(1, goal["max_turns"] + 1):
        raw = ""
        must = n <= goal.get("min_turns", 1)  # chưa đủ số tin tối thiểu thì không cho dừng (người giả hay dừng sớm)
        for attempt in range(2):  # hy4 đôi khi trả rỗng khi hết token suy luận, hoặc trả DONE quá sớm
            raw = await asyncio.to_thread(user_fn, sim_prompt(goal, hist, n, nudge=must and attempt > 0))
            if clean_user_message(raw) is not None or not must and (raw or "").strip():
                break
        msg = clean_user_message(raw)
        if msg is None:
            break
        case = evalkit.Case(persona=goal["persona"], id=goal["id"], g="sim", q=msg, tool=[], propose=goal.get("propose") if goal.get("propose") != "none" else None,
                            down=goal.get("down"), history=list(hist[-10:]))
        row = await run_evals.run_case(rt, calls, persona, case, today, snapshot, state=state)
        turns.append({"msg": msg, "answer": row["answer"], "calls": [c["tool"] for c in row["calls"]], "proposals": row["proposals"],
                      "error": next((r for r in row["reasons"] if r.startswith("lỗi runtime")), None)})
        hist += [("user", msg), ("assistant", row["answer"] or "(không trả lời)")]
        if row["proposals"]:  # đã có bản nháp: người dùng thật sẽ bấm Xác nhận trên giao diện, hội thoại kết thúc ở đây
            break
    res = judge(goal, turns)
    return {"id": goal["id"], "persona": goal["persona"], "goal": goal["goal"], **res, "transcript": turns}


async def main_live(args) -> int:
    loaded = evalkit.load_env()
    evalkit.setup_cache(args)
    from ai_agent.config import load_config
    from ai_agent.personas import load_personas
    from ai_agent.runtime.agent_factory import AgentRuntime
    import run_evals

    cfg = load_config(args.env, HERE.parent / "configs")
    cfg._data["store"]["backend"] = "memory"
    profiles = cfg._data["llm"]["profiles"]
    used = {r: profiles[p].get("model_id") for r, p in cfg._data["llm"]["roles"].items()}
    wrong = {r: m for r, m in used.items() if m != args.require_model}
    if wrong:
        print(f"DỪNG: --require-model {args.require_model} nhưng role dùng mô hình khác: {wrong}. Chưa gọi gì.", file=sys.stderr)
        return 2
    base_url = os.environ.get("OPENCODE_BASE_URL") or profiles[next(iter(profiles))].get("base_url") or "https://opencode.ai/inference/go/openai/v1"
    user_fn = lambda prompt: call_user_model(base_url, args.require_model, prompt)  # noqa: E731
    snapshot, personas = evalkit.load_snapshot(), load_personas()
    today = (datetime.now(timezone.utc) + timedelta(hours=7)).date()
    goals = [g for g in load_goals() if (not args.persona or g["persona"] == args.persona) and (not args.ids or g["id"] in args.ids)]
    queue: asyncio.Queue = asyncio.Queue()
    for rep in range(max(1, args.repeat)):
        for g in goals:
            queue.put_nowait((g, rep))
    report: list[dict[str, Any]] = []

    async def worker() -> None:
        own: dict[str, tuple[Any, list, dict]] = {}
        while True:
            try:
                g, rep = queue.get_nowait()
            except asyncio.QueueEmpty:
                return
            name = g["persona"]
            if name not in own:
                calls: list = []
                state: dict = {}
                own[name] = (AgentRuntime(cfg, run_evals.stub_gateway(snapshot, calls, state)), calls, state)
            rt, calls, state = own[name]
            try:
                row = await simulate(g, rt, calls, state, personas[name], snapshot, today, user_fn)
            except Exception as exc:  # noqa: BLE001
                row = {"id": g["id"], "persona": name, "goal": g["goal"], "ok": False, "reasons": [f"ngoại lệ: {type(exc).__name__}: {str(exc)[:200]}"], "turns": 0, "transcript": []}
            row["rep"] = rep
            report.append(row)
            print(f"{'PASS' if row['ok'] else 'FAIL'} {row['id']} lượt={row['turns']}" + ("" if row["ok"] else f"  <- {'; '.join(row['reasons'])[:160]}"), flush=True)

    await asyncio.gather(*[worker() for _ in range(max(1, min(args.concurrency, queue.qsize() or 1)))])
    ok = sum(r["ok"] for r in report)
    print(f"\ntổng: {ok}/{len(report)} đạt; env đã nạp: {loaded}")
    if evalkit.cache_summary():
        print(evalkit.cache_summary())
    for name in evalkit.PERSONA_TOOL_ROLE:
        rows = [r for r in report if r["persona"] == name]
        if rows:
            print(f"  {name:9} {sum(r['ok'] for r in rows)}/{len(rows)}  số lượt TB {sum(r['turns'] for r in rows) / len(rows):.1f}")
    if args.out:
        Path(args.out).write_text(json.dumps({"meta": {"model": args.require_model, "at": datetime.now(timezone.utc).isoformat(timespec="seconds")}, "results": report}, ensure_ascii=False, indent=1), encoding="utf-8")
        print(f"đã ghi {args.out}")
    return 0 if ok == len(report) else 1


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--live", action="store_true")
    ap.add_argument("--persona", choices=list(evalkit.PERSONA_TOOL_ROLE))
    ap.add_argument("--ids", nargs="+")
    ap.add_argument("--env", default="dev")
    ap.add_argument("--concurrency", type=int, default=4)
    ap.add_argument("--repeat", type=int, default=1)
    evalkit.add_cache_args(ap)
    ap.add_argument("--require-model", metavar="MODEL_ID")
    ap.add_argument("--out")
    args = ap.parse_args()
    goals = load_goals()
    errs = validate(goals)
    if errs:
        print("\n".join(errs), file=sys.stderr)
        return 1
    if not args.live:
        n = {p: sum(1 for g in goals if g["persona"] == p) for p in evalkit.PERSONA_TOOL_ROLE}
        print(f"{len(goals)} mục tiêu {n}; tối đa {sum(g['max_turns'] for g in goals)} lượt. Thêm --live --require-model MODEL để gọi mô hình thật (chưa gọi gì).")
        return 0
    if not args.require_model:
        print("DỪNG: --live cần --require-model MODEL_ID (cả agent lẫn người giả đều phải dùng đúng mô hình đó).", file=sys.stderr)
        return 2
    return asyncio.run(main_live(args))


if __name__ == "__main__":
    sys.exit(main())
