"""Đo (KHÔNG chặn) mức câu trả lời chat bám dữ liệu công cụ: con số lớn trong câu trả lời có xuất hiện trong kết quả công cụ của lượt đó
(hoặc trong câu hỏi) không. Đây là "critic" nhẹ và xác định (docs/ai/12 mục 1): chỉ ghi số liệu `mm_ai_answer_numbers_total`, vì số suy ra
(tổng, trung bình, làm tròn "1,5 triệu") có thể hợp lệ mà không có nguyên văn trong dữ liệu; chặn sẽ chặn nhầm. Dùng tỷ lệ theo thời gian và
theo persona để thấy mô hình có bịa số hay không, rồi mới quyết định có cần chặn."""
from __future__ import annotations

from ai_agent.guard.numbers import extract, missing

MIN_DIGITS = 4  # chỉ xét số từ 4 chữ số trở lên (giá, số lượng, mã đơn lớn); số nhỏ quá hay là thứ tự, phần trăm, ngày


def numbers_in(text: str) -> set[str]:
    """Các con số lớn trong văn bản, mỗi số một dạng chuẩn đại diện (để đếm/so sánh trong test)."""
    return {max(fs, key=len) for fs in extract(text, MIN_DIGITS)}


def ungrounded_numbers(answer: str, *sources: str) -> tuple[int, int]:
    """(số con số lớn trong câu trả lời, số con số không có trong các nguồn). Nguồn: kết quả công cụ, câu hỏi người dùng."""
    return missing(answer, *sources, min_digits=MIN_DIGITS)
