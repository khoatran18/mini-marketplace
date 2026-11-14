"""Đọc token agent (JWT) để biết ai và role gì. KHÔNG kiểm chữ ký: ai-agent không có khoá (khoá ký chỉ gateway giữ, ADR-42).

Vì vậy ai-agent CHỈ được nhận request từ gateway qua mạng nội bộ (không publish cổng, xem deploy/services.yml); gateway mới là nơi
kiểm chữ ký, và mọi lời gọi tool quay lại gateway kèm token này nên token giả sẽ bị gateway từ chối (401) ở lần gọi đầu tiên.
Ở đây chỉ đọc claim để chọn persona, kiểm hạn và gắn run_id; không dùng để cấp quyền."""
from __future__ import annotations

import base64
import json
import time
from dataclasses import dataclass


class AuthError(Exception):
    pass


@dataclass(frozen=True)
class AgentIdentity:
    user_id: int
    role: str
    username: str
    run_id: str  # jti
    exp: int


def _b64(part: str) -> bytes:
    return base64.urlsafe_b64decode(part + "=" * (-len(part) % 4))


def parse_agent_token(token: str, run_id_header: str = "", now: float | None = None) -> AgentIdentity:
    parts = token.split(".")
    if len(parts) != 3:
        raise AuthError("token không hợp lệ")
    try:
        claims = json.loads(_b64(parts[1]))
    except (ValueError, json.JSONDecodeError):
        raise AuthError("token không hợp lệ") from None
    if not isinstance(claims, dict) or claims.get("typ") != "agent":
        raise AuthError("không phải token agent")
    aud = claims.get("aud")
    if "gateway-agent" not in (aud if isinstance(aud, list) else [aud]):
        raise AuthError("sai đối tượng nhận (aud)")
    sub, jti, role, exp = claims.get("sub"), claims.get("jti"), claims.get("role"), claims.get("exp")
    if not (isinstance(sub, str) and sub.isdigit() and int(sub) > 0 and isinstance(jti, str) and jti and isinstance(role, str) and role and isinstance(exp, (int, float))):
        raise AuthError("thiếu claim bắt buộc")
    if exp <= (now if now is not None else time.time()):
        raise AuthError("token đã hết hạn")
    if run_id_header and run_id_header != jti:
        raise AuthError("X-Agent-Run-ID không khớp token")
    return AgentIdentity(int(sub), role, str(claims.get("username") or ""), jti, int(exp))
