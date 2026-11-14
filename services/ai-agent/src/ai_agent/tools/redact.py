"""Che dữ liệu nhạy cảm và cắt kích thước TRƯỚC khi kết quả tool vào ngữ cảnh mô hình (docs/ai/08 §14.6).

model_view = bản này. Bản đầy đủ chỉ đi tới UI qua sự kiện `card`, không bao giờ qua mô hình, DB chat hay log."""
from __future__ import annotations

import json
import re
from typing import Any

# khoá bị BỎ hẳn (khớp một phần tên, không phân biệt hoa thường)
_DROP_KEY = re.compile(r"(address|phone|mobile|email|receiver|recipient|password|secret|token|session|ip_hash|^ip$|lat$|lng$|longitude|latitude)", re.I)
_EMAIL = re.compile(r"[\w.+-]+@[\w-]+(?:\.[\w-]+)+")
_PHONE = re.compile(r"(?<!\d)(?:\+?84|0)(?:[\s.-]?\d){8,10}(?!\d)")

_JWT = re.compile(r"\beyJ[\w-]{8,}\.[\w-]{8,}\.[\w-]*")
_BEARER = re.compile(r"\b(?:Bearer|Basic)\s+[\w.~+/=-]{8,}", re.I)
_SECRET_KV = re.compile(r"\b(?:password|passwd|secret|token|api[_-]?key|authorization)\b\s*[=:]\s*\S+", re.I)
_IPV4 = re.compile(r"(?<![\d.])(?:\d{1,3}\.){3}\d{1,3}(?![\d.])")

MASK = "[đã ẩn]"


def mask_text(s: str) -> str:
    """Che những thứ không được vào ngữ cảnh mô hình khi lọt vào văn bản tự do (mô tả, ghi chú, dòng log): email, số điện thoại (kiểu
    Việt Nam), JWT, "Bearer ...", "password=..."/"token: ...", địa chỉ IPv4. Tên người và địa chỉ nhà viết thành chữ thì regex không bắt được:
    dữ liệu nào chứa chúng phải bị bỏ theo tên khoá (_DROP_KEY) hoặc không được cấp cho tool."""
    for rx in (_JWT, _BEARER, _SECRET_KV, _EMAIL, _IPV4, _PHONE):
        s = rx.sub(MASK, s)
    return s


def redact(obj: Any, max_items: int = 20, max_str: int = 500, _depth: int = 0) -> Any:
    """Sao chép sâu: bỏ khoá nhạy cảm, che email/số điện thoại trong chuỗi, cắt chuỗi dài và danh sách dài."""
    if _depth > 8:
        return "…"
    if isinstance(obj, dict):
        return {k: redact(v, max_items, max_str, _depth + 1) for k, v in obj.items() if not _DROP_KEY.search(str(k))}
    if isinstance(obj, list):
        out = [redact(v, max_items, max_str, _depth + 1) for v in obj[:max_items]]
        if len(obj) > max_items:
            out.append({"_truncated": f"còn {len(obj) - max_items} mục nữa, hãy hỏi thu hẹp hoặc xem ở thẻ"})
        return out
    if isinstance(obj, str):
        s = mask_text(obj)
        return s if len(s) <= max_str else s[:max_str] + "…"
    return obj


def to_model_text(obj: Any, max_chars: int, max_items: int = 20) -> str:
    """JSON gọn đã che, cắt theo ngưỡng ký tự (kèm dấu hiệu bị cắt để mô hình không tưởng là đủ)."""
    text = json.dumps(redact(obj, max_items=max_items), ensure_ascii=False, separators=(",", ":"), default=str)
    if len(text) > max_chars:
        return text[:max_chars] + "…[kết quả đã bị cắt]"
    return text
