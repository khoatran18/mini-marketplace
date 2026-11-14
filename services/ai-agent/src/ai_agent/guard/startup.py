"""Kiểm tra lúc khởi động: dừng nếu cấu hình có thể làm lộ dữ liệu người dùng (docs/ai/08 §14.2.4)."""
from __future__ import annotations

import os

_TRACING_VARS = ("LANGSMITH_TRACING", "LANGCHAIN_TRACING_V2", "LANGCHAIN_TRACING", "LANGSMITH_API_KEY", "LANGCHAIN_API_KEY")
_TRUE = {"1", "true", "yes", "on"}


def tracing_vars_set(env: dict[str, str] | None = None) -> list[str]:
    e = os.environ if env is None else env
    out = []
    for k in _TRACING_VARS:
        v = (e.get(k) or "").strip()
        if v and (k.endswith("API_KEY") or v.lower() in _TRUE):
            out.append(k)
    return out


def check_no_tracing(app_env: str, env: dict[str, str] | None = None) -> list[str]:
    """Trả về các biến tracing đang đặt. Ở prod thì ném lỗi (trace chứa nội dung chat, gửi ra dịch vụ ngoài)."""
    found = tracing_vars_set(env)
    if found and app_env == "prod":
        raise RuntimeError(f"biến tracing của LangSmith được đặt ở prod: {found}; bỏ chúng đi (nội dung chat không được gửi ra ngoài)")
    return found
