"""Anthropic (Claude) qua langchain-anthropic."""
from __future__ import annotations

from typing import Any

from langchain_core.language_models.chat_models import BaseChatModel

from ai_agent.llm.providers._common import model_id, require
from ai_agent.llm.registry import register_llm


@register_llm("anthropic")
def build(profile: dict[str, Any]) -> BaseChatModel:
    """Khoá dùng: model_id, api_key, temperature, timeout, max_tokens, max_retries."""
    from langchain_anthropic import ChatAnthropic

    kwargs: dict[str, Any] = {}
    if profile.get("timeout"):
        kwargs["timeout"] = float(profile["timeout"])
    if profile.get("max_retries") is not None:
        kwargs["max_retries"] = int(profile["max_retries"])
    if profile.get("max_tokens"):
        kwargs["max_tokens"] = int(profile["max_tokens"])
    return ChatAnthropic(
        **kwargs,
        model=model_id(profile),
        api_key=require(profile, "api_key", "điền ANTHROPIC_API_KEY trong .env"),
        temperature=profile.get("temperature", 0),
    )
