"""Checkpoint RCA: RAM luôn chạy; Postgres cần TEST_POSTGRES_DSN (mỗi test tạo DB riêng rồi xoá), không đặt thì bỏ qua."""
import os
import uuid

import pytest
from langgraph.types import Command

from ai_agent.rca.budget import Budget
from ai_agent.rca.checkpoint import make_conninfo, open_checkpointer
from ai_agent.rca.graph import build_graph
from rca_fakes import INCIDENT, FakeOps, ScriptedBrain

DSN = os.environ.get("TEST_POSTGRES_DSN")


def test_conninfo_quotes_awkward_passwords():
    s = make_conninfo(host="h", port=5432, user="u", password="p w'd", database="d")
    assert "dbname=d" in s and "p w" in s and s.count("password=") == 1


async def test_memory_checkpointer_is_default():
    async with open_checkpointer() as saver:
        assert "MemorySaver" in type(saver).__name__


@pytest.mark.skipif(not DSN, reason="TEST_POSTGRES_DSN chưa đặt")
async def test_postgres_checkpoint_survives_a_new_graph_and_connection():
    import psycopg

    kv = dict(p.split("=", 1) for p in DSN.split())
    name = "rca_" + uuid.uuid4().hex[:10]
    admin = await psycopg.AsyncConnection.connect(f"{DSN} dbname=postgres", autocommit=True)
    await admin.execute(f'CREATE DATABASE "{name}"')
    info = f"{DSN} dbname={name}"
    cfg = {"configurable": {"thread_id": "inc-1"}}
    try:
        async with open_checkpointer(conninfo=info) as saver:
            g = build_graph(ScriptedBrain(), FakeOps(), Budget(), checkpointer=saver)
            r = await g.ainvoke({"incident": INCIDENT}, cfg)
            assert r["__interrupt__"][0].value["waiting"] == "confirm"
        # đóng kết nối = "ai-agent restart"; kết nối mới cùng cơ sở dữ liệu phải tiếp tục đúng chỗ
        async with open_checkpointer(conninfo=info) as saver:
            g = build_graph(ScriptedBrain(), FakeOps(), Budget(), checkpointer=saver)
            assert (await g.aget_state(cfg)).next == ("wait_confirm",)
            r = await g.ainvoke(Command(resume={"event": "confirmed", "proposal_id": 4}), cfg)
            assert r["__interrupt__"][0].value["waiting"] == "approval"
            await saver.adelete_thread("inc-1")
            assert (await g.aget_state(cfg)).values == {}
    finally:
        await admin.execute(f'DROP DATABASE "{name}" WITH (FORCE)')
        await admin.close()
