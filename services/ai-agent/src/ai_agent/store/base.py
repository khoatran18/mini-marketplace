"""Kho lịch sử chat. Quy tắc (docs/ai/08 P3):
 - conversation_id do ai-agent sinh ở tin đầu; mọi lần sau kiểm conversation.user_id == sub (sai -> NotFound, không lộ có tồn tại hay không);
 - mỗi cuộc chat một lượt chạy tại một thời điểm (lượt thứ hai -> Busy);
 - client_message_id duy nhất trong cuộc chat để bỏ tin gửi trùng khi mạng lỗi (-> Duplicate);
 - chỉ lưu chữ người dùng và câu trả lời cuối; KHÔNG lưu kết quả tool đầy đủ (chỉ tham chiếu card)."""
from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone
from typing import Protocol

# Ngày của hạn mức tính theo giờ nền tảng (Asia/Ho_Chi_Minh = UTC+7, không có giờ mùa hè), không theo giờ máy chủ.
PLATFORM_TZ = timezone(timedelta(hours=7))


def day_start(now: float | None = None) -> float:
    """Epoch giây của 00:00 hôm nay theo giờ nền tảng."""
    t = datetime.fromtimestamp(now if now is not None else datetime.now(timezone.utc).timestamp(), PLATFORM_TZ)
    return t.replace(hour=0, minute=0, second=0, microsecond=0).timestamp()


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

    async def tokens_used_today(self, user_id: int) -> int:
        """Tổng token (vào + ra) các lượt của người dùng từ 00:00 hôm nay (giờ nền tảng). Lượt đang chạy chưa tính: hạn mức có thể vượt tối đa một lượt."""
        ...

    async def list_conversations(self, *, user_id: int | None, role: str | None = None, limit: int = 30) -> list[dict]:
        """Cuộc trò chuyện mới nhất trước: {id, user_id, role, title (tin đầu của người dùng, đã che), messages, created_at, updated_at}.
        user_id=None = mọi người dùng (chỉ dành cho admin xem lại); role lọc theo role lúc chat."""
        ...

    async def get_conversation(self, conversation_id: str, *, user_id: int | None) -> dict:
        """{id, user_id, role, created_at, updated_at, messages: [{role, content, created_at}]} (tối đa 200 tin, cũ -> mới).
        user_id khác None mà cuộc chat của người khác -> NotFound (không lộ có tồn tại)."""
        ...
