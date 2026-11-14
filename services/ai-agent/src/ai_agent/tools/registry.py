"""Dựng tool LangChain từ bảng của gateway (GET /ai/tools) và hàm bọc chạy mỗi lần gọi tool.

Hàm bọc là đường DUY NHẤT từ mô hình tới gateway (docs/ai/08 §8A): kiểm tham số, gọi bằng token agent trong RunContext,
tách kết quả thành card (đầy đủ -> UI) và model_view (đã che -> mô hình), phát sự kiện, ghi log. Quyền thật ở gateway."""
from __future__ import annotations

import asyncio
from typing import Any

from langchain_core.tools import StructuredTool

from ai_agent.config.logging import get_logger
from ai_agent.runtime import events
from ai_agent.runtime.context import current_run
from ai_agent.tools import views
from ai_agent.tools.gateway import GatewayClient, GatewayError
from ai_agent.tools.spec import ActionSpec, ToolSpec, args_model, build_request

logger = get_logger(__name__)


class TurnAborted(Exception):
    """Dừng cả lượt (token agent hết hạn)."""


def make_tool(spec: ToolSpec, gateway: GatewayClient, *, tool_timeout: float, max_chars: int, max_items: int) -> StructuredTool:
    async def run(**kwargs: Any) -> str:
        ctx = current_run()
        ctx.tool_calls += 1
        label = events.label_for(spec.name)
        await ctx.events.put(events.make(events.TOOL_CALL, ctx.run_id, tool=spec.name, label=label))
        try:
            path, query = build_request(spec, kwargs)
            raw = await asyncio.wait_for(gateway.get(ctx.token, path, query, ctx.request_id), timeout=tool_timeout)
        except asyncio.TimeoutError:
            err: GatewayError | ValueError = GatewayError(504, "dịch vụ phản hồi quá chậm")
        except (GatewayError, ValueError) as exc:
            err = exc
        else:
            await ctx.events.put(events.make(events.CARD, ctx.run_id, **views.card_view(spec, raw)))
            ctx.cards.append(views.card_ref(spec, kwargs))
            await ctx.events.put(events.make(events.CITATION, ctx.run_id, tool=spec.name, label=events.source_label(spec.name)))
            await ctx.events.put(events.make(events.TOOL_RESULT, ctx.run_id, tool=spec.name, ok=True))
            logger.info("tool ok", extra={"tool": spec.name})
            return views.model_view(spec, raw, max_chars, max_items)

        msg = err.message if isinstance(err, GatewayError) else str(err)
        logger.warning("tool failed", extra={"tool": spec.name, "status": getattr(err, "status", None)})
        await ctx.events.put(events.make(events.TOOL_RESULT, ctx.run_id, tool=spec.name, ok=False))
        if isinstance(err, GatewayError) and err.fatal:
            raise TurnAborted(msg)
        return f"[lỗi công cụ] {msg}"

    return StructuredTool.from_function(coroutine=run, name=spec.name, description=spec.description, args_schema=args_model(spec))


def parse_specs(payload: dict[str, Any]) -> list[ToolSpec]:
    return [ToolSpec.from_json(t) for t in payload.get("tools") or []]


def parse_actions(payload: dict[str, Any]) -> list[ActionSpec]:
    return [ActionSpec.from_json(a) for a in payload.get("actions") or []]
