"""Giới hạn cứng cho một lượt, dựng bằng middleware dựng sẵn của LangChain (docs/ai/08 §14.4).

Trần TOKEN không có middleware dựng sẵn nên nằm ở runtime/sse_adapter.py (cộng usage_metadata của từng chunk).
Timeout cả lượt nằm ở runtime/chat.py; timeout từng tool nằm ở tools/registry.py."""
from __future__ import annotations

from typing import Any

from ai_agent.config.loader import Config


def build_middleware(limits: Config) -> list[Any]:
    # nhập muộn: chỉ cần khi thực sự dựng agent (test không dùng langchain.agents vẫn chạy)
    from langchain.agents.middleware import ModelCallLimitMiddleware, ToolCallLimitMiddleware

    return [
        # hết vòng thì kết thúc lượt (mô hình không gọi thêm); tool vượt trần thì trả lỗi cho mô hình thay vì chạy
        ModelCallLimitMiddleware(run_limit=int(limits.max_model_calls), exit_behavior="end"),
        ToolCallLimitMiddleware(run_limit=int(limits.max_tool_calls), exit_behavior="continue"),
    ]
