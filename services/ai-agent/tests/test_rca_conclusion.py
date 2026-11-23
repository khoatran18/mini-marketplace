"""RCA: kết luận tạm khi dừng non (a), lý do lỗi 400 cho persona không ẩn nội bộ (b), ngân sách chỉ tính truy vấn thành công (c)."""
import httpx

from ai_agent.rca.budget import Budget
from ai_agent.rca.conclusion import interim_conclusion
from ai_agent.runtime.context import RunContext, reset_run, set_run
from ai_agent.tools.gateway import GatewayClient
from ai_agent.tools.registry import make_tool, parse_specs

from helpers import agent_token
from rca_fakes import ScriptedBrain
from test_rca_graph import make, start, waiting

EV = [{"id": "e1", "source": "ops_metrics_query", "args": {"name": "reconcile_ratio"}, "summary": "ratio 0.951"},
      {"id": "e2", "source": "ops_system_health", "args": {}, "summary": "all ready"}]
HYP = [{"id": "h1", "cause": "Lệch thật giữa ClickHouse và Postgres", "confidence": 0.45, "evidence_ids": ["e1", "e2"], "refuted_by": "lag = 0", "grounded": True},
       {"id": "h2", "cause": "ClickHouse lỗi ghi", "confidence": 0.3, "evidence_ids": [], "refuted_by": ""}]


# ---- (a) kết luận tạm
def test_interim_conclusion_names_top_hypothesis_evidence_reason_and_others():
    t = interim_conclusion({"hypotheses": HYP, "evidence": EV, "stop_reason": "model_calls"}, "độ tin cậy thấp")
    assert "Lệch thật giữa ClickHouse và Postgres" in t and "45%" in t and "hết lượt gọi mô hình" in t
    assert "e1 ops_metrics_query" in t and "reconcile_ratio" in t and "ClickHouse lỗi ghi (30%)" in t and "Chưa khớp: lag = 0" in t


def test_interim_conclusion_without_hypotheses_still_says_where_it_stopped():
    t = interim_conclusion({"hypotheses": [], "evidence": EV, "stop_reason": "queries"})
    assert "chưa lập được giả thuyết" in t and "hết lượt truy vấn" in t and "2 bằng chứng" in t


async def test_budget_stop_puts_the_conclusion_in_the_escalation_draft():
    coll = [{"evidence": EV, "queries": 2, "model_calls": 20}]
    hyp = [{"hypotheses": [{"cause": "Lệch thật giữa ClickHouse và Postgres", "confidence": 0.45, "evidence_ids": ["e1"], "refuted_by": ""}], "model_calls": 1}]
    g, _ = make(ScriptedBrain(collect=coll, hypothesize=hyp, choose=[{"action_id": "", "params": {}, "model_calls": 1}]), budget=Budget(max_model_calls=12))
    r = await start(g)
    assert waiting(r) == "confirm"
    d = r["__interrupt__"][0].value["draft"]
    assert d["action_id"] == "escalate_to_human" and d["escalated"]
    assert "Kết luận tạm" in d["rationale"] and "Lệch thật giữa ClickHouse và Postgres" in d["rationale"] and "hết lượt gọi mô hình" in d["rationale"]


# ---- (b) lý do lỗi 400
SPEC = parse_specs({"tools": [{"name": "ops_metrics_query", "method": "GET", "path": "/ops/queries/:name", "description": "q",
                               "params": [{"name": "name", "in": "path", "type": "string", "required": True}, {"name": "service", "in": "query", "type": "string"}]}]})[0]


async def _call(reveal: bool) -> str:
    def handler(req: httpx.Request) -> httpx.Response:
        return httpx.Response(400, json={"error": "service: giá trị hợp lệ là api-gateway, order-service"})
    gw = GatewayClient("http://gw", 2.0, client=httpx.AsyncClient(transport=httpx.MockTransport(handler)))
    ctx = RunContext(token=agent_token(role="operator"), run_id="run-0000-1111", request_id="req-1", user_id=1, role="operator")
    tok = set_run(ctx)
    try:
        return await make_tool(SPEC, gw, tool_timeout=2, max_chars=2000, max_items=10, reveal_route=reveal).ainvoke({"name": "outbox_dead", "service": "analytics-service"})
    finally:
        reset_run(tok)


async def test_operator_sees_why_a_400_happened_so_it_can_fix_the_param():
    out = await _call(True)
    assert out.startswith("[lỗi công cụ] tham số không hợp lệ (HTTP 400)") and "giá trị hợp lệ là api-gateway, order-service" in out


async def test_hidden_internals_personas_never_see_the_gateway_error_text():
    out = await _call(False)
    assert "giá trị hợp lệ" not in out and out == "[lỗi công cụ] tham số không hợp lệ (HTTP 400)"
