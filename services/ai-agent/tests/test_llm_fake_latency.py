"""Mô hình giả có độ trễ (ttft_ms, tokens_per_sec) dùng cho thử tải: chờ bằng asyncio.sleep, kết quả không đổi."""
import asyncio
import time

from langchain_core.messages import HumanMessage

from ai_agent.llm.providers.fake import ScriptedChatModel, build

SCRIPT = [{"content": "một hai ba bốn năm"}]


async def _collect(model):
    return [c async for c in model.astream([HumanMessage(content="hi")])]


def _text(chunks):
    return "".join(str(c.content) for c in chunks)


def test_no_latency_by_default_and_same_text():
    chunks = asyncio.run(_collect(ScriptedChatModel(script=SCRIPT)))
    assert _text(chunks) == "một hai ba bốn năm"


def test_ttft_and_token_rate_delay_the_stream():
    m = ScriptedChatModel(script=SCRIPT, ttft_ms=100, tokens_per_sec=50)  # 100 ms + 4 khoảng x 20 ms
    t0 = time.perf_counter()
    chunks = asyncio.run(_collect(m))
    assert time.perf_counter() - t0 >= 0.17
    assert _text(chunks) == "một hai ba bốn năm"


def test_latency_does_not_block_the_event_loop():
    m = ScriptedChatModel(script=SCRIPT, ttft_ms=200)

    async def run():
        t0 = time.perf_counter()
        await asyncio.gather(*[_collect(m) for _ in range(20)])
        return time.perf_counter() - t0

    assert asyncio.run(run()) < 0.8  # 20 lượt song song xong gần bằng một lượt (không nối đuôi 20 x 200 ms)


def test_profile_reads_latency_keys():
    m = build({"provider": "fake", "script": SCRIPT, "ttft_ms": 800, "tokens_per_sec": 60})
    assert m.ttft_ms == 800 and m.tokens_per_sec == 60
    assert build({"provider": "fake"}).ttft_ms == 0
