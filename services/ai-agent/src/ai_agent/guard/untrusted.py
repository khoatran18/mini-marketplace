"""Đánh dấu dữ liệu không tin cậy (mô tả sản phẩm, review, tên shop... do người dùng nhập) trước khi đưa cho mô hình.
Chống prompt injection: prompt nói rõ nội dung trong khối này là DỮ LIỆU, không phải chỉ thị."""
from __future__ import annotations

OPEN, CLOSE = "<du_lieu_khong_tin_cay>", "</du_lieu_khong_tin_cay>"


def wrap(text: str) -> str:
    # nếu dữ liệu tự chứa thẻ đóng thì phá để nó không thoát khỏi khối
    safe = text.replace(CLOSE, "[/du_lieu]").replace(OPEN, "[du_lieu]")
    return f"{OPEN}{safe}{CLOSE}"
