"""Dựng agent cho từng persona: mô hình (từ llm.roles) + tool (từ gateway theo role trong token) + prompt + middleware giới hạn.

Dùng create_agent của LangChain (đã chốt, docs/ai/08 §14). Tool lấy từ GET /ai/tools nên bảng quyền chỉ có một nguồn (gateway)."""
from __future__ import annotations

import time
from typing import Any

from ai_agent.config.loader import Config
from ai_agent.config.logging import get_logger
from ai_agent.guard.limits import build_middleware
from ai_agent.llm import get_llm_for_role
from ai_agent.personas.base import Persona
from ai_agent.runtime.context import RunContext
from ai_agent.tools.gateway import GatewayClient
from ai_agent.tools.propose import make_action_tool
from ai_agent.tools.registry import make_tool, parse_actions, parse_specs
from ai_agent.tools.spec import ActionSpec, ToolSpec

logger = get_logger(__name__)


class AgentRuntime:
    def __init__(self, cfg: Config, gateway: GatewayClient, *, llm_factory=get_llm_for_role, agent_builder=None) -> None:
        self.cfg = cfg
        self.gateway = gateway
        self._llm_factory = llm_factory
        self._agent_builder = agent_builder  # test có thể thay; mặc định create_agent
        self._specs: dict[str, tuple[float, list[ToolSpec], list[ActionSpec]]] = {}
        self._agents: dict[tuple, Any] = {}
        self._models: dict[str, Any] = {}

    async def specs_for(self, ctx: RunContext) -> tuple[list[ToolSpec], list[ActionSpec]]:
        """Tool đọc và hành động đề xuất của role trong token, lưu tạm theo role (TTL gateway.tools_cache_ttl_sec)."""
        now = time.monotonic()
        hit = self._specs.get(ctx.role)
        if hit and hit[0] > now:
            return hit[1], hit[2]
        payload = await self.gateway.list_tools(ctx.token, ctx.request_id)
        specs, actions = parse_specs(payload), parse_actions(payload)
        self._specs[ctx.role] = (now + float(self.cfg.gateway.tools_cache_ttl_sec), specs, actions)
        return specs, actions

    def tool_names(self, role: str) -> list[str]:
        """Tên mọi tool (đọc + đề xuất) của role, đã lấy ở specs_for. Dùng cho bộ lọc lộ tên công cụ."""
        hit = self._specs.get(role)
        return [t.name for t in (*hit[1], *hit[2])] if hit else []

    def _model(self, llm_role: str):
        if llm_role not in self._models:
            self._models[llm_role] = self._llm_factory(self.cfg, llm_role)  # lỗi cấu hình báo ở đây, kèm tên profile
        return self._models[llm_role]

    def read_tools(self, persona: Persona, specs: list[ToolSpec]) -> list:
        limits = self.cfg.limits
        return [
            make_tool(s, self.gateway, tool_timeout=float(self.cfg.gateway.timeout_sec),
                      max_chars=int(limits.tool_result_max_chars), max_items=int(limits.tool_result_max_items), reveal_route=not persona.hide_internals)
            for s in specs
        ]

    async def agent_for(self, persona: Persona, ctx: RunContext):
        specs, actions = await self.specs_for(ctx)
        if not persona.allow_actions:
            actions = []  # persona chỉ đọc (RCA): không nhận tool đề xuất dù token có quyền
        if not specs:
            raise LookupError(f"role {ctx.role!r} chưa có tool nào ở gateway")
        key = (persona.name, tuple((s.name, s.method, s.path) for s in specs), tuple((a.name, a.method, a.path) for a in actions))
        if key not in self._agents:
            limits = self.cfg.limits
            tools = self.read_tools(persona, specs)
            tools += [make_action_tool(a, self.gateway, tool_timeout=float(self.cfg.gateway.timeout_sec)) for a in actions]
            model = self._model(persona.llm_role)
            if self._agent_builder is not None:
                agent = self._agent_builder(model, tools, persona, [])  # bản thay thế (test) không dùng middleware của langchain
            else:
                from langchain.agents import create_agent

                agent = create_agent(model=model, tools=tools, system_prompt=persona.prompt, middleware=build_middleware(limits))
            self._agents[key] = agent
            logger.info("agent built", extra={"persona": persona.name, "tools": [s.name for s in specs], "actions": [a.name for a in actions]})
        return self._agents[key]
