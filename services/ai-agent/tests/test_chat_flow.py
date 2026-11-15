"""Cả lượt chat với agent giả (không cần langchain.agents): sự kiện theo đúng thứ tự, dữ liệu đầy đủ chỉ ở card, lưu kho, giới hạn."""
import asyncio

from ai_agent.config import Config
from ai_agent.personas import load_personas
from ai_agent.runtime import events
from ai_agent.runtime.agent_factory import AgentRuntime
from ai_agent.runtime.chat import run_turn
from ai_agent.runtime.context import RunContext
from ai_agent.store import MemoryStore

from helpers import agent_token, cfg, fake_builder, mock_gateway


def make(config=None, gateway=None):
    config = config or cfg()
    rt = AgentRuntime(config, gateway or mock_gateway(), agent_builder=fake_builder)
    return rt, MemoryStore(), load_personas()


async def collect(rt, store, persona, text="đơn của tôi", user_id=42, role="buyer", run_id="run-aaaa-0001", conv=None):
    ctx = RunContext(token=agent_token(user_id, role, run_id), run_id=run_id, request_id="req", user_id=user_id, role=role)
    turn = await store.begin_turn(user_id=user_id, conversation_id=conv, client_message_id=None, run_id=run_id, role=role, text=text, max_history=10)
    return [e async for e in run_turn(rt, store, persona, ctx, turn, text)], turn


async def test_buyer_turn_event_order_card_and_answer():
    rt, store, personas = make()
    evs, turn = await collect(rt, store, personas["buyer"])
    names = [e["event"] for e in evs]
    assert names[0] == events.START and names[-1] == events.DONE
    assert names.index(events.TOOL_CALL) < names.index(events.CARD) < names.index(events.TOKEN)
    assert all(e["data"]["run_id"] == "run-aaaa-0001" for e in evs)
    card = next(e for e in evs if e["event"] == events.CARD)["data"]
    assert "Nguyen Trai" in str(card)                                         # UI thấy đầy đủ
    answer = "".join(e["data"]["text"] for e in evs if e["event"] == events.TOKEN)
    assert "đơn gần đây" in answer
    run = store.runs["run-aaaa-0001"]
    assert run["status"] == "ok" and run["tool_calls"] == 1 and run["cards"][0]["tool"] == "list_my_orders"
    assert store.conversations[turn.conversation_id]["messages"][-1] == ("assistant", answer)   # không lưu dữ liệu card


async def test_second_turn_reuses_the_conversation_history():
    rt, store, personas = make()
    _, t1 = await collect(rt, store, personas["buyer"], run_id="run-aaaa-0001")
    evs, t2 = await collect(rt, store, personas["buyer"], text="còn gì nữa", run_id="run-aaaa-0002", conv=t1.conversation_id)
    assert t2.conversation_id == t1.conversation_id and len(t2.history) == 2 and evs[-1]["event"] == events.DONE


async def test_expired_token_at_the_gateway_ends_with_an_error_event_and_frees_the_conversation():
    rt, store, personas = make(gateway=mock_gateway(status=401))
    evs, turn = await collect(rt, store, personas["buyer"])
    assert evs[-1]["event"] == events.ERROR and evs[-1]["data"]["code"] == "session_expired"
    assert store.runs["run-aaaa-0001"]["status"] == "session_expired"
    assert store.conversations[turn.conversation_id]["active"] is None


async def test_token_budget_stops_the_turn():
    c = cfg()
    d = c.to_dict(masked=False)
    d["limits"]["token_budget_per_turn"] = 1
    d["limits"]["token_budget_by_persona"] = {}  # trần riêng theo persona (config thật) sẽ che trần chung
    rt, store, personas = make(config=Config(d))
    evs, _ = await collect(rt, store, personas["buyer"])
    assert evs[-1]["event"] == events.ERROR and evs[-1]["data"]["code"] == "token_budget_exceeded"


async def test_turn_timeout_cancels_the_agent():
    c = cfg()
    d = c.to_dict(masked=False)
    d["limits"]["turn_timeout_sec"] = 0.05

    class Slow:
        async def astream(self, *a, **k):
            await asyncio.sleep(5)
            yield ("updates", {})

    rt = AgentRuntime(Config(d), mock_gateway(), agent_builder=lambda *a: Slow())
    store, personas = MemoryStore(), load_personas()
    evs, turn = await collect(rt, store, personas["buyer"])
    assert evs[-1]["data"]["code"] == "timeout" and store.runs["run-aaaa-0001"]["status"] == "timeout"


async def test_missing_tools_or_model_config_is_an_error_event_not_a_crash():
    c = cfg()
    d = c.to_dict(masked=False)
    d["llm"]["roles"]["buyer"] = "deepseek_flash"          # profile thiếu api_key
    rt, store, personas = make(config=Config(d))
    evs, _ = await collect(rt, store, personas["buyer"])
    assert evs[-1]["event"] == events.ERROR and evs[-1]["data"]["code"] == "unavailable"
    assert "deepseek" not in str(evs[-1]).lower()           # không lộ chi tiết cấu hình


async def test_seller_persona_uses_the_seller_script():
    rt, store, personas = make()
    evs, _ = await collect(rt, store, personas["seller"], role="seller_admin")
    assert next(e for e in evs if e["event"] == events.TOOL_CALL)["data"]["tool"] == "count_store_orders"


async def test_real_create_agent_with_langchain_middleware_and_scripted_model():
    """Không dùng agent_builder giả: chạy create_agent + middleware giới hạn thật của LangChain với mô hình giả có kịch bản."""
    rt = AgentRuntime(cfg(), mock_gateway())
    store, personas = MemoryStore(), load_personas()
    evs, _ = await collect(rt, store, personas["buyer"])
    names = [e["event"] for e in evs]
    assert names[0] == events.START and names[-1] == events.DONE, evs[-1]
    assert names.index(events.TOOL_CALL) < names.index(events.CARD) < names.index(events.TOKEN)
    assert "đơn gần đây" in "".join(e["data"]["text"] for e in evs if e["event"] == events.TOKEN)


async def test_citation_shows_a_friendly_name_never_the_api_route():
    rt, store, personas = make()
    evs, _ = await collect(rt, store, personas["buyer"])
    cit = next(e for e in evs if e["event"] == events.CITATION)["data"]
    assert cit["label"] == "Đơn hàng của bạn" and "source" not in cit and "/" not in str(cit.values())


async def test_empty_model_answer_gets_a_fallback_text():
    rt, store, personas = make()
    rt._agent_builder = lambda model, tools, persona, mw: _SilentAgent()
    evs, _ = await collect(rt, store, personas["buyer"])
    text = "".join(e["data"]["text"] for e in evs if e["event"] == events.TOKEN)
    assert "chưa tổng hợp xong" in text and evs[-1]["event"] == events.DONE


class _SilentAgent:
    async def astream(self, *_a, **_k):
        if False:
            yield None


def test_turn_budget_per_persona_override():
    from ai_agent.runtime.chat import _turn_budget

    class L(dict):
        token_budget_per_turn = 20000

    limits = L({"token_budget_by_persona": {"admin": 45000}})
    assert _turn_budget(limits, "admin") == 45000
    assert _turn_budget(limits, "buyer") == 20000
    assert _turn_budget(L(), "admin") == 20000


def test_today_line_uses_platform_timezone_and_weekday():
    from datetime import datetime, timezone

    from ai_agent.runtime.chat import today_line

    # 2026-11-13 18:00 UTC = 2026-11-14 01:00 giờ Việt Nam (Thứ Bảy)
    assert today_line(datetime(2026, 11, 13, 18, 0, tzinfo=timezone.utc)) == "[Hôm nay là Thứ Bảy, 2026-11-14 (giờ Việt Nam).]"


def test_every_persona_with_a_budget_override_is_a_real_persona():
    """token_budget_by_persona chỉ được nêu persona có thật, ở cả hai config (gõ sai tên thì trần riêng âm thầm không có tác dụng)."""
    for env in ("dev", "prod"):
        d = cfg(env).to_dict(masked=False)
        assert set(d["limits"].get("token_budget_by_persona", {})) <= set(d["personas"]), env
        assert d["limits"]["token_budget_by_persona"] == cfg("dev").to_dict(masked=False)["limits"]["token_budget_by_persona"]  # dev và prod cùng giá trị
