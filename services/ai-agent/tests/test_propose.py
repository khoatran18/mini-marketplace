"""Đề xuất hành động: agent chỉ xin gateway ký, UI mới thực hiện. Kiểm tham số, câu tóm tắt, sự kiện, giới hạn, lỗi."""
import pytest

from ai_agent.personas import load_personas
from ai_agent.runtime import events
from ai_agent.runtime.agent_factory import AgentRuntime
from ai_agent.runtime.context import RunContext, reset_run, set_run
from ai_agent.store import MemoryStore
from ai_agent.tools.propose import MAX_PROPOSALS_PER_TURN, make_action_tool
from ai_agent.tools.registry import TurnAborted, parse_actions
from ai_agent.tools.spec import build_action

from helpers import BUYER_TOOLS, agent_token, cfg, mock_gateway
from test_chat_flow import collect

ACTIONS = {a.name: a for a in parse_actions(BUYER_TOOLS)}


def ctx(role="buyer"):
    return RunContext(token=agent_token(role=role), run_id="run-0000-1111", request_id="req-1", user_id=42, role=role)


def test_build_action_fills_path_body_and_a_summary_that_matches():
    path, body, summary = build_action(ACTIONS["cancel_order"], {"id": 12, "reason": "đặt nhầm"})
    assert path == "/orders/12/cancel" and body == {"reason": "đặt nhầm"} and summary == "Huỷ đơn hàng #12. Lý do: đặt nhầm"
    path, body, summary = build_action(ACTIONS["adjust_inventory"], {"id": 7, "delta": -3, "reason": "hàng hỏng"})
    assert path == "/seller/products/7/inventory/adjust" and body == {"delta": -3, "reason": "hàng hỏng"}
    assert summary.startswith("Điều chỉnh tồn kho sản phẩm #7: -3")
    assert build_action(ACTIONS["cancel_order"], {"id": 5})[1] is None            # không có lý do: body rỗng


@pytest.mark.parametrize("args", [{"id": "../admin", "reason": "x"}, {"id": "1/2"}, {"id": 5, "delta": 0, "reason": "x"}])
def test_build_action_rejects_bad_values(args):
    spec = ACTIONS["adjust_inventory" if "delta" in args else "cancel_order"]
    with pytest.raises(ValueError):
        build_action(spec, args)


async def test_tool_creates_a_proposal_event_and_tells_the_model_nothing_was_done():
    calls = []
    tool = make_action_tool(ACTIONS["cancel_order"], mock_gateway(calls), tool_timeout=5)
    c = ctx()
    tok = set_run(c)
    try:
        out = await tool.coroutine(id=12, reason="đặt nhầm")
    finally:
        reset_run(tok)
    evs = [c.events.get_nowait() for _ in range(c.events.qsize())]
    prop = next(e for e in evs if e["event"] == events.ACTION_PROPOSAL)["data"]
    assert prop["proposal"] == "SIGNED.PROPOSAL.TOKEN" and prop["summary"].startswith("Huỷ đơn hàng #12")
    assert prop["request"] == {"method": "POST", "path": "/orders/12/cancel", "body": {"reason": "đặt nhầm"}}
    assert "CHƯA thực hiện" in out and "SIGNED" not in out                     # token đề xuất không đi qua mô hình
    method, path, _q, auth = calls[-1]
    assert (method, path) == ("POST", "/ai/proposals") and auth == f"Bearer {c.token}"
    assert c.proposals == 1 and c.cards[-1]["proposal_id"] == "prop-1"


async def test_tool_caps_proposals_per_turn():
    tool = make_action_tool(ACTIONS["cancel_order"], mock_gateway(), tool_timeout=5)
    c = ctx()
    tok = set_run(c)
    try:
        for i in range(MAX_PROPOSALS_PER_TURN):
            await tool.coroutine(id=10 + i)
        out = await tool.coroutine(id=99)
    finally:
        reset_run(tok)
    assert "[giới hạn]" in out and c.proposals == MAX_PROPOSALS_PER_TURN


async def test_gateway_refusal_is_reported_to_the_model_and_expired_token_aborts_the_turn():
    c = ctx()
    tok = set_run(c)
    try:
        out = await make_action_tool(ACTIONS["cancel_order"], mock_gateway(status=403), tool_timeout=5).coroutine(id=1)
        assert out.startswith("[lỗi công cụ]") and c.proposals == 0
        with pytest.raises(TurnAborted):
            await make_action_tool(ACTIONS["cancel_order"], mock_gateway(status=401), tool_timeout=5).coroutine(id=1)
    finally:
        reset_run(tok)


async def test_whole_turn_with_the_real_agent_emits_action_proposal():
    c = cfg()
    c._data["llm"]["profiles"]["fake_propose"] = {"provider": "fake", "script": [
        {"tool_calls": [{"name": "propose_cancel_order", "args": {"id": 12, "reason": "đặt nhầm"}}]},
        {"content": "Mình đã soạn đề xuất huỷ đơn #12, bạn bấm Xác nhận nhé."},
    ]}
    c._data["llm"]["roles"]["buyer"] = "fake_propose"
    rt, store, personas = AgentRuntime(c, mock_gateway()), MemoryStore(), load_personas()
    evs, _ = await collect(rt, store, personas["buyer"], text="huỷ đơn 12 giúp tôi, tôi đặt nhầm")
    names = [e["event"] for e in evs]
    assert names[-1] == events.DONE and events.ACTION_PROPOSAL in names and events.ERROR not in names
    prop = next(e for e in evs if e["event"] == events.ACTION_PROPOSAL)["data"]
    assert prop["request"]["path"] == "/orders/12/cancel"
    assert names.index(events.ACTION_PROPOSAL) < names.index(events.TOKEN)
