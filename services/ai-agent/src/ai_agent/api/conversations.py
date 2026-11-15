"""Lịch sử chat. Gateway gọi (header X-Agent-Token như /chat). Văn bản đã được che (tools/redact.mask_text) lúc lưu, giữ 30 ngày.
  GET /conversations[?limit=]               cuộc chat của CHÍNH người dùng, mới nhất trước
  GET /conversations/{id}                   tin của một cuộc chat của chính họ (của người khác -> 404, không lộ có tồn tại)
  GET /admin/conversations[?role=&user_id=&limit=]   mọi cuộc chat, CHỈ role admin
  GET /admin/conversations/{id}             một cuộc chat bất kỳ, CHỈ role admin (ghi log: ai xem cuộc chat của ai)"""
from __future__ import annotations

import re

from fastapi import APIRouter, Request
from fastapi.responses import JSONResponse

from ai_agent.api.auth import AgentIdentity, AuthError, parse_agent_token
from ai_agent.config.logging import get_logger
from ai_agent.store.base import NotFound

logger = get_logger(__name__)
_UUID = re.compile(r"^[0-9a-fA-F-]{32,36}$")
_ROLES = {"buyer", "seller_admin", "seller_employee", "admin", "operator"}


def _err(status: int, msg: str) -> JSONResponse:
    return JSONResponse({"error": msg}, status_code=status)


def _limit(raw: str | None, default: int = 30, cap: int = 100) -> int:
    try:
        n = int(raw) if raw else default
    except ValueError:
        return default
    return max(1, min(n, cap))


def create_router(store) -> APIRouter:
    router = APIRouter()

    def identity(request: Request) -> AgentIdentity | JSONResponse:
        try:
            return parse_agent_token(request.headers.get("x-agent-token", ""), request.headers.get("x-agent-run-id", ""))
        except AuthError as exc:
            return _err(401, str(exc))

    @router.get("/conversations")
    async def mine(request: Request):
        ident = identity(request)
        if isinstance(ident, JSONResponse):
            return ident
        rows = await store.list_conversations(user_id=ident.user_id, limit=_limit(request.query_params.get("limit")))
        return {"items": rows}

    @router.get("/conversations/{conversation_id}")
    async def one(conversation_id: str, request: Request):
        ident = identity(request)
        if isinstance(ident, JSONResponse):
            return ident
        if not _UUID.match(conversation_id):
            return _err(404, "không tìm thấy cuộc trò chuyện")
        try:
            return await store.get_conversation(conversation_id, user_id=ident.user_id)
        except NotFound:
            return _err(404, "không tìm thấy cuộc trò chuyện")

    @router.get("/admin/conversations")
    async def admin_list(request: Request):
        ident = identity(request)
        if isinstance(ident, JSONResponse):
            return ident
        if ident.role != "admin":
            return _err(403, "chỉ admin")
        role = request.query_params.get("role") or None
        if role is not None and role not in _ROLES:
            return _err(422, "role không hợp lệ")
        uid_raw = request.query_params.get("user_id")
        if uid_raw and not uid_raw.isdigit():
            return _err(422, "user_id không hợp lệ")
        rows = await store.list_conversations(user_id=int(uid_raw) if uid_raw else None, role=role, limit=_limit(request.query_params.get("limit")))
        return {"items": rows}

    @router.get("/admin/conversations/{conversation_id}")
    async def admin_one(conversation_id: str, request: Request):
        ident = identity(request)
        if isinstance(ident, JSONResponse):
            return ident
        if ident.role != "admin":
            return _err(403, "chỉ admin")
        if not _UUID.match(conversation_id):
            return _err(404, "không tìm thấy cuộc trò chuyện")
        try:
            conv = await store.get_conversation(conversation_id, user_id=None)
        except NotFound:
            return _err(404, "không tìm thấy cuộc trò chuyện")
        logger.info("admin viewed conversation", extra={"admin_id": ident.user_id, "conversation_id": conversation_id, "owner_id": conv["user_id"]})
        return conv

    return router
