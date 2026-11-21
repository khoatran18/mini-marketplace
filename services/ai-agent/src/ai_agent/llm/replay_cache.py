"""Cache phát lại các lần gọi mô hình, để chạy lại eval mà không tốn lượt gọi LLM. CHỈ bật khi có biến môi trường (production không đặt):

    AI_LLM_CACHE_DIR=<thư mục>      bật cache, ghi/đọc ở đây
    AI_LLM_CACHE_MODE=rw|ro         rw (mặc định): trúng thì phát lại, trượt thì gọi thật rồi ghi; ro: trượt = lỗi CacheMiss, đảm bảo KHÔNG gọi mô hình

Đặt ở tầng HTTP (bọc transport của httpx mà SDK OpenAI dùng) nên lưu đúng byte đã gửi/nhận, kể cả luồng SSE, và không phụ thuộc LangChain.

KHOÁ = sha256 của JSON chuẩn hoá {phiên bản, host, body request} + số thứ tự mẫu k.
  - body request đã gồm model_id thật, temperature, max_tokens, tool_choice, extra_body (bật/tắt suy luận), toàn bộ messages (system prompt kèm ngày, lịch sử,
    kết quả tool) và schema tool, nên đổi prompt / tool / gateway giả / ngày thì khoá tự đổi (cache cũ tự vô hiệu).
  - KHÔNG vào khoá: header (API key, mã phiên x-opencode-session, User-Agent), timeout, thời điểm gọi, tên profile (chỉ là nhãn).
  - k: mô hình không xác định, nên lần thứ k gặp cùng một request trong một tiến trình là mẫu thứ k. Lần đầu ghi #0, #1, #2; chạy lại phát lại đúng các mẫu đó
    theo thứ tự, và chỉ gọi thêm khi cần mẫu mới (vd --repeat 3 rồi --repeat 5 chỉ gọi thêm 2). Cache theo nội dung thuần sẽ làm --repeat vô nghĩa.
Không ghi lỗi (429, 5xx, timeout) và không ghi phản hồi không phải 200.
"""
from __future__ import annotations

import base64
import hashlib
import json
import os
import threading
import time
from collections import Counter
from pathlib import Path
from typing import Any

import httpx

SCHEMA = 1
_PATH_SUFFIX = "/chat/completions"


class CacheMiss(RuntimeError):
    """Chế độ ro: request chưa có trong cache nên không được gọi mô hình."""


class ReplayCache:
    def __init__(self, directory: str | Path, mode: str = "rw") -> None:
        if mode not in ("rw", "ro"):
            raise ValueError(f"AI_LLM_CACHE_MODE phải là rw hoặc ro, nhận {mode!r}")
        self.dir = Path(directory)
        self.mode = mode
        self._seen: Counter[str] = Counter()
        self._lock = threading.Lock()
        self.hits = 0
        self.misses = 0

    @staticmethod
    def key(host: str, body: bytes) -> str:
        try:
            canon = json.dumps(json.loads(body), sort_keys=True, ensure_ascii=False, separators=(",", ":"))
        except ValueError:
            canon = base64.b64encode(body).decode()
        return hashlib.sha256(f"v{SCHEMA}|{host}|{canon}".encode()).hexdigest()

    def next_slot(self, key: str) -> tuple[Path, int]:
        with self._lock:
            k = self._seen[key]
            self._seen[key] += 1
        return self.dir / key[:2] / f"{key}.{k}.json", k

    def load(self, path: Path) -> dict[str, Any] | None:
        try:
            return json.loads(path.read_text("utf-8"))
        except (OSError, ValueError):
            return None

    def save(self, path: Path, entry: dict[str, Any]) -> None:
        path.parent.mkdir(parents=True, exist_ok=True)
        tmp = path.with_suffix(f".tmp{os.getpid()}{threading.get_ident()}")
        tmp.write_text(json.dumps(entry, ensure_ascii=False), "utf-8")
        os.replace(tmp, path)  # ghi nguyên tử: luồng khác không đọc phải file dở

    def count(self, hit: bool) -> None:
        with self._lock:
            if hit:
                self.hits += 1
            else:
                self.misses += 1

    # --- phát lại ---
    def replay(self, request: httpx.Request, path: Path) -> httpx.Response | None:
        entry = self.load(path)
        if entry is None:
            return None
        body = base64.b64decode(entry["body_b64"]) if "body_b64" in entry else entry["body"].encode()
        return httpx.Response(entry["status"], headers={"content-type": entry.get("content_type", "application/json")}, content=body, request=request)

    def entry_for(self, request: httpx.Request, response: httpx.Response, k: int) -> dict[str, Any]:
        raw = response.content
        try:
            body: dict[str, Any] = {"body": raw.decode("utf-8")}
        except UnicodeDecodeError:
            body = {"body_b64": base64.b64encode(raw).decode()}
        meta = {"host": request.url.host, "k": k, "at": time.strftime("%Y-%m-%dT%H:%M:%S%z")}
        try:
            req = json.loads(request.content)
            meta["model"] = req.get("model")
        except ValueError:
            pass
        return {"schema": SCHEMA, "status": response.status_code, "content_type": response.headers.get("content-type", "application/json"), **body, "meta": meta}

    def cacheable(self, request: httpx.Request) -> bool:
        return request.method == "POST" and request.url.path.endswith(_PATH_SUFFIX)

    def miss_guard(self, request: httpx.Request, path: Path) -> None:
        if self.mode == "ro":
            raise CacheMiss(f"cache ro: chưa có {path.name} cho {request.url.host}{request.url.path}; không gọi mô hình")

    def summary(self) -> str:
        return f"cache LLM: {self.hits} trúng, {self.misses} gọi mô hình thật ({self.mode}, {self.dir})"


class CachingTransport(httpx.BaseTransport):
    def __init__(self, inner: httpx.BaseTransport, cache: ReplayCache) -> None:
        self.inner, self.cache = inner, cache

    def handle_request(self, request: httpx.Request) -> httpx.Response:
        c = self.cache
        if not c.cacheable(request):
            return self.inner.handle_request(request)
        path, k = c.next_slot(c.key(request.url.host, request.content))
        hit = c.replay(request, path)
        if hit is not None:
            c.count(True)
            return hit
        c.miss_guard(request, path)
        resp = self.inner.handle_request(request)
        resp.read()
        if resp.status_code == 200:
            c.save(path, c.entry_for(request, resp, k))
        c.count(False)
        return httpx.Response(resp.status_code, headers={"content-type": resp.headers.get("content-type", "application/json")}, content=resp.content, request=request)

    def close(self) -> None:
        self.inner.close()


class CachingAsyncTransport(httpx.AsyncBaseTransport):
    def __init__(self, inner: httpx.AsyncBaseTransport, cache: ReplayCache) -> None:
        self.inner, self.cache = inner, cache

    async def handle_async_request(self, request: httpx.Request) -> httpx.Response:
        c = self.cache
        if not c.cacheable(request):
            return await self.inner.handle_async_request(request)
        path, k = c.next_slot(c.key(request.url.host, request.content))
        hit = c.replay(request, path)
        if hit is not None:
            c.count(True)
            return hit
        c.miss_guard(request, path)
        resp = await self.inner.handle_async_request(request)
        await resp.aread()
        if resp.status_code == 200:
            c.save(path, c.entry_for(request, resp, k))
        c.count(False)
        return httpx.Response(resp.status_code, headers={"content-type": resp.headers.get("content-type", "application/json")}, content=resp.content, request=request)

    async def aclose(self) -> None:
        await self.inner.aclose()


_shared: ReplayCache | None = None
_shared_key: tuple[str, str] | None = None


def from_env() -> ReplayCache | None:
    """Cache dùng chung của tiến trình theo biến môi trường; None = tắt (mặc định)."""
    global _shared, _shared_key
    d = os.environ.get("AI_LLM_CACHE_DIR", "").strip()
    if not d:
        return None
    mode = os.environ.get("AI_LLM_CACHE_MODE", "rw").strip() or "rw"
    if _shared is None or _shared_key != (d, mode):
        _shared, _shared_key = ReplayCache(d, mode), (d, mode)
    return _shared
