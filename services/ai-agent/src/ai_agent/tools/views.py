"""Hai cách nhìn của một kết quả tool: card_view (đầy đủ, cho UI) và model_view (đã che/cắt, cho mô hình)."""
from __future__ import annotations

import time
from typing import Any

from ai_agent.guard import untrusted
from ai_agent.runtime.events import source_label
from ai_agent.tools.redact import to_model_text
from ai_agent.tools.spec import ToolSpec


def card_view(spec: ToolSpec, data: Any) -> dict[str, Any]:
    """Dữ liệu đầy đủ cho UI vẽ thẻ. Không đi qua mô hình."""
    return {"tool": spec.name, "kind": spec.name, "data": data}


def card_ref(spec: ToolSpec, args: dict[str, Any]) -> dict[str, Any]:
    """Chỉ tham chiếu (loại + tham số) để lưu theo cuộc chat; mở lại cuộc cũ thì UI gọi API thường để lấy dữ liệu mới."""
    return {"tool": spec.name, "args": {k: v for k, v in args.items() if v is not None}}


def model_view(spec: ToolSpec, data: Any, max_chars: int, max_items: int, reveal_route: bool = True) -> str:
    """Văn bản đưa cho mô hình: đã che, đã cắt, kèm nguồn + thời điểm lấy, bọc làm dữ liệu không tin cậy."""
    # reveal_route=False (buyer/seller): mô hình chỉ thấy nhãn nguồn, không thấy đường dẫn API để có gì mà kể lại
    source = f"{spec.method} {spec.path}" if reveal_route else source_label(spec.name)
    meta = {"tool": spec.name, "source": source, "fetched_at": int(time.time())}
    body = to_model_text(data, max_chars=max_chars, max_items=max_items)
    return untrusted.wrap(f"{meta} {body}")
