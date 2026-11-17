"""Kiểm xác định kết luận RCA bám bằng chứng (rca/grounding.py): không LLM."""
from rca_fakes import INCIDENT, FakeOps, ScriptedBrain
from test_rca_graph import make, start, waiting

from ai_agent.rca.grounding import checkable, hypothesis_supported, ungrounded_params

EV = [{"id": "e1", "source": "ops_system_health", "args": {}, "summary": "payment-service replicas 0/1, kafka ready"},
      {"id": "e2", "source": "ops_metrics_query", "args": {"name": "kafka_consumer_lag"}, "summary": "lag 5000 on order.events"}]


def test_checkable_picks_hyphenated_names_components_and_numbers_only():
    assert checkable("payment-service dừng vì Kafka lag 5000") == ["payment-service", "kafka", "5000"]
    assert checkable("hệ thống chậm bất thường") == []


def test_hypothesis_is_supported_only_when_cited_evidence_mentions_what_it_claims():
    assert hypothesis_supported("payment-service dừng", [EV[0]]) is True
    assert hypothesis_supported("order-service dừng", [EV[0]]) is False      # nêu service mà bằng chứng không nhắc
    assert hypothesis_supported("lag 9000 trên order.events", [EV[1]]) is False  # sai con số
    assert hypothesis_supported("hệ thống chậm", [EV[0]]) is None             # không có gì để đối chiếu: không chê


def test_params_must_appear_in_evidence_or_incident():
    inc = {"component": "payment-service", "labels": {}}
    assert ungrounded_params({"service": "payment-service", "limit": 3}, EV, inc) == []
    assert ungrounded_params({"service": "order-service"}, EV, inc) == ["service"]
    assert ungrounded_params({"since": "2026-11-01", "n": 5}, EV, inc) == []  # ngày và số không bị xét


async def test_unsupported_hypothesis_is_demoted_and_the_graph_escalates_instead_of_acting():
    hyp = [{"hypotheses": [{"cause": "order-service dừng", "confidence": 0.9, "evidence_ids": ["e1"], "refuted_by": "x"}], "model_calls": 1}]
    g, _ = make(ScriptedBrain(collect=[{"evidence": [{"source": "ops_system_health", "args": {}, "summary": EV[0]["summary"]}], "queries": 1, "model_calls": 1}], hypothesize=hyp))
    r = await start(g)
    assert waiting(r) == "confirm" and r["__interrupt__"][0].value["draft"]["action_id"] == "escalate_to_human"  # tin cậy bị hạ xuống 0.3
    h = r["hypotheses"][0]
    assert h["grounded"] is False and h["confidence"] == 0.3
    assert any("không được bằng chứng đã dẫn ủng hộ" in t["note"] for t in r["timeline"])


async def test_invented_evidence_ids_are_dropped_and_reported():
    hyp = [{"hypotheses": [{"cause": "payment-service dừng", "confidence": 0.9, "evidence_ids": ["e1", "e99"], "refuted_by": "x"}], "model_calls": 1}]
    g, _ = make(ScriptedBrain(hypothesize=hyp))
    r = await start(g)
    h = r["hypotheses"][0]
    assert h["evidence_ids"] == ["e1"] and h["bogus_refs"] == ["e99"] and h["grounded"] is True
    assert any("không tồn tại" in t["note"] for t in r["timeline"])


async def test_a_target_the_model_invented_never_reaches_a_draft():
    choose = [{"action_id": "restart_service", "params": {"service": "order-service"}, "rationale": "x", "evidence_ids": ["e1"], "model_calls": 1}]
    g, _ = make(ScriptedBrain(choose=choose))  # sự cố và bằng chứng chỉ nói về payment-service
    r = await start(g, component="payment-service", summary="payment-service down")
    d = r["__interrupt__"][0].value["draft"]
    assert d["action_id"] == "escalate_to_human" and "không có trong bằng chứng" in d["rationale"]


async def test_a_param_that_is_in_the_evidence_is_allowed():
    g, _ = make()
    r = await start(g)
    assert r["__interrupt__"][0].value["draft"]["action_id"] == "restart_service"
    assert INCIDENT["component"] == "payment-service" and FakeOps is not None
