"""Kho lịch sử chat. Quy tắc (docs/ai/08 P3):
 - conversation_id do ai-agent sinh ở tin đầu; mọi lần sau kiểm conversation.user_id == sub (sai -> NotFound, không lộ có tồn tại hay không);
 - mỗi cuộc chat một lượt chạy tại một thời điểm (lượt thứ hai -> Busy);
 - client_message_id duy nhất trong cuộc chat để bỏ tin gửi trùng khi mạng lỗi (-> Duplicate);
 - chỉ lưu chữ người dùng và câu trả lời cuối; KHÔNG lưu kết quả tool đầy đủ (chỉ tham chiếu card)."""
from __future__ import annotations

from dataclasses import dataclass, field
from typing import Protocol


class NotFound(Exception):
    pass


class Busy(Exception):
    pass


class Duplicate(Exception):
    def __init__(self, run_id: str) -> None:
        super().__init__("duplicate client_message_id")
        self.run_id = run_id


@dataclass
class Turn:
    conversation_id: str
    run_id: str
    history: list[tuple[str, str]] = field(default_factory=list)  # [("user"|"assistant", text)] cũ -> mới, chưa gồm tin hiện tại


class Store(Protocol):
    async def begin_turn(self, *, user_id: int, conversation_id: str | None, client_message_id: str | None,
                         run_id: str, role: str, text: str, max_history: int) -> Turn: ...

    async def end_turn(self, turn: Turn, *, status: str, answer: str, cards: list[dict], tool_calls: int,
                       input_tokens: int = 0, output_tokens: int = 0) -> None: ...
