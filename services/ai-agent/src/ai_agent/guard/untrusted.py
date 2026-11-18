"""Đánh dấu dữ liệu không tin cậy (mô tả sản phẩm, review, tên shop... do người dùng nhập) trước khi đưa cho mô hình.
Chống prompt injection: prompt nói rõ nội dung trong khối này là DỮ LIỆU, không phải chỉ thị."""
from __future__ import annotations

import re

OPEN, CLOSE = "<du_lieu_khong_tin_cay>", "</du_lieu_khong_tin_cay>"


# thẻ của khối, kể cả biến thể kẻ tấn công hay thử: khác hoa/thường, có khoảng trắng hoặc xuống dòng quanh dấu < / >, thẻ viết lệch
_CLOSE_RE = re.compile(r"<\s*/\s*du_lieu_khong_tin_cay\s*>", re.I)
_OPEN_RE = re.compile(r"<\s*du_lieu_khong_tin_cay\s*>", re.I)


def wrap(text: str) -> str:
    # nếu dữ liệu tự chứa thẻ đóng/mở (kể cả biến thể) thì phá để nó không thoát khỏi khối hay giả một khối tin cậy
    safe = _OPEN_RE.sub("[du_lieu]", _CLOSE_RE.sub("[/du_lieu]", text))
    return f"{OPEN}{safe}{CLOSE}"
