"""Công cụ dùng chung cho test: token giả, cấu hình tối thiểu, gateway giả (httpx.MockTransport), agent giả."""
from __future__ import annotations

import base64
import json
import time
from pathlib import Path
from typing import Any

import httpx
from langchain_core.messages import AIMessage, ToolMessage

from ai_agent.config import load_config
from ai_agent.tools.gateway import GatewayClient

CONFIG_DIR = Path(__file__).resolve().parents[1] / "configs"


def b64(d: dict) -> str:
    return base64.urlsafe_b64encode(json.dumps(d).encode()).decode().rstrip("=")


def agent_token(user_id: int = 42, role: str = "buyer", jti: str = "run-0000-1111", ttl: int = 150, **over: Any) -> str:
    claims = {"typ": "agent", "aud": ["gateway-agent"], "sub": str(user_id), "role": role, "username": "u", "jti": jti,
              "exp": int(time.time()) + ttl, "act": {"sub": "ai-agent"}, **over}
    return f"{b64({'alg': 'HS256', 'kid': 'k1'})}.{b64(claims)}.sig"


def cfg(env: str = "dev"):
    """Cấu hình thật của env, nhưng role LLM trỏ về mô hình giả: test không được phụ thuộc key hay mạng."""
    c = load_config(env, CONFIG_DIR)
    c._data["llm"]["roles"].update(buyer="fake_buyer", seller="fake_seller", summarizer="fake_text")
    c._data["store"]["backend"] = "memory"   # test không cần Postgres (test_store_postgres.py tự dựng kết nối riêng)
    return c


BUYER_TOOLS = {"role": "buyer", "tools": [
    {"name": "list_my_orders", "method": "GET", "path": "/orders", "description": "orders", "params": [
        {"name": "status", "in": "query", "type": "string"}, {"name": "page_size", "in": "query", "type": "integer"}]},
    {"name": "get_my_order", "method": "GET", "path": "/orders/:id", "description": "one order", "params": [
        {"name": "id", "in": "path", "type": "integer", "required": True}]},
    {"name": "count_store_orders", "method": "GET", "path": "/seller/orders/summary", "description": "x"},
], "actions": [
    {"name": "cancel_order", "method": "POST", "path": "/orders/:id/cancel", "description": "Cancel an order.", "summary": "Huỷ đơn hàng #{id}", "params": [
        {"name": "id", "in": "path", "type": "integer", "required": True}, {"name": "reason", "in": "body", "type": "string"}]},
    {"name": "adjust_inventory", "method": "POST", "path": "/seller/products/:id/inventory/adjust", "description": "Adjust stock.", "summary": "Điều chỉnh tồn kho sản phẩm #{id}: {delta}", "params": [
        {"name": "id", "in": "path", "type": "integer", "required": True}, {"name": "delta", "in": "body", "type": "integer", "required": True},
        {"name": "reason", "in": "body", "type": "string", "required": True}]},
]}

ORDERS = {"orders": [{"id": 1, "status": "PAID", "total": 100000, "shipping_address": "12 Nguyen Trai, Ha Noi",
                      "phone": "0901234567", "receiver_name": "Nguyen Van A", "note": "goi 0912345678 hoac a@b.com"}]}


def mock_gateway(calls: list | None = None, orders: Any = ORDERS, status: int = 200) -> GatewayClient:
    def handler(req: httpx.Request) -> httpx.Response:
        if calls is not None:
            calls.append((req.method, req.url.path, dict(req.url.params), req.headers.get("authorization", "")))
        if req.url.path == "/ai/tools":
            return httpx.Response(200, json=BUYER_TOOLS)
        if req.url.path == "/ai/proposals":
            if status != 200:
                return httpx.Response(status, json={"error": "x"})
            return httpx.Response(201, json={"proposal": "SIGNED.PROPOSAL.TOKEN", "id": "prop-1", "expires_at": "2026-11-14T10:05:00Z"})
        if status != 200:
            return httpx.Response(status, json={"error": "x"})
        if req.url.path == "/orders":
            return httpx.Response(200, json=orders)
        if req.url.path.startswith("/seller/orders/summary"):
            return httpx.Response(200, json={"counts": {"PAID": 2}})
        return httpx.Response(404, json={"error": "nf"})

    return GatewayClient("http://gw", 2.0, client=httpx.AsyncClient(transport=httpx.MockTransport(handler)))


class FakeAgent:
    """Vòng lặp tối giản giống create_agent: gọi mô hình (stream), chạy tool, lặp. Dùng khi không có langchain.agents."""

    def __init__(self, model, tools):
        self.model, self.tools = model, {t.name: t for t in tools}

    async def astream(self, inp, stream_mode=None, config=None):
        msgs = list(inp["messages"])
        for _ in range(6):
            full = None
            async for ch in self.model.astream(msgs):
                full = ch if full is None else full + ch
                yield ("messages", (ch, {"langgraph_node": "model"}))
            ai = AIMessage(content=full.content, tool_calls=full.tool_calls)
            yield ("updates", {"model": {"messages": [ai]}})
            msgs.append(ai)
            if not ai.tool_calls:
                return
            for tc in ai.tool_calls:
                out = await self.tools[tc["name"]].ainvoke(tc["args"])
                tm = ToolMessage(out, tool_call_id=tc["id"])
                yield ("updates", {"tools": {"messages": [tm]}})
                msgs.append(tm)


def fake_builder(model, tools, persona, middleware):
    return FakeAgent(model, tools)
