"""LlmBrain với mô hình GIẢ theo thứ tự và gateway giả: bằng chứng lấy từ kết quả công cụ thật, JSON chặt, verify xác định, agent chỉ đọc."""
import json

import pytest
from langgraph.checkpoint.memory import MemorySaver
from langgraph.types import Command

from ai_agent.rca.brain import LlmBrain, parse_json, _Choice
from ai_agent.rca.budget import Budget
from ai_agent.rca.graph import build_graph
from rca_fakes import CATALOG, INCIDENT, FakeOps, brain_env, ops_gateway

COLLECT = [{"tool_calls": [{"name": "ops_get_playbook", "args": {"id": "service-unavailable"}}, {"name": "ops_metrics_query", "args": {"q": "up"}}]},
           {"content": "XONG"}]
HYP = {"hypotheses": [{"cause": "payment-service ở 0 replica", "confidence": 0.85, "evidence_ids": ["e2"], "refuted_by": "replica > 0"}]}
CHOICE = {"action_id": "restart_service", "params": {"service": "payment-service"}, "rationale": "up=0 (e2)", "evidence_ids": ["e2"]}


def state(**kw):
    return {"incident": INCIDENT, "round": 1, "evidence": [], "queries": 0, "model_calls": 0, **kw}


def test_parse_json_tolerates_prose_and_fences_but_not_garbage():
    assert parse_json('Đây:\n```json\n{"action_id":"x"}\n```', _Choice).action_id == "x"
    with pytest.raises(ValueError):
        parse_json("không có gì", _Choice)


async def test_collect_records_real_tool_results_not_model_claims_and_detects_playbook():
    rt, persona, ctx, _ = brain_env(COLLECT)
    r = await LlmBrain(rt, persona, ctx, Budget()).collect(state())
    assert [e["source"] for e in r["evidence"]] == ["ops_get_playbook", "ops_metrics_query"]
    assert "up=0" in r["evidence"][1]["summary"] and r["queries"] == 2 and r["playbook"] == "service-unavailable" and r["model_calls"] == 2


async def test_collect_skips_failed_tool_calls_but_still_counts_them_as_queries():
    rt, persona, ctx, _ = brain_env([{"tool_calls": [{"name": "ops_metrics_query", "args": {"q": "up"}}]}, {"content": "XONG"}], gateway=ops_gateway(metric_status=500))
    r = await LlmBrain(rt, persona, ctx, Budget()).collect(state())
    assert r["evidence"] == [] and r["queries"] == 1  # lỗi công cụ không thành "bằng chứng", nhưng vẫn tốn một truy vấn


async def test_rca_agent_gets_no_proposal_tool_even_though_the_operator_token_has_one():
    names: list = []
    rt, persona, ctx, _ = brain_env([{"content": "XONG"}], tool_names=names)
    await rt.agent_for(persona, ctx)
    assert "ops_action" not in names and "ops_metrics_query" in names
    assert any(a.name == "ops_action" for a in (await rt.specs_for(ctx))[1])  # token operator CÓ quyền; chính persona rca chặn


async def test_hypothesize_choose_report_parse_json_and_retry_once_on_bad_format():
    script = [{"content": "tôi nghĩ là do mạng"}, {"content": json.dumps(HYP)}, {"content": json.dumps(CHOICE)}, {"content": json.dumps({"summary": "ok", "ready_to_close": True, "reason": "r"})}]
    rt, persona, ctx, _ = brain_env(script)
    b = LlmBrain(rt, persona, ctx, Budget())
    s = state(evidence=[{"id": "e1", "source": "a", "args": {}, "summary": "x"}, {"id": "e2", "source": "b", "args": {}, "summary": "up=0"}])
    h = await b.hypothesize(s)
    assert h["model_calls"] == 2 and h["hypotheses"][0]["confidence"] == 0.85  # lần đầu sai định dạng, lần hai đúng
    c = await b.choose_action({**s, "hypotheses": h["hypotheses"]}, CATALOG)
    assert c["action_id"] == "restart_service" and c["params"] == {"service": "payment-service"}
    rep = await b.report({**s, "outcome": "recovered"})
    assert rep["ready_to_close"] is True


async def test_bad_json_twice_raises_so_the_graph_ends_in_error():
    rt, persona, ctx, _ = brain_env([{"content": "a"}, {"content": "b"}])
    with pytest.raises(ValueError):
        await LlmBrain(rt, persona, ctx, Budget()).hypothesize(state(evidence=[{"id": "e1", "source": "a", "args": {}, "summary": "x"}]))


async def test_verify_reruns_exactly_the_cited_queries_without_asking_the_model_what_to_run():
    calls: list = []
    rt, persona, ctx, model = brain_env([{"content": json.dumps({"recovered": True, "note": "up=1"})}], gateway=ops_gateway(calls, metric="up=1"))
    ev = [{"id": "e1", "source": "ops_get_playbook", "args": {"id": "service-unavailable"}, "summary": "pb"},
          {"id": "e2", "source": "ops_metrics_query", "args": {"q": "up"}, "summary": "up=0"}]
    s = state(evidence=ev, hypotheses=[{"id": "h1", "cause": "c", "confidence": 0.9, "evidence_ids": ["e2"], "refuted_by": ""}],
              draft={"action_id": "restart_service", "params": {"service": "payment-service"}})
    r = await LlmBrain(rt, persona, ctx, Budget()).verify(s)
    gw = [c for c in calls if c[1] != "/ai/tools"]
    assert gw == [("GET", "/ops/metrics", {"q": "up"})]  # chỉ truy vấn đã dẫn cho giả thuyết hàng đầu
    assert r["recovered"] is True and r["queries"] == 1 and "up=0" in json.dumps(model.seen) and "up=1" in json.dumps(model.seen)


async def test_full_run_graph_plus_llm_brain_with_fake_model():
    script = [*COLLECT, {"content": json.dumps(HYP)}, {"content": json.dumps(CHOICE)},
              {"content": json.dumps({"recovered": True, "note": "up=1"})}, {"content": json.dumps({"summary": "payment-service đã chạy lại", "ready_to_close": True, "reason": "up=1 sau restart"})}]
    rt, persona, ctx, _ = brain_env(script)
    g = build_graph(LlmBrain(rt, persona, ctx, Budget()), FakeOps(), Budget(), checkpointer=MemorySaver())
    c = {"configurable": {"thread_id": "t"}}
    r = await g.ainvoke({"incident": INCIDENT}, c)
    assert r["__interrupt__"][0].value["draft"]["action_id"] == "restart_service"
    await g.ainvoke(Command(resume={"event": "confirmed", "proposal_id": 12}), c)
    r = await g.ainvoke(Command(resume={"event": "executed"}), c)
    assert r["outcome"] == "recovered" and r["report"]["ready_to_close"] is True and r["model_calls"] == 6


async def test_json_requests_carry_the_run_in_the_contextvar_so_the_provider_gets_its_session_header():
    """OpenCode trả 400 nếu thiếu x-opencode-session; header lấy từ contextvar lúc gửi request (llm/providers/openai.py)."""
    from ai_agent.runtime.context import peek_run

    rt, persona, ctx, model = brain_env([{"content": json.dumps(HYP)}])
    seen = []
    orig = model._generate

    def spy(*a, **kw):
        run = peek_run()
        seen.append(run.run_id if run else None)
        return orig(*a, **kw)

    object.__setattr__(model, "_generate", spy)
    await LlmBrain(rt, persona, ctx, Budget()).hypothesize(state(evidence=[{"id": "e1", "source": "a", "args": {}, "summary": "x"}]))
    assert seen == [ctx.run_id] and peek_run() is None  # có trong lúc gọi, gỡ ngay sau đó


async def test_each_step_reports_its_own_token_use_and_feeds_the_rca_counter():
    from ai_agent import metrics
    from ai_agent.llm.metrics import attach

    rt, persona, ctx, model = brain_env([{"content": json.dumps(HYP)}])
    attach(model, "rca")  # bản chạy thật gắn đo lường lúc dựng mô hình; fake_builder thì không
    before = metrics.TOKENS.labels("rca", "input")._value.get()
    r = await LlmBrain(rt, persona, ctx, Budget()).hypothesize(state(evidence=[{"id": "e1", "source": "a", "args": {}, "summary": "x"}]))
    assert r["input_tokens"] > 0 and r["output_tokens"] > 0
    assert (ctx.input_tokens, ctx.output_tokens) == (r["input_tokens"], r["output_tokens"])
    assert metrics.TOKENS.labels("rca", "input")._value.get() == before + r["input_tokens"]
