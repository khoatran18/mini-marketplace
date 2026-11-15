"""Sổ các phiên RCA (bảng `ai_rca_runs`): ai đang chờ gì, để bộ theo dõi biết phiên nào cần đánh thức và màn hình RCA liệt kê.
Trạng thái chi tiết (bằng chứng, giả thuyết, dòng thời gian) nằm trong checkpoint của langgraph, không lặp lại ở đây.

status: running | waiting_confirm | waiting_approval | done | error."""
from __future__ import annotations

import json
import time
from typing import Any, Protocol

WAITING = ("waiting_confirm", "waiting_approval")


class RunStore(Protocol):
    async def create(self, incident_id: int, trigger: str) -> dict[str, Any]: ...
    async def get(self, run_id: int) -> dict[str, Any] | None: ...
    async def latest(self, incident_id: int) -> dict[str, Any] | None: ...
    async def waiting(self) -> list[dict[str, Any]]: ...
    async def update(self, run_id: int, **fields: Any) -> dict[str, Any]: ...
    async def count_auto_since(self, since: float) -> int: ...
    async def recent(self, incident_id: int | None = None, limit: int = 30) -> list[dict[str, Any]]: ...
    async def fail_running(self) -> int: ...


class MemoryRunStore:
    def __init__(self, clock=time.time) -> None:
        self.rows: dict[int, dict[str, Any]] = {}
        self._clock = clock

    async def create(self, incident_id: int, trigger: str) -> dict[str, Any]:
        rid = len(self.rows) + 1
        row = {"id": rid, "incident_id": incident_id, "thread_id": f"inc-{incident_id}-{rid}", "trigger": trigger, "status": "running", "draft": None,
               "proposal_id": None, "waiting_since": None, "outcome": None, "report": None, "created_at": self._clock()}
        self.rows[rid] = row
        return dict(row)

    async def get(self, run_id: int):
        return dict(self.rows[run_id]) if run_id in self.rows else None

    async def latest(self, incident_id: int):
        mine = [r for r in self.rows.values() if r["incident_id"] == incident_id]
        return dict(max(mine, key=lambda r: r["id"])) if mine else None

    async def waiting(self):
        return [dict(r) for r in self.rows.values() if r["status"] in WAITING]

    async def update(self, run_id: int, **fields: Any):
        self.rows[run_id].update(fields)
        return dict(self.rows[run_id])

    async def recent(self, incident_id: int | None = None, limit: int = 30):
        rows = [r for r in self.rows.values() if incident_id is None or r["incident_id"] == incident_id]
        return [dict(r) for r in sorted(rows, key=lambda r: -r["id"])[:limit]]

    async def fail_running(self) -> int:
        n = 0
        for r in self.rows.values():
            if r["status"] == "running":
                r.update(status="error", outcome="interrupted", waiting_since=None)
                n += 1
        return n

    async def count_auto_since(self, since: float) -> int:
        return sum(1 for r in self.rows.values() if r["trigger"] == "auto" and r["created_at"] >= since)


_DDL = """
CREATE TABLE IF NOT EXISTS ai_rca_runs (
    id bigserial PRIMARY KEY,
    incident_id bigint NOT NULL,
    thread_id text NOT NULL DEFAULT '',
    trigger text NOT NULL,
    status text NOT NULL DEFAULT 'running',
    draft jsonb,
    proposal_id bigint,
    waiting_since double precision,
    outcome text,
    report jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ai_rca_runs_incident_idx ON ai_rca_runs (incident_id, id DESC);
CREATE INDEX IF NOT EXISTS ai_rca_runs_status_idx ON ai_rca_runs (status) WHERE status IN ('waiting_confirm', 'waiting_approval');
"""
_COLS = "id, incident_id, thread_id, trigger, status, draft, proposal_id, waiting_since, outcome, report, extract(epoch from created_at) AS created_at"
_JSON = ("draft", "report")
_FIELDS = {"thread_id", "status", "draft", "proposal_id", "waiting_since", "outcome", "report"}
SCHEMA_VERSION = 2  # ai_agent_schema: 1 = bảng chat (store/postgres.py), 2 = ai_rca_runs


def _row(r: Any) -> dict[str, Any]:
    d = dict(r)
    for k in _JSON:
        if isinstance(d.get(k), str):
            d[k] = json.loads(d[k])
    d["created_at"] = float(d["created_at"])
    return d


class PostgresRunStore:
    def __init__(self, pool: Any) -> None:
        self._pool = pool

    async def migrate(self) -> None:
        async with self._pool.acquire() as con, con.transaction():
            await con.execute("SELECT pg_advisory_xact_lock(724001)")  # cùng khoá với store/postgres.py
            await con.execute("CREATE TABLE IF NOT EXISTS ai_agent_schema (version int PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())")
            await con.execute(_DDL)
            await con.execute("INSERT INTO ai_agent_schema (version) VALUES ($1) ON CONFLICT DO NOTHING", SCHEMA_VERSION)

    async def create(self, incident_id: int, trigger: str) -> dict[str, Any]:
        async with self._pool.acquire() as con:
            rid = await con.fetchval("INSERT INTO ai_rca_runs (incident_id, trigger) VALUES ($1, $2) RETURNING id", incident_id, trigger)
            await con.execute("UPDATE ai_rca_runs SET thread_id = $2 WHERE id = $1", rid, f"inc-{incident_id}-{rid}")
        return await self.get(rid)  # type: ignore[return-value]

    async def get(self, run_id: int):
        async with self._pool.acquire() as con:
            r = await con.fetchrow(f"SELECT {_COLS} FROM ai_rca_runs WHERE id = $1", run_id)
        return _row(r) if r else None

    async def latest(self, incident_id: int):
        async with self._pool.acquire() as con:
            r = await con.fetchrow(f"SELECT {_COLS} FROM ai_rca_runs WHERE incident_id = $1 ORDER BY id DESC LIMIT 1", incident_id)
        return _row(r) if r else None

    async def waiting(self):
        async with self._pool.acquire() as con:
            rows = await con.fetch(f"SELECT {_COLS} FROM ai_rca_runs WHERE status IN ('waiting_confirm', 'waiting_approval') ORDER BY id")
        return [_row(r) for r in rows]

    async def update(self, run_id: int, **fields: Any):
        bad = set(fields) - _FIELDS
        if bad:
            raise ValueError(f"trường không được cập nhật: {sorted(bad)}")
        sets, args = [], [run_id]
        for k, v in fields.items():
            args.append(json.dumps(v, ensure_ascii=False) if k in _JSON and v is not None else v)
            sets.append(f"{k} = ${len(args)}" + ("::jsonb" if k in _JSON else ""))
        async with self._pool.acquire() as con:
            await con.execute(f"UPDATE ai_rca_runs SET {', '.join(sets)}, updated_at = now() WHERE id = $1", *args)
        return await self.get(run_id)  # type: ignore[return-value]

    async def recent(self, incident_id: int | None = None, limit: int = 30):
        async with self._pool.acquire() as con:
            if incident_id is None:
                rows = await con.fetch(f"SELECT {_COLS} FROM ai_rca_runs ORDER BY id DESC LIMIT $1", limit)
            else:
                rows = await con.fetch(f"SELECT {_COLS} FROM ai_rca_runs WHERE incident_id = $1 ORDER BY id DESC LIMIT $2", incident_id, limit)
        return [_row(r) for r in rows]

    async def fail_running(self) -> int:
        """Phiên đang `running` lúc dịch vụ khởi động là phiên bị ngắt giữa chừng (tiến trình chết): đánh dấu lỗi, không tự chạy lại."""
        async with self._pool.acquire() as con:
            res = await con.execute("UPDATE ai_rca_runs SET status = 'error', outcome = 'interrupted', waiting_since = NULL, updated_at = now() WHERE status = 'running'")
        return int(res.split()[-1])

    async def count_auto_since(self, since: float) -> int:
        async with self._pool.acquire() as con:
            return int(await con.fetchval("SELECT count(*) FROM ai_rca_runs WHERE trigger = 'auto' AND created_at >= to_timestamp($1)", since))
