"""Số liệu Prometheus của ai-agent (endpoint GET /metrics). Registry riêng, không lẫn metric mặc định của process.
Nhãn chỉ gồm giá trị có tập hữu hạn (persona, trạng thái, tên tool): không bao giờ đưa user_id, run_id hay nội dung chat vào nhãn."""
from __future__ import annotations

from prometheus_client import CollectorRegistry, Counter, Gauge, Histogram, generate_latest, CONTENT_TYPE_LATEST

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
ANSWER_NUMBERS = Counter("mm_ai_answer_numbers_total", "Con số lớn (4+ chữ số) trong câu trả lời chat, theo persona và kết quả: grounded = có trong kết quả công cụ hoặc câu hỏi, ungrounded = không có (guard/grounding.py; chỉ đo, không chặn)", ["persona", "result"], registry=REGISTRY)
LLM_FALLBACK = Counter("mm_ai_llm_fallback_total", "Lần một profile mô hình bị bỏ qua để thử profile kế tiếp, theo profile và lý do (loại lỗi hoặc circuit_open)", ["profile", "reason"], registry=REGISTRY)


# ---- RCA (rca/): nhãn chỉ có tập hữu hạn (trigger auto|manual, kết cục trong rca/state.OUTCOMES); không bao giờ có id sự cố hay nội dung ----
RCA_STARTED = Counter("mm_ai_rca_runs_started_total", "Phiên RCA đã bắt đầu, theo cách bắt đầu (auto = bộ theo dõi, manual = operator bấm)", ["trigger"], registry=REGISTRY)
RCA_FINISHED = Counter("mm_ai_rca_runs_finished_total", "Phiên RCA đã kết thúc, theo kết cục (recovered, escalated, expired, rejected, error...)", ["outcome"], registry=REGISTRY)
RCA_RUN_SECONDS = Histogram("mm_ai_rca_run_duration_seconds", "Từ lúc bắt đầu đến lúc kết thúc một phiên RCA (giây, GỒM thời gian chờ người)", registry=REGISTRY,
                            buckets=(30, 60, 120, 300, 600, 1800, 3600, 7200, 21600))
RCA_RUN_TOKENS = Histogram("mm_ai_rca_run_tokens", "Tổng token (vào + ra) của một phiên RCA đã kết thúc", registry=REGISTRY,
                           buckets=(5_000, 10_000, 20_000, 40_000, 80_000, 160_000, 320_000))
RCA_COST_USD = Counter("mm_ai_rca_cost_usd_total", "Chi phí ước tính (USD) của các phiên RCA đã kết thúc, theo đơn giá trong config (rca.usd_per_mtok_*; 0 = không tính)", registry=REGISTRY)
RCA_TOKEN_CAP_HITS = Counter("mm_ai_rca_token_cap_hits_total", "Số lần bộ theo dõi bỏ qua việc tự chạy phiên mới vì chạm trần token 24 giờ (rca.daily_tokens)", registry=REGISTRY)
RCA_WAITING = Gauge("mm_ai_rca_runs_waiting", "Số phiên RCA đang chờ người, theo trạng thái (waiting_confirm, waiting_approval)", ["status"], registry=REGISTRY)
# Nhịp tim của bộ theo dõi: có nhãn nên chỉ xuất hiện sau tick đầu tiên (RCA tắt thì không có mẫu nào, cảnh báo không bắn nhầm).
RCA_WATCHER_LAST_TICK = Gauge("mm_ai_rca_watcher_last_tick_timestamp_seconds", "Thời điểm (epoch giây) tick thành công gần nhất của bộ theo dõi RCA", ["watcher"], registry=REGISTRY)
RCA_WATCHER_TICKS = Counter("mm_ai_rca_watcher_ticks_total", "Tick của bộ theo dõi RCA, theo kết quả (ok, error)", ["result"], registry=REGISTRY)


def render() -> tuple[bytes, str]:
    return generate_latest(REGISTRY), CONTENT_TYPE_LATEST
