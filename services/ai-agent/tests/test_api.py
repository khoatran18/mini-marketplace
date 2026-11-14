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


@pytest.mark.parametrize("headers,body,status", [
    ({}, {"message": "x"}, 401),                                           # không token
    (hdr(role="admin"), {"message": "x"}, 403),                            # role chưa có persona
    (hdr(), {"message": "  "}, 422),
    (hdr(), {"message": "x" * 5000}, 413),
    (hdr(), {"message": "x", "conversation_id": "not-a-uuid"}, 422),
    (hdr(), {"message": "x", "conversation_id": "00000000-0000-0000-0000-000000000000"}, 404),
])
async def test_chat_rejections(headers, body, status):
    async with client() as c:
        assert (await c.post("/chat", json=body, headers=headers)).status_code == status
