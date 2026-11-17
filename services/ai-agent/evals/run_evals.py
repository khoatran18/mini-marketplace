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
import tracekit  # noqa: E402

READ_ONLY = evalkit.READ_ONLY
TOKEN_ROLE = {"buyer": "buyer", "seller": "seller_admin", "admin": "admin", "operator": "operator"}


def _template_regex(path: str) -> re.Pattern[str]:
    return re.compile("^" + re.sub(r":\w+", r"([^/]+)", path) + "$")


# Trạng thái đơn mà gateway giả trả cho từng hành động đề xuất: đúng điều kiện tiên quyết thì agent mới có lý do đề xuất.
PRECONDITION_STATUS = {"cancel_order": "PAID", "request_return": "DELIVERED", "approve_return": "REFUND_REQUESTED", "reject_return": "REFUND_REQUESTED"}


_SERVICES = ["analytics-service", "api-gateway", "auth-service", "order-service", "ops-service", "payment-service", "product-service", "user-service"]
_ACTIONS = [
    {"id": "requeue_outbox", "title": "Gửi lại các dòng outbox", "risk": "high", "mode": "http", "params": [
        {"name": "service", "type": "enum", "required": True, "enum": ["order-service", "product-service", "payment-service"]}, {"name": "limit", "type": "int", "min": 1, "max": 100}]},
    {"id": "restart_service", "title": "Khởi động lại một service", "risk": "low", "mode": "command", "params": [{"name": "service", "type": "enum", "required": True, "enum": _SERVICES}]},
    {"id": "scale_service", "title": "Đổi số replica", "risk": "low", "mode": "command", "params": [
        {"name": "service", "type": "enum", "required": True, "enum": _SERVICES}, {"name": "replicas", "type": "int", "required": True, "min": 1, "max": 5}]},
    {"id": "escalate_to_human", "title": "Chuyển cho người", "risk": "observe", "mode": "manual", "params": []},
]
_INCIDENT = {"id": 4, "alert_name": "OutboxDeadRows", "severity": "warning", "component": "order-service", "state": "open", "summary": "order-service có dòng outbox chết",
             "runbook": "§12", "suggested_action": "requeue_outbox", "opened_at": "2026-11-17T08:00:00Z"}
_PROPOSAL = {"id": 1, "incident_id": 4, "action_id": "requeue_outbox", "params": {"service": "order-service"}, "rationale": "dòng chết sau khi Kafka phục hồi", "risk": "high",
             "state": "pending", "created_by": "operator", "created_at": "2026-11-17T08:05:00Z"}


def domain_body(tool: str, args: dict[str, Any]) -> dict[str, Any] | None:
    """Dữ liệu đúng HÌNH DẠNG thật cho tool ops/admin (gateway giả trước đây trả bản ghi kiểu đơn hàng cho mọi thứ, làm model operator
    nhận ra dữ liệu sai và gọi thêm nhiều lần, méo phép đo). None = dùng dạng chung (đơn/sản phẩm)."""
    rid = int(args["id"]) if str(args.get("id", "")).isdigit() else 4
    if tool == "ops_list_incidents":
        return {"items": [_INCIDENT], "total": 1}
    if tool == "ops_get_incident":
        return {**_INCIDENT, "id": rid}
    if tool == "ops_list_audit":
        return {"items": [{"id": 1, "subject_type": "incident", "subject_id": "4", "action": "opened", "actor": "system", "at": "2026-11-17T08:00:00Z"}], "total": 1}
    if tool == "ops_outbox_dead":
        return {"service": args.get("service", "order-service"), "items": [{"id": 10, "topic": "order.created", "attempts": 5, "last_error": "kafka timeout"}], "total": 1}
    if tool == "ops_list_actions":
        return {"actions": _ACTIONS}
    if tool == "ops_list_proposals":
        return {"items": [_PROPOSAL], "total": 1}
    if tool == "ops_get_proposal":
        return {**_PROPOSAL, "id": rid}
    if tool == "ops_metrics_query":
        name = str(args.get("name", ""))
        if name.startswith("logs_"):
            return {"query": name, "params": {k: str(v) for k, v in args.items()}, "backend": "loki", "total": 1, "truncated": False, "untrusted": True,
                    "lines": [{"ts": "2026-11-17T08:01:00Z", "line": "level=error msg=\"kafka publish timeout\""}]}
        return {"query": name, "params": {k: str(v) for k, v in args.items()}, "backend": "prometheus", "expr": name, "total": 1, "truncated": False,
                "series": [{"labels": {"service": args.get("service", "order-service")}, "value": "1"}]}
    if tool == "ops_list_playbooks":
        return {"items": [{"id": "outbox-stuck", "title": "Outbox bị kẹt", "alerts": ["OutboxDeadRows"]}], "total": 1}
    if tool == "ops_get_playbook":
        return {"id": args.get("id", "outbox-stuck"), "title": "Outbox bị kẹt", "diagnose": [{"id": "outbox-pending", "query": "outbox_pending", "read_only": True}],
                "causes": [{"id": "kafka-down", "when": "outbox_pending tăng và kafka_consumer_lag cao", "fix": ["requeue_outbox"]}], "actions": [a for a in _ACTIONS[:1]]}
    if tool == "ops_system_health":
        return {"as_of": "2026-11-17T08:00:00Z", "status": "ready", "services": [{"name": n, "status": "ready", "latency_ms": 4} for n in _SERVICES]}
    if tool == "data_health":
        return {"lag_seconds": 3, "reconcile": "ok", "as_of": "2026-11-17T08:00:00Z"}
    if tool in ("admin_list_payments", "admin_get_payment"):
        row = {"id": rid if "id" in args else 55, "order_id": 1, "status": "PAID", "amount": 250000, "method": "mockpay", "created_at": "2026-11-16T10:00:00Z"}
        return {"items": [row], "total": 1, "page": 1} if tool == "admin_list_payments" else row
    if tool == "admin_analytics_summary":
        return {"revenue": 15000000, "orders": 42, "average_order_value": 357142, "currency": "VND"}
    if tool.startswith("admin_analytics_"):
        return {"items": [{"label": "mẫu", "value": 12}], "total": 1}
    return None


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
                special = domain_body(t["name"], args)
                if special is not None:
                    return httpx.Response(200, json=special)
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
    m0 = tracekit.metrics_snapshot()
    with tracekit.traced() as tracer:
        evs = [e async for e in run_turn(rt, store, persona, ctx, turn, case.q)]
    trace = tracer.summary()
    trace["metrics_delta"] = tracekit.metrics_delta(m0, tracekit.metrics_snapshot())
    trace["turn"] = {k: store.runs.get(run_id, {}).get(k) for k in ("status", "input_tokens", "output_tokens", "tool_calls")}
    trace["events"] = [e["event"] for e in evs]
    answer = "".join(e["data"].get("text", "") for e in evs if e["event"] == events.TOKEN)
    proposals = [_action_name(snapshot, role, e["data"].get("request") or {}) for e in evs if e["event"] == events.ACTION_PROPOSAL]
    error = next((e["data"].get("code") for e in evs if e["event"] == events.ERROR), None)
    result = evalkit.score(case, list(calls), answer, proposals, today)
    if error:
        result = evalkit.Result(False, [*result.reasons, f"lỗi runtime: {error}"])
    return {"id": case.id, "g": case.g, "q": case.q, "ok": result.ok, "reasons": result.reasons, "calls": list(calls),
            "proposals": proposals, "answer": answer, "seconds": round(time.monotonic() - started, 1), "trace": trace, "systems": tracer.systems}


async def main_live(args) -> int:
    import platform

    loaded = evalkit.load_env()  # khoá từ deploy/.env, chỉ biết TÊN biến đã nạp
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

    plan: dict[str, list[evalkit.Case]] = {}
    for name in names:
        cases = [c for c in evalkit.select_split(evalkit.load_cases(name), args.split) if not args.group or c.g == args.group]
        if args.ids:
            cases = [c for c in cases if c.id in args.ids]
        if args.only_failed:
            failed = {r["id"] for f in args.only_failed for r in evalkit.read_rows(f) if not r["ok"]}
            cases = [c for c in cases if c.id in failed]
        if args.limit:
            cases = cases[: args.limit]
        plan[name] = cases

    label = "-".join(x for x in ("live", args.split, args.group or "all", args.persona) if x)
    log = tracekit.RunLog(Path(args.log_dir) if args.log_dir else HERE / "results" / "runs", label)
    import importlib.metadata as md
    log.write_meta({
        **evalkit.fingerprint(cfg._data["llm"]),
        "run": {"argv": sys.argv[1:], "split": args.split, "group": args.group, "persona": args.persona, "limit": args.limit, "ids": args.ids, "env": args.env,
                "started_at": datetime.now(timezone.utc).isoformat(timespec="seconds"), "cases_planned": {k: len(v) for k, v in plan.items()}},
        "env_vars_loaded_from_dotenv": loaded,  # chỉ tên
        "config_masked": {"limits": cfg.to_dict(masked=True).get("limits"), "llm": cfg.to_dict(masked=True).get("llm")},
        "versions": {"python": platform.python_version(), **{p: md.version(p) for p in ("langchain", "langchain-core", "langgraph", "openai") if _has(p)}},
    })
    print(f"ghi log vào {log.dir}", flush=True)
    report: list[dict[str, Any]] = []
    try:
        for name, cases in plan.items():
            calls: list[dict[str, Any]] = []
            state: dict[str, Any] = {}
            rt = AgentRuntime(cfg, stub_gateway(snapshot, calls, state))
            for case in cases:
                try:
                    row = await run_case(rt, calls, personas[name], case, today, snapshot, state=state)
                except Exception as exc:  # noqa: BLE001
                    row = {"id": case.id, "g": case.g, "q": case.q, "ok": False, "reasons": [f"ngoại lệ: {type(exc).__name__}: {str(exc)[:200]}"], "calls": [], "proposals": [],
                           "answer": "", "seconds": 0, "trace": None, "systems": {}}
                log.add_systems(name, row.pop("systems", {}))
                row["split"] = evalkit.split_of(case.q)
                row["expect"] = {"tool": case.tool, "args": case.args, "max": case.max, "propose": case.propose, "deny": case.deny, "need": case.need}
                log.append(row)
                report.append(row)
                t = (row.get("trace") or {}).get("totals") or {}
                print(f"{'PASS' if row['ok'] else 'FAIL'} {case.id} [{case.g}] {case.q[:50]}  | llm={t.get('model_calls', '?')} tool={t.get('tool_calls', '?')} "
                      f"tok={t.get('input_tokens', '?')}+{t.get('output_tokens', '?')} {row['seconds']}s" + ("" if row["ok"] else f"  <- {'; '.join(row['reasons'])[:140]}"), flush=True)
    finally:
        summarize(report)
        summ = tracekit.summarize(report)
        print(f"model_calls tổng {summ['model_calls']['sum']:.0f} (TB {summ['model_calls']['mean']}/ca, lớn nhất {summ['model_calls']['max']}); tool_calls tổng {summ['tool_calls']['sum']:.0f}; "
              f"token vào {summ['input_tokens']['sum']:.0f} + ra {summ['output_tokens']['sum']:.0f} = {summ['total_tokens']['sum']:.0f}")
        log.finish(summ)
        print(f"đã ghi {log.dir}/(meta.json, cases.jsonl, summary.json, run.log)", file=sys.__stdout__)
    if args.out:
        lean = [{k: v for k, v in r.items() if k not in ("trace", "systems")} for r in report]
        meta = {**evalkit.fingerprint(cfg._data["llm"]), "run": {"split": args.split, "group": args.group, "persona": args.persona, "limit": args.limit,
                "ids": args.ids, "at": datetime.now(timezone.utc).isoformat(timespec="seconds"), "log_dir": str(log.dir)}}
        evalkit.write_report(args.out, lean, meta)
        print(f"đã ghi {args.out}")
    return 0 if all(r["ok"] for r in report) else 1


def _has(pkg: str) -> bool:
    import importlib.metadata as md
    try:
        md.version(pkg)
        return True
    except md.PackageNotFoundError:
        return False


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
    ap.add_argument("--split", choices=("dev", "heldout", "all"), default="all", help="dev = chỉnh prompt trên đây; heldout = tập giữ kín (1/5 số câu, theo băm câu hỏi), chỉ chạy khi chốt phiên bản")
    ap.add_argument("--limit", type=int, default=0, help="chỉ chạy N câu đầu mỗi persona")
    ap.add_argument("--env", default="dev", help="config.<env>.yaml dùng cho llm.roles")
    ap.add_argument("--out", help="ghi báo cáo JSON gọn (không có trace), dùng cho --only-failed")
    ap.add_argument("--log-dir", help="thư mục gốc cho log đầy đủ của lần chạy live (mặc định evals/results/runs/); luôn bật khi --live")
    ap.add_argument("--ids", nargs="+", help="chỉ chạy các case có id này")
    ap.add_argument("--only-failed", nargs="+", metavar="REPORT.json", help="chỉ chạy lại các câu có ok=false trong các báo cáo JSON này")
    args = ap.parse_args()
    if not args.live:
        total = 0
        for name in evalkit.PERSONA_TOOL_ROLE:
            cases = evalkit.select_split(evalkit.load_cases(name), args.split)
            total += len(cases)
            groups = Counter(c.g for c in cases)
            print(f"{name:9} {len(cases):3} câu  " + "  ".join(f"{g}={groups[g]}" for g in evalkit.GROUPS))
        print(f"tổng {total} câu. Thêm --live để gọi mô hình thật (chưa gọi gì).")
        return 0
    return asyncio.run(main_live(args))


if __name__ == "__main__":
    raise SystemExit(main())
