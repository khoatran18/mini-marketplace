"""Mô hình giả có kịch bản: không gọi mạng. Dùng cho dev, CI và test an toàn (docs/ai/08 §8A).

Profile:
    fake_buyer:
      provider: "fake"
      script:                       # bước k = lần gọi mô hình thứ k TRONG MỘT LƯỢT (tính từ tin người dùng cuối cùng)
        - tool_calls: [{name: list_my_orders, args: {page_size: 3}}]
        - content: "Đây là các đơn của bạn."
Hết kịch bản thì lặp lại bước cuối. Một bước có thể vừa có content vừa có tool_calls.
Kịch bản không phụ thuộc trạng thái của đối tượng nên dùng chung được cho nhiều lượt chạy song song.
"""
from __future__ import annotations

import json
import uuid
from collections.abc import Iterator
from typing import Any

from langchain_core.callbacks import CallbackManagerForLLMRun
from langchain_core.language_models.chat_models import BaseChatModel
from langchain_core.messages import AIMessage, AIMessageChunk, BaseMessage, HumanMessage
from langchain_core.outputs import ChatGeneration, ChatGenerationChunk, ChatResult

from ai_agent.llm.registry import register_llm


class ScriptedChatModel(BaseChatModel):
    script: list[dict[str, Any]] = [{"content": "ok"}]

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


@register_llm("fake")
def build(profile: dict[str, Any]) -> BaseChatModel:
    script = profile.get("script") or [{"content": "ok"}]
    if not isinstance(script, list):
        raise ValueError("'script' phải là một danh sách các bước")
    return ScriptedChatModel(script=script)
