"""Kiểm tra dùng chung cho các provider."""
from __future__ import annotations

from typing import Any


def require(profile: dict[str, Any], key: str, hint: str = "") -> Any:
    """Lấy profile[key]; thiếu hoặc rỗng thì báo lỗi rõ. hint gợi ý nơi điền (vd tên biến trong .env)."""
    value = profile.get(key)
    if value in (None, ""):
        raise ValueError(f"thiếu '{key}'" + (f" ({hint})" if hint else ""))
    return value


def model_id(profile: dict[str, Any]) -> str:
    """Lấy model_id, từ chối giá trị mẫu kiểu '<điền model id>' còn sót trong yaml."""
    value = str(require(profile, "model_id"))
    if value.startswith("<"):
        raise ValueError(f"model_id chưa điền (đang là {value!r})")
    return value
