"""OpenAI và API tương thích OpenAI (đặt base_url: DeepSeek, OpenCode Zen, proxy) qua langchain-openai."""
from __future__ import annotations

from typing import Any

from langchain_core.language_models.chat_models import BaseChatModel

from ai_agent.llm import replay_cache
from ai_agent.llm.providers._common import model_id, require
from ai_agent.llm.registry import register_llm


@register_llm("openai")
def build(profile: dict[str, Any]) -> BaseChatModel:
    """Khoá dùng: model_id, api_key, temperature, base_url, timeout, extra_body (tham số riêng của nhà cung cấp, vd bật/tắt
    thinking của DeepSeek), max_retries (số lần SDK tự thử lại 408/409/429/5xx và lỗi kết nối), default_headers, session_header (tên header chứa mã phiên = conversation_id của lượt đang chạy;
    OpenCode đòi `x-opencode-session`, thiếu thì trả 400), stream_usage (true: streaming trả kèm token vào/ra; langchain-openai mặc định
    TẮT khi có base_url tuỳ chỉnh, mà ta cần usage để đếm token và áp trần)."""
    from langchain_openai import ChatOpenAI

    session_header = profile.get("session_header")
    extra: dict[str, Any] = {}
    if profile.get("stream_usage") is not None:
        extra["stream_usage"] = bool(profile["stream_usage"])
    if profile.get("default_headers"):
        extra["default_headers"] = dict(profile["default_headers"])
    cache = replay_cache.from_env()  # chỉ eval đặt AI_LLM_CACHE_DIR; production không bao giờ có cache
    if session_header or cache:
        extra.update(_session_clients(str(session_header or ""), cache))
    if profile.get("timeout"):
        extra["timeout"] = float(profile["timeout"])
    if profile.get("max_retries") is not None:  # tầng thử lại DUY NHẤT (SDK: backoff có jitter, tôn trọng Retry-After); xem llm/resilience.py
        extra["max_retries"] = int(profile["max_retries"])
    return ChatOpenAI(
        **extra,
        model=model_id(profile),
        api_key=require(profile, "api_key", "điền biến key của profile trong .env, vd DEEPSEEK_API_KEY"),
        temperature=profile.get("temperature", 0),
        base_url=profile.get("base_url"),
        extra_body=profile.get("extra_body"),
    )


def _session_clients(header: str, cache: replay_cache.ReplayCache | None = None) -> dict[str, Any]:
    """HTTP client có hook gắn header mã phiên theo lượt chat hiện tại (nếu có header) và, khi cache replay bật, bọc transport để phát lại/ghi
    phản hồi (llm/replay_cache.py). Mô hình được dựng một lần và dùng chung cho mọi cuộc hội thoại nên mã phiên phải lấy lúc gửi request
    (contextvar), không cố định lúc dựng."""
    import httpx

    from ai_agent.runtime.context import peek_run

    def stamp(request: httpx.Request) -> None:
        run = peek_run()
        sid = (run.conversation_id or run.run_id) if run else ""
        if sid:
            request.headers[header] = sid

    async def astamp(request: httpx.Request) -> None:
        stamp(request)

    hooks = [stamp] if header else []
    ahooks = [astamp] if header else []
    kw: dict[str, Any] = {}
    akw: dict[str, Any] = {}
    if cache is not None:
        kw["transport"] = replay_cache.CachingTransport(httpx.HTTPTransport(), cache)
        akw["transport"] = replay_cache.CachingAsyncTransport(httpx.AsyncHTTPTransport(), cache)
    return {
        "http_client": httpx.Client(event_hooks={"request": hooks}, **kw),
        "http_async_client": httpx.AsyncClient(event_hooks={"request": ahooks}, **akw),
    }
