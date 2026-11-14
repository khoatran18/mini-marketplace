"""Một lượt chat: dựng agent, chạy trong một task, phát sự kiện SSE từ một hàng đợi chung (token từ mô hình + card/tool_call
từ hàm bọc tool), áp timeout cả lượt, và luôn kết thúc bằng `done` hoặc `error` + ghi kho."""
from __future__ import annotations

import asyncio
import time
from collections.abc import AsyncIterator
from typing import Any

from langchain_core.messages import AIMessage, HumanMessage

from ai_agent import metrics
from ai_agent.config.logging import get_logger, request_id_var, run_id_var
from ai_agent.guard.leak import LeakFilter
from ai_agent.personas.base import Persona
from ai_agent.runtime import events
from ai_agent.runtime.agent_factory import AgentRuntime
from ai_agent.runtime.context import RunContext, reset_run, set_run
from ai_agent.runtime.sse_adapter import BudgetExceeded, PumpResult, pump
from ai_agent.store.base import Store, Turn
from ai_agent.tools.registry import TurnAborted

logger = get_logger(__name__)


class _Final:
    def __init__(self, result: PumpResult) -> None:
        self.result = result


class _Failed:
    def __init__(self, exc: BaseException) -> None:
        self.exc = exc


def _error(ctx: RunContext, code: str, message: str) -> dict[str, Any]:
    return events.make(events.ERROR, ctx.run_id, code=code, message=message)


async def run_turn(rt: AgentRuntime, store: Store, persona: Persona, ctx: RunContext, turn: Turn, text: str) -> AsyncIterator[dict[str, Any]]:
    limits = rt.cfg.limits
    status, answer, usage = "error", "", PumpResult()
    started = time.monotonic()
    ctx.conversation_id = turn.conversation_id
    run_tok = set_run(ctx)
    run_id_var.set(ctx.run_id)
    request_id_var.set(ctx.request_id)
    task: asyncio.Task | None = None
    try:
        yield events.make(events.START, ctx.run_id, conversation_id=turn.conversation_id)
        try:
            agent = await rt.agent_for(persona, ctx)
        except Exception as exc:  # cấu hình model thiếu, gateway không trả tool...
            logger.error("cannot build agent", extra={"error": type(exc).__name__})
            yield _error(ctx, "unavailable", "Trợ lý tạm thời không khả dụng.")
            return

        leak = LeakFilter(rt.tool_names(ctx.role)) if persona.hide_internals else None
        msgs = [HumanMessage(t) if r == "user" else AIMessage(t) for r, t in turn.history] + [HumanMessage(text)]

        async def drive() -> None:
            try:
                stream = agent.astream({"messages": msgs}, stream_mode=["messages", "updates"], config={"recursion_limit": 40})
                res = await pump(stream, ctx, int(limits.token_budget_per_turn), leak)
                await ctx.events.put(_Final(res))
            except asyncio.CancelledError:
                raise
            except BaseException as exc:  # noqa: BLE001 - mọi lỗi đều phải thành sự kiện error
                await ctx.events.put(_Failed(exc))

        task = asyncio.create_task(drive())  # task sao chép contextvars -> tool thấy RunContext
        loop = asyncio.get_running_loop()
        deadline = loop.time() + float(limits.turn_timeout_sec)
        while True:
            try:
                item = await asyncio.wait_for(ctx.events.get(), timeout=max(0.0, deadline - loop.time()))
            except asyncio.TimeoutError:
                yield _error(ctx, "timeout", "Lượt trả lời mất quá nhiều thời gian, hãy thử lại.")
                status = "timeout"
                return
            if isinstance(item, _Final):
                usage, answer = item.result, item.result.answer
                status = "ok"
                break
            if isinstance(item, _Failed):
                exc = item.exc
                if isinstance(exc, BudgetExceeded):
                    status = "budget"
                    yield _error(ctx, "token_budget_exceeded", "Câu hỏi này cần quá nhiều tài nguyên, hãy hỏi gọn hơn.")
                elif isinstance(exc, TurnAborted):
                    status = "session_expired"
                    yield _error(ctx, "session_expired", str(exc))
                else:
                    logger.error("turn failed", extra={"error": type(exc).__name__}, exc_info=exc)
                    yield _error(ctx, "model_error", "Có lỗi khi xử lý, hãy thử lại.")
                return
            yield item
        if not answer.strip():  # hết vòng gọi mô hình trước khi kịp viết câu trả lời: không để người dùng nhìn khung trống
            answer = ("Mình chưa tổng hợp xong câu trả lời trong giới hạn cho phép. Dữ liệu tìm được (nếu có) nằm ở thẻ bên dưới; "
                      "bạn thử hỏi cụ thể hơn giúp mình nhé.")
            yield events.make(events.TOKEN, ctx.run_id, text=answer)
        while not ctx.events.empty():  # sự kiện còn lại (card/citation đến sau)
            item = ctx.events.get_nowait()
            if isinstance(item, dict):
                yield item
        yield events.make(events.DONE, ctx.run_id, conversation_id=turn.conversation_id,
                          input_tokens=usage.input_tokens, output_tokens=usage.output_tokens, tool_calls=ctx.tool_calls)
    except (asyncio.CancelledError, GeneratorExit):
        status = "cancelled"  # client ngắt: dừng, không tốn thêm tiền gọi mô hình
        raise
    finally:
        if task is not None:
            if not task.done():
                task.cancel()
            await asyncio.gather(task, return_exceptions=True)  # đợi tác vụ dọn xong (đóng stream mô hình)
        metrics.RUNS.labels(persona.name, status).inc()
        metrics.RUN_SECONDS.labels(persona.name).observe(time.monotonic() - started)
        metrics.TOKENS.labels(persona.name, "input").inc(usage.input_tokens or 0)
        metrics.TOKENS.labels(persona.name, "output").inc(usage.output_tokens or 0)
        try:
            await asyncio.shield(store.end_turn(turn, status=status, answer=answer, cards=ctx.cards, tool_calls=ctx.tool_calls,
                                                input_tokens=usage.input_tokens, output_tokens=usage.output_tokens))
        except Exception:  # noqa: BLE001
            logger.error("cannot record turn")
        try:
            reset_run(run_tok)
        except ValueError:  # generator đóng ở context khác (client ngắt): không có gì để khôi phục
            pass
