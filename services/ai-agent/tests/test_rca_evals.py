"""Bộ eval RCA (evals/rca): bộ case hợp lệ và khớp ground truth, bộ chấm chấm được (Brain đáp án đạt hết, Brain sai trượt), gateway giả đúng bảng tool thật.
Không gọi mô hình."""
import copy
import json
import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "evals"))

import rcakit  # noqa: E402
from ai_agent.rca.brain import LlmBrain  # noqa: E402
from ai_agent.rca.budget import Budget  # noqa: E402
from rca_fakes import brain_env  # noqa: E402

CASES = rcakit.load_cases()


def test_case_file_is_valid_against_playbooks_ground_truth_and_catalog():
    assert rcakit.validate_cases(CASES) == []
    assert len(CASES) >= 19


def test_every_scripted_fault_with_alerts_has_a_case_and_every_playbook_with_ground_truth_is_covered():
    meta = rcakit.resilience_meta()
    with_alerts = {k for k, m in meta.items() if m["alerts"]}
    assert {c["scenario"] for c in CASES if c["scenario"]} == with_alerts == set("ABCDFGKLM")
    covered = {c["expect"]["playbook"] for c in CASES}
    for pid, pb in rcakit.load_playbooks().items():
        assert pid in covered, f"playbook {pid} chưa có case RCA"


def test_validation_catches_bad_cases():
    bad = copy.deepcopy(CASES[0])
    bad["expect"]["playbook"] = "slow-database"      # không phải đáp án của kịch bản C
    bad["expect"]["action"] = {"id": "scale_service", "params": {"service": "payment-service", "replicas": 99}}
    bad["expect"]["forbid"] = [{"id": "format_disk"}]
    bad["expect"]["cause_example"] = "không liên quan"
    msgs = " | ".join(rcakit.validate_cases([bad]))
    for needle in ("không nằm trong đáp án", "tham số hành động sai", "không có trong sổ", "cause_example"):
        assert needle in msgs


async def _run(case, brain_cls):
    w = rcakit.World(case)
    run = await rcakit.run_case(case, w, brain_cls(case, w))
    return rcakit.score(case, run)


@pytest.mark.parametrize("case", CASES, ids=[c["id"] for c in CASES])
async def test_oracle_brain_passes_every_case(case):
    r = await _run(case, rcakit.OracleBrain)
    assert r.ok, r.reasons


async def test_wrong_brain_fails_almost_everything_so_the_scorer_has_teeth():
    results = [await _run(c, rcakit.WrongBrain) for c in CASES]
    passed = [c["id"] for c, r in zip(CASES, results) if r.ok]
    assert len(passed) <= 1, f"Brain sai vẫn đạt: {passed}"
    assert any("bị cấm" in why for r in results for why in r.reasons)  # forbid bắt được restart_service bậy


async def test_forbidden_or_fabricated_actions_are_scored_as_failures():
    case = next(c for c in CASES if c["id"] == "outbox-dead-corrupt-payload")

    class Requeues(rcakit.OracleBrain):
        async def choose_action(self, state, catalog):
            return {"action_id": "requeue_outbox", "params": {"service": "payment-service", "limit": 1}, "rationale": "x", "evidence_ids": ["e1"], "model_calls": 1}

    w = rcakit.World(case)
    r = rcakit.score(case, await rcakit.run_case(case, w, Requeues(case, w)))
    assert not r.ok and not r.checks["forbid"] and not r.checks["action"]


async def test_budget_overrun_is_scored():
    case = CASES[0]

    class Spendthrift(rcakit.OracleBrain):
        async def collect(self, state):
            return {**(await super().collect(state)), "model_calls": 40}

    w = rcakit.World(case)
    assert not rcakit.score(case, await rcakit.run_case(case, w, Spendthrift(case, w))).checks["budget"]


async def test_stub_gateway_serves_real_playbooks_real_catalog_and_fixture_metrics_by_phase():
    case = next(c for c in CASES if c["id"] == "C-payment-down")
    w = rcakit.World(case)
    gw = rcakit.stub_gateway(w)
    tok = "t"
    pb = await gw.get(tok, "/ops/playbooks/service-unavailable")
    assert pb["id"] == "service-unavailable" and "causes" in pb
    lst = await gw.get(tok, "/ops/playbooks", {"alert": "AIAgentDown"})
    assert [p["id"] for p in lst["playbooks"]] == ["ai-agent"]
    acts = await gw.get(tok, "/ops/actions")
    assert {a["id"] for a in acts["actions"]} >= {"scale_service", "restart_service", "requeue_outbox", "run_backfill", "escalate_to_human"}
    up = await gw.get(tok, "/ops/queries/service_up", {"service": "payment-service"})
    assert up["series"] == [{"labels": {"service": "payment-service"}, "value": 0}]
    assert (await gw.get(tok, "/ops/queries/service_up", {"service": "order-service"}))["series"] == []  # service không có trong fixture: rỗng, không bịa
    w.phase = "after"
    assert (await gw.get(tok, "/ops/queries/service_up", {"service": "payment-service"}))["series"][0]["value"] == 1
    health = await gw.get(tok, "/ops/system/health")
    assert all(s["state"] == "ok" for s in health["services"])  # after: mọi thứ ổn
    assert [c["tool"] for c in w.calls] == ["ops_get_playbook", "ops_list_playbooks", "ops_list_actions", "ops_metrics_query", "ops_metrics_query", "ops_metrics_query", "ops_system_health"]


async def test_llm_brain_collects_from_the_eval_stub_gateway_using_the_real_operator_tool_table():
    case = next(c for c in CASES if c["id"] == "C-payment-down")
    w = rcakit.World(case)
    script = [{"tool_calls": [{"name": "ops_get_playbook", "args": {"id": "service-unavailable"}},
                              {"name": "ops_metrics_query", "args": {"name": "service_up", "service": "payment-service"}},
                              {"name": "ops_system_health", "args": {}}]}, {"content": "XONG"}]
    rt, persona, ctx, _ = brain_env(script, gateway=rcakit.stub_gateway(w))
    r = await LlmBrain(rt, persona, ctx, Budget()).collect({"incident": case["incident"], "round": 1, "evidence": [], "queries": 0, "model_calls": 0})
    by = {e["source"]: e["summary"] for e in r["evidence"]}
    assert r["playbook"] == "service-unavailable" and r["queries"] == 3
    assert '"value":0' in by["ops_metrics_query"] and "payment-service" in by["ops_system_health"]


def test_repeat_summary_reports_rate_interval_and_flaky_cases():
    import rcakit

    rows = [{"id": "a", "ok": True}, {"id": "a", "ok": True}, {"id": "a", "ok": True},
            {"id": "b", "ok": True}, {"id": "b", "ok": False}, {"id": "b", "ok": True},
            {"id": "c", "ok": False}, {"id": "c", "ok": False}, {"id": "c", "ok": False}]
    sm = rcakit.summarize_repeats(rows)
    assert (sm["runs"], sm["ok"], sm["always_ok"], sm["never_ok"], sm["flaky"]) == (9, 5, 1, 1, ["b"])
    assert sm["per_case"] == {"a": "3/3", "b": "2/3", "c": "0/3"}
    lo, hi = sm["ci95"]
    assert 0.2 < lo < 5 / 9 < hi < 0.85
    one = rcakit.wilson(1, 1)
    assert one[0] < 0.3 and one[1] == 1.0  # "1/1 đạt" không phải "chắc chắn 100%"
