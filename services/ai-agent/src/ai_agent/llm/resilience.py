"""Chịu lỗi nhà cung cấp mô hình: phân loại lỗi, circuit breaker theo profile, model dự phòng (docs/ai/12 mục 4, §16.35).

Chỉ MỘT tầng thử lại: SDK của nhà cung cấp (`max_retries` trong profile, có backoff và tôn trọng Retry-After). Ở đây KHÔNG thử lại lần nữa
cùng một profile (nhiều tầng thử lại chồng nhau là cách tạo retry storm). Việc của lớp này, đứng TRÊN SDK:
  1. phân loại lỗi (rate limit, hết hạn mức, quá tải, timeout, yêu cầu sai, khoá sai) để quyết định đổi sang profile dự phòng hay dừng;
  2. circuit breaker theo profile: tỷ lệ lỗi cao trong cửa sổ trượt thì NGẮT, các lượt sau thất bại ngay (không chờ timeout 60 s) hoặc đi thẳng
     sang profile dự phòng; sau `open_sec` cho vài lượt thăm dò (half-open);
  3. profile dự phòng theo thứ tự (`llm.fallbacks.<role>`). Lỗi `bad_request` (400, do mình gửi sai) KHÔNG đổi sang dự phòng và KHÔNG tính vào
     breaker: gửi lại cũng sai và không phải lỗi của nhà cung cấp.
Streaming: nếu lỗi xảy ra sau khi đã có token gửi đi thì lượt chat vẫn báo lỗi (không ghép hai mô hình vào một câu trả lời)."""
from __future__ import annotations

import time
from collections import deque
from enum import Enum
from typing import Any, Awaitable, Callable

from ai_agent.config.logging import get_logger

logger = get_logger(__name__)


class ErrorKind(str, Enum):
    RATE_LIMIT = "rate_limit"      # 429 tạm thời
    QUOTA = "quota"                # 429 hết hạn mức / hết tiền: thử lại vô ích, nên đổi nhà cung cấp
    OVERLOADED = "overloaded"      # 5xx, 529
    TIMEOUT = "timeout"            # quá hạn hoặc mất kết nối
    AUTH = "auth"                  # 401/403: khoá của profile này sai
    BAD_REQUEST = "bad_request"    # 400/404/422: do mình gửi sai
    UNKNOWN = "unknown"


# lỗi do nhà cung cấp (đếm vào breaker, được đổi sang dự phòng)
PROVIDER_FAULT = {ErrorKind.RATE_LIMIT, ErrorKind.QUOTA, ErrorKind.OVERLOADED, ErrorKind.TIMEOUT, ErrorKind.AUTH, ErrorKind.UNKNOWN}
_LABEL = {
    ErrorKind.RATE_LIMIT: "nhà cung cấp mô hình đang giới hạn tốc độ",
    ErrorKind.QUOTA: "hạn mức mô hình đã hết",
    ErrorKind.OVERLOADED: "nhà cung cấp mô hình đang quá tải hoặc lỗi",
    ErrorKind.TIMEOUT: "nhà cung cấp mô hình không phản hồi kịp",
    ErrorKind.AUTH: "khoá nhà cung cấp mô hình bị từ chối",
    ErrorKind.BAD_REQUEST: "yêu cầu gửi tới mô hình bị từ chối (400)",
    ErrorKind.UNKNOWN: "lỗi mô hình không xác định",
}


def label(kind: ErrorKind) -> str:
    return _LABEL[kind]


def classify(exc: BaseException) -> ErrorKind:
    """Phân loại theo mã HTTP (thuộc tính `status_code` của lỗi SDK openai/anthropic) rồi theo tên lớp lỗi (timeout, kết nối)."""
    status = getattr(exc, "status_code", None) or getattr(getattr(exc, "response", None), "status_code", None)
    if isinstance(status, int):
        if status == 429:
            blob = (str(getattr(exc, "body", "")) + " " + str(exc)).lower()
            return ErrorKind.QUOTA if any(w in blob for w in ("insufficient_quota", "quota", "billing", "credit", "exceeded your current")) else ErrorKind.RATE_LIMIT
        if status in (401, 403):
            return ErrorKind.AUTH
        if status in (408, 504):
            return ErrorKind.TIMEOUT
        if status >= 500:
            return ErrorKind.OVERLOADED
        if 400 <= status < 500:
            return ErrorKind.BAD_REQUEST
    name = type(exc).__name__.lower()
    if "timeout" in name or isinstance(exc, TimeoutError) or "connect" in name:
        return ErrorKind.TIMEOUT
    return ErrorKind.UNKNOWN


class LlmUnavailable(Exception):
    """Mọi profile (chính và dự phòng) đều lỗi hoặc đang bị ngắt. `kind` là loại lỗi cuối cùng; `tried` là các profile đã thử."""

    def __init__(self, kind: ErrorKind, tried: list[str], detail: str = "") -> None:
        self.kind, self.tried = kind, tried
        super().__init__(f"{label(kind)} (đã thử: {', '.join(tried) or 'không có'}){': ' + detail if detail else ''}")


class CircuitBreaker:
    """Cửa sổ trượt theo thời gian. CLOSED -> OPEN khi (số lần ≥ min_calls) và tỷ lệ lỗi ≥ failure_ratio; sau open_sec -> HALF_OPEN cho
    tối đa `probes` lượt thăm dò cùng lúc; thăm dò thành công -> CLOSED (xoá cửa sổ), thất bại -> OPEN lại. Một lần gọi CHẬM hơn `slow_sec` tính là lỗi."""

    def __init__(self, *, failure_ratio: float = 0.5, min_calls: int = 6, window_sec: float = 60.0, open_sec: float = 30.0,
                 probes: int = 1, clock: Callable[[], float] = time.monotonic) -> None:
        self.failure_ratio, self.min_calls, self.window_sec, self.open_sec, self.probes = failure_ratio, min_calls, window_sec, open_sec, probes
        self._clock = clock
        self._events: deque[tuple[float, bool]] = deque()
        self._opened_at: float | None = None
        self._inflight = 0

    @property
    def state(self) -> str:
        if self._opened_at is None:
            return "closed"
        return "half_open" if self._clock() - self._opened_at >= self.open_sec else "open"

    def allow(self) -> bool:
        st = self.state
        if st == "closed":
            return True
        if st == "half_open" and self._inflight < self.probes:
            self._inflight += 1
            return True
        return False

    def release(self) -> None:
        """Trả lại suất thăm dò khi lượt thăm dò kết thúc mà không nói gì về sức khoẻ nhà cung cấp (vd 400 do mình gửi sai)."""
        self._inflight = max(0, self._inflight - 1)

    def record(self, ok: bool) -> None:
        now = self._clock()
        if self._opened_at is not None:  # kết quả của một lượt thăm dò
            self._inflight = max(0, self._inflight - 1)
            if ok:
                self._opened_at, self._inflight = None, 0
                self._events.clear()
            else:
                self._opened_at = now
            return
        self._events.append((now, ok))
        while self._events and now - self._events[0][0] > self.window_sec:
            self._events.popleft()
        n = len(self._events)
        if n >= self.min_calls and sum(1 for _, o in self._events if not o) / n >= self.failure_ratio:
            self._opened_at = now
            logger.warning("llm circuit opened", extra={"calls": n})


class Resilience:
    """Một breaker cho mỗi profile (tạo khi cần). Dùng chung cho cả chat (middleware) và RCA (`Brain._ask`)."""

    def __init__(self, *, enabled: bool = True, clock: Callable[[], float] = time.monotonic, **breaker_kw: Any) -> None:
        self.enabled = enabled
        self._clock, self._kw = clock, breaker_kw
        self._breakers: dict[str, CircuitBreaker] = {}

    def breaker(self, name: str) -> CircuitBreaker:
        if name not in self._breakers:
            self._breakers[name] = CircuitBreaker(clock=self._clock, **self._kw)
        return self._breakers[name]

    async def run(self, candidates: list[tuple[str, Callable[[], Awaitable[Any]]]]) -> Any:
        """Thử lần lượt các (tên profile, hàm gọi); trả kết quả của cái đầu tiên thành công. Hết đường thì LlmUnavailable."""
        if not self.enabled:
            return await candidates[0][1]()
        from ai_agent import metrics as prom

        tried: list[str] = []
        last: ErrorKind | None = None
        detail = ""
        for name, fn in candidates:
            br = self.breaker(name)
            if not br.allow():
                tried.append(f"{name} (đang ngắt)")
                last = last or ErrorKind.OVERLOADED
                prom.LLM_FALLBACK.labels(name, "circuit_open").inc()
                continue
            tried.append(name)
            try:
                result = await fn()
            except Exception as exc:  # noqa: BLE001 - CancelledError không bị bắt (BaseException)
                kind = classify(exc)
                last, detail = kind, str(exc)[:200]
                if kind in PROVIDER_FAULT:
                    br.record(False)
                    prom.LLM_FALLBACK.labels(name, kind.value).inc()
                    continue  # thử profile kế tiếp
                if br.state != "closed":
                    br.release()
                raise  # bad_request: gửi sang dự phòng cũng sai, để nguyên lỗi gốc
            br.record(True)
            return result
        raise LlmUnavailable(last or ErrorKind.UNKNOWN, tried, detail)


def build_middleware_for(res: Resilience, primary: str, fallbacks: list[tuple[str, Any]]):
    """AgentMiddleware bọc mỗi lần gọi mô hình của agent chat. Nhập muộn: chỉ cần khi dựng agent thật."""
    from langchain.agents.middleware import AgentMiddleware

    class ResilientModel(AgentMiddleware):
        async def awrap_model_call(self, request, handler):
            cands: list[tuple[str, Callable[[], Awaitable[Any]]]] = [(primary, lambda: handler(request))]
            cands += [(n, (lambda m=m: handler(request.override(model=m)))) for n, m in fallbacks]
            return await res.run(cands)

    return ResilientModel()
