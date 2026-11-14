"""Sổ đăng ký provider LLM: tên trong yaml (`provider: "openai"`) -> hàm build(profile) tạo chat model.

    @register_llm("openai")
    def build(profile: dict) -> BaseChatModel: ...

"Đăng ký" = ghi hàm vào dict lúc file provider được import; chưa tạo chat model nào. Hàm chỉ chạy khi factory gọi get_llm.
Vì vậy providers/__init__.py phải import từng file provider. Kết quả build là BaseChatModel của langchain-core nên
`bind_tools`/`stream` dùng được bất kể provider.
"""
from __future__ import annotations

from collections.abc import Callable
from typing import Any

from langchain_core.language_models.chat_models import BaseChatModel

LLM_PROVIDERS: dict[str, Callable[[dict[str, Any]], BaseChatModel]] = {}


def register_llm(name: str):
    def deco(build):
        if name in LLM_PROVIDERS:
            raise ValueError(f"Provider LLM {name!r} đã được đăng ký")
        LLM_PROVIDERS[name] = build
        return build

    return deco
