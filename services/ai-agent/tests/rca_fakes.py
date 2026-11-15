"""Bản giả của Brain và OpsPort cho test RCA: kịch bản cố định, đếm số lần gọi, không mạng, không LLM."""
from __future__ import annotations

from typing import Any

CATALOG = [
    {"id": "requeue_outbox", "risk": "high", "mode": "http", "params": [
        {"name": "service", "type": "enum", "required": True, "enum": ["order-service", "product-service", "payment-service"]},
        {"name": "ids", "type": "int_list", "min": 1, "max": 100}, {"name": "limit", "type": "int", "min": 1, "max": 100}]},
    {"id": "restart_service", "risk": "low", "mode": "command", "params": [
        {"name": "service", "type": "enum", "required": True, "enum": ["order-service", "payment-service", "product-service"]}]},
    {"id": "scale_service", "risk": "low", "mode": "command", "params": [
        {"name": "service", "type": "enum", "required": True, "enum": ["order-service", "payment-service"]},
        {"name": "replicas", "type": "int", "required": True, "min": 1, "max": 5}]},
    {"id": "escalate_to_human", "risk": "observe", "mode": "manual", "params": []},
]

INCIDENT = {"id": 7, "alert_name": "ServiceDown", "severity": "critical", "component": "payment-service", "summary": "payment-service down", "labels": {}}


class FakeOps:
    def __init__(self, pending: int = 0, catalog: list | None = None) -> None:
        self.pending, self.catalog, self.calls = pending, catalog if catalog is not None else CATALOG, []

    async def list_actions(self):
        self.calls.append("list_actions")
        return self.catalog

    async def pending_proposals(self, incident_id: int) -> int:
        self.calls.append(("pending", incident_id))
        return self.pending


class ScriptedBrain:
    """Mỗi phương thức lấy phần tử kế tiếp của danh sách (phần tử cuối lặp lại). Ghi lại mọi lần gọi trong `calls`."""

    def __init__(self, *, collect=None, hypothesize=None, choose=None, verify=None, report=None, raises: str | None = None) -> None:
        self.s = {"collect": collect or [{"evidence": [{"source": "ops_list_incidents", "args": {}, "summary": "service down"}], "queries": 1, "model_calls": 1}],
                  "hypothesize": hypothesize or [{"hypotheses": [{"cause": "payment-service dừng", "confidence": 0.9, "evidence_ids": ["e1"], "refuted_by": "replica > 0"}], "model_calls": 1}],
                  "choose": choose or [{"action_id": "restart_service", "params": {"service": "payment-service"}, "rationale": "replica 0/1", "evidence_ids": ["e1"], "model_calls": 1}],
                  "verify": verify or [{"recovered": True, "note": "ok", "queries": 1, "model_calls": 1}],
                  "report": report or [{"summary": "xong", "ready_to_close": True, "reason": "đã hồi phục", "model_calls": 1}]}
        self.i = {k: 0 for k in self.s}
        self.calls: list[str] = []
        self.raises = raises

    def _next(self, key: str) -> dict[str, Any]:
        self.calls.append(key)
        if self.raises == key:
            raise RuntimeError("boom")
        lst = self.s[key]
        v = lst[min(self.i[key], len(lst) - 1)]
        self.i[key] += 1
        return v

    async def collect(self, state):
        return self._next("collect")

    async def hypothesize(self, state):
        return self._next("hypothesize")

    async def choose_action(self, state, catalog):
        return self._next("choose")

    async def verify(self, state):
        return self._next("verify")

    async def report(self, state):
        return self._next("report")


# ---- mô hình giả theo THỨ TỰ gọi (không theo tin nhắn): cho test LlmBrain, nơi một mô hình phục vụ cả agent thu bằng chứng và các yêu cầu JSON ----
import json as _json
import uuid as _uuid

import httpx as _httpx
from langchain_core.messages import AIMessage as _AI
from langchain_core.outputs import ChatGeneration as _CG, ChatResult as _CR

from ai_agent.config import load_config
from ai_agent.llm.providers.fake import ScriptedChatModel
from ai_agent.runtime.agent_factory import AgentRuntime
from ai_agent.runtime.context import RunContext
from ai_agent.tools.gateway import GatewayClient
from helpers import CONFIG_DIR, agent_token, fake_builder


class SeqModel(ScriptedChatModel):
    """Lần gọi thứ k (toàn cục) trả script[k]; hết thì lặp bước cuối. Bước: {content?, tool_calls?}."""
    n: int = 0
    seen: list = []

    def _generate(self, messages, stop=None, run_manager=None, **kw):
        step = self.script[min(self.n, len(self.script) - 1)]
        self.n += 1
        self.seen.append([str(m.content) for m in messages])
        msg = _AI(content=step.get("content", ""), tool_calls=self._calls(step), usage_metadata=self._usage(messages, step.get("content", "")))
        return _CR(generations=[_CG(message=msg)])

    def _stream(self, messages, stop=None, run_manager=None, **kw):
        res = self._generate(messages, stop, run_manager, **kw)
        from langchain_core.messages import AIMessageChunk
        from langchain_core.outputs import ChatGenerationChunk
        m = res.generations[0].message
        yield ChatGenerationChunk(message=AIMessageChunk(content=m.content, tool_calls=m.tool_calls, usage_metadata=m.usage_metadata))


OPS_TOOLS = {"role": "operator", "tools": [
    {"name": "ops_get_playbook", "method": "GET", "path": "/ops/playbooks/:id", "description": "playbook", "params": [{"name": "id", "in": "path", "type": "string", "required": True}]},
    {"name": "ops_metrics_query", "method": "GET", "path": "/ops/metrics", "description": "metric", "params": [{"name": "q", "in": "query", "type": "string"}]},
], "actions": [
    {"name": "ops_action", "method": "POST", "path": "/ops/proposals", "description": "propose", "summary": "Đề xuất vận hành", "params": [
        {"name": "action_id", "in": "body", "type": "string", "required": True}]}]}


def ops_gateway(calls: list | None = None, metric: str = "up=0", metric_status: int = 200) -> GatewayClient:
    def handler(req: _httpx.Request) -> _httpx.Response:
        if calls is not None:
            calls.append((req.method, req.url.path, dict(req.url.params)))
        if req.url.path == "/ai/tools":
            return _httpx.Response(200, json=OPS_TOOLS)
        if req.url.path.startswith("/ops/playbooks/"):
            return _httpx.Response(200, json={"id": req.url.path.rsplit("/", 1)[1], "diagnose": ["x"]})
        if req.url.path == "/ops/metrics":
            return _httpx.Response(metric_status, json={"value": metric})
        return _httpx.Response(404, json={"error": "nf"})
    return GatewayClient("http://gw", 2.0, client=_httpx.AsyncClient(transport=_httpx.MockTransport(handler)))


def brain_env(script: list, *, gateway=None, builder=None, tool_names: list | None = None):
    """(rt, persona, ctx, model): runtime thật với mô hình SeqModel + gateway giả của operator."""
    from ai_agent.personas import load_personas

    cfg = load_config("dev", CONFIG_DIR)
    cfg._data["store"]["backend"] = "memory"
    model = SeqModel(script=script, seen=[])

    def build(cfg_, tools, persona, mw):
        if tool_names is not None:
            tool_names.extend(t.name for t in tools)
        return fake_builder(model, tools, persona, mw)

    rt = AgentRuntime(cfg, gateway or ops_gateway(), llm_factory=lambda c, role: model, agent_builder=build)
    ctx = RunContext(token=agent_token(7, "operator", ttl=900), run_id="rca-" + _uuid.uuid4().hex[:6], request_id="t", user_id=7, role="operator")
    return rt, load_personas()["rca"], ctx, model
