"""Đo mỗi lần gọi mô hình (độ trễ, token vào/ra, lỗi) bằng callback; ghi log JSON kèm run_id (contextvar).
Không ghi nội dung prompt hay câu trả lời."""
from __future__ import annotations

import time
from typing import Any

from langchain_core.callbacks import BaseCallbackHandler
from langchain_core.language_models.chat_models import BaseChatModel

from ai_agent import metrics as _prom
from ai_agent.config.logging import get_logger

logger = get_logger("ai_agent.llm.metrics")


class LLMMetrics(BaseCallbackHandler):
    """Một đối tượng dùng chung cho một profile; trạng thái theo run_id của LangChain nên an toàn khi gọi song song."""

    def __init__(self, profile: str) -> None:
        self.profile = profile
        self._start: dict[Any, float] = {}
        self._first: dict[Any, float] = {}

    def on_chat_model_start(self, serialized, messages, *, run_id, **kwargs) -> None:
        self._start[run_id] = time.monotonic()

    def on_llm_new_token(self, token, *, run_id, **kwargs) -> None:
        self._first.setdefault(run_id, time.monotonic())

    def on_llm_end(self, response, *, run_id, **kwargs) -> None:
        t0 = self._start.pop(run_id, None)
        first = self._first.pop(run_id, None)
        usage: dict[str, Any] = {}
        try:
            msg = response.generations[0][0].message
            usage = dict(getattr(msg, "usage_metadata", None) or {})
        except (IndexError, AttributeError):
            pass
        if t0:
            _prom.LLM_SECONDS.labels(self.profile).observe(time.monotonic() - t0)
        logger.info("llm call", extra={
            "profile": self.profile, "e2e_ms": round((time.monotonic() - t0) * 1000) if t0 else None,
            "ttft_ms": round((first - t0) * 1000) if (t0 and first) else None,
            "input_tokens": usage.get("input_tokens"), "output_tokens": usage.get("output_tokens"),
        })

    def on_llm_error(self, error, *, run_id, **kwargs) -> None:
        self._start.pop(run_id, None)
        self._first.pop(run_id, None)
        _prom.LLM_ERRORS.labels(self.profile).inc()
        logger.warning("llm error", extra={"profile": self.profile, "error": type(error).__name__})


def attach(model: BaseChatModel, profile: str) -> None:
    """Gắn đo lường vào model (giữ callback đã có)."""
    handler = LLMMetrics(profile)
    if model.callbacks is None:
        model.callbacks = [handler]
    elif isinstance(model.callbacks, list):
        model.callbacks = [*model.callbacks, handler]
