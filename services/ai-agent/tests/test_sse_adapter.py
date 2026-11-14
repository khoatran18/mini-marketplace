import pytest
from langchain_core.messages import AIMessage, AIMessageChunk

from ai_agent.runtime import events
from ai_agent.runtime.context import RunContext
from ai_agent.runtime.sse_adapter import BudgetExceeded, pump, text_of


def ctx():
    return RunContext(token="t", run_id="r", request_id="", user_id=1, role="buyer")


async def gen(items):
    for i in items:
        yield i


def chunk(text, **kw):
    return ("messages", (AIMessageChunk(content=text, **kw), {"langgraph_node": "model"}))


def drain(c):
    out = []
    while not c.events.empty():
        out.append(c.events.get_nowait())
    return out


def test_text_of_handles_block_lists():
    assert text_of([{"type": "text", "text": "a"}, {"type": "tool_use"}, "b"]) == "ab" and text_of(None) == ""


async def test_pump_streams_tokens_sums_usage_and_returns_the_last_model_answer():
    c = ctx()
    res = await pump(gen([
        chunk("Xin "), chunk("chào"),
        ("messages", (AIMessageChunk(content="IGNORED"), {"langgraph_node": "tools"})),       # nút tool bị bỏ qua
        chunk("", usage_metadata={"input_tokens": 10, "output_tokens": 5, "total_tokens": 15}),
        ("updates", {"model": {"messages": [AIMessage(content="Xin chào")]}}),
    ]), c, token_budget=100)
    assert [e["data"]["text"] for e in drain(c)] == ["Xin ", "chào"]
    assert (res.answer, res.input_tokens, res.output_tokens) == ("Xin chào", 10, 5)


async def test_pump_enforces_the_token_budget():
    with pytest.raises(BudgetExceeded):
        await pump(gen([chunk("x", usage_metadata={"input_tokens": 90, "output_tokens": 20, "total_tokens": 110})]), ctx(), token_budget=100)


async def test_pump_emits_the_answer_once_when_the_provider_does_not_stream():
    c = ctx()
    await pump(gen([("updates", {"model": {"messages": [AIMessage(content="đủ câu")]}})]), c)
    ev = drain(c)
    assert len(ev) == 1 and ev[0]["event"] == events.TOKEN and ev[0]["data"]["text"] == "đủ câu"
