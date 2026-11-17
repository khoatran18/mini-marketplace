"""Bộ theo dõi RCA + Runner + sổ phiên, bằng ops-service giả, Brain giả và đồng hồ giả: không LLM, không mạng."""
import os
import uuid

import pytest
from langgraph.checkpoint.memory import MemorySaver

from ai_agent.rca.budget import Budget
from ai_agent.rca.graph import build_graph
from ai_agent.rca.runner import Runner
from ai_agent.rca.runs import MemoryRunStore, PostgresRunStore
from ai_agent.rca.watcher import Watcher
from rca_fakes import FakeOps, ScriptedBrain


class FakeOpsReader:
    def __init__(self):
        self.incidents, self.props = [], {}

    async def open_incidents(self):
        return self.incidents

    async def proposals_for_incident(self, incident_id):
        return [p for p in self.props.values() if p["incident_id"] == incident_id]

    async def proposal(self, pid):
        return self.props.get(pid)


class Clock:
    def __init__(self, t=1_000_000.0):
        self.t = t

    def __call__(self):
        return self.t


def inc(i=7, severity="critical", state="open"):
    return {"id": i, "alert_name": "ServiceDown", "severity": severity, "component": "payment-service", "summary": "down", "labels": {}, "state": state}


def prop(pid, incident_id=7, state="PENDING", exec_state=None, finished=None, created=1_000_001.0, action="restart_service", params=None):
    return {"id": pid, "incident_id": incident_id, "action_id": action, "params": params or {"service": "payment-service"}, "state": state,
            "exec_state": exec_state, "exec_finished_at": finished, "created_at": created}


def make(budget=None, brain=None, clock=None):
    clock = clock or Clock()
    runs, ops, saver = MemoryRunStore(clock), FakeOpsReader(), MemorySaver()
    budget = budget or Budget(verify_wait_sec=180, wait_timeout_sec=1800, daily_auto_runs=3)

    async def graph_for(run):
        return build_graph(brain or ScriptedBrain(), FakeOps(), budget, checkpointer=saver, clock=clock)

    runner = Runner(graph_for, runs, clock, budget)
    return Watcher(runner, runs, ops, budget, clock), runs, ops, clock


async def test_starts_one_auto_run_per_critical_incident_only():
    w, runs, ops, _ = make()
    ops.incidents = [inc(7), inc(8, severity="warning"), inc(9, state="investigating")]
    assert (await w.tick())["started"] == 1
    assert (await w.tick())["started"] == 0  # tick lần hai: sự cố 7 đã có phiên
    assert (await runs.latest(7))["status"] == "waiting_confirm" and await runs.latest(8) is None and await runs.latest(9) is None


async def test_daily_cap_and_off_switch():
    w, runs, ops, _ = make()
    ops.incidents = [inc(i) for i in range(1, 8)]
    assert (await w.tick())["started"] == 3  # daily_auto_runs=3
    w2, runs2, ops2, _ = make(budget=Budget(auto_critical=False))
    ops2.incidents = [inc(1)]
    assert (await w2.tick())["started"] == 0 and await runs2.latest(1) is None


async def test_confirm_is_detected_only_for_the_same_action_and_params_created_after_the_draft():
    w, runs, ops, clock = make()
    ops.incidents = [inc()]
    await w.tick()
    ops.props = {1: prop(1, action="scale_service", params={"service": "payment-service", "replicas": 2}),  # hành động khác
                 2: prop(2, params={"service": "order-service"}),                                          # tham số khác
                 3: prop(3, created=900_000.0)}                                                            # có từ trước nháp
    await w.tick()
    assert (await runs.latest(7))["status"] == "waiting_confirm"
    ops.props[4] = prop(4)
    assert (await w.tick())["woken"] == 1
    r = await runs.latest(7)
    assert r["status"] == "waiting_approval" and r["proposal_id"] == 4


async def test_unconfirmed_draft_expires_after_the_wait_timeout_and_run_ends():
    w, runs, ops, clock = make()
    ops.incidents = [inc()]
    await w.tick()
    clock.t += 1799
    await w.tick()
    assert (await runs.latest(7))["status"] == "waiting_confirm"
    clock.t += 5
    await w.tick()
    r = await runs.latest(7)
    assert r["status"] == "done" and r["outcome"] == "expired" and r["report"]["ready_to_close"] is False


async def _to_approval(w, runs, ops):
    ops.incidents = [inc()]
    await w.tick()
    ops.props = {4: prop(4)}
    await w.tick()
    assert (await runs.latest(7))["status"] == "waiting_approval"


async def test_executed_wakes_the_run_only_after_verify_wait_then_verifies():
    w, runs, ops, clock = make()
    await _to_approval(w, runs, ops)
    ops.props[4] = prop(4, state="ACCEPTED")  # duyệt rồi nhưng chưa chạy
    await w.tick()
    assert (await runs.latest(7))["status"] == "waiting_approval"
    ops.props[4] = prop(4, state="ACCEPTED", exec_state="EXECUTED", finished="2026-11-15T10:00:00+00:00")
    clock.t = 1_780_000_000.0  # rất lâu sau finished
    ops.props[4]["exec_finished_at"] = clock.t - 100  # mới 100 s < verify_wait_sec 180
    await w.tick()
    assert (await runs.latest(7))["status"] == "waiting_approval"
    clock.t += 100  # 200 s
    await w.tick()
    r = await runs.latest(7)
    assert r["status"] == "done" and r["outcome"] == "recovered" and r["report"]["ready_to_close"] is True


@pytest.mark.parametrize("p,outcome", [({"state": "REJECTED"}, "rejected"), ({"state": "WITHDRAWN"}, "rejected"), ({"state": "EXPIRED"}, "expired"),
                                       ({"state": "ACCEPTED", "exec_state": "FAILED"}, "rejected")])
async def test_rejected_expired_or_failed_execution_end_the_run_without_verify(p, outcome):
    brain = ScriptedBrain()
    w, runs, ops, _ = make(brain=brain)
    await _to_approval(w, runs, ops)
    ops.props[4] = prop(4, **p)
    await w.tick()
    r = await runs.latest(7)
    assert r["status"] == "done" and r["outcome"] == outcome and "verify" not in brain.calls


async def test_approval_that_never_runs_expires_after_the_timeout():
    w, runs, ops, clock = make()
    await _to_approval(w, runs, ops)
    ops.props[4] = prop(4, state="ACCEPTED")
    clock.t += 1801
    await w.tick()
    assert (await runs.latest(7))["outcome"] == "expired"


async def test_one_broken_run_does_not_stop_the_others():
    clock = Clock()
    runs, ops, saver = MemoryRunStore(clock), FakeOpsReader(), MemorySaver()
    budget = Budget(daily_auto_runs=5)

    async def graph_for(run):
        if run["incident_id"] == 1 and run["status"] != "running":
            raise RuntimeError("boom")  # chỉ phiên 1 hỏng khi đánh thức
        return build_graph(ScriptedBrain(), FakeOps(), budget, checkpointer=saver, clock=clock)

    w = Watcher(Runner(graph_for, runs, clock), runs, ops, budget, clock)
    ops.incidents = [inc(1), inc(2)]
    await w.tick()
    ops.props = {1: prop(1, incident_id=1), 2: prop(2, incident_id=2)}
    await w.tick()
    assert (await runs.latest(2))["status"] == "waiting_approval"


async def test_runner_marks_a_crashing_graph_as_error_not_an_exception():
    runs = MemoryRunStore()

    async def graph_for(run):
        raise RuntimeError("không dựng được Brain")

    r = await Runner(graph_for, runs).start(inc(), "manual")
    assert r["status"] == "error" and r["outcome"] == "error"


# ---- Postgres (cần TEST_POSTGRES_DSN) ----
DSN = os.environ.get("TEST_POSTGRES_DSN")


@pytest.mark.skipif(not DSN, reason="TEST_POSTGRES_DSN chưa đặt")
async def test_postgres_run_store_round_trip_and_migration_is_idempotent():
    import asyncpg

    kv = dict(p.split("=", 1) for p in DSN.split())
    kw = dict(host=kv["host"], port=int(kv.get("port", 5432)), user=kv["user"], password=kv.get("password", ""))
    admin = await asyncpg.connect(database="postgres", **kw)
    name = "rcaruns_" + uuid.uuid4().hex[:10]
    await admin.execute(f'CREATE DATABASE "{name}"')
    pool = await asyncpg.create_pool(database=name, min_size=1, max_size=3, **kw)
    try:
        st = PostgresRunStore(pool)
        await st.migrate()
        await st.migrate()
        run = await st.create(7, "auto")
        assert run["thread_id"] == f"inc-7-{run['id']}" and run["status"] == "running"
        await st.update(run["id"], status="waiting_confirm", draft={"action_id": "restart_service", "params": {"service": "payment-service"}}, waiting_since=123.5)
        got = await st.get(run["id"])
        assert got["draft"]["params"]["service"] == "payment-service" and got["waiting_since"] == 123.5
        assert [r["id"] for r in await st.waiting()] == [run["id"]]
        await st.update(run["id"], status="done", outcome="recovered", report={"summary": "ok", "ready_to_close": True}, waiting_since=None)
        assert await st.waiting() == [] and (await st.latest(7))["report"]["ready_to_close"] is True
        assert await st.count_auto_since(0) == 1 and await st.count_auto_since(4_102_444_800) == 0
        with pytest.raises(ValueError):
            await st.update(run["id"], incident_id=9)  # không cho đổi trường ngoài danh sách
        assert await pool.fetchval("SELECT max(version) FROM ai_agent_schema") == 2
        # danh sách cho API và dọn phiên bị ngắt giữa chừng lúc khởi động
        a, b = await st.create(8, "manual:42"), await st.create(8, "manual:42")
        assert [r["id"] for r in await st.recent(8)] == [b["id"], a["id"]] and len(await st.recent()) == 3 and len(await st.recent(limit=2)) == 2
        await st.update(a["id"], status="waiting_approval", proposal_id=3)
        assert await st.fail_running() == 1  # b (đang running) bị ngắt; a đang chờ người thì giữ nguyên
        assert (await st.get(b["id"]))["status"] == "error" and (await st.get(b["id"]))["outcome"] == "interrupted"
        assert (await st.get(a["id"]))["status"] == "waiting_approval"
        # dọn lịch sử cũ: chỉ phiên done/error quá hạn bị xoá (kèm nhật ký); phiên đang chờ giữ nguyên
        await st.add_event(run["id"], "note", "x")
        await st.add_event(a["id"], "note", "y")
        await pool.execute("UPDATE ai_rca_runs SET updated_at = now() - interval '40 days' WHERE id = ANY($1::bigint[])", [run["id"], a["id"], b["id"]])
        assert sorted(await st.purge_finished(30)) == sorted([run["thread_id"], b["thread_id"]])
        assert await st.get(run["id"]) is None and await st.get(b["id"]) is None and (await st.get(a["id"]))["status"] == "waiting_approval"
        assert await st.events(run["id"]) == [] and len(await st.events(a["id"])) == 1
        assert await st.purge_finished(30) == []
    finally:
        await pool.close()
        await admin.execute(f'DROP DATABASE "{name}" WITH (FORCE)')
        await admin.close()


# ---- số liệu Prometheus của RCA ----
def _val(metric, *labels):
    return (metric.labels(*labels) if labels else metric)._value.get()


async def test_metrics_count_starts_finishes_waiting_and_the_watcher_heartbeat():
    from ai_agent import metrics

    started = {t: _val(metrics.RCA_STARTED, t) for t in ("auto", "manual")}
    fin = _val(metrics.RCA_FINISHED, "recovered")
    ticks = _val(metrics.RCA_WATCHER_TICKS, "ok")
    w, runs, ops, clock = make()
    ops.incidents = [inc()]
    await w.tick()
    assert _val(metrics.RCA_STARTED, "auto") == started["auto"] + 1 and _val(metrics.RCA_STARTED, "manual") == started["manual"]
    assert _val(metrics.RCA_WAITING, "waiting_confirm") == 1 and _val(metrics.RCA_WAITING, "waiting_approval") == 0
    assert _val(metrics.RCA_WATCHER_LAST_TICK, "rca") == clock.t and _val(metrics.RCA_WATCHER_TICKS, "ok") == ticks + 1
    ops.props = {4: prop(4)}
    await w.tick()
    assert _val(metrics.RCA_WAITING, "waiting_approval") == 1
    ops.props[4] = prop(4, state="ACCEPTED", exec_state="EXECUTED", finished=clock.t - 1000)
    await w.tick()
    assert _val(metrics.RCA_FINISHED, "recovered") == fin + 1 and _val(metrics.RCA_WAITING, "waiting_approval") == 0


async def test_manual_trigger_is_counted_apart_and_a_crashing_graph_counts_as_an_error_outcome():
    from ai_agent import metrics

    manual, err = _val(metrics.RCA_STARTED, "manual"), _val(metrics.RCA_FINISHED, "error")

    async def graph_for(run):
        raise RuntimeError("boom")

    await Runner(graph_for, MemoryRunStore()).start(inc(), "manual:42")
    assert _val(metrics.RCA_STARTED, "manual") == manual + 1 and _val(metrics.RCA_FINISHED, "error") == err + 1


async def test_a_failing_tick_in_run_forever_is_counted_as_an_error():
    import asyncio

    from ai_agent import metrics

    w, runs, ops, clock = make(budget=Budget(poll_interval_sec=0))
    errs = _val(metrics.RCA_WATCHER_TICKS, "error")

    async def boom():
        raise RuntimeError("ops-service không trả lời")

    ops.open_incidents = boom
    task = asyncio.create_task(w.run_forever())
    await asyncio.sleep(0.05)
    task.cancel()
    assert _val(metrics.RCA_WATCHER_TICKS, "error") > errs


async def test_run_token_histogram_observes_the_session_total_when_a_run_ends():
    from ai_agent import metrics

    before = metrics.RCA_RUN_TOKENS._sum.get()
    t = {"input_tokens": 1000, "output_tokens": 100}
    brain = ScriptedBrain(report=[{"summary": "xong", "ready_to_close": True, "reason": "r", "model_calls": 1, **t}])
    w, runs, ops, clock = make(brain=brain)
    await _to_approval(w, runs, ops)
    ops.props[4] = prop(4, state="REJECTED")
    await w.tick()
    assert metrics.RCA_RUN_TOKENS._sum.get() == before + 1100


async def test_daily_token_cap_stops_auto_runs_but_not_manual_ones():
    from ai_agent import metrics

    t = {"input_tokens": 900, "output_tokens": 100}
    brain = ScriptedBrain(collect=[{"evidence": [], "queries": 0, "model_calls": 1, **t}] * 3)
    w, runs, ops, clock = make(budget=Budget(daily_auto_runs=5, daily_tokens=1000), brain=brain)
    ops.incidents = [inc(1)]
    assert (await w.tick())["started"] == 1
    assert (await runs.latest(1))["tokens"] >= 1000  # tổng token của phiên được ghi vào sổ
    hits = metrics.RCA_TOKEN_CAP_HITS._value.get()
    ops.incidents = [inc(2)]
    assert (await w.tick())["started"] == 0 and await runs.latest(2) is None
    assert metrics.RCA_TOKEN_CAP_HITS._value.get() == hits + 1
    await w.runner.start(inc(2), "manual:operator:1")  # bấm tay không bị trần chặn
    assert await runs.latest(2) is not None


async def test_cost_is_computed_from_the_configured_prices_and_zero_price_means_off():
    from ai_agent import metrics

    assert Budget(usd_per_mtok_in=2.0, usd_per_mtok_out=10.0).cost_usd(500_000, 100_000) == pytest.approx(2.0)
    assert Budget().cost_usd(500_000, 100_000) == 0.0
    before = metrics.RCA_COST_USD._value.get()
    t = {"input_tokens": 1_000_000, "output_tokens": 0}
    brain = ScriptedBrain(report=[{"summary": "xong", "ready_to_close": True, "reason": "r", "model_calls": 1, **t}])
    w, runs, ops, clock = make(brain=brain, budget=Budget(usd_per_mtok_in=0.5, daily_auto_runs=3))
    await _to_approval(w, runs, ops)
    ops.props[4] = prop(4, state="REJECTED")
    await w.tick()
    assert metrics.RCA_COST_USD._value.get() == pytest.approx(before + 0.5)


async def test_run_is_narrated_in_the_event_log_from_start_to_the_wait_for_confirm():
    from ai_agent.rca.events import EventLog

    runs, saver = MemoryRunStore(), MemorySaver()

    async def graph_for(run):
        return build_graph(ScriptedBrain(), FakeOps(), Budget(), checkpointer=saver, events=EventLog(runs, run["id"]))

    run = await Runner(graph_for, runs).start({"id": 7, "alert_name": "ServiceDown", "severity": "critical", "component": "payment-service"}, "manual:1")
    kinds = [e["kind"] for e in await runs.events(run["id"])]
    assert kinds[0] == "step" and "hypotheses" in kinds and "draft" in kinds and kinds[-1] == "wait"
    assert (await runs.events(run["id"]))[-1]["data"]["waiting"] == "confirm"
