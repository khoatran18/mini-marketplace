"""Lớp HTTP (cần fastapi + httpx)."""
import json

import httpx
import pytest

fastapi = pytest.importorskip("fastapi")

from ai_agent.main import create_app  # noqa: E402
from ai_agent.runtime.agent_factory import AgentRuntime  # noqa: E402

from helpers import agent_token, cfg, fake_builder, mock_gateway  # noqa: E402


def client():
    c = cfg()
    app = create_app(c, runtime=AgentRuntime(c, mock_gateway(), agent_builder=fake_builder))
    return httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://ai")


def hdr(**kw):
    t = agent_token(**kw)
    return {"X-Agent-Token": t, "X-Agent-Run-ID": kw.get("jti", "run-0000-1111"), "X-Request-ID": "req-1"}


def parse(text):
    out = []
    for block in text.strip().split("\n\n"):
        ev, data = block.split("\n", 1)
        out.append((ev.removeprefix("event: "), json.loads(data.removeprefix("data: "))))
    return out


async def test_health_and_chat_stream():
    async with client() as c:
        assert (await c.get("/health")).json() == {"status": "ok"}
        r = await c.post("/chat", json={"message": "đơn của tôi"}, headers=hdr())
        assert r.status_code == 200 and r.headers["content-type"].startswith("text/event-stream")
        evs = parse(r.text)
        assert evs[0][0] == "start" and evs[-1][0] == "done" and "conversation_id" in evs[0][1]


@pytest.mark.parametrize("role", ["admin", "operator"])
async def test_admin_and_operator_personas_are_enabled(role):
    async with client() as c:
        r = await c.post("/chat", json={"message": "x"}, headers=hdr(role=role))
        assert r.status_code == 200, r.text


@pytest.mark.parametrize("headers,body,status", [
    ({}, {"message": "x"}, 401),                                           # không token
    (hdr(role="nobody"), {"message": "x"}, 403),                           # role chưa có persona
    (hdr(), {"message": "  "}, 422),
    (hdr(), {"message": "x" * 5000}, 413),
    (hdr(), {"message": "x", "conversation_id": "not-a-uuid"}, 422),
    (hdr(), {"message": "x", "conversation_id": "00000000-0000-0000-0000-000000000000"}, 404),
])
async def test_chat_rejections(headers, body, status):
    async with client() as c:
        assert (await c.post("/chat", json=body, headers=headers)).status_code == status


def _capped(limit, role="buyer"):
    c = cfg()
    d = c.to_dict(masked=False)
    d["limits"]["daily_tokens"] = {role: limit}
    from ai_agent.config import Config

    cc = Config(d)
    return cc, create_app(cc, runtime=AgentRuntime(cc, mock_gateway(), agent_builder=fake_builder))


async def test_daily_token_limit_blocks_before_the_model_and_only_that_user():
    cc, app = _capped(50)
    async with httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://ai") as c:
        r = await c.post("/chat", json={"message": "đơn của tôi"}, headers=hdr(user_id=42, jti="run-aaaa-0001"))
        assert r.status_code == 200 and parse(r.text)[-1][0] == "done"        # dưới hạn mức: chạy bình thường
        used = parse(r.text)[-1][1]["input_tokens"] + parse(r.text)[-1][1]["output_tokens"]
        assert used >= 50, "lượt mẫu phải vượt hạn mức 50 để test này có nghĩa"
        r = await c.post("/chat", json={"message": "tiếp"}, headers=hdr(user_id=42, jti="run-aaaa-0002"))
        assert r.status_code == 429
        body = r.json()
        assert body["code"] == "daily_token_limit" and body["limit"] == 50 and body["used"] >= 50 and "hạn mức" in body["error"]
        r = await c.post("/chat", json={"message": "đơn của tôi"}, headers=hdr(user_id=43, jti="run-aaaa-0003"))
        assert r.status_code == 200                                              # người dùng khác không bị ảnh hưởng
        r = await c.post("/chat", json={"message": "tiếp"}, headers=hdr(user_id=42, role="seller_admin", jti="run-aaaa-0004"))
        assert r.status_code == 200                                              # persona khác không có hạn mức này


async def test_no_daily_limit_configured_means_unlimited():
    c = cfg()
    app = create_app(c, runtime=AgentRuntime(c, mock_gateway(), agent_builder=fake_builder))
    async with httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://ai") as cl:
        for i in range(3):
            r = await cl.post("/chat", json={"message": "đơn của tôi"}, headers=hdr(jti=f"run-bbbb-000{i}"))
            assert r.status_code == 200
