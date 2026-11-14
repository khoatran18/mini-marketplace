"""Ngữ cảnh của MỘT lượt chạy: ai, token agent, run_id, nơi phát sự kiện SSE. Tool đọc nó qua contextvar nên token không
bao giờ vào prompt hay tham số tool mà mô hình điền."""
from __future__ import annotations

import asyncio
import contextvars
from dataclasses import dataclass, field
from typing import Any


@dataclass
class RunContext:
    token: str                 # token agent do gateway cấp; chỉ dùng để gọi gateway
    run_id: str                # = jti của token
    request_id: str
    user_id: int               # sub trong token (gateway điền), không bao giờ lấy từ mô hình
    role: str
    events: "asyncio.Queue[Any]" = field(default_factory=asyncio.Queue)  # sự kiện SSE: token, tool_call, card...
    conversation_id: str = ""  # điền ở đầu lượt; dùng làm mã phiên gửi nhà cung cấp mô hình (cache/định tuyến)
    tool_calls: int = 0
    proposals: int = 0         # số đề xuất đã tạo trong lượt này (có trần)
    cards: list[dict[str, Any]] = field(default_factory=list)            # tham chiếu card để lưu (loại + id), không lưu nội dung


_current: contextvars.ContextVar[RunContext | None] = contextvars.ContextVar("ai_agent_run", default=None)


def set_run(ctx: RunContext) -> contextvars.Token:
    return _current.set(ctx)


def reset_run(tok: contextvars.Token) -> None:
    _current.reset(tok)


def current_run() -> RunContext:
    ctx = _current.get()
    if ctx is None:
        raise RuntimeError("tool được gọi ngoài một lượt chạy (không có RunContext)")
    return ctx


def peek_run() -> RunContext | None:
    """Như current_run nhưng không báo lỗi khi ngoài lượt chạy (dùng ở lớp HTTP gọi mô hình)."""
    return _current.get()
