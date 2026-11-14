"""Đổi stream của LangChain/LangGraph thành sự kiện SSE của mình (hợp đồng là của mình, không phải của thư viện).

Đầu vào: agent.astream(..., stream_mode=["messages", "updates"]) cho ra (mode, data):
  - ("messages", (AIMessageChunk|ToolMessage, metadata)): từng mẩu chữ; chỉ lấy của nút mô hình;
  - ("updates", {node: {"messages": [...]}}): kết quả hoàn chỉnh của từng nút (dùng lấy câu trả lời cuối).
Tên nút và kiểu stream còn đổi theo phiên bản LangChain: ghim phiên bản (requirements.txt) và chạy tests/test_sse_adapter.py khi nâng."""
from __future__ import annotations

from collections.abc import AsyncIterator
from dataclasses import dataclass
from typing import Any

from langchain_core.messages import AIMessage, AIMessageChunk

from ai_agent.guard.leak import LeakFilter
from ai_agent.runtime import events
from ai_agent.runtime.context import RunContext

MODEL_NODES = {"model", "agent"}  # "agent" là tên cũ


class BudgetExceeded(Exception):
    pass


@dataclass
class PumpResult:
    answer: str = ""
    input_tokens: int = 0
    output_tokens: int = 0


def text_of(content: Any) -> str:
    """Chữ của một message: content có thể là chuỗi hoặc danh sách khối (Anthropic)."""
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        return "".join(b.get("text", "") if isinstance(b, dict) and b.get("type") == "text" else (b if isinstance(b, str) else "") for b in content)
    return ""


async def pump(stream: AsyncIterator[Any], ctx: RunContext, token_budget: int = 0, leak: LeakFilter | None = None) -> PumpResult:
    """Đọc hết stream, đẩy sự kiện `token` vào ctx.events, cộng token, áp trần. Trả câu trả lời cuối."""
    res = PumpResult()
    streamed = False
    async for item in stream:
        mode, data = item
        if mode == "messages":
            chunk, meta = data
            if not isinstance(chunk, AIMessageChunk) or (meta or {}).get("langgraph_node") not in MODEL_NODES:
                continue
            text = text_of(chunk.content)
            if text and leak is not None:
                text = leak.feed(text)  # có thể rỗng: phần đuôi đang được giữ lại
                streamed = True
            elif text:
                streamed = True
            if text:
                await ctx.events.put(events.make(events.TOKEN, ctx.run_id, text=text))
            u = chunk.usage_metadata
            if u:
                res.input_tokens += int(u.get("input_tokens", 0) or 0)
                res.output_tokens += int(u.get("output_tokens", 0) or 0)
                if token_budget and res.input_tokens + res.output_tokens > token_budget:
                    raise BudgetExceeded(f"{res.input_tokens + res.output_tokens} > {token_budget}")
        elif mode == "updates":
            for node, upd in (data or {}).items():
                if node in MODEL_NODES and isinstance(upd, dict):
                    msgs = [m for m in upd.get("messages", []) if isinstance(m, AIMessage)]
                    if msgs:
                        res.answer = text_of(msgs[-1].content)
    if leak is not None:
        tail = leak.flush()
        if tail:
            await ctx.events.put(events.make(events.TOKEN, ctx.run_id, text=tail))
        res.answer = leak.scrub(res.answer)  # bản lưu vào kho cũng không chứa chi tiết nội bộ
    if res.answer and not streamed:  # nhà cung cấp không stream: gửi một lần để UI vẫn thấy chữ
        await ctx.events.put(events.make(events.TOKEN, ctx.run_id, text=res.answer))
    return res
