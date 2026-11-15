"""Bộ theo dõi RCA (KHÔNG gọi LLM): mỗi `poll_interval_sec` làm hai việc nhờ ops-service.

1. Phát hiện: sự cố `critical` đang mở mà chưa có phiên thì bắt đầu một phiên tự động (mỗi sự cố MỘT lần, có trần theo ngày).
2. Đánh thức: phiên đang chờ người thì xem ops-service rồi gọi `resume` với đúng sự kiện:
   - chờ xác nhận: tìm đề xuất cùng sự cố, cùng hành động và tham số, tạo SAU khi nháp xuất hiện -> `confirmed`; quá hạn -> `expired`;
   - chờ duyệt: ACCEPTED/ALTERNATIVE và exec_state=EXECUTED (đã qua `verify_wait_sec`) -> `executed`; FAILED -> `failed`;
     REJECTED -> `rejected`; EXPIRED hoặc quá hạn -> `expired`.
Danh tính hệ thống để tự chạy (token agent không gắn người đăng nhập) CHƯA có: xem ADR-46; `Runner.graph_for` là nơi cấp token cho phiên."""
from __future__ import annotations

import asyncio
import json
import time
from typing import Any, Protocol

from ai_agent.config.logging import get_logger
from ai_agent.rca.budget import Budget
from ai_agent.rca.runner import Runner

logger = get_logger(__name__)
DAY = 86400.0


class OpsReader(Protocol):
    async def open_incidents(self) -> list[dict[str, Any]]: ...
    async def proposals_for_incident(self, incident_id: int) -> list[dict[str, Any]]: ...
    async def proposal(self, proposal_id: int) -> dict[str, Any] | None: ...


def _same(p: dict[str, Any], draft: dict[str, Any]) -> bool:
    params = p.get("params")
    if isinstance(params, str):
        params = json.loads(params or "{}")
    return p.get("action_id") == draft["action_id"] and (params or {}) == (draft.get("params") or {})


def _ts(v: Any) -> float:
    """Thời điểm (giây epoch) từ chuỗi RFC3339 của ops-service; None/sai = 0."""
    if isinstance(v, (int, float)):
        return float(v)
    if not v:
        return 0.0
    from datetime import datetime

    try:
        return datetime.fromisoformat(str(v).replace("Z", "+00:00")).timestamp()
    except ValueError:
        return 0.0


class Watcher:
    def __init__(self, runner: Runner, runs: Any, ops: OpsReader, budget: Budget, clock=time.time) -> None:
        self.runner, self.runs, self.ops, self.budget, self.clock = runner, runs, ops, budget, clock
        self.used: set[int] = set()  # đề xuất đã gắn cho một phiên: hai phiên không nhận cùng một đề xuất

    async def tick(self) -> dict[str, int]:
        started = await self._discover()
        woken = 0
        for run in await self.runs.waiting():
            try:
                if await self.check_run(run) is not None:
                    woken += 1
            except Exception as exc:  # noqa: BLE001 - một phiên lỗi không chặn các phiên khác
                logger.warning("rca watcher: run failed", extra={"run": run["id"], "error": type(exc).__name__})
        return {"started": started, "woken": woken}

    async def check_run(self, run: dict[str, Any]) -> dict[str, Any] | None:
        """Xem ops-service cho MỘT phiên đang chờ; có sự kiện thì đánh thức và trả phiên đã cập nhật, chưa có thì None (nút "Kiểm tra lại")."""
        event = await self._event_for(run)
        return await self.runner.resume(run, event) if event else None

    async def _discover(self) -> int:
        if not self.budget.auto_critical:
            return 0
        now = self.clock()
        used = await self.runs.count_auto_since(now - DAY)
        n = 0
        for inc in await self.ops.open_incidents():
            if inc.get("severity") != "critical" or inc.get("state") != "open":
                continue
            if await self.runs.latest(int(inc["id"])) is not None:
                continue  # đã có phiên (tự chạy hoặc bấm tay): mỗi sự cố một lần
            if used + n >= self.budget.daily_auto_runs:
                logger.warning("rca watcher: daily auto-run cap reached", extra={"cap": self.budget.daily_auto_runs})
                break
            await self.runner.start(inc, "auto")
            n += 1
        return n

    async def _event_for(self, run: dict[str, Any]) -> dict[str, Any] | None:
        waited = self.clock() - float(run.get("waiting_since") or self.clock())
        late = waited >= self.budget.wait_timeout_sec
        if run["status"] == "waiting_confirm":
            for p in await self.ops.proposals_for_incident(run["incident_id"]):
                if p["id"] not in self.used and _same(p, run["draft"]) and _ts(p.get("created_at")) >= float(run["waiting_since"] or 0) - 5:
                    self.used.add(p["id"])
                    return {"event": "confirmed", "proposal_id": p["id"]}
            return {"event": "expired"} if late else None
        p = await self.ops.proposal(int(run["proposal_id"]))
        if p is None:
            return {"event": "expired"} if late else None
        state, ex = p.get("state"), p.get("exec_state")
        if state == "REJECTED":
            return {"event": "rejected"}
        if state == "EXPIRED":
            return {"event": "expired"}
        if ex == "FAILED":
            return {"event": "failed"}
        if ex == "EXECUTED":
            done_at = _ts(p.get("exec_finished_at"))
            return {"event": "executed"} if self.clock() - done_at >= self.budget.verify_wait_sec else None
        return {"event": "expired"} if late else None

    async def run_forever(self) -> None:
        while True:
            try:
                await self.tick()
            except Exception as exc:  # noqa: BLE001
                logger.warning("rca watcher tick failed", extra={"error": type(exc).__name__})
            await asyncio.sleep(self.budget.poll_interval_sec)
