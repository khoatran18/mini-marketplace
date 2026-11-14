"""Số liệu Prometheus của ai-agent (endpoint GET /metrics). Registry riêng, không lẫn metric mặc định của process.
Nhãn chỉ gồm giá trị có tập hữu hạn (persona, trạng thái, tên tool): không bao giờ đưa user_id, run_id hay nội dung chat vào nhãn."""
from __future__ import annotations

from prometheus_client import CollectorRegistry, Counter, Histogram, generate_latest, CONTENT_TYPE_LATEST

REGISTRY = CollectorRegistry()

RUNS = Counter("mm_ai_runs_total", "Số lượt chat kết thúc, theo persona và trạng thái (ok, error, timeout, budget, cancelled, session_expired)",
               ["persona", "status"], registry=REGISTRY)
RUN_SECONDS = Histogram("mm_ai_run_duration_seconds", "Thời gian một lượt chat (giây)", ["persona"],
                        buckets=(1, 2, 5, 10, 20, 30, 60, 120), registry=REGISTRY)
TOKENS = Counter("mm_ai_tokens_total", "Token mô hình đã dùng, theo persona và hướng (input, output)", ["persona", "direction"], registry=REGISTRY)
TOOL_CALLS = Counter("mm_ai_tool_calls_total", "Số lần agent gọi tool, theo tên và kết quả (ok, error)", ["tool", "result"], registry=REGISTRY)
REJECTED = Counter("mm_ai_rejected_total", "Yêu cầu chat bị từ chối trước khi chạy (daily_token_limit)", ["persona", "reason"], registry=REGISTRY)
LLM_SECONDS = Histogram("mm_ai_llm_call_seconds", "Độ trễ một lần gọi mô hình (giây), theo profile", ["profile"],
                        buckets=(0.5, 1, 2, 5, 10, 20, 40, 80), registry=REGISTRY)
LLM_ERRORS = Counter("mm_ai_llm_errors_total", "Lần gọi mô hình lỗi, theo profile", ["profile"], registry=REGISTRY)


def render() -> tuple[bytes, str]:
    return generate_latest(REGISTRY), CONTENT_TYPE_LATEST
