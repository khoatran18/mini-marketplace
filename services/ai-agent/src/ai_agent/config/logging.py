"""Log JSON một dòng, có run_id của lượt đang chạy (contextvar). Không log nội dung tin nhắn hay token."""
from __future__ import annotations

import contextvars
import json
import logging
import sys
import time

run_id_var: contextvars.ContextVar[str] = contextvars.ContextVar("run_id", default="")
request_id_var: contextvars.ContextVar[str] = contextvars.ContextVar("request_id", default="")

_RESERVED = set(logging.LogRecord("", 0, "", 0, "", (), None).__dict__) | {"message", "asctime"}


class JsonFormatter(logging.Formatter):
    def format(self, record: logging.LogRecord) -> str:
        out = {
            "ts": time.strftime("%Y-%m-%dT%H:%M:%S", time.gmtime(record.created)) + f".{int(record.msecs):03d}Z",
            "level": record.levelname.lower(),
            "logger": record.name,
            "msg": record.getMessage(),
        }
        if run_id_var.get():
            out["run_id"] = run_id_var.get()
        if request_id_var.get():
            out["request_id"] = request_id_var.get()
        for k, v in record.__dict__.items():  # logger.info("x", extra={"tool": "..."})
            if k not in _RESERVED and not k.startswith("_"):
                out[k] = v
        if record.exc_info:
            out["exc"] = self.formatException(record.exc_info)
        return json.dumps(out, ensure_ascii=False, default=str)


def setup_logging(level: str = "INFO") -> None:
    handler = logging.StreamHandler(sys.stdout)
    handler.setFormatter(JsonFormatter())
    root = logging.getLogger()
    root.handlers[:] = [handler]
    root.setLevel(level.upper())
    logging.getLogger("httpx").setLevel(logging.WARNING)  # httpx ghi URL (có thể chứa tham số tìm kiếm của người dùng)


def get_logger(name: str) -> logging.Logger:
    return logging.getLogger(name)
