import os

import pytest
from langchain_core.messages import HumanMessage, ToolMessage

from ai_agent.config import ConfigError, load_config
from ai_agent.guard.startup import check_no_tracing
from ai_agent.llm import get_llm, get_llm_for_role
from ai_agent.llm.registry import LLM_PROVIDERS, register_llm

from helpers import CONFIG_DIR, cfg


def test_dev_and_prod_load_and_roles_resolve():
    for env in ("dev", "prod"):
        c = cfg(env)
        assert c.app.env == env
        for role in ("buyer", "seller", "admin", "dev", "summarizer"):
            assert c.llm.roles[role] in c.llm.profiles


def test_env_expansion_and_missing_required(tmp_path, monkeypatch):
    (tmp_path / "config.t.yaml").write_text("a: '${X_REQ}'\nb: '${X_OPT:-dflt}'\nc: '${X_EMPTY:-}'\n")
    monkeypatch.delenv("X_REQ", raising=False)
    with pytest.raises(ConfigError, match="X_REQ"):
        load_config("t", tmp_path)
    monkeypatch.setenv("X_REQ", "v")
    c = load_config("t", tmp_path)
    assert (c.a, c.b, c.c) == ("v", "dflt", None)


def test_secrets_are_masked_when_printed():
    c = cfg()
    assert "sk-123" not in repr(c)
    c2 = load_config("dev", CONFIG_DIR)
    os.environ["DEEPSEEK_API_KEY"] = "sk-123"
    try:
        c3 = load_config("dev", CONFIG_DIR)
        assert c3.llm.profiles.deepseek_flash.api_key == "sk-123"
        assert "sk-123" not in repr(c3) and "********" in repr(c3)
    finally:
        del os.environ["DEEPSEEK_API_KEY"]
    assert c2.llm.profiles.deepseek_flash.api_key is None


def test_factory_errors_name_the_profile():
    c = cfg()
    with pytest.raises(ValueError, match="deepseek_flash.*api_key"):
        get_llm(c, "deepseek_flash")                    # khoá để trống: chỉ lỗi khi profile này được dùng
    with pytest.raises(ValueError, match="claude_haiku.*model_id"):
        get_llm(c, "claude_haiku")                      # giá trị mẫu còn sót
    with pytest.raises(ValueError, match="Không có llm.profiles.nope"):
        get_llm(c, "nope")
    with pytest.raises(ValueError, match="llm.roles.zzz"):
        get_llm_for_role(c, "zzz")


def test_registry_rejects_duplicates_and_openai_builds_with_key(monkeypatch):
    with pytest.raises(ValueError):
        register_llm("fake")(lambda p: None)
    assert {"openai", "anthropic", "fake"} <= set(LLM_PROVIDERS)
    monkeypatch.setenv("DEEPSEEK_API_KEY", "sk-test")
    m = get_llm(load_config("dev", CONFIG_DIR), "deepseek_flash")   # chỉ dựng đối tượng, không gọi mạng
    assert type(m).__name__ == "ChatOpenAI" and m.openai_api_base == "https://api.deepseek.com"


def test_fake_model_follows_the_script_per_turn():
    m = get_llm_for_role(cfg(), "buyer")
    first = m.invoke([HumanMessage("don cua toi")])
    assert first.tool_calls[0]["name"] == "list_my_orders" and first.usage_metadata["total_tokens"] > 0
    second = m.invoke([HumanMessage("x"), first, ToolMessage("{}", tool_call_id=first.tool_calls[0]["id"])])
    assert second.content and not second.tool_calls
    # lượt mới (tin người dùng mới) bắt đầu lại từ bước 0
    again = m.invoke([HumanMessage("x"), first, second, HumanMessage("y")])
    assert again.tool_calls


def test_fake_stream_merges_tool_call_chunks():
    m = get_llm_for_role(cfg(), "buyer")
    agg = None
    for ch in m.stream([HumanMessage("x")]):
        agg = ch if agg is None else agg + ch
    assert agg.tool_calls[0]["args"] == {"page_size": 3}


def test_tracing_vars_fatal_in_prod_only():
    env = {"LANGSMITH_TRACING": "true", "LANGSMITH_API_KEY": "k"}
    assert check_no_tracing("dev", env) == ["LANGSMITH_TRACING", "LANGSMITH_API_KEY"]
    with pytest.raises(RuntimeError):
        check_no_tracing("prod", env)
    assert check_no_tracing("prod", {"LANGSMITH_TRACING": "false"}) == []


def test_openai_session_header_follows_conversation():
    import asyncio
    import httpx
    from ai_agent.llm.providers.openai import _session_clients
    from ai_agent.runtime.context import RunContext, reset_run, set_run

    seen = {}
    clients = _session_clients("x-opencode-session")
    hook = clients["http_client"]._event_hooks["request"][0]

    req = httpx.Request("POST", "http://x/v1/chat/completions")
    hook(req)
    assert "x-opencode-session" not in req.headers  # ngoài lượt chạy: không gắn

    ctx = RunContext(token="t", run_id="run-1", request_id="", user_id=1, role="buyer")
    ctx.conversation_id = "conv-9"
    tok = set_run(ctx)
    try:
        req = httpx.Request("POST", "http://x/v1/chat/completions")
        hook(req)
        seen["h"] = req.headers["x-opencode-session"]
        req2 = httpx.Request("POST", "http://x/")
        asyncio.run(clients["http_async_client"]._event_hooks["request"][0](req2))
        seen["a"] = req2.headers["x-opencode-session"]
    finally:
        reset_run(tok)
    assert seen == {"h": "conv-9", "a": "conv-9"}
