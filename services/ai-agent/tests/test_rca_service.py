"""RcaService + API /rca (ADR-46/47): phiên chạy nền bằng token HỆ THỐNG, xác nhận nháp bằng token của operator, bộ theo dõi nối với ops-service qua gateway.
Không LLM, không mạng thật: gateway giả (httpx.MockTransport), Brain kịch bản."""
from __future__ import annotations

import asyncio
import json

import httpx
import pytest
from fastapi import FastAPI
from langgraph.checkpoint.memory import MemorySaver

from ai_agent.api import rca as rca_api
from ai_agent.personas import load_personas
from ai_agent.rca.budget import Budget
from ai_agent.rca.runs import MemoryRunStore
from ai_agent.rca.service import RcaError, RcaService
from ai_agent.runtime.agent_factory import AgentRuntime
from ai_agent.runtime.context import RunContext
from ai_agent.tools.gateway import GatewayClient
from helpers import agent_token, cfg
from rca_fakes import CATALOG, INCIDENT, ScriptedBrain

SYS = agent_token(user_id=1, role="operator", jti="sys-jti-1", ttl=900)
KEY = "k" * 32
OPS_ACTION = {"name": "ops_action", "method": "POST", "path": "/ops/proposals", "description": "propose", "summary": "Đề xuất vận hành: {action_id}", "params": [
    {"name": "action_id", "in": "body", "type": "string", "required": True},
    {"name": "incident_id", "in": "body", "type": "integer"},
    {"name": "rationale", "in": "body", "type": "string", "required": True},
    {"name": "evidence", "in": "body", "type": "string_list"},
    {"name": "service", "in": "body", "nest": "params", "type": "string"},
]}


class World:
    """Trạng thái của ops-service giả + nhật ký các lời gọi tới gateway."""

    def __init__(self) -> None:
        self.calls: list[tuple[str, str, dict, str]] = []
        self.system_token_calls = 0
        self.proposals: list[dict] = []
        self.signed: list[dict] = []
        self.incidents = [{**INCIDENT, "state": "open"}]

    def handler(self, req: httpx.Request) -> httpx.Response:
        p, auth = req.url.path, req.headers.get("authorization", "")
        self.calls.append((req.method, p, dict(req.url.params), auth))
        if p == "/ai/system-token":
            self.system_token_calls += 1
            if req.headers.get("x-system-key") != KEY:
                return httpx.Response(401, json={"error": "x"})
            return httpx.Response(200, json={"token": SYS, "run_id": "sys-jti-1", "expires_in": 900})
        if p == "/ai/tools":
            return httpx.Response(200, json={"role": "operator", "tools": [], "actions": [OPS_ACTION]})
        if p == "/ai/proposals":
            body = json.loads(req.content)
            self.signed.append({"auth": auth, **body})
            return httpx.Response(201, json={"proposal": "SIGNED.TOKEN", "id": "prop-1", "expires_at": "2026-11-15T10:05:00Z"})
        if p == "/ops/incidents":
            return httpx.Response(200, json={"incidents": self.incidents})
        if p.startswith("/ops/incidents/"):
            n = int(p.rsplit("/", 1)[1])
            inc = next((i for i in self.incidents if i["id"] == n), None)
            return httpx.Response(200, json={"incident": inc}) if inc else httpx.Response(404, json={"error": "nf"})
        if p == "/ops/actions":
            return httpx.Response(200, json={"actions": CATALOG})
        if p == "/ops/proposals":
            want = req.url.params.get("state")
            rows = [x for x in self.proposals if not want or x["state"] == want]
            return httpx.Response(200, json={"proposals": rows})
        if p.startswith("/ops/proposals/"):
            n = int(p.rsplit("/", 1)[1])
            row = next((x for x in self.proposals if x["id"] == n), None)
            return httpx.Response(200, json={"proposal": row}) if row else httpx.Response(404, json={"error": "nf"})
        return httpx.Response(404, json={"error": "nf"})


def make(key: str = KEY, brain: ScriptedBrain | None = None, runs=None):
    w = World()
    gw = GatewayClient("http://gw", 2.0, client=httpx.AsyncClient(transport=httpx.MockTransport(w.handler)))
    c = cfg()
    rt = AgentRuntime(c, gw)
    brain = brain or ScriptedBrain()
    svc = RcaService(rt=rt, gateway=gw, runs=runs or MemoryRunStore(), saver=MemorySaver(), persona=load_personas()["rca"], budget=Budget(),
                     system_key=key, brain_factory=lambda ctx: brain)
    return svc, w, brain


async def settle(svc: RcaService) -> None:
    await asyncio.gather(*list(svc._tasks))


async def test_manual_start_runs_in_background_with_the_system_token():
    svc, w, brain = make()
    run, created = await svc.start_manual(7, user_id=42)
    assert created and run["trigger"] == "manual:42" and run["status"] == "running"
    await settle(svc)
    run = await svc.runs.get(run["id"])
    assert run["status"] == "waiting_confirm" and run["draft"]["action_id"] == "restart_service"
    ops_calls = [c for c in w.calls if c[1].startswith("/ops/")]
    assert ops_calls and all(c[3] == f"Bearer {SYS}" for c in ops_calls), "every ops read must use the system token"
    assert w.system_token_calls == 1, "the token is cached, not requested for every call"
    view = await svc.view(run["id"])
    assert view["hypotheses"][0]["confidence"] == 0.9 and view["timeline"] and view["evidence"][0]["id"] == "e1"


async def test_a_second_click_returns_the_active_run():
    svc, _, _ = make()
    first, created1 = await svc.start_manual(7, 42)
    again, created2 = await svc.start_manual(7, 43)
    assert created1 and not created2 and again["id"] == first["id"]
    await settle(svc)
    assert len(await svc.runs.recent()) == 1


async def test_without_the_system_key_nothing_starts():
    svc, w, _ = make(key="")
    assert not svc.enabled
    with pytest.raises(RcaError) as e:
        await svc.start_manual(7, 42)
    assert e.value.status == 503 and not w.calls


async def test_unknown_incident_is_404():
    svc, _, _ = make()
    with pytest.raises(RcaError) as e:
        await svc.start_manual(999, 42)
    assert e.value.status == 404


async def test_draft_is_signed_with_the_operators_token_and_matches_the_ops_body():
    svc, w, _ = make()
    run, _ = await svc.start_manual(7, 42)
    await settle(svc)
    op = agent_token(user_id=42, role="operator", jti="op-jti")
    out = await svc.draft_proposal(run["id"], RunContext(token=op, run_id="op-jti", request_id="r1", user_id=42, role="operator"))
    assert out["proposal"] == "SIGNED.TOKEN" and out["request"]["method"] == "POST" and out["request"]["path"] == "/ops/proposals"
    body = out["request"]["body"]
    assert body["action_id"] == "restart_service" and body["incident_id"] == 7 and body["params"] == {"service": "payment-service"}
    assert body["rationale"] == "replica 0/1" and body["evidence"] and "ops_list_incidents" in body["evidence"][0]
    assert w.signed[0]["auth"] == f"Bearer {op}", "the signature is requested with the OPERATOR's token, never the system token"
    assert w.signed[0]["body"] == body


async def test_draft_needs_a_waiting_run():
    svc, _, _ = make()
    run, _ = await svc.start_manual(7, 42)
    await settle(svc)
    await svc.runs.update(run["id"], status="waiting_approval")
    ctx = RunContext(token=agent_token(role="operator"), run_id="x", request_id="", user_id=42, role="operator")
    with pytest.raises(RcaError) as e:
        await svc.draft_proposal(run["id"], ctx)
    assert e.value.status == 409
    with pytest.raises(RcaError) as e2:
        await svc.draft_proposal(404, ctx)
    assert e2.value.status == 404


async def test_recheck_wakes_the_run_when_ops_has_the_confirmed_proposal():
    svc, w, _ = make()
    run, _ = await svc.start_manual(7, 42)
    await settle(svc)
    assert (await svc.recheck(run["id"]))["status"] == "waiting_confirm"  # nothing at ops yet: stays
    w.proposals.append({"id": 5, "incident_id": 7, "action_id": "restart_service", "params": {"service": "payment-service"}, "state": "PENDING",
                        "created_at": "2999-01-01T00:00:00Z"})
    woken = await svc.recheck(run["id"])
    assert woken["status"] == "waiting_approval" and woken["proposal_id"] == 5


async def test_start_marks_runs_interrupted_by_a_restart():
    runs = MemoryRunStore()
    r = await runs.create(7, "auto")
    svc, _, _ = make(runs=runs)
    await svc.start()
    try:
        assert (await runs.get(r["id"]))["status"] == "error"
    finally:
        await svc.stop()


async def test_the_watcher_starts_a_critical_incident_by_itself():
    svc, w, brain = make()
    out = await svc.watcher.tick()  # (start() would also run this loop in the background; one explicit tick keeps the test deterministic)
    assert out["started"] == 1
    run = await svc.runs.latest(7)
    assert run and run["trigger"] == "auto" and run["status"] == "waiting_confirm" and "collect" in brain.calls


# ---------------- HTTP ----------------
def api(svc):
    app = FastAPI()
    app.include_router(rca_api.create_router({"rca": svc}))
    return httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://ai")


def hdr(role="operator", user_id=42):
    return {"X-Agent-Token": agent_token(user_id=user_id, role=role, jti="h-1"), "X-Agent-Run-ID": "h-1"}


async def test_api_flow_and_roles():
    svc, _, _ = make()
    async with api(svc) as c:
        assert (await c.get("/rca/runs")).status_code == 401
        for role in ("buyer", "seller_admin"):
            assert (await c.get("/rca/runs", headers=hdr(role))).status_code == 403
        r = await c.post("/rca/runs", json={"incident_id": 7}, headers=hdr("admin"))
        assert r.status_code == 202 and r.json()["created"] is True
        rid = r.json()["run"]["id"]
        assert (await c.post("/rca/runs", json={"incident_id": 7}, headers=hdr())).status_code == 200  # đang chạy: trả phiên cũ
        await settle(svc)
        assert (await c.post("/rca/runs", json={"incident_id": 0}, headers=hdr())).status_code == 422
        listed = (await c.get("/rca/runs?incident_id=7", headers=hdr("admin"))).json()["items"]
        assert [x["id"] for x in listed] == [rid]
        assert (await c.get("/rca/runs?incident_id=x", headers=hdr())).status_code == 422
        got = (await c.get(f"/rca/runs/{rid}", headers=hdr("admin"))).json()
        assert got["run"]["status"] == "waiting_confirm" and got["hypotheses"] and got["timeline"]
        assert (await c.get("/rca/runs/999", headers=hdr())).status_code == 404
        # admin xem được nhưng không xác nhận được nháp
        assert (await c.post(f"/rca/runs/{rid}/proposal", headers=hdr("admin"))).status_code == 403
        p = await c.post(f"/rca/runs/{rid}/proposal", headers=hdr("operator"))
        assert p.status_code == 200 and p.json()["proposal"] == "SIGNED.TOKEN" and p.json()["request"]["path"] == "/ops/proposals"
        assert (await c.post(f"/rca/runs/{rid}/recheck", headers=hdr("admin"))).json()["run"]["status"] == "waiting_confirm"


async def test_api_when_rca_is_off():
    app = FastAPI()
    app.include_router(rca_api.create_router({"rca": None}))
    async with httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://ai") as c:
        assert (await c.get("/rca/runs", headers=hdr())).status_code == 503


async def test_app_wiring_starts_and_stops_rca():
    from ai_agent.main import create_app

    c = cfg()
    c._data["rca"].update(enabled=True, system_key=KEY)
    w = World()
    gw = GatewayClient("http://gw", 2.0, client=httpx.AsyncClient(transport=httpx.MockTransport(w.handler)))
    app = create_app(c, runtime=AgentRuntime(c, gw))
    async with app.router.lifespan_context(app):
        async with httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://ai") as cl:
            r = await cl.get("/rca/runs", headers=hdr())
            assert r.status_code == 200 and r.json() == {"items": []}
    off = cfg()
    off._data["rca"]["enabled"] = False
    app2 = create_app(off, runtime=AgentRuntime(off, gw))
    async with app2.router.lifespan_context(app2):
        async with httpx.AsyncClient(transport=httpx.ASGITransport(app=app2), base_url="http://ai") as cl:
            assert (await cl.get("/rca/runs", headers=hdr())).status_code == 503
