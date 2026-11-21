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
_ACTION_SERVICES = [n for n in _SERVICES if n != "ops-service"]  # đúng ops-service/internal/actions/catalog.go: statelessServices không có ops-service (nó là nơi duyệt/ghi nhận, không tự restart mình)
_ACTIONS = [
    {"id": "requeue_outbox", "title": "Gửi lại các dòng outbox", "risk": "high", "mode": "http", "params": [
        {"name": "service", "type": "enum", "required": True, "enum": ["order-service", "product-service", "payment-service"]}, {"name": "limit", "type": "int", "min": 1, "max": 100}]},
    {"id": "restart_service", "title": "Khởi động lại một service", "risk": "low", "mode": "command", "params": [{"name": "service", "type": "enum", "required": True, "enum": _ACTION_SERVICES}]},
    {"id": "scale_service", "title": "Đổi số replica", "risk": "low", "mode": "command", "params": [
        {"name": "service", "type": "enum", "required": True, "enum": _ACTION_SERVICES}, {"name": "replicas", "type": "int", "required": True, "min": 1, "max": 5}]},
    {"id": "escalate_to_human", "title": "Chuyển cho người", "risk": "observe", "mode": "manual", "params": []},
]
_INCIDENT = {"id": 4, "alert_name": "OutboxDeadRows", "severity": "warning", "component": "order-service", "state": "open", "summary": "order-service có dòng outbox chết",
             "runbook": "§12", "suggested_action": "requeue_outbox", "opened_at": "2026-11-17T08:00:00Z"}
_MORE_INCIDENTS = [
    {**_INCIDENT, "id": 5, "alert_name": "HighErrorRate", "severity": "warning", "component": "api-gateway", "summary": "api-gateway tỉ lệ lỗi 5xx cao", "runbook": "§3", "suggested_action": "escalate_to_human", "opened_at": "2026-11-17T09:10:00Z"},
    {**_INCIDENT, "id": 6, "alert_name": "KafkaConsumerLag", "severity": "critical", "component": "product-service", "summary": "product-service consumer lag cao", "runbook": "§7", "suggested_action": "restart_service", "opened_at": "2026-11-17T10:30:00Z"},
]
_PROPOSAL = {"id": 1, "incident_id": 4, "action_id": "requeue_outbox", "params": {"service": "order-service"}, "rationale": "dòng chết sau khi Kafka phục hồi", "risk": "high",
             "state": "pending", "created_by": "operator", "created_at": "2026-11-17T08:05:00Z"}


_MORE_PROPOSALS = [
    {**_PROPOSAL, "id": 2, "incident_id": 5, "action_id": "scale_service", "params": {"service": "api-gateway", "replicas": 3}, "rationale": "tỉ lệ lỗi 5xx cao khi tải tăng", "risk": "low", "created_at": "2026-11-17T10:40:00Z"},
    {**_PROPOSAL, "id": 3, "incident_id": 5, "action_id": "escalate_to_human", "params": {}, "rationale": "cần người xem lỗi 5xx", "risk": "observe", "state": "executed", "created_at": "2026-11-17T09:20:00Z"},
]
_UP_METRICS = ("service_up", "service_ready")  # 0 khi service đang treo (đúng tên truy vấn trong bảng ops_metrics_query); replica vẫn 1/1 (xem _down_body)


def _down_body(tool: str, args: dict[str, Any], down: str) -> dict[str, Any] | None:
    """Dữ liệu giả khi MỘT service đang treo: process còn nhưng không trả lời (`service_up` 0, health không sẵn sàng, sự cố ServiceDown đang mở,
    playbook service-unavailable nói restart). Chỉ đổi phần liên quan tới service đó; phần còn lại dùng dạng chung. None = không đổi."""
    incident = {"id": 4, "alert_name": "ServiceDown", "severity": "critical", "component": down, "state": "open", "summary": f"{down} không trả lời",
                "runbook": "§1", "suggested_action": "restart_service", "opened_at": "2026-11-17T08:00:00Z"}
    rid = int(args["id"]) if str(args.get("id", "")).isdigit() else 4
    if tool == "ops_list_incidents":
        return {"items": [incident], "total": 1}
    if tool == "ops_get_incident":
        return {**incident, "id": rid}
    if tool == "ops_system_health":
        return {"as_of": "2026-11-17T08:00:00Z", "status": "degraded",
                "services": [{"name": n, "status": "not_ready" if n == down else "ready", "latency_ms": None if n == down else 4} for n in _SERVICES]}
    if tool == "ops_metrics_query":
        name = str(args.get("name", ""))
        if name == "alerts_firing":
            return {"query": name, "params": {}, "backend": "prometheus", "expr": name, "total": 1, "truncated": False,
                    "series": [{"labels": {"alertname": "ServiceDown", "service": down}, "value": "1"}]}
        if (name in _UP_METRICS or name == "service_replicas_up") and args.get("service", down) == down:
            # process còn nhưng không trả lời: up/ready = 0, replica vẫn 1/1 (đúng điều kiện `process-hung` của playbook service-unavailable)
            return {"query": name, "params": {k: str(v) for k, v in args.items()}, "backend": "prometheus", "expr": name, "total": 1, "truncated": False,
                    "series": [{"labels": {"service": down}, "value": "1" if name == "service_replicas_up" else "0"}]}
    if tool == "ops_list_playbooks":
        return {"items": [{"id": "service-unavailable", "title": "Service không sẵn sàng", "alerts": ["ServiceDown"]}], "total": 1}
    if tool == "ops_get_playbook":
        return {"id": "service-unavailable", "title": "Service không sẵn sàng", "diagnose": [{"id": "ready-checks", "query": "service_up", "read_only": True}],
                "causes": [{"id": "process-hung", "when": "service_up bằng 0 hoặc service_ready false trong khi replica vẫn 1/1", "fix": ["restart_service"]}],
                "actions": [a for a in _ACTIONS if a["id"] == "restart_service"]}
    return None


def domain_body(tool: str, args: dict[str, Any], down: str | None = None) -> dict[str, Any] | None:
    """Dữ liệu đúng HÌNH DẠNG thật cho tool ops/admin (gateway giả trước đây trả bản ghi kiểu đơn hàng cho mọi thứ, làm model operator
    nhận ra dữ liệu sai và gọi thêm nhiều lần, méo phép đo). None = dùng dạng chung (đơn/sản phẩm)."""
    if down:
        hit = _down_body(tool, args, down)
        if hit is not None:
            return hit
    rid = int(args["id"]) if str(args.get("id", "")).isdigit() else 4
    if tool == "ops_list_incidents":
        return {"items": [_INCIDENT, *_MORE_INCIDENTS], "total": 3}
    if tool == "ops_get_incident":
        return {**next((i for i in [_INCIDENT, *_MORE_INCIDENTS] if i["id"] == rid), _INCIDENT), "id": rid}
    if tool == "ops_list_audit":
        return {"items": [{"id": 1, "subject_type": "incident", "subject_id": "4", "action": "opened", "actor": "system", "at": "2026-11-17T08:00:00Z"},
                          {"id": 2, "subject_type": "incident", "subject_id": "5", "action": "opened", "actor": "system", "at": "2026-11-17T09:10:00Z"},
                          {"id": 3, "subject_type": "proposal", "subject_id": "2", "action": "created", "actor": "operator", "at": "2026-11-17T10:40:00Z"}], "total": 3}
    if tool == "ops_outbox_dead":
        return {"service": args.get("service", "order-service"), "items": [{"id": 10, "topic": "order.created", "attempts": 5, "last_error": "kafka timeout"},
                {"id": 11, "topic": "order.paid", "attempts": 5, "last_error": "kafka timeout"}, {"id": 12, "topic": "order.created", "attempts": 3, "last_error": "broker not available"}], "total": 3}
    if tool == "ops_list_actions":
        return {"actions": _ACTIONS}
    if tool == "ops_list_proposals":
        return {"items": [_PROPOSAL, *_MORE_PROPOSALS], "total": 3}
    if tool == "ops_get_proposal":
        return {**next((p for p in [_PROPOSAL, *_MORE_PROPOSALS] if p["id"] == rid), _PROPOSAL), "id": rid}
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
                special = domain_body(t["name"], args, state.get("down"))
                if special is not None:
                    return httpx.Response(200, json=special)
                row = {"id": 1, "status": status, "total": 250000, "name": "Mẫu", "state": "ok", "ok": True, "delivered_at": state.get("delivered_at")}
                if "product" in t["path"] or t["name"] in ("search_products", "list_low_stock"):  # đúng dạng sản phẩm, không phải bản ghi đơn
                    row = {"id": 1, "name": "Sản phẩm mẫu", "brand": "Mẫu", "category_id": 1, "price": 150000, "status": "ACTIVE", "inventory": 42, "available": 42, "low_stock_threshold": 5}
                if "id" in args:
                    body = {**row, "id": int(args["id"]) if str(args["id"]).isdigit() else args["id"], "items": [row]}
                elif "product" in t["path"] or t["name"] in ("search_products", "list_low_stock"):
                    body = {"items": [row], "total": 1, "page": 1}  # seller chỉ có sản phẩm 1 (ca history nói vậy)
                else:  # danh sách đơn/thanh toán/...: nhiều dòng cùng trạng thái, id khác nhau, để "đơn đó" không tự suy ra được
                    rows = [row, {**row, "id": 2, "total": 480000}, {**row, "id": 3, "total": 90000}]
                    body = {"items": rows, "total": 3, "page": 1}
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


def world_state(case: evalkit.Case, now: datetime) -> dict[str, Any]:
    """Trạng thái đơn mà gateway giả trả cho một ca. Ca huỷ đơn (PAID) KHÔNG có `delivered_at`: trước đây mọi ca đều có ngày giao hôm qua nên model
    thấy đơn "đã giao" và từ chối huỷ (đúng quy tắc), làm sai phép đo của các ca huỷ đơn. Ca khác giữ nguyên để số đo cũ còn so được."""
    return {"status": PRECONDITION_STATUS.get(case.propose or "", "PAID"),
            "delivered_at": None if case.propose == "cancel_order" else (now - timedelta(days=1)).replace(hour=0, minute=0, second=0, microsecond=0).isoformat(),
            "down": case.down}


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
        state.update(world_state(case, datetime.now(timezone.utc)))
    ctx = RunContext(token=agent_token(user_id, role, run_id, ttl=900), run_id=run_id, request_id="eval", user_id=user_id, role=role)
    turn = await store.begin_turn(user_id=user_id, conversation_id=None, client_message_id=None, run_id=run_id, role=role, text=case.q, max_history=10)
    if case.history:  # ca nhiều lượt: nạp các tin cũ như thể đã trò chuyện trước đó (chạy như lượt thứ N của cuộc trò chuyện)
        turn.history = list(case.history)
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
    evalkit.setup_cache(args)
    from ai_agent.config import load_config
    from ai_agent.personas import load_personas
    from ai_agent.runtime.agent_factory import AgentRuntime

    cfg = load_config(args.env, HERE.parent / "configs")
    cfg._data["store"]["backend"] = "memory"
    if args.require_model:  # chốt chặn: không gọi nhầm mô hình khác (vd mô hình trả tiền) khi chạy thử
        used = {r: cfg._data["llm"]["profiles"][p].get("model_id") for r, p in cfg._data["llm"]["roles"].items()}
        wrong = {r: m for r, m in used.items() if m != args.require_model}
        if wrong:
            print(f"DỪNG: --require-model {args.require_model} nhưng role dùng mô hình khác: {wrong}. Chưa gọi gì.", file=sys.stderr)
            return 2
    # trần theo lượt của config vẫn áp dụng; bỏ rate/daily vì chạy trong tiến trình, không qua gateway
    snapshot = evalkit.load_snapshot()
    personas = load_personas()
    today = (datetime.now(timezone.utc) + timedelta(hours=7)).date()
    names = [args.persona] if args.persona else list(evalkit.PERSONA_TOOL_ROLE)

    plan: dict[str, list[evalkit.Case]] = {}
    for name in names:
        cases = [c for c in evalkit.select_split(evalkit.load_cases(name), args.split) if not args.group or c.g == args.group]
        if args.source != "all":  # hand = viết tay; hy4 = do hy4-flash-free sinh (tránh thiên lệch khi so với model khác)
            cases = [c for c in cases if c.generated == (args.source == "hy4")]
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
        "run": {"argv": sys.argv[1:], "split": args.split, "group": args.group, "persona": args.persona, "limit": args.limit, "ids": args.ids, "env": args.env, "concurrency": args.concurrency, "repeat": args.repeat, "require_model": args.require_model,
                "started_at": datetime.now(timezone.utc).isoformat(timespec="seconds"), "cases_planned": {k: len(v) for k, v in plan.items()}},
        "env_vars_loaded_from_dotenv": loaded,  # chỉ tên
        "config_masked": {"limits": cfg.to_dict(masked=True).get("limits"), "llm": cfg.to_dict(masked=True).get("llm")},
        "versions": {"python": platform.python_version(), **{p: md.version(p) for p in ("langchain", "langchain-core", "langgraph", "openai") if _has(p)}},
    })
    print(f"ghi log vào {log.dir}", flush=True)
    report: list[dict[str, Any]] = []
    # Hàng đợi chung (persona, câu) cho `--concurrency` luồng. Mỗi luồng giữ RIÊNG một AgentRuntime + gateway giả + `calls`/`state` cho mỗi persona nó gặp
    # (gateway giả ghi `calls` và `state` theo từng câu nên không dùng chung được). Một luồng = chạy tuần tự như trước.
    queue: asyncio.Queue[tuple[str, evalkit.Case, int]] = asyncio.Queue()
    for rep in range(max(1, args.repeat)):  # mỗi lượt lặp là một vòng đủ các câu (câu cùng lượt nằm gần nhau trong hàng đợi)
        for name, cases in plan.items():
            for case in cases:
                queue.put_nowait((name, case, rep))

    async def worker() -> None:
        own: dict[str, tuple[Any, list[dict[str, Any]], dict[str, Any]]] = {}
        while True:
            try:
                name, case, rep = queue.get_nowait()
            except asyncio.QueueEmpty:
                return
            if name not in own:
                calls: list[dict[str, Any]] = []
                state: dict[str, Any] = {}
                own[name] = (AgentRuntime(cfg, stub_gateway(snapshot, calls, state)), calls, state)
            rt, calls, state = own[name]
            try:
                row = await run_case(rt, calls, personas[name], case, today, snapshot, state=state)
            except Exception as exc:  # noqa: BLE001
                row = {"id": case.id, "g": case.g, "q": case.q, "ok": False, "reasons": [f"ngoại lệ: {type(exc).__name__}: {str(exc)[:200]}"], "calls": [], "proposals": [],
                       "answer": "", "seconds": 0, "trace": None, "systems": {}}
            log.add_systems(name, row.pop("systems", {}))
            row["split"] = evalkit.split_of(case.q)
            row["rep"] = rep
            row["expect"] = {"tool": case.tool, "args": case.args, "max": case.max, "propose": case.propose, "deny": case.deny, "need": case.need}
            log.append(row)
            report.append(row)
            t = (row.get("trace") or {}).get("totals") or {}
            print(f"{'PASS' if row['ok'] else 'FAIL'} {case.id} [{case.g}] {case.q[:50]}  | llm={t.get('model_calls', '?')} tool={t.get('tool_calls', '?')} "
                  f"tok={t.get('input_tokens', '?')}+{t.get('output_tokens', '?')} {row['seconds']}s" + ("" if row["ok"] else f"  <- {'; '.join(row['reasons'])[:140]}"), flush=True)

    try:
        await asyncio.gather(*[worker() for _ in range(max(1, min(args.concurrency, queue.qsize() or 1)))])
    finally:
        summarize(report)
        if args.repeat > 1:
            summarize_repeat(report, args.repeat)
        summ = tracekit.summarize(report)
        print(f"model_calls tổng {summ['model_calls']['sum']:.0f} (TB {summ['model_calls']['mean']}/ca, lớn nhất {summ['model_calls']['max']}); tool_calls tổng {summ['tool_calls']['sum']:.0f}; "
              f"token vào {summ['input_tokens']['sum']:.0f} + ra {summ['output_tokens']['sum']:.0f} = {summ['total_tokens']['sum']:.0f}")
        log.finish(summ)
        if evalkit.cache_summary():
            print(evalkit.cache_summary(), flush=True)
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


def wilson(k: int, n: int, z: float = 1.96) -> tuple[float, float]:
    """Khoảng tin cậy Wilson 95% cho tỉ lệ k/n."""
    if n == 0:
        return (0.0, 0.0)
    p = k / n
    d = 1 + z * z / n
    c = (p + z * z / (2 * n)) / d
    h = z * ((p * (1 - p) / n + z * z / (4 * n * n)) ** 0.5) / d
    return (max(0.0, c - h), min(1.0, c + h))


def summarize_repeat(report: list[dict[str, Any]], k: int) -> None:
    """Chạy lặp: tỉ lệ đạt trung bình theo lượt, độ lệch giữa các lượt, và các câu lúc đạt lúc trượt (không ổn định)."""
    per_rep: dict[int, list[bool]] = {}
    per_case: dict[str, list[bool]] = {}
    for r in report:
        per_rep.setdefault(r.get("rep", 0), []).append(bool(r["ok"]))
        per_case.setdefault(r["id"], []).append(bool(r["ok"]))
    rates = [sum(v) / len(v) for _, v in sorted(per_rep.items())]
    mean = sum(rates) / len(rates)
    sd = (sum((x - mean) ** 2 for x in rates) / max(1, len(rates) - 1)) ** 0.5
    ok = sum(sum(v) for v in per_case.values())
    n = sum(len(v) for v in per_case.values())
    lo, hi = wilson(ok, n)
    print(f"\nlặp {k} lượt: đạt theo lượt {[f'{100 * x:.1f}%' for x in rates]}  TB {100 * mean:.1f}%  độ lệch chuẩn {100 * sd:.1f} điểm %")
    print(f"gộp {ok}/{n} = {100 * ok / n:.1f}%  khoảng tin cậy 95% (Wilson) [{100 * lo:.1f}%, {100 * hi:.1f}%]")
    always = sum(1 for v in per_case.values() if all(v))
    never = sum(1 for v in per_case.values() if not any(v))
    flaky = {i: v for i, v in per_case.items() if any(v) and not all(v)}
    print(f"{len(per_case)} câu: luôn đạt {always}, luôn trượt {never}, lúc đạt lúc trượt {len(flaky)}")
    for i, v in sorted(flaky.items())[:40]:
        print(f"  {i}: {sum(v)}/{len(v)}")


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
    ap.add_argument("--concurrency", type=int, default=1, help="số câu chạy song song (mặc định 1 = tuần tự như cũ). Mỗi luồng có gateway giả riêng. Khi > 1 thì `metrics_delta` trong trace không đáng tin (bộ đếm Prometheus dùng chung); các số đo theo callback (token, số lần gọi) vẫn đúng. Nhà cung cấp có thể trả 429")
    ap.add_argument("--repeat", type=int, default=1, help="chạy mỗi câu k lượt rồi báo trung bình, độ lệch, khoảng tin cậy Wilson và các câu không ổn định (mô hình suy luận lệch 3-4 câu/37 giữa hai lần chạy). Tốn k lần số lệnh gọi")
    ap.add_argument("--require-model", metavar="MODEL_ID", help="từ chối chạy nếu có role dùng mô hình khác MODEL_ID (chốt chặn tránh gọi nhầm mô hình trả tiền)")
    ap.add_argument("--source", choices=("all", "hand", "hy4"), default="all", help="hand = ca viết tay; hy4 = ca do hy4-flash-free sinh (lọc tay). Báo cáo riêng khi so với model khác để tránh thiên lệch (hy4 vừa sinh ca vừa bị đo)")
    evalkit.add_cache_args(ap)
    ap.add_argument("--ids", nargs="+", help="chỉ chạy các case có id này")
    ap.add_argument("--only-failed", nargs="+", metavar="REPORT.json", help="chỉ chạy lại các câu có ok=false trong các báo cáo JSON này")
    args = ap.parse_args()
    if not args.live:
        total = 0
        for name in evalkit.PERSONA_TOOL_ROLE:
            cases = evalkit.select_split(evalkit.load_cases(name), args.split)
            if args.source != "all":
                cases = [c for c in cases if c.generated == (args.source == "hy4")]
            total += len(cases)
            groups = Counter(c.g for c in cases)
            print(f"{name:9} {len(cases):3} câu  " + "  ".join(f"{g}={groups[g]}" for g in evalkit.GROUPS))
        print(f"tổng {total} câu. Thêm --live để gọi mô hình thật (chưa gọi gì).")
        return 0
    return asyncio.run(main_live(args))


if __name__ == "__main__":
    raise SystemExit(main())
