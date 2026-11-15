"""Đọc ops-service QUA GATEWAY bằng token hệ thống (ADR-47): cổng `OpsPort` của đồ thị và `OpsReader` của bộ theo dõi.
Chỉ GET. Mọi lời gọi đi qua gateway nên RBAC và nhật ký như các tool đọc của agent."""
from __future__ import annotations

import base64
import json
import time
from typing import Any

from ai_agent.tools.gateway import GatewayClient, GatewayError

_SKEW = 60.0  # xin token mới khi còn dưới ngần này giây


class SystemTokens:
    """Giữ token hệ thống còn hạn; xin lại khi sắp hết. `key` rỗng = không có danh tính hệ thống (get() báo lỗi rõ ràng)."""

    def __init__(self, gateway: GatewayClient, key: str, clock=time.monotonic) -> None:
        self._gw, self._key, self._clock = gateway, key, clock
        self._tok = ""
        self._until = 0.0

    @property
    def enabled(self) -> bool:
        return bool(self._key)

    async def get(self) -> str:
        if not self._key:
            raise GatewayError(503, "chưa cấu hình AI_SYSTEM_KEY: không có danh tính hệ thống")
        now = self._clock()
        if not self._tok or now >= self._until - _SKEW:
            self._tok, ttl = await self._gw.system_token(self._key)
            self._until = now + ttl
        return self._tok


def jti_of(token: str) -> str:
    """Mã phiên (jti) trong token agent: dùng làm run_id của RunContext. Chỉ đọc claim, không kiểm chữ ký (như api/auth.py)."""
    try:
        part = token.split(".")[1]
        return str(json.loads(base64.urlsafe_b64decode(part + "=" * (-len(part) % 4))).get("jti") or "")
    except (IndexError, ValueError):
        return ""


class GatewayOps:
    def __init__(self, gateway: GatewayClient, tokens: SystemTokens) -> None:
        self._gw, self._tokens = gateway, tokens

    async def _get(self, path: str, query: dict[str, Any] | None = None) -> Any:
        return await self._gw.get(await self._tokens.get(), path, query)

    # ---- OpsPort (đồ thị) ----
    async def list_actions(self) -> list[dict[str, Any]]:
        return list((await self._get("/ops/actions")).get("actions") or [])

    async def pending_proposals(self, incident_id: int) -> int:
        out = await self._get("/ops/proposals", {"state": "PENDING", "incident_id": incident_id, "limit": 50})
        return len(out.get("proposals") or [])

    # ---- OpsReader (bộ theo dõi) ----
    async def open_incidents(self) -> list[dict[str, Any]]:
        return list((await self._get("/ops/incidents", {"state": "open", "limit": 50})).get("incidents") or [])

    async def incident(self, incident_id: int) -> dict[str, Any] | None:
        try:
            return (await self._get(f"/ops/incidents/{int(incident_id)}")).get("incident")
        except GatewayError as exc:
            if exc.status == 404:
                return None
            raise

    async def proposals_for_incident(self, incident_id: int) -> list[dict[str, Any]]:
        return list((await self._get("/ops/proposals", {"incident_id": incident_id, "limit": 50})).get("proposals") or [])

    async def proposal(self, proposal_id: int) -> dict[str, Any] | None:
        try:
            return (await self._get(f"/ops/proposals/{int(proposal_id)}")).get("proposal")
        except GatewayError as exc:
            if exc.status == 404:
                return None
            raise
