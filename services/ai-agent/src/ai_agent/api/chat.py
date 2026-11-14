"""POST /chat (SSE). Gateway gọi; header: X-Agent-Token, X-Agent-Run-ID, X-Request-ID. Body: {message, conversation_id?, client_message_id?, page_context?}."""
from __future__ import annotations

import re
from collections.abc import AsyncIterator
from typing import Any

from fastapi import APIRouter, Request
from fastapi.responses import JSONResponse, StreamingResponse
from pydantic import BaseModel

from ai_agent.api.auth import AuthError, parse_agent_token
from ai_agent.config.logging import get_logger
from ai_agent.personas import persona_for_role
from ai_agent.runtime import events
from ai_agent.runtime.chat import run_turn
from ai_agent.runtime.context import RunContext
from ai_agent.store.base import Busy, Duplicate, NotFound

logger = get_logger(__name__)
_UUID = re.compile(r"^[0-9a-fA-F-]{32,36}$")
_CLIENT_ID = re.compile(r"^[A-Za-z0-9._-]{1,64}$")


class ChatBody(BaseModel):
    message: str
    conversation_id: str | None = None
    client_message_id: str | None = None
    page_context: dict[str, Any] | None = None  # chưa dùng; nhận để UI gửi sẵn (không tin nội dung)


def _err(status: int, msg: str, **extra: Any) -> JSONResponse:
    return JSONResponse({"error": msg, **extra}, status_code=status)


async def _sse(gen: AsyncIterator[dict[str, Any]]) -> AsyncIterator[str]:
    try:
        async for ev in gen:
            yield events.sse(ev)
    finally:
        await gen.aclose()


def create_router(rt, store, personas) -> APIRouter:
    router = APIRouter()
    limits = rt.cfg.limits

    @router.post("/chat")
    async def chat(request: Request, body: ChatBody):
        token = request.headers.get("x-agent-token", "")
        try:
            ident = parse_agent_token(token, request.headers.get("x-agent-run-id", ""))
        except AuthError as exc:
            return _err(401, str(exc))
        persona = persona_for_role(personas, ident.role)
        if persona is None or not rt.cfg.personas.get(persona.name, {}).get("enabled", False):
            return _err(403, "vai trò này chưa được hỗ trợ")
        text = body.message.strip()
        if not text:
            return _err(422, "message trống")
        if len(text) > int(limits.max_message_chars):
            return _err(413, f"message dài quá {limits.max_message_chars} ký tự")
        if body.conversation_id is not None and not _UUID.match(body.conversation_id):
            return _err(422, "conversation_id không hợp lệ")
        if body.client_message_id is not None and not _CLIENT_ID.match(body.client_message_id):
            return _err(422, "client_message_id không hợp lệ")
        try:
            turn = await store.begin_turn(
                user_id=ident.user_id, conversation_id=body.conversation_id, client_message_id=body.client_message_id,
                run_id=ident.run_id, role=ident.role, text=text, max_history=int(limits.max_history_messages),
            )
        except NotFound:
            return _err(404, "không tìm thấy cuộc trò chuyện")  # cũng là câu trả lời khi cuộc chat của người khác
        except Busy:
            return _err(409, "cuộc trò chuyện đang xử lý tin trước, hãy đợi")
        except Duplicate as dup:
            return _err(409, "tin nhắn đã được gửi", run_id=dup.run_id)
        ctx = RunContext(token=token, run_id=ident.run_id, request_id=request.headers.get("x-request-id", ""),
                         user_id=ident.user_id, role=ident.role)
        return StreamingResponse(
            _sse(run_turn(rt, store, persona, ctx, turn, text)), media_type="text/event-stream",
            headers={"Cache-Control": "no-cache, no-transform", "X-Accel-Buffering": "no"},
        )

    return router
