import pytest

from ai_agent.api.auth import AuthError, parse_agent_token
from ai_agent.store import Busy, Duplicate, MemoryStore, NotFound

from helpers import agent_token


def test_parse_agent_token_ok_and_rejections():
    t = agent_token(7, "seller_admin", jti="abcdefgh-1")
    i = parse_agent_token(t, "abcdefgh-1")
    assert (i.user_id, i.role, i.run_id) == (7, "seller_admin", "abcdefgh-1")
    for bad in (
        agent_token(ttl=-1),                       # hết hạn
        agent_token(typ="proposal"),                # gói đề xuất không phải token agent
        agent_token(aud=["gateway-proposal"]),
        agent_token(sub="0"), agent_token(role=""), "abc", "a.b.c",
    ):
        with pytest.raises(AuthError):
            parse_agent_token(bad)
    with pytest.raises(AuthError):
        parse_agent_token(t, "other-run-id")        # header run id lệch jti


async def _begin(s, **kw):
    base = dict(user_id=1, conversation_id=None, client_message_id=None, run_id="r1", role="buyer", text="hi", max_history=10)
    return await s.begin_turn(**{**base, **kw})


async def test_store_ownership_busy_duplicate_and_history():
    s = MemoryStore()
    t1 = await _begin(s, client_message_id="m1")
    with pytest.raises(Busy):
        await _begin(s, conversation_id=t1.conversation_id, run_id="r2")     # một lượt tại một thời điểm
    with pytest.raises(Duplicate) as d:
        await _begin(s, conversation_id=t1.conversation_id, run_id="r3", client_message_id="m1")
    assert d.value.run_id == "r1"
    await s.end_turn(t1, status="ok", answer="chào", cards=[], tool_calls=0)
    with pytest.raises(NotFound):
        await _begin(s, user_id=2, conversation_id=t1.conversation_id, run_id="r4")   # cuộc chat của người khác
    with pytest.raises(NotFound):
        await _begin(s, conversation_id="00000000-0000-0000-0000-000000000000", run_id="r5")
    t2 = await _begin(s, conversation_id=t1.conversation_id, run_id="r6", text="tiếp")
    assert t2.history == [("user", "hi"), ("assistant", "chào")]
