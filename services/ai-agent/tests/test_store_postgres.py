"""Kho Postgres. Cần một Postgres: đặt TEST_POSTGRES_DSN="host=... port=... user=... password=..." (mỗi test tạo DB riêng rồi xoá),
không đặt thì bỏ qua (giống các service Go)."""
import os
import uuid

import pytest

asyncpg = pytest.importorskip("asyncpg")

from ai_agent.store import Busy, Duplicate, NotFound  # noqa: E402
from ai_agent.store.postgres import PostgresStore  # noqa: E402

DSN = os.environ.get("TEST_POSTGRES_DSN")
pytestmark = pytest.mark.skipif(not DSN, reason="TEST_POSTGRES_DSN chưa đặt")


def parts():
    return dict(kv.split("=", 1) for kv in DSN.split())


@pytest.fixture
async def store():
    p = parts()
    admin = await asyncpg.connect(host=p["host"], port=int(p.get("port", 5432)), user=p["user"], password=p.get("password", ""), database="postgres")
    name = "aiagent_" + uuid.uuid4().hex[:10]
    await admin.execute(f'CREATE DATABASE "{name}"')
    st = await PostgresStore.connect(host=p["host"], port=int(p.get("port", 5432)), user=p["user"], password=p.get("password", ""), database=name, retention_days=30)
    try:
        yield st
    finally:
        await st.aclose()
        await admin.execute(f'DROP DATABASE "{name}" WITH (FORCE)')
        await admin.close()


async def begin(st, **kw):
    base = dict(user_id=7, conversation_id=None, client_message_id=None, run_id=uuid.uuid4().hex, role="buyer", text="xin chào", max_history=10)
    base.update(kw)
    return await st.begin_turn(**base)


async def test_two_turns_keep_history_and_masked_text(store):
    t1 = await begin(store, text="gọi tôi số 0912 345 678 hoặc a@b.vn nhé")
    assert t1.history == []
    await store.end_turn(t1, status="ok", answer="Dạ, mình đã ghi nhận.", cards=[{"tool": "list_my_orders"}], tool_calls=1, input_tokens=100, output_tokens=20)
    t2 = await begin(store, conversation_id=t1.conversation_id, text="còn gì nữa")
    assert [r for r, _ in t2.history] == ["user", "assistant"]
    assert "0912" not in t2.history[0][1] and "a@b.vn" not in t2.history[0][1]      # số điện thoại, email đã che trước khi lưu
    row = await store._pool.fetchrow("SELECT status, tool_calls, input_tokens, refs FROM ai_runs WHERE run_id = $1", t1.run_id)
    assert (row["status"], row["tool_calls"], row["input_tokens"]) == ("ok", 1, 100) and "list_my_orders" in row["refs"]


async def test_other_users_conversation_is_not_found_and_bad_ids_too(store):
    t = await begin(store)
    await store.end_turn(t, status="ok", answer="a", cards=[], tool_calls=0)
    with pytest.raises(NotFound):
        await begin(store, user_id=8, conversation_id=t.conversation_id)
    with pytest.raises(NotFound):
        await begin(store, conversation_id="khong-phai-uuid")
    with pytest.raises(NotFound):
        await begin(store, conversation_id=str(uuid.uuid4()))


async def test_busy_until_the_turn_ends_then_free(store):
    t = await begin(store)
    with pytest.raises(Busy):
        await begin(store, conversation_id=t.conversation_id)
    await store.end_turn(t, status="cancelled", answer="", cards=[], tool_calls=0)
    await begin(store, conversation_id=t.conversation_id)


async def test_a_stuck_run_does_not_lock_the_conversation_forever(store):
    t = await begin(store)
    await store._pool.execute("UPDATE ai_conversations SET active_since = now() - interval '10 minutes' WHERE id = $1::uuid", t.conversation_id)
    await begin(store, conversation_id=t.conversation_id)


async def test_duplicate_client_message_id(store):
    t = await begin(store, client_message_id="m1")
    await store.end_turn(t, status="ok", answer="a", cards=[], tool_calls=0)
    with pytest.raises(Duplicate) as e:
        await begin(store, conversation_id=t.conversation_id, client_message_id="m1")
    assert e.value.run_id == t.run_id


async def test_history_is_limited_to_the_latest_messages(store):
    t = await begin(store, text="m0")
    cid = t.conversation_id
    await store.end_turn(t, status="ok", answer="r0", cards=[], tool_calls=0)
    for i in range(1, 4):
        t = await begin(store, conversation_id=cid, text=f"m{i}")
        await store.end_turn(t, status="ok", answer=f"r{i}", cards=[], tool_calls=0)
    t = await begin(store, conversation_id=cid, text="m4", max_history=3)
    assert [c for _, c in t.history] == ["r2", "m3", "r3"]


async def test_purge_deletes_only_old_conversations_with_their_messages(store):
    old, new = await begin(store), await begin(store)
    for t in (old, new):
        await store.end_turn(t, status="ok", answer="a", cards=[], tool_calls=0)
    await store._pool.execute("UPDATE ai_conversations SET updated_at = now() - interval '31 days' WHERE id = $1::uuid", old.conversation_id)
    assert await store.purge() == 1
    assert await store._pool.fetchval("SELECT count(*) FROM ai_conversations") == 1
    assert await store._pool.fetchval("SELECT count(*) FROM ai_messages") == 2     # chỉ còn tin của cuộc mới


async def test_migrate_is_idempotent(store):
    await store.migrate()
    await store.migrate()
    assert await store._pool.fetchval("SELECT count(*) FROM ai_agent_schema") == 1


async def test_app_with_postgres_backend_stores_a_real_chat_turn():
    """create_app(store.backend=postgres): lifespan kết nối + tạo bảng, /chat lưu tin, tắt app thì đóng kết nối."""
    import httpx

    from ai_agent.main import create_app
    from ai_agent.runtime.agent_factory import AgentRuntime
    from helpers import agent_token, cfg, fake_builder, mock_gateway

    p = parts()
    admin = await asyncpg.connect(host=p["host"], port=int(p.get("port", 5432)), user=p["user"], password=p.get("password", ""), database="postgres")
    name = "aiagent_" + uuid.uuid4().hex[:10]
    await admin.execute(f'CREATE DATABASE "{name}"')
    try:
        c = cfg()
        c._data["store"].update(backend="postgres", postgres={"host": p["host"], "port": int(p.get("port", 5432)), "user": p["user"],
                                                              "password": p.get("password", ""), "database": name})
        app = create_app(c, runtime=AgentRuntime(c, mock_gateway(), agent_builder=fake_builder))
        async with app.router.lifespan_context(app):
            async with httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://ai") as cl:
                hdr = {"X-Agent-Token": agent_token(jti="run-pg-0001"), "X-Agent-Run-ID": "run-pg-0001"}
                r = await cl.post("/chat", json={"message": "đơn của tôi"}, headers=hdr)
                assert r.status_code == 200 and "event: done" in r.text
        con = await asyncpg.connect(host=p["host"], port=int(p.get("port", 5432)), user=p["user"], password=p.get("password", ""), database=name)
        try:
            roles = [r["role"] for r in await con.fetch("SELECT role FROM ai_messages ORDER BY id")]
            status = await con.fetchval("SELECT status FROM ai_runs WHERE run_id = 'run-pg-0001'")
        finally:
            await con.close()
        assert roles == ["user", "assistant"] and status == "ok"
    finally:
        await admin.execute(f'DROP DATABASE "{name}" WITH (FORCE)')
        await admin.close()


async def test_tokens_used_today_counts_this_user_and_this_platform_day_only(store):
    t1 = await begin(store, user_id=7)
    await store.end_turn(t1, status="ok", answer="a", cards=[], tool_calls=1, input_tokens=1000, output_tokens=500)
    t2 = await begin(store, user_id=7)
    await store.end_turn(t2, status="ok", answer="b", cards=[], tool_calls=0, input_tokens=200, output_tokens=100)
    other = await begin(store, user_id=8)
    await store.end_turn(other, status="ok", answer="c", cards=[], tool_calls=0, input_tokens=9999, output_tokens=1)
    running = await begin(store, user_id=7)   # chưa kết thúc: chưa có token
    assert running.run_id
    assert await store.tokens_used_today(7) == 1800
    assert await store.tokens_used_today(8) == 10000
    assert await store.tokens_used_today(99) == 0

    # một lượt từ hôm qua (theo giờ nền tảng) không tính
    await store._pool.execute("UPDATE ai_runs SET started_at = started_at - interval '2 days' WHERE run_id = $1", t1.run_id)
    assert await store.tokens_used_today(7) == 300


async def test_list_and_get_conversation_scoping(store):
    a = await begin(store, user_id=7, text="đơn của tôi, số 0912 345 678")
    await store.end_turn(a, status="ok", answer="Có 2 đơn.", cards=[], tool_calls=1)
    b = await begin(store, user_id=8, role="seller_admin", text="tồn kho")
    mine = await store.list_conversations(user_id=7)
    assert [m["id"] for m in mine] == [a.conversation_id] and mine[0]["messages"] == 2 and "0912" not in mine[0]["title"]
    assert {m["id"] for m in await store.list_conversations(user_id=None)} == {a.conversation_id, b.conversation_id}
    assert [m["id"] for m in await store.list_conversations(user_id=None, role="seller_admin")] == [b.conversation_id]
    conv = await store.get_conversation(a.conversation_id, user_id=7)
    assert [m["role"] for m in conv["messages"]] == ["user", "assistant"]
    with pytest.raises(NotFound):
        await store.get_conversation(a.conversation_id, user_id=8)       # của người khác
    assert (await store.get_conversation(a.conversation_id, user_id=None))["user_id"] == 7   # admin xem
    with pytest.raises(NotFound):
        await store.get_conversation("not-a-uuid", user_id=None)
