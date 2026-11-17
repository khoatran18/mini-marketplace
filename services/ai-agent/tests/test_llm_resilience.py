"""Chịu lỗi nhà cung cấp mô hình: phân loại lỗi, breaker, dự phòng, và việc RCA ghi đúng lý do lỗi."""
from __future__ import annotations

import pytest

from ai_agent.llm.resilience import CircuitBreaker, ErrorKind, LlmUnavailable, Resilience, classify


class Err(Exception):
    def __init__(self, status=None, body="", msg="boom"):
        super().__init__(msg)
        self.status_code, self.body = status, body


class Clock:
    def __init__(self):
        self.t = 1000.0

    def __call__(self):
        return self.t


@pytest.mark.parametrize("exc,kind", [
    (Err(429, "rate limit reached"), ErrorKind.RATE_LIMIT),
    (Err(429, {"error": {"code": "insufficient_quota"}}), ErrorKind.QUOTA),
    (Err(500), ErrorKind.OVERLOADED), (Err(529), ErrorKind.OVERLOADED), (Err(503), ErrorKind.OVERLOADED),
    (Err(401), ErrorKind.AUTH), (Err(403), ErrorKind.AUTH),
    (Err(400), ErrorKind.BAD_REQUEST), (Err(422), ErrorKind.BAD_REQUEST),
    (Err(504), ErrorKind.TIMEOUT), (TimeoutError("x"), ErrorKind.TIMEOUT),
    (ValueError("x"), ErrorKind.UNKNOWN),
])
def test_classify(exc, kind):
    assert classify(exc) is kind


def test_breaker_opens_on_failure_ratio_then_half_open_probe_closes_or_reopens():
    clk = Clock()
    br = CircuitBreaker(failure_ratio=0.5, min_calls=4, window_sec=60, open_sec=30, probes=1, clock=clk)
    for ok in (True, False, False):
        assert br.allow()
        br.record(ok)
    assert br.state == "closed"  # mới 3 lần: chưa đủ min_calls
    br.record(False)
    assert br.state == "open" and not br.allow()
    clk.t += 31
    assert br.state == "half_open" and br.allow() and not br.allow()  # chỉ một lượt thăm dò
    br.record(False)
    assert br.state == "open"  # thăm dò hỏng: ngắt lại
    clk.t += 31
    assert br.allow()
    br.record(True)
    assert br.state == "closed" and br.allow()


def test_breaker_forgets_old_failures_outside_the_window():
    clk = Clock()
    br = CircuitBreaker(failure_ratio=0.5, min_calls=4, window_sec=10, clock=clk)
    for _ in range(3):
        br.record(False)
    clk.t += 20
    for _ in range(3):
        br.record(True)
    br.record(False)
    assert br.state == "closed"


async def _ok(v):
    return v


def _fail(exc):
    async def f():
        raise exc
    return f


async def test_falls_back_on_provider_fault_but_not_on_bad_request():
    res = Resilience()
    assert await res.run([("a", _fail(Err(503))), ("b", lambda: _ok("B"))]) == "B"
    assert await res.run([("a", _fail(Err(429, "insufficient_quota"))), ("b", lambda: _ok("B"))]) == "B"
    with pytest.raises(Err) as ei:  # 400 là lỗi của mình: gửi sang dự phòng cũng sai, giữ nguyên lỗi gốc
        await res.run([("a", _fail(Err(400))), ("b", lambda: _ok("B"))])
    assert ei.value.status_code == 400
    assert sum(1 for _, ok in res.breaker("a")._events if not ok) == 2  # 503 và 429 đã đếm vào breaker; 400 thì không


async def test_all_profiles_failing_raises_llm_unavailable_with_last_kind_and_tried_list():
    res = Resilience()
    with pytest.raises(LlmUnavailable) as ei:
        await res.run([("a", _fail(Err(503))), ("b", _fail(Err(429, "rate limit")))])
    assert ei.value.kind is ErrorKind.RATE_LIMIT and ei.value.tried == ["a", "b"]


async def test_open_breaker_fails_fast_without_calling_the_provider_and_uses_fallback():
    clk = Clock()
    res = Resilience(clock=clk, min_calls=2, failure_ratio=0.5, open_sec=30)
    calls = {"a": 0}

    async def a():
        calls["a"] += 1
        raise Err(503)

    for _ in range(2):
        with pytest.raises(LlmUnavailable):
            await res.run([("a", a)])
    assert calls["a"] == 2 and res.breaker("a").state == "open"
    with pytest.raises(LlmUnavailable) as ei:
        await res.run([("a", a)])
    assert calls["a"] == 2 and "đang ngắt" in ei.value.tried[0]  # không gọi nhà cung cấp nữa
    assert await res.run([("a", a), ("b", lambda: _ok("B"))]) == "B" and calls["a"] == 2
    clk.t += 31  # sau open_sec: thăm dò, nhà cung cấp đã khoẻ lại thì đóng mạch
    assert await res.run([("a", lambda: _ok("A"))]) == "A" and res.breaker("a").state == "closed"


async def test_a_bad_request_probe_does_not_wedge_a_half_open_breaker():
    clk = Clock()
    res = Resilience(clock=clk, min_calls=1, failure_ratio=0.5, open_sec=30)
    with pytest.raises(LlmUnavailable):
        await res.run([("a", _fail(Err(503)))])
    clk.t += 31
    with pytest.raises(Err):
        await res.run([("a", _fail(Err(400)))])  # lượt thăm dò bị 400
    assert await res.run([("a", lambda: _ok("A"))]) == "A"  # suất thăm dò đã được trả lại


async def test_disabled_resilience_just_calls_the_primary():
    res = Resilience(enabled=False)
    with pytest.raises(Err):
        await res.run([("a", _fail(Err(503))), ("b", lambda: _ok("B"))])


async def test_runtime_invoke_model_uses_configured_fallback_profile(monkeypatch):
    from types import SimpleNamespace

    from ai_agent.runtime.agent_factory import AgentRuntime

    class M:
        def __init__(self, name, fail):
            self.name, self.fail = name, fail

        async def ainvoke(self, msgs):
            if self.fail:
                raise Err(503)
            return self.name

    rt = AgentRuntime.__new__(AgentRuntime)
    rt.cfg = SimpleNamespace(_data={}, llm=SimpleNamespace(roles={"rca": "main"}))
    rt._llm_factory = lambda cfg, role: M("main", True)
    rt._models, rt._fb_models = {}, {"backup": M("backup", False)}
    rt._fallbacks = {"rca": ["backup"]}
    rt.resilience = Resilience()
    assert await rt.invoke_model("rca", ["x"]) == "backup"


async def test_rca_run_records_why_it_failed_when_the_provider_is_down():
    from ai_agent.rca.budget import Budget
    from ai_agent.rca.graph import build_graph
    from rca_fakes import FakeOps, ScriptedBrain  # noqa: F401

    class DownBrain:
        async def collect(self, state):
            raise LlmUnavailable(ErrorKind.QUOTA, ["main"])

        async def report(self, state):
            raise LlmUnavailable(ErrorKind.QUOTA, ["main"])

    g = build_graph(DownBrain(), FakeOps(), Budget())
    out = await g.ainvoke({"incident": {"id": 1, "alert_name": "X", "severity": "critical", "component": "c", "summary": "", "labels": {}}})
    assert out["outcome"] == "error"
    assert "hạn mức" in out["report"]["error_reason"] and "hạn mức" in out["timeline"][0]["note"]


async def test_real_create_agent_switches_to_the_fallback_model_when_the_primary_is_down():
    """Đường dựng agent THẬT (create_agent + middleware), không phải agent giả của test khác."""
    from langchain.agents import create_agent
    from langchain_core.language_models.fake_chat_models import GenericFakeChatModel
    from langchain_core.messages import AIMessage, HumanMessage

    from ai_agent.llm.resilience import build_middleware_for

    class Down(GenericFakeChatModel):
        async def _agenerate(self, *a, **k):
            raise Err(503)

        async def _astream(self, *a, **k):
            raise Err(503)
            yield

    res = Resilience()
    mw = build_middleware_for(res, "main", [("backup", GenericFakeChatModel(messages=iter([AIMessage(content="từ dự phòng")])))])
    agent = create_agent(model=Down(messages=iter([])), tools=[], system_prompt="x", middleware=[mw])
    out = await agent.ainvoke({"messages": [HumanMessage("hi")]})
    assert out["messages"][-1].content == "từ dự phòng"
    assert sum(1 for _, ok in res.breaker("main")._events if not ok) == 1
