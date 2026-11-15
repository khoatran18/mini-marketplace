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

    runner = Runner(graph_for, runs, clock)
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


@pytest.mark.parametrize("p,outcome", [({"state": "REJECTED"}, "rejected"), ({"state": "EXPIRED"}, "expired"),
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
    finally:
        await pool.close()
        await admin.execute(f'DROP DATABASE "{name}" WITH (FORCE)')
        await admin.close()
