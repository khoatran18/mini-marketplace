"""Nơi lưu điểm dừng (checkpoint) của phiên RCA: Postgres (cùng cụm với bảng ai_*) hoặc RAM (test).

Bảng của langgraph (checkpoints, checkpoint_blobs, checkpoint_writes, checkpoint_migrations) do `setup()` của thư viện tạo,
NẰM NGOÀI `ai_agent_schema` của store/postgres.py (ADR-46). Dọn checkpoint của sự cố đã đóng: `delete_thread` (xem watcher)."""
from __future__ import annotations

from contextlib import asynccontextmanager
from typing import Any, AsyncIterator


def make_conninfo(*, host: str, port: int, user: str, password: str, database: str) -> str:
    from psycopg.conninfo import make_conninfo as _make

    return _make(host=host, port=port, user=user, password=password or None, dbname=database)


@asynccontextmanager
async def open_checkpointer(pg: dict[str, Any] | None = None, *, conninfo: str | None = None) -> AsyncIterator[Any]:
    """pg=None và conninfo=None: MemorySaver. Ngược lại AsyncPostgresSaver đã `setup()`."""
    if pg is None and conninfo is None:
        from langgraph.checkpoint.memory import MemorySaver

        yield MemorySaver()
        return
    from langgraph.checkpoint.postgres.aio import AsyncPostgresSaver

    info = conninfo or make_conninfo(host=str(pg["host"]), port=int(pg["port"]), user=str(pg["user"]), password=str(pg.get("password") or ""), database=str(pg["database"]))
    async with AsyncPostgresSaver.from_conn_string(info) as saver:
        await saver.setup()
        yield saver
