"""Dọn lịch sử RCA cũ: chỉ phiên đã xong/lỗi quá hạn bị xoá (kèm nhật ký, checkpoint); phiên đang chạy/chờ giữ nguyên."""
from __future__ import annotations

import pytest
from langgraph.checkpoint.memory import MemorySaver

from ai_agent.main import purge_rca
from ai_agent.rca.runs import MemoryRunStore

DAY = 86400.0


@pytest.mark.asyncio
async def test_purge_removes_only_old_finished_runs_with_events_and_checkpoints():
    now = [10_000 * DAY]
    runs = MemoryRunStore(clock=lambda: now[0])
    saver = MemorySaver()
    old_done = await runs.create(1, "auto")
    old_err = await runs.create(1, "manual")
    old_wait = await runs.create(2, "auto")
    now[0] += 40 * DAY  # ba phiên trên đã 40 ngày tuổi
    fresh_done = await runs.create(3, "auto")
    await runs.update(old_done["id"], status="done")
    await runs.update(old_err["id"], status="error")
    await runs.update(old_wait["id"], status="waiting_approval")
    await runs.update(fresh_done["id"], status="done")
    for r in (old_done, old_wait, fresh_done):
        await runs.add_event(r["id"], "note", "x")
        await saver.aput({"configurable": {"thread_id": r["thread_id"], "checkpoint_ns": ""}}, {"v": 1, "id": "c" + str(r["id"]), "ts": "t", "channel_values": {}, "channel_versions": {}, "versions_seen": {}, "pending_sends": []}, {}, {})

    assert await purge_rca(runs, saver, 30) == 2
    assert set(runs.rows) == {old_wait["id"], fresh_done["id"]}
    assert await runs.events(old_done["id"]) == []
    assert await runs.events(old_wait["id"]) and await runs.events(fresh_done["id"])
    assert await saver.aget_tuple({"configurable": {"thread_id": old_done["thread_id"]}}) is None
    assert await saver.aget_tuple({"configurable": {"thread_id": old_wait["thread_id"]}}) is not None
    assert await purge_rca(runs, saver, 30) == 0


@pytest.mark.asyncio
async def test_a_failing_checkpoint_delete_does_not_stop_the_rest():
    class Bad:
        async def adelete_thread(self, t):
            raise RuntimeError("boom")

    now = [10_000 * DAY]
    runs = MemoryRunStore(clock=lambda: now[0])
    r = await runs.create(1, "auto")
    await runs.update(r["id"], status="done")
    now[0] += 40 * DAY
    assert await purge_rca(runs, Bad(), 30) == 1
