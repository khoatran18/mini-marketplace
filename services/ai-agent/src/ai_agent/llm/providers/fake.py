"""Mô hình giả có kịch bản: không gọi mạng. Dùng cho dev, CI và test an toàn (docs/ai/08 §8A).

Profile:
    fake_buyer:
      provider: "fake"
      script:                       # bước k = lần gọi mô hình thứ k TRONG MỘT LƯỢT (tính từ tin người dùng cuối cùng)
        - tool_calls: [{name: list_my_orders, args: {page_size: 3}}]
        - content: "Đây là các đơn của bạn."
Độ trễ tuỳ chọn (cho thử tải, không cần ở dev/CI): `ttft_ms` = chờ trước token đầu (và trước khi trả tool_calls), `tokens_per_sec` = tốc độ phát
token (từ trong content, đếm theo từ); đều chờ KHÔNG chặn event loop (asyncio.sleep) như một lời gọi mạng thật. 0/thiếu = không chờ.
Hết kịch bản thì lặp lại bước cuối. Một bước có thể vừa có content vừa có tool_calls.
Kịch bản không phụ thuộc trạng thái của đối tượng nên dùng chung được cho nhiều lượt chạy song song.
"""
from __future__ import annotations

import asyncio
import json
import uuid
from collections.abc import AsyncIterator, Iterator
from typing import Any

from langchain_core.callbacks import AsyncCallbackManagerForLLMRun, CallbackManagerForLLMRun
from langchain_core.language_models.chat_models import BaseChatModel
from langchain_core.messages import AIMessage, AIMessageChunk, BaseMessage, HumanMessage
from langchain_core.outputs import ChatGeneration, ChatGenerationChunk, ChatResult

from ai_agent.llm.registry import register_llm


class ScriptedChatModel(BaseChatModel):
    script: list[dict[str, Any]] = [{"content": "ok"}]
    ttft_ms: float = 0.0
    tokens_per_sec: float = 0.0

    @property
    def _llm_type(self) -> str:
        return "scripted-fake"

    def bind_tools(self, tools: Any, **kwargs: Any) -> "ScriptedChatModel":
        return self  # kịch bản không phụ thuộc định nghĩa tool

    def _step(self, messages: list[BaseMessage]) -> dict[str, Any]:
        last_human = max((i for i, m in enumerate(messages) if isinstance(m, HumanMessage)), default=-1)
        k = sum(1 for m in messages[last_human + 1:] if isinstance(m, AIMessage))
        return self.script[min(k, len(self.script) - 1)]

    @staticmethod
    def _calls(step: dict[str, Any]) -> list[dict[str, Any]]:
        return [
            {"name": c["name"], "args": dict(c.get("args") or {}), "id": c.get("id") or f"call_{uuid.uuid4().hex[:12]}", "type": "tool_call"}
            for c in step.get("tool_calls", [])
        ]

    def _usage(self, messages: list[BaseMessage], out: str) -> dict[str, int]:
        i = sum(len(str(m.content)) for m in messages) // 4 + 1
        o = len(out) // 4 + 1
        return {"input_tokens": i, "output_tokens": o, "total_tokens": i + o}

    def _generate(self, messages: list[BaseMessage], stop: list[str] | None = None,
                  run_manager: CallbackManagerForLLMRun | None = None, **kwargs: Any) -> ChatResult:
        step = self._step(messages)
        content = step.get("content", "")
        msg = AIMessage(content=content, tool_calls=self._calls(step), usage_metadata=self._usage(messages, content))
        return ChatResult(generations=[ChatGeneration(message=msg)])

    def _stream(self, messages: list[BaseMessage], stop: list[str] | None = None,
                run_manager: CallbackManagerForLLMRun | None = None, **kwargs: Any) -> Iterator[ChatGenerationChunk]:
        step = self._step(messages)
        content = step.get("content", "")
        words = content.split(" ") if content else []
        for i, w in enumerate(words):
            piece = w if i == len(words) - 1 else w + " "
            chunk = ChatGenerationChunk(message=AIMessageChunk(content=piece))
            if run_manager:
                run_manager.on_llm_new_token(piece, chunk=chunk)
            yield chunk
        calls = self._calls(step)
        tcc = [
            {"name": c["name"], "args": json.dumps(c["args"]), "id": c["id"], "index": i, "type": "tool_call_chunk"}
            for i, c in enumerate(calls)
        ]
        yield ChatGenerationChunk(message=AIMessageChunk(
            content="", tool_call_chunks=tcc, usage_metadata=self._usage(messages, content), chunk_position="last",
        ))

    async def _astream(self, messages: list[BaseMessage], stop: list[str] | None = None,
                       run_manager: AsyncCallbackManagerForLLMRun | None = None, **kwargs: Any) -> AsyncIterator[ChatGenerationChunk]:
        """Bọc `_stream` (lớp con ghi đè _stream vẫn đúng) và thêm độ trễ bằng asyncio.sleep; không đặt độ trễ thì cùng kết quả."""
        if self.ttft_ms > 0:
            await asyncio.sleep(self.ttft_ms / 1000)
        gap = 1.0 / self.tokens_per_sec if self.tokens_per_sec > 0 else 0.0
        first_text = True
        for chunk in self._stream(messages, stop, None, **kwargs):
            piece = str(chunk.message.content or "")
            if piece:
                if gap and not first_text:
                    await asyncio.sleep(gap)
                first_text = False
                if run_manager:
                    await run_manager.on_llm_new_token(piece, chunk=chunk)
            yield chunk


@register_llm("fake")
def build(profile: dict[str, Any]) -> BaseChatModel:
    script = profile.get("script") or [{"content": "ok"}]
    if not isinstance(script, list):
        raise ValueError("'script' phải là một danh sách các bước")
    return ScriptedChatModel(script=script, ttft_ms=float(profile.get("ttft_ms") or 0), tokens_per_sec=float(profile.get("tokens_per_sec") or 0))
