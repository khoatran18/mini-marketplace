"""Kho trong bộ nhớ (dev/test): mất khi restart. Bản Postgres riêng thay thế cùng giao diện (docs/ai/08 §5 bước 0)."""
from __future__ import annotations

import asyncio
import time
import uuid
from typing import Any

from ai_agent.store.base import Busy, Duplicate, NotFound, Turn, day_start


class MemoryStore:
    def __init__(self) -> None:
        self._lock = asyncio.Lock()
        self.conversations: dict[str, dict[str, Any]] = {}
        self.runs: dict[str, dict[str, Any]] = {}

    async def begin_turn(self, *, user_id: int, conversation_id: str | None, client_message_id: str | None,
                         run_id: str, role: str, text: str, max_history: int) -> Turn:
        async with self._lock:
            if conversation_id is None:
                conversation_id = str(uuid.uuid4())
                self.conversations[conversation_id] = {"user_id": user_id, "role": role, "messages": [], "stamps": [], "active": None, "client_ids": {}, "created": time.time(), "updated": time.time()}
            conv = self.conversations.get(conversation_id)
            if conv is None or conv["user_id"] != user_id:
                raise NotFound(conversation_id)
            if client_message_id and client_message_id in conv["client_ids"]:
                raise Duplicate(conv["client_ids"][client_message_id])
            if conv["active"] is not None:
                raise Busy(conversation_id)
            conv["active"] = run_id
            if client_message_id:
                conv["client_ids"][client_message_id] = run_id
            history = list(conv["messages"])[-max_history:]
            conv["messages"].append(("user", text))
            conv["stamps"].append(time.time())
            conv["updated"] = time.time()
            self.runs[run_id] = {"conversation_id": conversation_id, "user_id": user_id, "role": role, "status": "running", "started": time.time()}
            return Turn(conversation_id=conversation_id, run_id=run_id, history=history)

    async def end_turn(self, turn: Turn, *, status: str, answer: str, cards: list[dict], tool_calls: int,
                       input_tokens: int = 0, output_tokens: int = 0) -> None:
        async with self._lock:
            conv = self.conversations.get(turn.conversation_id)
            if conv is not None:
                if answer:
                    conv["messages"].append(("assistant", answer))
                    conv["stamps"].append(time.time())
                    conv["updated"] = time.time()
                if conv["active"] == turn.run_id:
                    conv["active"] = None
            run = self.runs.get(turn.run_id)
            if run is not None:
                run.update(status=status, finished=time.time(), cards=cards, tool_calls=tool_calls,
                           input_tokens=input_tokens, output_tokens=output_tokens)

    async def tokens_used_today(self, user_id: int) -> int:
        since = day_start()
        async with self._lock:
            return sum(int(r.get("input_tokens", 0)) + int(r.get("output_tokens", 0))
                       for r in self.runs.values() if r["user_id"] == user_id and r["started"] >= since)

    async def list_conversations(self, *, user_id: int | None, role: str | None = None, limit: int = 30) -> list[dict]:
        async with self._lock:
            rows = [(cid, c) for cid, c in self.conversations.items()
                    if (user_id is None or c["user_id"] == user_id) and (role is None or c.get("role") == role)]
            rows.sort(key=lambda r: r[1]["updated"], reverse=True)
            return [{"id": cid, "user_id": c["user_id"], "role": c.get("role", ""),
                     "title": next((t for r, t in c["messages"] if r == "user"), ""), "messages": len(c["messages"]),
                     "created_at": c["created"], "updated_at": c["updated"]} for cid, c in rows[:limit]]

    async def get_conversation(self, conversation_id: str, *, user_id: int | None) -> dict:
        async with self._lock:
            c = self.conversations.get(conversation_id)
            if c is None or (user_id is not None and c["user_id"] != user_id):
                raise NotFound(conversation_id)
            return {"id": conversation_id, "user_id": c["user_id"], "role": c.get("role", ""), "created_at": c["created"], "updated_at": c["updated"],
                    "messages": [{"role": r, "content": t, "created_at": ts} for (r, t), ts in zip(c["messages"][-200:], c["stamps"][-200:])]}
