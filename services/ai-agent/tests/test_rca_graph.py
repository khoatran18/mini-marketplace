"""Đồ thị RCA bằng Brain/Ops giả: mọi nhánh, ngân sách, hai bước chờ người, và những việc agent KHÔNG được làm."""
import itertools

import pytest
from langgraph.checkpoint.memory import MemorySaver
from langgraph.types import Command

from ai_agent.rca.budget import Budget
from ai_agent.rca.graph import build_graph
from rca_fakes import INCIDENT, FakeOps, ScriptedBrain


def make(brain=None, ops=None, budget=None, clock=None):
    brain = brain or ScriptedBrain()
    g = build_graph(brain, ops or FakeOps(), budget or Budget(), checkpointer=MemorySaver(), clock=clock or itertools.count().__next__)
    return g, brain


def cfg(n="inc-7"):
    return {"configurable": {"thread_id": n}}


async def start(g, c=None, **inc):
    return await g.ainvoke({"incident": {**INCIDENT, **inc}}, c or cfg())


def waiting(r):
    return r["__interrupt__"][0].value["waiting"]


async def test_happy_path_two_waits_verify_and_report():
    g, brain = make()
    r = await start(g)
    assert waiting(r) == "confirm" and r["__interrupt__"][0].value["draft"]["action_id"] == "restart_service"
    r = await g.ainvoke(Command(resume={"event": "confirmed", "proposal_id": 31}), cfg())
    assert waiting(r) == "approval"
    r = await g.ainvoke(Command(resume={"event": "executed"}), cfg())
    assert r["outcome"] == "recovered" and r["report"]["ready_to_close"] is True
    assert [n["node"] for n in r["timeline"]][:3] == ["collect", "hypothesize", "decide"]
    assert brain.calls == ["collect", "hypothesize", "choose", "verify", "report"]


async def test_low_confidence_loops_then_proposes_with_what_it_has():
    low = [{"hypotheses": [{"cause": "không rõ", "confidence": 0.5, "evidence_ids": ["e1"], "refuted_by": ""}], "model_calls": 1}]
    coll = [{"evidence": [{"source": f"t{i}", "args": {"i": i}, "summary": "x"}], "queries": 1, "model_calls": 1} for i in range(5)]
    g, brain = make(ScriptedBrain(collect=coll, hypothesize=low))
    r = await start(g)
    assert brain.calls.count("collect") == 3  # tối đa 3 vòng
    assert waiting(r) == "confirm"  # 0.5 >= 0.4 nên vẫn đề xuất hành động thật theo những gì có


async def test_very_low_confidence_escalates_to_a_human_and_skips_approval_and_verify():
    vlow = [{"hypotheses": [{"cause": "?", "confidence": 0.2, "evidence_ids": ["e1"], "refuted_by": ""}], "model_calls": 1}]
    g, brain = make(ScriptedBrain(hypothesize=vlow))
    r = await start(g)
    assert r["__interrupt__"][0].value["draft"]["action_id"] == "escalate_to_human"
    r = await g.ainvoke(Command(resume={"event": "confirmed", "proposal_id": 5}), cfg())
    assert r["outcome"] == "escalated" and "verify" not in brain.calls and r["report"]["ready_to_close"] is False


@pytest.mark.parametrize("choice,why", [
    ({"action_id": "format_disk", "params": {}}, "không có trong sổ"),
    ({"action_id": "restart_service", "params": {"service": "hacker-service"}}, "tham số không hợp lệ"),
    ({"action_id": "scale_service", "params": {"service": "payment-service", "replicas": 99}}, "tham số không hợp lệ"),
    ({"action_id": "requeue_outbox", "params": {"service": "order-service"}}, "tham số không hợp lệ"),
    ({"action_id": "", "params": {}}, "không có hành động nào"),
])
async def test_bad_or_unknown_action_becomes_escalation_never_an_invented_action(choice, why):
    g, _ = make(ScriptedBrain(choose=[{**choice, "rationale": "x", "evidence_ids": ["e1"], "model_calls": 1}]))
    r = await start(g)
    d = r["__interrupt__"][0].value["draft"]
    assert d["action_id"] == "escalate_to_human" and why in d["rationale"]


async def test_operator_does_not_confirm_or_approval_rejected_or_expired_end_the_run():
    for resume, wait, outcome in [({"event": "dismissed"}, 1, "dismissed"), ({"event": "expired"}, 1, "expired")]:
        g, _ = make()
        c = cfg(outcome)
        await start(g, c)
        r = await g.ainvoke(Command(resume=resume), c)
        assert r["outcome"] == outcome and r["report"]["ready_to_close"] is False
    for ev, outcome in (("rejected", "rejected"), ("expired", "expired")):
        g, brain = make()
        c = cfg("a" + ev)
        await start(g, c)
        await g.ainvoke(Command(resume={"event": "confirmed", "proposal_id": 1}), c)
        r = await g.ainvoke(Command(resume={"event": ev}), c)
        assert r["outcome"] == outcome and "verify" not in brain.calls
        assert r["failed"][0]["action_id"] == "restart_service"


async def test_verify_failure_runs_one_more_round_then_reports_to_a_human():
    bad = [{"recovered": False, "note": "vẫn 0/1", "queries": 1, "model_calls": 1}]
    coll = [{"evidence": [{"source": f"t{i}", "args": {"i": i}, "summary": "x"}], "queries": 1, "model_calls": 1} for i in range(9)]
    choose = [{"action_id": "restart_service", "params": {"service": "payment-service"}, "rationale": "a", "evidence_ids": ["e1"], "model_calls": 1},
              {"action_id": "scale_service", "params": {"service": "payment-service", "replicas": 2}, "rationale": "b", "evidence_ids": ["e1"], "model_calls": 1}]
    g, brain = make(ScriptedBrain(collect=coll, verify=bad, choose=choose))
    await start(g)
    await g.ainvoke(Command(resume={"event": "confirmed", "proposal_id": 1}), cfg())
    r = await g.ainvoke(Command(resume={"event": "executed"}), cfg())
    assert waiting(r) == "confirm" and r["__interrupt__"][0].value["draft"]["action_id"] == "scale_service"  # vòng thứ hai, đề xuất KHÁC
    await g.ainvoke(Command(resume={"event": "confirmed", "proposal_id": 2}), cfg())
    r = await g.ainvoke(Command(resume={"event": "executed"}), cfg())
    assert r["outcome"] == "not_recovered" and r["report"]["ready_to_close"] is False
    assert brain.calls.count("verify") == 2 and brain.calls.count("collect") == 2


async def test_does_not_repeat_a_failed_action_without_new_evidence():
    bad = [{"recovered": False, "note": "x", "queries": 0, "model_calls": 1}]
    same = [{"evidence": [{"source": "ops_list_incidents", "args": {}, "summary": "service down"}], "queries": 0, "model_calls": 0}]  # không có bằng chứng mới
    g, _ = make(ScriptedBrain(collect=same, verify=bad))  # choose luôn trả restart payment-service
    await start(g)
    await g.ainvoke(Command(resume={"event": "confirmed", "proposal_id": 1}), cfg())
    r = await g.ainvoke(Command(resume={"event": "executed"}), cfg())
    d = r["__interrupt__"][0].value["draft"]
    assert d["action_id"] == "escalate_to_human" and "chưa có bằng chứng mới" in d["rationale"]


async def test_budget_model_calls_stops_collecting_but_still_proposes():
    coll = [{"evidence": [{"source": "a", "args": {}, "summary": "x"}], "queries": 1, "model_calls": 20}]
    g, brain = make(ScriptedBrain(collect=coll), budget=Budget(max_model_calls=12))
    r = await start(g)
    assert brain.calls.count("collect") == 1 and waiting(r) == "confirm"


async def test_budget_time_stops_the_run_when_no_evidence_at_all():
    t = iter([0.0, 1000.0, 1000.0, 1000.0, 1000.0])  # lần đọc đồng hồ đầu = 0, sau đó đã quá max_seconds
    g, _ = make(ScriptedBrain(collect=[{"evidence": [], "queries": 0, "model_calls": 0}]), clock=lambda: next(t, 1000.0))
    r = await start(g)
    assert r["outcome"] in ("budget", "no_evidence") and "__interrupt__" not in r


async def test_too_many_pending_proposals_blocks_a_new_one():
    g, _ = make(ops=FakeOps(pending=2))
    r = await start(g)
    assert r["outcome"] == "blocked" and "__interrupt__" not in r


async def test_brain_error_ends_in_a_report_not_a_crash():
    g, _ = make(ScriptedBrain(raises="collect"))
    r = await start(g)
    assert r["outcome"] == "error" and r["report"]["ready_to_close"] is False


async def test_report_cannot_claim_ready_to_close_unless_verified_recovered():
    liar = [{"summary": "ổn rồi", "ready_to_close": True, "reason": "tôi nghĩ vậy", "model_calls": 1}]
    vlow = [{"hypotheses": [{"cause": "?", "confidence": 0.1, "evidence_ids": [], "refuted_by": ""}], "model_calls": 1}]
    g, _ = make(ScriptedBrain(report=liar, hypothesize=vlow))
    await start(g)
    r = await g.ainvoke(Command(resume={"event": "confirmed", "proposal_id": 3}), cfg())
    assert r["report"]["ready_to_close"] is False


async def test_hypotheses_are_capped_sorted_and_unbacked_ones_lose_confidence():
    many = [{"hypotheses": [{"cause": f"c{i}", "confidence": 0.1 * i, "evidence_ids": ["e1"], "refuted_by": ""} for i in range(1, 6)]
             + [{"cause": "bịa", "confidence": 0.99, "evidence_ids": ["e99"], "refuted_by": ""}], "model_calls": 1}]
    g, _ = make(ScriptedBrain(hypothesize=many))
    await start(g)
    h = (await g.aget_state(cfg())).values["hypotheses"]
    assert len(h) == 3 and h[0]["confidence"] >= h[1]["confidence"] >= h[2]["confidence"]
    assert all(x["cause"] != "bịa" or x["confidence"] <= 0.3 for x in h) and [x["id"] for x in h] == ["h1", "h2", "h3"]


async def test_run_survives_a_restart_state_is_in_the_checkpoint():
    saver = MemorySaver()
    g1 = build_graph(ScriptedBrain(), FakeOps(), Budget(), checkpointer=saver)
    await g1.ainvoke({"incident": INCIDENT}, cfg())
    g2 = build_graph(ScriptedBrain(), FakeOps(), Budget(), checkpointer=saver)  # "tiến trình mới" cùng checkpoint
    r = await g2.ainvoke(Command(resume={"event": "confirmed", "proposal_id": 9}), cfg())
    assert waiting(r) == "approval" and r["proposal_id"] == 9


def test_graph_has_no_node_that_approves_executes_or_closes():
    g, _ = make()
    nodes = set(g.get_graph().nodes) - {"__start__", "__end__"}
    assert nodes == {"collect", "hypothesize", "decide", "propose", "wait_confirm", "wait_approval", "verify", "report"}
    assert not {n for n in nodes if any(w in n for w in ("approve_", "execute", "close", "decision"))}


def test_budget_from_config_has_defaults_and_overrides():
    assert Budget.from_config(None) == Budget()
    b = Budget.from_config({"max_rounds": 2, "confident_at": 0.8, "auto_critical": False})
    assert (b.max_rounds, b.confident_at, b.auto_critical, b.max_queries) == (2, 0.8, False, 25)
