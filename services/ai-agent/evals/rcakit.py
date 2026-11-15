"""Bộ eval RCA: đọc case, kiểm tĩnh theo playbook/ground truth/sổ hành động, gateway giả, chạy đồ thị với một Brain, chấm. KHÔNG gọi mô hình
(chỉ `run_rca_evals.py --live` làm việc đó). Đáp án đúng lấy từ nguồn có sẵn: kịch bản `scripts/e2e/resilience.sh` (do chính script gây lỗi nên biết chắc),
`docs/playbooks` (nguyên nhân -> bước sửa) và sổ hành động của ops-service (bản sao `rca/actions.snapshot.json`, có test giữ cho khớp)."""
from __future__ import annotations

import json
import re
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

import httpx
import yaml

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[2]  # gốc repo
sys.path.insert(0, str(HERE.parent / "src"))
sys.path.insert(0, str(HERE.parent / "tests"))

from ai_agent.rca.budget import Budget  # noqa: E402
from ai_agent.rca.graph import build_graph  # noqa: E402
from ai_agent.rca.validate import check_params  # noqa: E402
from ai_agent.tools.gateway import GatewayClient  # noqa: E402

CASES = HERE / "rca" / "cases.yaml"
ACTIONS = HERE / "rca" / "actions.snapshot.json"
SNAPSHOT = HERE / "tools.snapshot.json"
PLAYBOOKS = ROOT / "docs" / "playbooks"
RESILIENCE = ROOT / "scripts" / "e2e" / "resilience.sh"
KNOWN = {"id", "scenario", "incident", "tools", "expect"}
EXPECT_KEYS = {"cause_any", "cause_all", "cause_example", "playbook", "action", "action_any", "forbid", "outcome", "max_model_calls"}
DEFAULT_SERVICES = ("api-gateway", "auth-service", "user-service", "product-service", "order-service", "payment-service", "analytics-service", "ai-agent", "ops-service")
DEFAULT_INFRA = ("postgres", "redis", "clickhouse", "kafka-broker1", "kafka-broker2", "kafka-broker3", "loki", "prometheus")


def load_cases(path: Path = CASES) -> list[dict[str, Any]]:
    return yaml.safe_load(path.read_text(encoding="utf-8")) or []


def load_catalog() -> list[dict[str, Any]]:
    return json.loads(ACTIONS.read_text(encoding="utf-8"))["actions"]


def load_playbooks() -> dict[str, dict[str, Any]]:
    return {d["id"]: d for d in (yaml.safe_load(p.read_text(encoding="utf-8")) for p in sorted(PLAYBOOKS.glob("*.yml")))}


def resilience_meta() -> dict[str, dict[str, Any]]:
    """Kịch bản A..M của resilience.sh -> {fault, playbooks, alerts, signals} (đọc từ hàm gt_meta)."""
    out = {}
    for m in re.finditer(r'^\s+([A-Z])\) echo "([^|"]*)\|([^|"]*)\|([^|"]*)\|([^"]*)"', RESILIENCE.read_text(encoding="utf-8"), flags=re.M):
        out[m.group(1)] = {"fault": m.group(2), "playbooks": [p for p in m.group(3).split(",") if p], "alerts": m.group(4), "signals": m.group(5)}
    return out


def _subset(want: dict[str, Any], have: dict[str, Any]) -> bool:
    return all(have.get(k) == v for k, v in want.items())


def validate_cases(cases: list[dict[str, Any]]) -> list[str]:
    """Danh sách lỗi của bộ case (rỗng = hợp lệ)."""
    bad: list[str] = []
    catalog = {a["id"]: a for a in load_catalog()}
    playbooks, meta = load_playbooks(), resilience_meta()
    tools = {t["name"]: t for t in json.loads(SNAPSHOT.read_text(encoding="utf-8"))["scopes"]["operator"]}
    seen_ids, seen_inc = set(), set()
    for c in cases:
        cid = c.get("id", "?")
        if set(c) - KNOWN or KNOWN - set(c):
            bad.append(f"{cid}: khoá không đúng: {sorted(set(c) ^ KNOWN)}")
            continue
        if cid in seen_ids or c["incident"].get("id") in seen_inc:
            bad.append(f"{cid}: trùng id hoặc id sự cố")
        seen_ids.add(cid), seen_inc.add(c["incident"].get("id"))
        e = c["expect"]
        if set(e) - EXPECT_KEYS:
            bad.append(f"{cid}: khoá expect lạ {sorted(set(e) - EXPECT_KEYS)}")
        if c["incident"].get("severity") not in ("critical", "warning") or not c["incident"].get("alert_name"):
            bad.append(f"{cid}: incident thiếu severity/alert_name")
        if c["scenario"] is not None:
            m = meta.get(c["scenario"])
            if not m:
                bad.append(f"{cid}: kịch bản {c['scenario']!r} không có trong resilience.sh")
            elif e.get("playbook") not in m["playbooks"]:
                bad.append(f"{cid}: playbook {e.get('playbook')!r} không nằm trong đáp án của kịch bản {c['scenario']} ({m['playbooks']})")
        if e.get("playbook") is not None and e["playbook"] not in playbooks:
            bad.append(f"{cid}: playbook {e['playbook']!r} không tồn tại")
        pb = playbooks.get(e.get("playbook") or "")
        if pb and c["incident"]["alert_name"] not in pb["triggers"]["alerts"]:
            bad.append(f"{cid}: alert {c['incident']['alert_name']} không kích hoạt playbook {e['playbook']}")
        for rx in (e.get("cause_any") or []) + (e.get("cause_all") or []):
            try:
                re.compile(rx)
            except re.error as ex:
                bad.append(f"{cid}: regex sai {rx!r}: {ex}")
        if bool(e.get("cause_any")) == bool(e.get("cause_all")):
            bad.append(f"{cid}: cần đúng một trong cause_any / cause_all")
        ex_text = e.get("cause_example", "")
        if not ex_text or not _cause_matches(e, [ex_text]):
            bad.append(f"{cid}: cause_example không khớp chính regex của nó")
        acts = [e["action"]] if e.get("action") else e.get("action_any") or []
        if not acts or (e.get("action") and e.get("action_any")):
            bad.append(f"{cid}: cần đúng một trong action / action_any")
        for a in acts + (e.get("forbid") or []):
            if a["id"] not in catalog:
                bad.append(f"{cid}: hành động {a['id']!r} không có trong sổ")
        for a in acts:
            if a["id"] in catalog and a["id"] != "escalate_to_human" and (why := check_params(catalog[a["id"]], a.get("params") or {})):
                bad.append(f"{cid}: tham số hành động sai: {why}")
        escal = all(a["id"] == "escalate_to_human" for a in acts)
        if e.get("outcome") != ("escalated" if escal else "recovered"):
            bad.append(f"{cid}: outcome {e.get('outcome')!r} không khớp hành động mong đợi")
        for t in c["tools"]:
            spec = tools.get(t["tool"])
            if spec is None:
                bad.append(f"{cid}: tool {t['tool']!r} không phải tool đọc của operator")
                continue
            names = {p["name"] for p in spec.get("params", [])}
            if set(t.get("match") or {}) - names:
                bad.append(f"{cid}: match {sorted(set(t['match']) - names)} không phải tham số của {t['tool']}")
    return bad


# ---------------- gateway giả ----------------
class World:
    """Trạng thái giả của hệ thống: phase before -> after khi hành động đã chạy. Ghi lại mọi lời gọi tool."""

    def __init__(self, case: dict[str, Any]) -> None:
        self.case, self.phase, self.calls = case, "before", []
        self.playbooks = load_playbooks()
        self.catalog = load_catalog()

    def pick(self, entry: dict[str, Any]) -> Any:
        return entry.get("after", entry["before"]) if self.phase == "after" else entry["before"]

    def health(self) -> dict[str, Any]:
        over: dict[str, str] = {}
        for e in self.case["tools"]:
            if e["tool"] == "ops_system_health":
                over = self.pick(e) or {}
        def row(n):
            s = over.get(n, "ok")
            return {"name": n, "state": s.split(":")[0], "detail": s if s != "ok" else ""}
        return {"services": [row(n) for n in DEFAULT_SERVICES if n in over or n in DEFAULT_SERVICES],
                "infra": [row(n) for n in DEFAULT_INFRA], "other": [{"name": n, "state": s.split(":")[0], "detail": s} for n, s in over.items() if n not in DEFAULT_SERVICES + DEFAULT_INFRA]}

    def metric(self, name: str, args: dict[str, Any]) -> dict[str, Any]:
        ents = [e for e in self.case["tools"] if e["tool"] == "ops_metrics_query" and (e.get("match") or {}).get("name") == name]
        exact = [e for e in ents if _subset(e["match"], {**args, "name": name})]
        if exact:
            use = [max(exact, key=lambda e: len(e["match"]))]
        elif ents and not args.get("service"):
            use = ents  # không lọc theo service: hiện mọi chuỗi đã có
        else:
            use = []
        series = [{"labels": lab, "value": val} for e in use for lab, val in self.pick(e)]
        return {"query": name, "params": {k: str(v) for k, v in args.items() if k != "name"}, "backend": "prometheus", "series": series, "total": len(series), "truncated": False}

    def generic(self, tool: str, args: dict[str, Any]) -> Any:
        for e in self.case["tools"]:
            if e["tool"] == tool and _subset(e.get("match") or {}, args):
                return self.pick(e)
        return None


def _template_regex(path: str) -> re.Pattern[str]:
    return re.compile("^" + re.sub(r":\w+", r"([^/]+)", path) + "$")


def stub_gateway(world: World, token_role: str = "operator") -> GatewayClient:
    snapshot = json.loads(SNAPSHOT.read_text(encoding="utf-8"))
    tools = snapshot["scopes"][token_role]
    inc = world.case["incident"]

    def handler(req: httpx.Request) -> httpx.Response:
        if req.url.path == "/ai/tools":
            return httpx.Response(200, json={"role": token_role, "tools": tools, "actions": snapshot["actions"].get(token_role, [])})
        for t in tools:
            m = _template_regex(t["path"]).match(req.url.path)
            if not (m and t["method"] == req.method):
                continue
            args = dict(req.url.params)
            args.update(dict(zip(re.findall(r":(\w+)", t["path"]), m.groups())))
            world.calls.append({"tool": t["name"], "method": req.method, "args": args})
            n = t["name"]
            if n == "ops_get_playbook":
                pb = world.playbooks.get(args["id"])
                return httpx.Response(200, json=pb) if pb else httpx.Response(404, json={"error": "không có playbook này"})
            if n == "ops_list_playbooks":
                a = args.get("alert")
                pbs = [{"id": p["id"], "title": p["title"], "alerts": p["triggers"]["alerts"]} for p in world.playbooks.values() if not a or a in p["triggers"]["alerts"]]
                return httpx.Response(200, json={"playbooks": pbs})
            if n == "ops_list_actions":
                return httpx.Response(200, json={"actions": world.catalog})
            if n == "ops_list_incidents":
                return httpx.Response(200, json=world.generic(n, args) or {"incidents": [inc]})
            if n == "ops_get_incident":
                return httpx.Response(200, json=inc) if str(args["id"]) == str(inc["id"]) else httpx.Response(404, json={"error": "not found"})
            if n == "ops_metrics_query":
                return httpx.Response(200, json=world.metric(args["name"], args))
            if n == "ops_system_health":
                return httpx.Response(200, json=world.health())
            if n == "ops_outbox_dead":
                return httpx.Response(200, json=world.generic(n, args) or {"rows": [], "total": 0})
            if n == "ops_list_proposals":
                return httpx.Response(200, json={"proposals": []})
            if n == "ops_list_audit":
                return httpx.Response(200, json={"audit": []})
            if n == "ops_get_proposal":
                return httpx.Response(404, json={"error": "not found"})
            return httpx.Response(200, json=world.generic(n, args) or {"status": "ok"})
        return httpx.Response(404, json={"error": "not found"})

    return GatewayClient("http://gw", 15.0, client=httpx.AsyncClient(transport=httpx.MockTransport(handler)))


class StubOps:
    """OpsPort cho đồ thị: sổ hành động thật (bản sao), không có đề xuất chờ."""

    def __init__(self, catalog: list[dict[str, Any]]) -> None:
        self.catalog = catalog

    async def list_actions(self):
        return self.catalog

    async def pending_proposals(self, incident_id: int) -> int:
        return 0


# ---------------- chạy và chấm ----------------
@dataclass
class CaseRun:
    values: dict[str, Any]
    drafts: list[dict[str, Any]] = field(default_factory=list)
    calls: list[dict[str, Any]] = field(default_factory=list)


@dataclass
class Result:
    ok: bool
    reasons: list[str]
    checks: dict[str, bool]


async def run_case(case: dict[str, Any], world: World, brain: Any, budget: Budget | None = None) -> CaseRun:
    """Chạy đồ thị tới khi xong. Người được GIẢ LẬP: nháp nào cũng được operator xác nhận, người duyệt luôn duyệt và hành động chạy xong ngay
    (world chuyển sang phase `after` để verify thấy hệ thống đã hồi phục). Điều cần đo là ĐIỀU TRA, không phải thao tác của người."""
    from langgraph.checkpoint.memory import MemorySaver
    from langgraph.types import Command

    g = build_graph(brain, StubOps(world.catalog), budget or Budget(), checkpointer=MemorySaver())
    cfg = {"configurable": {"thread_id": case["id"]}}
    r = await g.ainvoke({"incident": dict(case["incident"])}, cfg)
    drafts: list[dict[str, Any]] = []
    pid = 0
    for _ in range(10):
        intr = r.get("__interrupt__")
        if not intr:
            break
        v = intr[0].value
        if v["waiting"] == "confirm":
            drafts.append(v["draft"])
            pid += 1
            r = await g.ainvoke(Command(resume={"event": "confirmed", "proposal_id": pid}), cfg)
        else:
            world.phase = "after"
            r = await g.ainvoke(Command(resume={"event": "executed"}), cfg)
    return CaseRun((await g.aget_state(cfg)).values, drafts, list(world.calls))


def _cause_matches(expect: dict[str, Any], causes: list[str]) -> bool:
    if expect.get("cause_all"):
        return any(all(re.search(rx, c, re.I | re.S) for rx in expect["cause_all"]) for c in causes)
    return any(re.search(rx, c, re.I | re.S) for rx in expect.get("cause_any") or [] for c in causes)


def _is(d: dict[str, Any], want: dict[str, Any]) -> bool:
    return d["action_id"] == want["id"] and _subset(want.get("params") or {}, d.get("params") or {})


def score(case: dict[str, Any], run: CaseRun) -> Result:
    e, v = case["expect"], run.values
    causes = [h["cause"] for h in (v.get("hypotheses") or [])[:3]]
    acts = [e["action"]] if e.get("action") else e["action_any"]
    first = run.drafts[0] if run.drafts else None
    checks = {
        "cause": _cause_matches(e, causes),
        "playbook": e.get("playbook") is None or any(c["tool"] == "ops_get_playbook" and c["args"].get("id") == e["playbook"] for c in run.calls),
        "action": first is not None and any(_is(first, a) for a in acts),
        "forbid": not any(_is(d, f) for d in run.drafts for f in e.get("forbid") or []),
        "outcome": v.get("outcome") == e["outcome"],
        "budget": v.get("model_calls", 0) <= int(e.get("max_model_calls", 12)) and v.get("queries", 0) <= 25,
        "read_only": all(c["method"] == "GET" for c in run.calls),
    }
    why = {"cause": f"giả thuyết top-3 không khớp: {causes}", "playbook": f"chưa mở playbook {e.get('playbook')}",
           "action": f"hành động đầu {first and (first['action_id'], first['params'])} khác mong đợi {[(a['id'], a.get('params')) for a in acts]}",
           "forbid": f"đề xuất hành động bị cấm: {[(d['action_id'], d['params']) for d in run.drafts]}", "outcome": f"kết cục {v.get('outcome')!r} khác {e['outcome']!r}",
           "budget": f"vượt ngân sách (lượt mô hình {v.get('model_calls')}, truy vấn {v.get('queries')})", "read_only": "có lời gọi không phải GET"}
    reasons = [why[k] for k, ok in checks.items() if not ok]
    return Result(not reasons, reasons, checks)


# ---------------- Brain mẫu cho kiểm thử bộ eval (không LLM) ----------------
class OracleBrain:
    """Làm ĐÚNG đáp án của case: để chứng minh bộ eval chấm được và case tự nhất quán."""

    def __init__(self, case: dict[str, Any], world: World) -> None:
        self.case, self.world = case, world

    async def collect(self, state):
        ev = [{"source": "ops_get_playbook", "args": {"id": self.case["expect"]["playbook"]}, "summary": "playbook"}] if self.case["expect"].get("playbook") else []
        for i, t in enumerate(self.case["tools"]):
            ev.append({"source": t["tool"], "args": t.get("match") or {"i": i}, "summary": json.dumps(self.world.pick(t), ensure_ascii=False)[:300]})
        calls = [{"tool": "ops_get_playbook", "method": "GET", "args": {"id": self.case["expect"]["playbook"]}}] if self.case["expect"].get("playbook") else []
        self.world.calls.extend(calls)
        return {"evidence": ev, "queries": len(ev), "model_calls": 2, "playbook": self.case["expect"].get("playbook")}

    async def hypothesize(self, state):
        unknown = self.case["expect"]["action"]["id"] == "escalate_to_human" if self.case["expect"].get("action") else False
        return {"hypotheses": [{"cause": self.case["expect"]["cause_example"], "confidence": 0.9, "evidence_ids": [e["id"] for e in state["evidence"]][:3], "refuted_by": "x"}], "model_calls": 1}

    async def choose_action(self, state, catalog):
        a = self.case["expect"].get("action") or self.case["expect"]["action_any"][0]
        return {"action_id": a["id"], "params": dict(a.get("params") or {}), "rationale": "đáp án", "evidence_ids": ["e1"], "model_calls": 1}

    async def verify(self, state):
        return {"recovered": True, "note": "hồi phục", "queries": 1, "model_calls": 1}

    async def report(self, state):
        return {"summary": "xong", "ready_to_close": state.get("outcome") == "recovered", "reason": "oracle", "model_calls": 1}


class WrongBrain(OracleBrain):
    """Đối chứng: luôn kết luận sai và đề xuất restart api-gateway. Bộ eval phải chấm nó TRƯỢT hầu hết case."""

    async def hypothesize(self, state):
        return {"hypotheses": [{"cause": "gateway chạy chậm", "confidence": 0.9, "evidence_ids": ["e1"], "refuted_by": ""}], "model_calls": 1}

    async def choose_action(self, state, catalog):
        return {"action_id": "restart_service", "params": {"service": "api-gateway"}, "rationale": "x", "evidence_ids": ["e1"], "model_calls": 1}

    async def verify(self, state):
        return {"recovered": False, "note": "không đổi", "queries": 1, "model_calls": 1}
