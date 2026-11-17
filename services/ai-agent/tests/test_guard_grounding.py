"""Đo mức câu trả lời bám dữ liệu công cụ (guard/grounding.py) và lỗi nhà cung cấp hiện đúng cho người dùng."""
from ai_agent.guard.grounding import numbers_in, ungrounded_numbers


def test_numbers_are_normalised_across_separators_and_small_numbers_ignored():
    assert numbers_in("giá 1.250.000đ, còn 12 cái, mã 4567") == {"1250000", "4567"}
    assert numbers_in("1,250,000 VND") == {"1250000"}


def test_answer_numbers_found_in_tools_or_question_are_grounded_others_are_not():
    tools = '{"price": 1250000, "order_id": 90210}'
    assert ungrounded_numbers("Giá là 1.250.000đ cho đơn 90210", tools, "") == (2, 0)
    assert ungrounded_numbers("Giá là 1.300.000đ", tools, "") == (1, 1)         # số bịa
    assert ungrounded_numbers("Bạn hỏi đơn 77123, mình tìm thấy 90210", tools, "đơn 77123 đâu") == (2, 0)  # số lấy từ câu hỏi
    assert ungrounded_numbers("Không có số lớn nào, chỉ 3 món", tools, "") == (0, 0)
