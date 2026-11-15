"""Chạy và đánh thức phiên RCA: bọc đồ thị, ghi trạng thái vào sổ `ai_rca_runs`. Không biết HTTP hay LLM (đồ thị đã dựng sẵn)."""
from __future__ import annotations

import time
from typing import Any, Awaitable, Callable

from langgraph.types import Command

from ai_agent.config.logging import get_logger

logger = get_logger(__name__)

GraphFor = Callable[[dict[str, Any]], Awaitable[Any]]  # run -> đồ thị đã biên dịch (Brain gắn token của phiên này)


class Runner:
    def __init__(self, graph_for: GraphFor, runs: Any, clock: Callable[[], float] = time.time) -> None:
        self.graph_for, self.runs, self.clock = graph_for, runs, clock

    async def begin(self, incident: dict[str, Any], trigger: str) -> dict[str, Any]:
        """Chỉ ghi phiên vào sổ (status running), chưa chạy: để API trả ngay rồi `go` chạy nền."""
        return await self.runs.create(int(incident["id"]), trigger)

    async def go(self, run: dict[str, Any], incident: dict[str, Any]) -> dict[str, Any]:
        return await self._drive(run, {"incident": incident, "started_at": self.clock()})

    async def start(self, incident: dict[str, Any], trigger: str) -> dict[str, Any]:
        return await self.go(await self.begin(incident, trigger), incident)

    async def resume(self, run: dict[str, Any], event: dict[str, Any]) -> dict[str, Any]:
        return await self._drive(run, Command(resume=event))

    async def _drive(self, run: dict[str, Any], payload: Any) -> dict[str, Any]:
        try:
            graph = await self.graph_for(run)
            result = await graph.ainvoke(payload, {"configurable": {"thread_id": run["thread_id"]}})
        except Exception as exc:  # noqa: BLE001 - phiên hỏng không được làm sập bộ theo dõi
            logger.warning("rca run %s crashed: %s: %s", run["id"], type(exc).__name__, str(exc)[:300])
            return await self.runs.update(run["id"], status="error", outcome="error", waiting_since=None)
        intr = result.get("__interrupt__")
        if intr:
            v = intr[0].value
            if v.get("waiting") == "confirm":
                return await self.runs.update(run["id"], status="waiting_confirm", draft=v.get("draft"), waiting_since=self.clock())
            return await self.runs.update(run["id"], status="waiting_approval", proposal_id=v.get("proposal_id"), waiting_since=self.clock())
        return await self.runs.update(run["id"], status="done", outcome=result.get("outcome"), report=result.get("report"), draft=result.get("draft"), waiting_since=None)
