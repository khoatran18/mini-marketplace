"""Kho lịch sử chat trong Postgres (cùng cụm với các service khác, bảng riêng tiền tố ai_). Cùng giao diện với MemoryStore.

Lưu: cuộc chat, từng tin (CHỮ của người dùng và câu trả lời cuối, đã che email/số điện thoại), và mỗi lượt chạy (trạng thái, số
tool, token, tham chiếu card/đề xuất). KHÔNG lưu kết quả tool đầy đủ, token agent hay token đề xuất. Giữ `retention_days` ngày rồi xoá.
Quy tắc nghiệp vụ như store/base.py: NotFound nếu sai chủ, Busy nếu đang có lượt chạy, Duplicate nếu trùng client_message_id."""
from __future__ import annotations

import json
import uuid
from typing import Any

from ai_agent.config.logging import get_logger
from ai_agent.store.base import Busy, Duplicate, NotFound, Turn
from ai_agent.tools.redact import mask_text

logger = get_logger(__name__)

SCHEMA_VERSION = 1
# Lượt chạy treo (process chết giữa chừng) không được khoá cuộc chat mãi: quá ngưỡng này coi như đã hết.
STALE_RUN_SECONDS = 180

_DDL = """
CREATE TABLE IF NOT EXISTS ai_agent_schema (version int PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS ai_conversations (
    id uuid PRIMARY KEY,
    user_id bigint NOT NULL,
    role text NOT NULL,
    active_run_id text,
    active_since timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ai_conversations_user_idx ON ai_conversations (user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS ai_conversations_updated_idx ON ai_conversations (updated_at);
CREATE TABLE IF NOT EXISTS ai_messages (
    id bigserial PRIMARY KEY,
    conversation_id uuid NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    run_id text,
    role text NOT NULL CHECK (role IN ('user', 'assistant')),
    content text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ai_messages_conv_idx ON ai_messages (conversation_id, id);
CREATE TABLE IF NOT EXISTS ai_runs (
    run_id text PRIMARY KEY,
    conversation_id uuid NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    user_id bigint NOT NULL,
    role text NOT NULL,
    client_message_id text,
    status text NOT NULL DEFAULT 'running',
    tool_calls int NOT NULL DEFAULT 0,
    input_tokens int NOT NULL DEFAULT 0,
    output_tokens int NOT NULL DEFAULT 0,
    refs jsonb NOT NULL DEFAULT '[]'::jsonb,
    started_at timestamptz NOT NULL DEFAULT now(),
    ended_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS ai_runs_client_msg_uq ON ai_runs (conversation_id, client_message_id) WHERE client_message_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS ai_runs_user_idx ON ai_runs (user_id, started_at DESC);
"""


class PostgresStore:
    def __init__(self, pool: Any, retention_days: int = 30) -> None:
        self._pool = pool
        self.retention_days = retention_days

    @classmethod
    async def connect(cls, *, host: str, port: int, user: str, password: str, database: str, retention_days: int = 30,
                      min_size: int = 1, max_size: int = 5) -> "PostgresStore":
        import asyncpg

        pool = await asyncpg.create_pool(host=host, port=port, user=user, password=password, database=database,
                                         min_size=min_size, max_size=max_size, command_timeout=10)
        store = cls(pool, retention_days)
        await store.migrate()
        return store

    async def aclose(self) -> None:
        await self._pool.close()

    async def migrate(self) -> None:
        """Tạo bảng nếu chưa có. Khoá advisory để nhiều bản sao khởi động cùng lúc không tranh nhau."""
        async with self._pool.acquire() as con, con.transaction():
            await con.execute("SELECT pg_advisory_xact_lock(724001)")
            await con.execute(_DDL)
            await con.execute("INSERT INTO ai_agent_schema (version) VALUES ($1) ON CONFLICT DO NOTHING", SCHEMA_VERSION)

    async def begin_turn(self, *, user_id: int, conversation_id: str | None, client_message_id: str | None,
                         run_id: str, role: str, text: str, max_history: int) -> Turn:
        async with self._pool.acquire() as con, con.transaction():
            if conversation_id is None:
                conv_id = uuid.uuid4()
                await con.execute("INSERT INTO ai_conversations (id, user_id, role) VALUES ($1, $2, $3)", conv_id, user_id, role)
            else:
                try:
                    conv_id = uuid.UUID(conversation_id)
                except ValueError:
                    raise NotFound(conversation_id) from None
                row = await con.fetchrow("SELECT user_id, active_run_id, active_since > now() - make_interval(secs => $2) AS busy "
                                         "FROM ai_conversations WHERE id = $1 FOR UPDATE", conv_id, float(STALE_RUN_SECONDS))
                if row is None or row["user_id"] != user_id:
                    raise NotFound(conversation_id)
                if client_message_id:
                    dup = await con.fetchval("SELECT run_id FROM ai_runs WHERE conversation_id = $1 AND client_message_id = $2", conv_id, client_message_id)
                    if dup:
                        raise Duplicate(dup)
                if row["active_run_id"] and row["busy"]:
                    raise Busy(conversation_id)
            rows = await con.fetch("SELECT role, content FROM (SELECT id, role, content FROM ai_messages WHERE conversation_id = $1 "
                                   "ORDER BY id DESC LIMIT $2) t ORDER BY id", conv_id, max_history)
            history = [(r["role"], r["content"]) for r in rows]
            await con.execute("INSERT INTO ai_messages (conversation_id, run_id, role, content) VALUES ($1, $2, 'user', $3)",
                              conv_id, run_id, mask_text(text))
            await con.execute("INSERT INTO ai_runs (run_id, conversation_id, user_id, role, client_message_id) VALUES ($1, $2, $3, $4, $5)",
                              run_id, conv_id, user_id, role, client_message_id)
            await con.execute("UPDATE ai_conversations SET active_run_id = $2, active_since = now(), updated_at = now() WHERE id = $1", conv_id, run_id)
        return Turn(conversation_id=str(conv_id), run_id=run_id, history=history)

    async def end_turn(self, turn: Turn, *, status: str, answer: str, cards: list[dict], tool_calls: int,
                       input_tokens: int = 0, output_tokens: int = 0) -> None:
        conv_id = uuid.UUID(turn.conversation_id)
        async with self._pool.acquire() as con, con.transaction():
            if answer:
                await con.execute("INSERT INTO ai_messages (conversation_id, run_id, role, content) VALUES ($1, $2, 'assistant', $3)",
                                  conv_id, turn.run_id, mask_text(answer))
            await con.execute("UPDATE ai_runs SET status = $2, tool_calls = $3, input_tokens = $4, output_tokens = $5, refs = $6::jsonb, ended_at = now() "
                              "WHERE run_id = $1", turn.run_id, status, tool_calls, input_tokens, output_tokens, json.dumps(cards, ensure_ascii=False, default=str))
            await con.execute("UPDATE ai_conversations SET active_run_id = NULL, active_since = NULL, updated_at = now() "
                              "WHERE id = $1 AND active_run_id = $2", conv_id, turn.run_id)

    async def tokens_used_today(self, user_id: int) -> int:
        """Token (vào + ra) của người dùng từ 00:00 hôm nay theo giờ nền tảng (Asia/Ho_Chi_Minh). Lượt đang chạy chưa tính."""
        return int(await self._pool.fetchval(
            "SELECT COALESCE(SUM(input_tokens + output_tokens), 0) FROM ai_runs WHERE user_id = $1 "
            "AND started_at >= date_trunc('day', now() AT TIME ZONE 'Asia/Ho_Chi_Minh') AT TIME ZONE 'Asia/Ho_Chi_Minh'", user_id))

    async def purge(self) -> int:
        """Xoá cuộc chat không hoạt động quá retention_days (kéo theo tin và lượt chạy). Trả số cuộc đã xoá."""
        res = await self._pool.execute("DELETE FROM ai_conversations WHERE updated_at < now() - make_interval(days => $1)", int(self.retention_days))
        n = int(res.split()[-1]) if res else 0
        if n:
            logger.info("chat history purged", extra={"conversations": n, "retention_days": self.retention_days})
        return n
