"""Client gọi gateway bằng TOKEN AGENT (không phải JWT người dùng). Timeout cứng; chỉ GET, cộng đúng MỘT POST là /ai/proposals (tạo đề xuất, chưa thực hiện gì); lỗi đổi thành thông điệp an toàn."""
from __future__ import annotations

from typing import Any

import httpx

from ai_agent.config.logging import get_logger

logger = get_logger(__name__)


class GatewayError(Exception):
    """Lỗi khi gọi gateway. `message` an toàn để đưa cho mô hình/người dùng; `fatal` = dừng cả lượt (token hết hạn)."""

    def __init__(self, status: int, message: str, fatal: bool = False) -> None:
        super().__init__(message)
        self.status, self.message, self.fatal = status, message, fatal


def _safe(status: int) -> GatewayError:
    if status == 401:
        return GatewayError(401, "phiên làm việc của lượt này đã hết hạn, hãy gửi lại câu hỏi", fatal=True)
    if status == 403:
        return GatewayError(403, "không có quyền truy cập dữ liệu này")
    if status == 404:
        return GatewayError(404, "không tìm thấy dữ liệu")
    if status == 429:
        return GatewayError(429, "hệ thống đang bận, thử lại sau ít phút")
    if status in (400, 422):
        return GatewayError(status, "tham số không hợp lệ")
    return GatewayError(status, "dịch vụ tạm thời không phản hồi")


class GatewayClient:
    def __init__(self, base_url: str, timeout: float = 5.0, client: httpx.AsyncClient | None = None) -> None:
        self.base_url = base_url.rstrip("/")
        self._client = client or httpx.AsyncClient(timeout=timeout, limits=httpx.Limits(max_connections=50))

    async def aclose(self) -> None:
        await self._client.aclose()

    async def get(self, token: str, path: str, query: dict[str, Any] | None = None, request_id: str = "") -> Any:
        headers = {"Authorization": f"Bearer {token}", "Accept": "application/json"}
        if request_id:
            headers["X-Request-ID"] = request_id
        try:
            resp = await self._client.get(self.base_url + path, params=query or None, headers=headers)
        except httpx.TimeoutException:
            raise GatewayError(504, "dịch vụ phản hồi quá chậm") from None
        except httpx.HTTPError as exc:
            logger.warning("gateway unreachable", extra={"error": type(exc).__name__})
            raise GatewayError(503, "dịch vụ tạm thời không phản hồi") from None
        if resp.status_code >= 400:
            raise _safe(resp.status_code)
        try:
            return resp.json()
        except ValueError:
            raise GatewayError(502, "phản hồi không hợp lệ") from None

    async def post_proposal(self, token: str, payload: dict[str, Any], request_id: str = "") -> dict[str, Any]:
        """POST /ai/proposals: xin gateway ký một đề xuất. Không thực hiện hành động; người dùng phải xác nhận trên UI."""
        headers = {"Authorization": f"Bearer {token}", "Accept": "application/json"}
        if request_id:
            headers["X-Request-ID"] = request_id
        try:
            resp = await self._client.post(self.base_url + "/ai/proposals", json=payload, headers=headers)
        except httpx.TimeoutException:
            raise GatewayError(504, "dịch vụ phản hồi quá chậm") from None
        except httpx.HTTPError as exc:
            logger.warning("gateway unreachable", extra={"error": type(exc).__name__})
            raise GatewayError(503, "dịch vụ tạm thời không phản hồi") from None
        if resp.status_code == 403:
            raise GatewayError(403, "hành động này không được phép đề xuất")
        if resp.status_code >= 400:
            raise _safe(resp.status_code)
        try:
            return resp.json()
        except ValueError:
            raise GatewayError(502, "phản hồi không hợp lệ") from None

    async def list_tools(self, token: str, request_id: str = "") -> dict[str, Any]:
        """GET /ai/tools: danh sách tool của role trong token."""
        return await self.get(token, "/ai/tools", None, request_id)

    async def system_token(self, key: str) -> tuple[str, float]:
        """POST /ai/system-token (ADR-47): đổi AI_SYSTEM_KEY lấy token hệ thống chỉ đọc. Trả (token, số giây còn hiệu lực). Lỗi -> GatewayError."""
        try:
            resp = await self._client.post(self.base_url + "/ai/system-token", headers={"X-System-Key": key, "Accept": "application/json"})
        except httpx.HTTPError as exc:
            logger.warning("gateway unreachable (system token)", extra={"error": type(exc).__name__})
            raise GatewayError(503, "dịch vụ tạm thời không phản hồi") from None
        if resp.status_code >= 400:
            logger.warning("system token refused", extra={"status": resp.status_code})
            raise GatewayError(resp.status_code, "gateway từ chối cấp token hệ thống")
        try:
            data = resp.json()
            return str(data["token"]), float(data.get("expires_in", 0))
        except (ValueError, KeyError, TypeError):
            raise GatewayError(502, "phản hồi không hợp lệ") from None
