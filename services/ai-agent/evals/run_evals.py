"""Chạy bộ eval qua agent THẬT trong tiến trình này: mô hình thật (llm.roles trong config), gateway GIẢ trả dữ liệu rỗng nhưng hợp lệ.
Đo: chọn đúng tool, đúng tham số lọc, gọi đủ ít lần, đề xuất thay vì ghi, không lộ route/tên tool, từ chối đúng chỗ.
KHÔNG đo độ đúng số liệu trong câu trả lời (gateway giả không có dữ liệu): phần đó đối chiếu với DB ở bước smoke trên Swarm.

Mặc định chỉ liệt kê kế hoạch và KHÔNG gọi mô hình. Gọi mô hình tốn tiền nên phải thêm --live.
  python evals/run_evals.py                          # kế hoạch: số câu mỗi persona, không gọi gì
  python evals/run_evals.py --live --persona buyer --limit 10 --out evals/results/buyer.json   # results/ is git-ignored and survives a reboot (/tmp does not)
Cần OPENCODE_API_KEY (hoặc khoá của profile trong config) trong môi trường; không in khoá."""
from __future__ import annotations

import argparse
import asyncio
import json
import re
import sys
import time
from collections import Counter
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any

import httpx

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
sys.path.insert(0, str(HERE.parent / "src"))
sys.path.insert(0, str(HERE.parent / "tests"))

import evalkit  # noqa: E402

READ_ONLY = evalkit.READ_ONLY
TOKEN_ROLE = {"buyer": "buyer", "seller": "seller_admin", "admin": "admin", "operator": "operator"}


def _template_regex(path: str) -> re.Pattern[str]:
    return re.compile("^" + re.sub(r":\w+", r"([^/]+)", path) + "$")


# Trạng thái đơn mà gateway giả trả cho từng hành động đề xuất: đúng điều kiện tiên quyết thì agent mới có lý do đề xuất.
PRECONDITION_STATUS = {"cancel_order": "PAID", "request_return": "DELIVERED", "approve_return": "REFUND_REQUESTED", "reject_return": "REFUND_REQUESTED"}


def stub_gateway(snapshot: dict[str, Any], calls: list[dict[str, Any]], state: dict[str, Any] | None = None):
    """Gateway giả dựng từ chính bảng tool thật: /ai/tools trả đúng bộ tool của role, mọi tool đọc trả dữ liệu rỗng, /ai/proposals ký giả."""
    from ai_agent.tools.gateway import GatewayClient

    state = state if state is not None else {}

    def handler(req: httpx.Request) -> httpx.Response:
        token = req.headers.get("authorization", "")
        role = _role_of(token)
        if req.url.path == "/ai/tools":
            tools = snapshot["scopes"].get(role, [])
            actions = [] if role in READ_ONLY else snapshot["actions"].get(role, [])
            return httpx.Response(200, json={"role": role, "tools": tools, "actions": actions})
        if req.url.path == "/ai/proposals":
            return httpx.Response(201, json={"proposal": "SIGNED.PROPOSAL.TOKEN", "id": "prop-eval", "expires_at": "2099-01-01T00:00:00Z"})
        for t in snapshot["scopes"].get(role, []):
            m = _template_regex(t["path"]).match(req.url.path)
            if m and t["method"] == req.method:
                args = dict(req.url.params)
                for p, v in zip(re.findall(r":(\w+)", t["path"]), m.groups()):
                    args[p] = v
                calls.append({"tool": t["name"], "args": args})
                status = state.get("status", "PAID")
                # Có MỘT dòng dữ liệu hợp lệ: gateway trả rỗng thì agent (đúng) dò thêm nhiều lần, làm sai phép đo "gọi bao nhiêu lần".
                row = {"id": 1, "status": status, "total": 250000, "name": "Mẫu", "state": "ok", "ok": True, "delivered_at": state.get("delivered_at")}
                if "product" in t["path"] or t["name"] in ("search_products", "list_low_stock"):  # đúng dạng sản phẩm, không phải bản ghi đơn
                    row = {"id": 1, "name": "Sản phẩm mẫu", "brand": "Mẫu", "category_id": 1, "price": 150000, "status": "ACTIVE", "inventory": 42, "available": 42, "low_stock_threshold": 5}
                body = {**row, "id": int(args["id"]) if str(args["id"]).isdigit() else args["id"], "items": [row]} if "id" in args else {"items": [row], "total": 1, "page": 1}
                return httpx.Response(200, json=body)
        return httpx.Response(404, json={"error": "not found"})

    return GatewayClient("http://gw", 15.0, client=httpx.AsyncClient(transport=httpx.MockTransport(handler)))


def _role_of(authorization: str) -> str:
    import base64
    try:
        payload = authorization.split(".")[1]
        return json.loads(base64.urlsafe_b64decode(payload + "=" * (-len(payload) % 4)))["role"]
    except Exception:  # noqa: BLE001
        return ""


def _action_name(snapshot: dict[str, Any], role: str, request: dict[str, Any]) -> str:
    """Tên hành động đề xuất, suy từ method+path trong `request` của sự kiện (sự kiện không mang tên)."""
    for a in snapshot["actions"].get(role, []):
        if a["method"] == request.get("method") and _template_regex(a["path"]).match(str(request.get("path", ""))):
            return a["name"]
    return f"{request.get('method')} {request.get('path')}"


async def run_case(rt, calls: list[dict[str, Any]], persona, case: evalkit.Case, today, snapshot: dict[str, Any], user_id: int = 9001, state: dict[str, Any] | None = None) -> dict[str, Any]:
    from ai_agent.runtime import events
    from ai_agent.runtime.chat import run_turn
    from ai_agent.runtime.context import RunContext
    from ai_agent.store import MemoryStore
    from helpers import agent_token

    store = MemoryStore()
    role = TOKEN_ROLE[case.persona]
    run_id = f"eval-{case.id}-{int(time.time() * 1000) % 10**8:08d}"
    calls.clear()
    if state is not None:
        state.clear()
        state["status"] = PRECONDITION_STATUS.get(case.propose or "", "PAID")
        state["delivered_at"] = (datetime.now(timezone.utc) - timedelta(days=1)).isoformat()
    ctx = RunContext(token=agent_token(user_id, role, run_id, ttl=900), run_id=run_id, request_id="eval", user_id=user_id, role=role)
    turn = await store.begin_turn(user_id=user_id, conversation_id=None, client_message_id=None, run_id=run_id, role=role, text=case.q, max_history=10)
    started = time.monotonic()
    evs = [e async for e in run_turn(rt, store, persona, ctx, turn, case.q)]
    answer = "".join(e["data"].get("text", "") for e in evs if e["event"] == events.TOKEN)
    proposals = [_action_name(snapshot, role, e["data"].get("request") or {}) for e in evs if e["event"] == events.ACTION_PROPOSAL]
    error = next((e["data"].get("code") for e in evs if e["event"] == events.ERROR), None)
    result = evalkit.score(case, list(calls), answer, proposals, today)
    if error:
        result = evalkit.Result(False, [*result.reasons, f"lỗi runtime: {error}"])
    return {"id": case.id, "g": case.g, "q": case.q, "ok": result.ok, "reasons": result.reasons, "calls": list(calls),
            "proposals": proposals, "answer": answer, "seconds": round(time.monotonic() - started, 1)}


async def main_live(args) -> int:
    from ai_agent.config import load_config
    from ai_agent.personas import load_personas
    from ai_agent.runtime.agent_factory import AgentRuntime

    cfg = load_config(args.env, HERE.parent / "configs")
    cfg._data["store"]["backend"] = "memory"
    # trần theo lượt của config vẫn áp dụng; bỏ rate/daily vì chạy trong tiến trình, không qua gateway
    snapshot = evalkit.load_snapshot()
    personas = load_personas()
    today = (datetime.now(timezone.utc) + timedelta(hours=7)).date()
    names = [args.persona] if args.persona else list(evalkit.PERSONA_TOOL_ROLE)
    report: list[dict[str, Any]] = []
    for name in names:
        cases = [c for c in evalkit.load_cases(name) if not args.group or c.g == args.group]
        if args.ids:
            cases = [c for c in cases if c.id in args.ids]
        if args.only_failed:
            failed = {r["id"] for f in args.only_failed for r in json.loads(Path(f).read_text(encoding="utf-8")) if not r["ok"]}
            cases = [c for c in cases if c.id in failed]
        if args.limit:
            cases = cases[: args.limit]
        calls: list[dict[str, Any]] = []
        state: dict[str, Any] = {}
        rt = AgentRuntime(cfg, stub_gateway(snapshot, calls, state))
        for i, case in enumerate(cases, start=1):
            try:
                row = await run_case(rt, calls, personas[name], case, today, snapshot, state=state)
            except Exception as exc:  # noqa: BLE001
                row = {"id": case.id, "g": case.g, "q": case.q, "ok": False, "reasons": [f"ngoại lệ: {type(exc).__name__}"], "calls": [], "proposals": [], "answer": ""}
            report.append(row)
            print(f"{'PASS' if row['ok'] else 'FAIL'} {case.id} [{case.g}] {case.q[:60]}" + ("" if row["ok"] else f"  <- {'; '.join(row['reasons'])[:140]}"), flush=True)
    summarize(report)
    if args.out:
        Path(args.out).write_text(json.dumps(report, ensure_ascii=False, indent=1), encoding="utf-8")
        print(f"đã ghi {args.out}")
    return 0 if all(r["ok"] for r in report) else 1


def summarize(report: list[dict[str, Any]]) -> None:
    by: dict[tuple[str, str], Counter] = {}
    for r in report:
        persona = r["id"].split("-")[0]
        by.setdefault((persona, r["g"]), Counter())["ok" if r["ok"] else "fail"] += 1
    print("\npersona   nhóm         đạt/tổng")
    for (persona, g), c in sorted(by.items()):
        total = c["ok"] + c["fail"]
        print(f"{persona:9} {g:12} {c['ok']}/{total}  ({100 * c['ok'] // total}%)")
    total = len(report)
    ok = sum(1 for r in report if r["ok"])
    print(f"\ntổng: {ok}/{total} đạt")


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--live", action="store_true", help="gọi mô hình thật (tốn tiền); không có cờ này chỉ in kế hoạch")
    ap.add_argument("--persona", choices=list(evalkit.PERSONA_TOOL_ROLE))
    ap.add_argument("--group", choices=evalkit.GROUPS)
    ap.add_argument("--limit", type=int, default=0, help="chỉ chạy N câu đầu mỗi persona")
    ap.add_argument("--env", default="dev", help="config.<env>.yaml dùng cho llm.roles")
    ap.add_argument("--out", help="ghi báo cáo JSON")
    ap.add_argument("--ids", nargs="+", help="chỉ chạy các case có id này")
    ap.add_argument("--only-failed", nargs="+", metavar="REPORT.json", help="chỉ chạy lại các câu có ok=false trong các báo cáo JSON này")
    args = ap.parse_args()
    if not args.live:
        total = 0
        for name in evalkit.PERSONA_TOOL_ROLE:
            cases = evalkit.load_cases(name)
            total += len(cases)
            groups = Counter(c.g for c in cases)
            print(f"{name:9} {len(cases):3} câu  " + "  ".join(f"{g}={groups[g]}" for g in evalkit.GROUPS))
        print(f"tổng {total} câu. Thêm --live để gọi mô hình thật (chưa gọi gì).")
        return 0
    return asyncio.run(main_live(args))


if __name__ == "__main__":
    raise SystemExit(main())
