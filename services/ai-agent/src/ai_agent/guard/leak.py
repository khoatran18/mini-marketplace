"""Chặn câu trả lời của buyer/seller lộ chi tiết nội bộ: đường dẫn API ("GET /orders/:id", "/ops/..."), tên công cụ (list_my_orders, propose_...).

Đây là lớp phụ. Lớp chính: mô hình KHÔNG được thấy đường dẫn (tools/views.py chỉ đưa nhãn nguồn cho persona ẩn nội bộ) và prompt dặn không nói về
công cụ. Quyền thật vẫn ở gateway (bảng agent/scopes.go): biết tên route cũng không gọi được route của role khác. Bộ lọc này chỉ để người dùng
cuối không thấy chi tiết kỹ thuật, kể cả khi mô hình lỡ nhắc tới (hoặc bị dụ nhắc tới).

Lọc theo luồng: giữ lại phần đuôi của chuỗi chưa gửi để một tên bị cắt giữa hai mẩu token vẫn bị bắt."""
from __future__ import annotations

import re
from collections.abc import Iterable

REPLACEMENT = "[thông tin nội bộ]"

_METHOD_PATH = r"(?:GET|POST|PUT|PATCH|DELETE)\s+/[\w/:.\-?=&%]*"
# đường dẫn không kèm method: chỉ các tiền tố chỉ có ở API (trang web của người dùng như /cart, /orders không bị đụng), hoặc có tham số :id
_API_PATH = r"/(?:ai|ops|admin|auth|users|shop/rules|internal|metrics|health)(?:/[\w:.\-?=&%]*)*|/[\w/\-]*/:[A-Za-z_]\w*[\w/:.\-?=&%]*"
_HOLD = 48


class LeakFilter:
    def __init__(self, tool_names: Iterable[str] = ()) -> None:
        names = sorted({n for n in tool_names if n}, key=len, reverse=True)
        parts = [_METHOD_PATH, _API_PATH, r"\bpropose_[a-z_]+\b"]
        if names:
            parts.append(r"\b(?:" + "|".join(re.escape(n) for n in names) + r")\b")
        self._re = re.compile("|".join(parts))
        self._buf = ""

    def scrub(self, text: str) -> str:
        """Lọc một đoạn hoàn chỉnh (câu trả lời cuối)."""
        return self._re.sub(REPLACEMENT, text)

    def feed(self, text: str) -> str:
        """Nhận thêm một mẩu token, trả phần đã chắc chắn an toàn để gửi."""
        self._buf += text
        cut = len(self._buf) - _HOLD
        if cut <= 0:
            return ""
        for m in self._re.finditer(self._buf):
            if m.start() < cut < m.end():
                cut = m.start()  # không cắt giữa một chuỗi bị che
                break
        if cut <= 0:
            return ""
        out, self._buf = self._buf[:cut], self._buf[cut:]
        return self._re.sub(REPLACEMENT, out)

    def flush(self) -> str:
        out, self._buf = self._buf, ""
        return self._re.sub(REPLACEMENT, out)
