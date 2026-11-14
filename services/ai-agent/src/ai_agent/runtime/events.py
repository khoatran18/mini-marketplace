"""Sự kiện SSE gửi về gateway/UI. Hợp đồng: docs/ai/05_AI_DRAFT_ORIGINAL.md §3 + docs/ai/08 P2. Mỗi sự kiện có run_id."""
from __future__ import annotations

import json
from typing import Any

START, TOKEN, TOOL_CALL, TOOL_RESULT, CARD, CITATION, ACTION_PROPOSAL, ERROR, DONE = (
    "start", "token", "tool_call", "tool_result", "card", "citation", "action_proposal", "error", "done",
)

# Nhãn thân thiện hiện cho người dùng khi agent đang gọi tool (không hiện tham số/kết quả thô).
TOOL_LABELS = {
    "search_products": "Đang tìm sản phẩm…", "get_product": "Đang xem sản phẩm…", "get_product_stock": "Đang kiểm tra tồn kho…",
    "list_categories": "Đang xem danh mục…", "get_cart": "Đang xem giỏ hàng…", "list_my_orders": "Đang tra cứu đơn hàng…",
    "count_my_orders": "Đang đếm đơn hàng…", "get_my_order": "Đang xem đơn hàng…", "list_payment_methods": "Đang xem phương thức thanh toán…",
    "list_my_products": "Đang xem sản phẩm của shop…", "get_inventory_ledger": "Đang xem sổ kho…", "list_low_stock": "Đang kiểm tra hàng sắp hết…",
    "list_store_orders": "Đang xem đơn của shop…", "count_store_orders": "Đang đếm đơn của shop…", "get_store_order": "Đang xem đơn của shop…",
    "analytics_summary": "Đang xem báo cáo…", "analytics_timeseries": "Đang xem báo cáo theo thời gian…",
    "analytics_top_products": "Đang xem sản phẩm bán chạy…", "analytics_funnel": "Đang xem phễu bán hàng…", "analytics_low_stock": "Đang xem báo cáo tồn kho…",
}


# Tên nguồn hiện cho người dùng dưới câu trả lời. Không bao giờ hiện đường dẫn API hay tên công cụ nội bộ.
SOURCE_LABELS = {
    "search_products": "Tìm kiếm sản phẩm", "get_product": "Trang sản phẩm", "get_product_stock": "Tồn kho", "list_categories": "Danh mục",
    "get_cart": "Giỏ hàng", "list_my_orders": "Đơn hàng của bạn", "count_my_orders": "Đơn hàng của bạn", "get_my_order": "Đơn hàng của bạn",
    "list_payment_methods": "Phương thức thanh toán", "list_my_products": "Sản phẩm của shop", "get_inventory_ledger": "Sổ kho",
    "list_low_stock": "Tồn kho của shop", "list_store_orders": "Đơn của shop", "count_store_orders": "Đơn của shop", "get_store_order": "Đơn của shop",
    "analytics_summary": "Báo cáo của shop", "analytics_timeseries": "Báo cáo của shop", "analytics_top_products": "Báo cáo của shop",
    "analytics_funnel": "Báo cáo của shop", "analytics_low_stock": "Báo cáo của shop",
}


def source_label(tool: str) -> str:
    return SOURCE_LABELS.get(tool, "Dữ liệu cửa hàng")


def label_for(tool: str) -> str:
    if tool.startswith("propose_"):
        return "Đang soạn đề xuất…"
    return TOOL_LABELS.get(tool, "Đang tra cứu…")


def make(name: str, run_id: str, **data: Any) -> dict[str, Any]:
    """Một sự kiện dạng dict nội bộ: {"event": name, "data": {..., "run_id": ...}}."""
    return {"event": name, "data": {"run_id": run_id, **data}}


def sse(event: dict[str, Any]) -> str:
    """Định dạng một sự kiện nội bộ thành khối text/event-stream."""
    return f"event: {event['event']}\ndata: {json.dumps(event['data'], ensure_ascii=False, default=str)}\n\n"
