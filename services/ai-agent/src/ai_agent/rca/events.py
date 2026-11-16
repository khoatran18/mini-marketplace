"""Nhật ký sự kiện của một phiên RCA, để giao diện hiện quá trình điều tra như một đoạn chat: gọi công cụ nào, kết quả ra sao, mô hình nhận định gì.
Lưu trong bảng `ai_rca_events` (cùng sổ phiên). Ghi sự kiện KHÔNG BAO GIỜ được làm hỏng phiên: mọi lỗi bị nuốt và ghi log.

kind: step (một bước của đồ thị) | tool_call | tool_result | note (lời mô hình) | hypotheses | draft | report | wait | error."""
from __future__ import annotations

from typing import Any

from ai_agent.config.logging import get_logger

logger = get_logger(__name__)
RESULT_CHARS = 1200  # bằng với bằng chứng: kết quả công cụ hiện trên giao diện bị cắt ở đây


class EventLog:
    def __init__(self, runs: Any, run_id: int) -> None:
        self.runs, self.run_id = runs, run_id

    async def emit(self, kind: str, title: str, data: dict[str, Any] | None = None) -> None:
        try:
            await self.runs.add_event(self.run_id, kind, title, data)
        except Exception as exc:  # noqa: BLE001 - nhật ký hỏng không được làm hỏng điều tra
            logger.warning("rca event not saved: %s: %s", type(exc).__name__, str(exc)[:200])


class NullEvents:
    async def emit(self, kind: str, title: str, data: dict[str, Any] | None = None) -> None:
        return None
