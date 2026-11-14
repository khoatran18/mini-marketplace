"""Tool ĐỀ XUẤT hành động ghi (huỷ đơn, trả hàng, điều chỉnh tồn kho...). Agent KHÔNG thực hiện gì ở đây: tool chỉ xin gateway ký một
đề xuất (POST /ai/proposals) rồi phát sự kiện `action_proposal`; người dùng bấm Xác nhận trên UI, UI mới gọi route thật bằng JWT
của chính họ (ADR-42). Danh sách hành động, tham số và câu tóm tắt đều lấy từ gateway (`actions` của GET /ai/tools)."""
from __future__ import annotations

import asyncio
from typing import Any

from langchain_core.tools import StructuredTool

from ai_agent.config.logging import get_logger
from ai_agent.runtime import events
from ai_agent.runtime.context import current_run
from ai_agent.tools.gateway import GatewayClient, GatewayError
from ai_agent.tools.registry import TurnAborted
from ai_agent.tools.spec import ActionSpec, args_model, build_action

logger = get_logger(__name__)

MAX_PROPOSALS_PER_TURN = 2  # một câu hỏi không được tạo hàng loạt nút xác nhận

_AFTER = ("Đã tạo đề xuất và hiện nút Xác nhận cho người dùng. Bạn CHƯA thực hiện gì. Hãy nói ngắn gọn bạn đã soạn đề xuất gì "
          "và nhắc người dùng kiểm tra rồi bấm Xác nhận (đề xuất hết hạn sau 5 phút). Không được nói là đã hoàn tất.")


def make_action_tool(spec: ActionSpec, gateway: GatewayClient, *, tool_timeout: float) -> StructuredTool:
    name = f"propose_{spec.name}"

    async def run(**kwargs: Any) -> str:
        ctx = current_run()
        ctx.tool_calls += 1
        await ctx.events.put(events.make(events.TOOL_CALL, ctx.run_id, tool=name, label=events.label_for(name)))
        if ctx.proposals >= MAX_PROPOSALS_PER_TURN:
            await ctx.events.put(events.make(events.TOOL_RESULT, ctx.run_id, tool=name, ok=False))
            return "[giới hạn] Đã soạn đủ số đề xuất cho một câu hỏi; bảo người dùng xác nhận các đề xuất đó trước rồi hỏi tiếp."
        try:
            path, body, summary = build_action(spec, kwargs)
            payload = {"method": spec.method, "path": path, "body": body, "summary": summary}
            res = await asyncio.wait_for(gateway.post_proposal(ctx.token, payload, ctx.request_id), timeout=tool_timeout)
            proposal, pid, expires = res["proposal"], res["id"], res.get("expires_at")
        except asyncio.TimeoutError:
            err: GatewayError | ValueError = GatewayError(504, "dịch vụ phản hồi quá chậm")
        except (GatewayError, ValueError, KeyError) as exc:
            err = exc if isinstance(exc, (GatewayError, ValueError)) else GatewayError(502, "phản hồi không hợp lệ")
        else:
            ctx.proposals += 1
            ctx.cards.append({"tool": name, "proposal_id": pid})
            # `request` là đúng yêu cầu UI phải gửi lại (gateway buộc băm body vào đề xuất, sai một chữ là bị từ chối)
            await ctx.events.put(events.make(events.ACTION_PROPOSAL, ctx.run_id, proposal=proposal, id=pid, expires_at=expires,
                                             summary=summary, request={"method": spec.method, "path": path, "body": body}))
            await ctx.events.put(events.make(events.TOOL_RESULT, ctx.run_id, tool=name, ok=True))
            logger.info("proposal created", extra={"tool": name, "proposal_id": pid})
            return _AFTER

        msg = err.message if isinstance(err, GatewayError) else str(err)
        logger.warning("proposal failed", extra={"tool": name, "status": getattr(err, "status", None)})
        await ctx.events.put(events.make(events.TOOL_RESULT, ctx.run_id, tool=name, ok=False))
        if isinstance(err, GatewayError) and err.fatal:
            raise TurnAborted(msg)
        return f"[lỗi công cụ] {msg}"

    description = f"ĐỀ XUẤT (chưa thực hiện, người dùng phải xác nhận): {spec.description} Chỉ gọi khi người dùng nói rõ muốn làm việc này."
    return StructuredTool.from_function(coroutine=run, name=name, description=description, args_schema=args_model(spec))
