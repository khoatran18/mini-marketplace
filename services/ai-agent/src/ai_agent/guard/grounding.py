"""Đo (KHÔNG chặn) mức câu trả lời chat bám dữ liệu công cụ: con số lớn trong câu trả lời có xuất hiện trong kết quả công cụ của lượt đó
(hoặc trong câu hỏi) không. Đây là "critic" nhẹ và xác định (docs/ai/12 mục 1): chỉ ghi số liệu `mm_ai_answer_numbers_total`, vì số suy ra
(tổng, trung bình, làm tròn "1,5 triệu") có thể hợp lệ mà không có nguyên văn trong dữ liệu; chặn sẽ chặn nhầm. Dùng tỷ lệ theo thời gian và
theo persona để thấy mô hình có bịa số hay không, rồi mới quyết định có cần chặn."""
from __future__ import annotations

import re

_NUM = re.compile(r"\d[\d.,]*\d|\d")
MIN_DIGITS = 4  # chỉ xét số từ 4 chữ số trở lên (giá, số lượng, mã đơn lớn); số nhỏ quá hay là thứ tự, phần trăm, ngày


def _norm(s: str) -> str:
    return re.sub(r"\D", "", s)  # "1.250.000" và "1,250,000" đều thành "1250000"


def numbers_in(text: str) -> set[str]:
    return {n for n in (_norm(m.group(0)) for m in _NUM.finditer(text)) if len(n) >= MIN_DIGITS}


def ungrounded_numbers(answer: str, *sources: str) -> tuple[int, int]:
    """(số con số lớn trong câu trả lời, số con số không có trong các nguồn). Nguồn: kết quả công cụ, câu hỏi người dùng."""
    mine = numbers_in(answer)
    if not mine:
        return 0, 0
    have: set[str] = set()
    for s in sources:
        have |= numbers_in(s)
    return len(mine), len(mine - have)
