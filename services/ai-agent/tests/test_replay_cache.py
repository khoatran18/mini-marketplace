"""Cache phát lại lời gọi mô hình (llm/replay_cache.py): khoá, mẫu thứ k, chế độ ro, không ghi lỗi, SSE, provider chỉ bật khi có biến môi trường."""
import json

import httpx
import pytest

from ai_agent.llm import replay_cache as rc

BODY = {"model": "hy4-flash-free", "temperature": 0, "messages": [{"role": "system", "content": "hôm nay 2026-11-21"}, {"role": "user", "content": "đơn của tôi"}]}
URL = "https://opencode.test/v1/chat/completions"


class Upstream(httpx.BaseTransport):
    """Mô hình giả: mỗi lần gọi trả một mẫu khác nhau (như mô hình không xác định)."""

    def __init__(self, status=200, sse=False):
        self.calls, self.status, self.sse = 0, status, sse

    def handle_request(self, request):
        self.calls += 1
        if self.sse:
            text = f'data: {{"choices":[{{"delta":{{"content":"mẫu {self.calls}"}}}}]}}\n\ndata: [DONE]\n\n'
            return httpx.Response(self.status, headers={"content-type": "text/event-stream"}, content=text.encode(), request=request)
        return httpx.Response(self.status, json={"choices": [{"message": {"content": f"mẫu {self.calls}"}}]}, request=request)


def post(client, body=BODY, headers=None):
    return client.post(URL, json=body, headers=headers or {})


def mk(tmp_path, mode="rw", **kw):
    up = Upstream(**kw)
    cache = rc.ReplayCache(tmp_path, mode)
    return up, cache, httpx.Client(transport=rc.CachingTransport(up, cache))


def test_second_run_replays_without_calling_model(tmp_path):
    up, cache, c = mk(tmp_path)
    first = post(c).json()
    assert up.calls == 1 and cache.misses == 1
    up2, cache2, c2 = mk(tmp_path)  # tiến trình mới, cùng thư mục
    assert post(c2).json() == first and up2.calls == 0 and cache2.hits == 1


def test_key_ignores_headers_but_not_body(tmp_path):
    up, _, c = mk(tmp_path)
    post(c, headers={"x-opencode-session": "a", "authorization": "Bearer k1"})
    _, _, c2 = mk(tmp_path)
    up2 = c2._transport.inner
    post(c2, headers={"x-opencode-session": "b", "authorization": "Bearer k2"})
    assert up2.calls == 0  # header khác vẫn trúng
    changed = {**BODY, "messages": [{"role": "user", "content": "khác"}]}
    post(c2, changed)
    assert up2.calls == 1  # nội dung khác thì trượt
    post(c2, {**BODY, "temperature": 0.7})
    assert up2.calls == 2  # tham số lấy mẫu khác thì trượt
    post(c2, {**BODY, "model": "model-khac"})
    assert up2.calls == 3  # model khác thì trượt


def test_json_key_order_does_not_matter():
    a = rc.ReplayCache.key("h", json.dumps({"a": 1, "b": 2}).encode())
    b = rc.ReplayCache.key("h", json.dumps({"b": 2, "a": 1}).encode())
    assert a == b and a != rc.ReplayCache.key("h2", json.dumps({"a": 1, "b": 2}).encode())


def test_kth_sample_keeps_repeat_meaningful(tmp_path):
    up, _, c = mk(tmp_path)
    got = [post(c).json()["choices"][0]["message"]["content"] for _ in range(3)]
    assert got == ["mẫu 1", "mẫu 2", "mẫu 3"] and up.calls == 3  # cùng request nhưng 3 mẫu khác nhau, không bị gộp
    up2, _, c2 = mk(tmp_path)
    again = [post(c2).json()["choices"][0]["message"]["content"] for _ in range(3)]
    assert again == got and up2.calls == 0  # chạy lại phát đúng 3 mẫu theo thứ tự
    assert post(c2).json()["choices"][0]["message"]["content"] == "mẫu 1" and up2.calls == 1  # mẫu thứ 4 mới gọi thêm (upstream mới đếm từ 1)


def test_ro_mode_never_calls_model(tmp_path):
    up, _, c = mk(tmp_path, mode="ro")
    with pytest.raises(rc.CacheMiss):
        post(c)
    assert up.calls == 0


def test_errors_are_not_cached(tmp_path):
    up, _, c = mk(tmp_path, status=429)
    assert post(c).status_code == 429
    assert not list(tmp_path.rglob("*.json"))
    up.status = 200
    assert post(c).status_code == 200 and up.calls == 2  # lần sau gọi lại, không dính 429 cũ


def test_sse_stream_is_replayed(tmp_path):
    up, _, c = mk(tmp_path, sse=True)
    with c.stream("POST", URL, json=BODY) as r:
        first = "".join(r.iter_lines())
    _, _, c2 = mk(tmp_path, sse=True)
    with c2.stream("POST", URL, json=BODY) as r:
        assert "".join(r.iter_lines()) == first and r.headers["content-type"] == "text/event-stream"
    assert c2._transport.inner.calls == 0


def test_other_paths_are_not_cached(tmp_path):
    up, cache, c = mk(tmp_path)
    c.get("https://opencode.test/v1/models")
    assert up.calls == 1 and cache.hits == cache.misses == 0


def test_env_switch(tmp_path, monkeypatch):
    monkeypatch.delenv("AI_LLM_CACHE_DIR", raising=False)
    assert rc.from_env() is None  # mặc định tắt
    monkeypatch.setenv("AI_LLM_CACHE_DIR", str(tmp_path))
    monkeypatch.setenv("AI_LLM_CACHE_MODE", "ro")
    cache = rc.from_env()
    assert cache is not None and cache.mode == "ro" and rc.from_env() is cache
    monkeypatch.setenv("AI_LLM_CACHE_MODE", "xx")
    with pytest.raises(ValueError):
        rc.from_env()


def test_openai_provider_wraps_transport_only_when_enabled(tmp_path, monkeypatch):
    from ai_agent.llm.providers import openai as prov

    monkeypatch.delenv("AI_LLM_CACHE_DIR", raising=False)
    plain = prov._session_clients("x-opencode-session", None)["http_client"]
    assert not isinstance(plain._transport, rc.CachingTransport)
    monkeypatch.setenv("AI_LLM_CACHE_DIR", str(tmp_path))
    cache = rc.from_env()
    wrapped = prov._session_clients("x-opencode-session", cache)
    assert isinstance(wrapped["http_client"]._transport, rc.CachingTransport)
    assert isinstance(wrapped["http_async_client"]._transport, rc.CachingAsyncTransport)
