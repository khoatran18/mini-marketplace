"""Lắp ráp RCA cho dịch vụ (ADR-46/47): sổ phiên + checkpoint + đồ thị + Runner + Watcher, và các thao tác mà API cần.

- Phiên chạy NỀN (API trả ngay). Đọc dữ liệu bằng token HỆ THỐNG (không gắn tài khoản nào, chỉ GET /ops/*), nên không lệ thuộc token
  của người bấm (sống 60 giây) và không phải xin lại khi phiên chờ 30 phút. Người bấm chỉ được ghi vào sổ (`manual:<user_id>`).
- GHI duy nhất là đề xuất của operator: `draft_proposal` dùng token CỦA CHÍNH operator đó để xin gateway ký (như chat, ADR-43/45)."""
from __future__ import annotations

import asyncio
import time
from typing import Any, Callable

from ai_agent.config.logging import get_logger
from ai_agent.rca.budget import Budget
from ai_agent.rca.gateway_ops import GatewayOps, SystemTokens, jti_of
from ai_agent.rca.graph import build_graph
from ai_agent.rca.runner import Runner
from ai_agent.rca.watcher import Watcher
from ai_agent.runtime.context import RunContext
from ai_agent.tools.gateway import GatewayError
from ai_agent.tools.spec import build_action

logger = get_logger(__name__)
ACTIVE = ("running", "waiting_confirm", "waiting_approval")
_EVIDENCE_CHARS = 600  # mỗi bằng chứng trả cho UI bị cắt ngần này (bản đầy đủ nằm trong checkpoint)


class RcaError(Exception):
    def __init__(self, status: int, message: str, **extra: Any) -> None:
        super().__init__(message)
        self.status, self.message, self.extra = status, message, extra


def _view_state(values: dict[str, Any]) -> dict[str, Any]:
    ev = [{**e, "summary": str(e.get("summary", ""))[:_EVIDENCE_CHARS]} for e in values.get("evidence") or []]
    return {"round": values.get("round", 0), "model_calls": values.get("model_calls", 0), "input_tokens": values.get("input_tokens", 0), "output_tokens": values.get("output_tokens", 0), "queries": values.get("queries", 0),
            "hypotheses": values.get("hypotheses") or [], "evidence": ev, "timeline": values.get("timeline") or [],
            "playbook": values.get("playbook"), "report": values.get("report"), "outcome": values.get("outcome") or ""}


class RcaService:
    def __init__(self, *, rt: Any, gateway: Any, runs: Any, saver: Any, persona: Any, budget: Budget, system_key: str = "",
                 clock: Callable[[], float] = time.time, brain_factory: Callable[[RunContext], Any] | None = None, max_concurrent: int = 2) -> None:
        self.rt, self.runs, self.saver, self.persona, self.budget, self.clock = rt, runs, saver, persona, budget, clock
        self.tokens = SystemTokens(gateway, system_key)
        self.ops = GatewayOps(gateway, self.tokens)
        self._brain_factory = brain_factory
        self.runner = Runner(self._graph_for, runs, clock)
        self.watcher = Watcher(self.runner, runs, self.ops, budget, clock)
        self._sem = asyncio.Semaphore(max_concurrent)
        self._tasks: set[asyncio.Task] = set()
        self._watch_task: asyncio.Task | None = None

    @property
    def enabled(self) -> bool:
        return self.tokens.enabled

    async def _graph_for(self, run: dict[str, Any]):
        token = await self.tokens.get()
        ctx = RunContext(token=token, run_id=jti_of(token), request_id=f"rca-{run['id']}", user_id=0, role="operator")
        if self._brain_factory:
            brain = self._brain_factory(ctx)
        else:
            from ai_agent.rca.brain import LlmBrain

            brain = LlmBrain(self.rt, self.persona, ctx, self.budget)
        return build_graph(brain, self.ops, self.budget, checkpointer=self.saver, clock=self.clock)

    # ---- vòng đời ----
    async def start(self) -> None:
        n = await self.runs.fail_running()
        if n:
            logger.warning("rca: runs interrupted by a restart were marked as error", extra={"count": n})
        if self.enabled:
            self._watch_task = asyncio.create_task(self.watcher.run_forever())
        logger.info("rca ready", extra={"watcher": self.enabled, "auto_critical": self.budget.auto_critical})

    async def stop(self) -> None:
        tasks = [t for t in (self._watch_task, *self._tasks) if t]
        for t in tasks:
            t.cancel()
        await asyncio.gather(*tasks, return_exceptions=True)

    def _spawn(self, coro) -> None:
        async def guarded():
            async with self._sem:
                await coro

        t = asyncio.create_task(guarded())
        self._tasks.add(t)
        t.add_done_callback(self._tasks.discard)

    # ---- thao tác của API ----
    async def start_manual(self, incident_id: int, user_id: int) -> tuple[dict[str, Any], bool]:
        """Operator/admin bấm "Điều tra". Đang có phiên chưa xong cho sự cố này thì trả phiên đó (created=False), không mở phiên thứ hai."""
        if not self.enabled:
            raise RcaError(503, "chưa có danh tính hệ thống (AI_SYSTEM_KEY): không điều tra được")
        latest = await self.runs.latest(incident_id)
        if latest and latest["status"] in ACTIVE:
            return latest, False
        try:
            incident = await self.ops.incident(incident_id)
        except GatewayError as exc:
            raise RcaError(502, exc.message) from None
        if incident is None:
            raise RcaError(404, "không tìm thấy sự cố")
        run = await self.runner.begin(incident, f"manual:{user_id}")
        self._spawn(self.runner.go(run, incident))
        return run, True

    async def state_of(self, run: dict[str, Any]) -> dict[str, Any]:
        tup = await self.saver.aget_tuple({"configurable": {"thread_id": run["thread_id"]}})
        return dict(tup.checkpoint.get("channel_values") or {}) if tup else {}

    async def view(self, run_id: int) -> dict[str, Any]:
        run = await self.runs.get(run_id)
        if run is None:
            raise RcaError(404, "không tìm thấy phiên")
        return {"run": run, **_view_state(await self.state_of(run))}

    async def recheck(self, run_id: int) -> dict[str, Any]:
        """"Kiểm tra lại": hỏi ops-service ngay thay vì chờ chu kỳ của bộ theo dõi."""
        run = await self.runs.get(run_id)
        if run is None:
            raise RcaError(404, "không tìm thấy phiên")
        if run["status"] not in ("waiting_confirm", "waiting_approval"):
            return run
        try:
            woken = await self.watcher.check_run(run)
        except GatewayError as exc:
            raise RcaError(502, exc.message) from None
        return woken or run

    async def draft_proposal(self, run_id: int, ctx: RunContext) -> dict[str, Any]:
        """Operator bấm Xác nhận trên nháp của phiên: xin gateway ký đề xuất bằng token CỦA OPERATOR (ctx), trả đúng thứ UI phải gửi lại.
        Cùng đường với nút xác nhận trong chat (ADR-43/45): gateway buộc băm body, 5 phút, dùng một lần."""
        run = await self.runs.get(run_id)
        if run is None:
            raise RcaError(404, "không tìm thấy phiên")
        draft = run.get("draft")
        if run["status"] != "waiting_confirm" or not draft:
            raise RcaError(409, "phiên không có nháp nào đang chờ xác nhận")
        try:
            _, actions = await self.rt.specs_for(ctx)
            spec = next((a for a in actions if a.name == "ops_action"), None)
            if spec is None:
                raise RcaError(403, "vai trò này không được đề xuất hành động vận hành")
            state = await self.state_of(run)
            by_id = {e["id"]: e for e in state.get("evidence") or []}
            facts = [f"{e['source']}: {str(e['summary'])[:150]}" for i in draft.get("evidence_ids", []) if (e := by_id.get(i))][:5]
            args = {"action_id": draft["action_id"], "incident_id": run["incident_id"], "rationale": draft.get("rationale") or "(không có)",
                    "evidence": facts or None, **(draft.get("params") or {})}
            path, body, summary = build_action(spec, args)
            payload = {"method": spec.method, "path": path, "body": body, "summary": summary}
            res = await self.rt.gateway.post_proposal(ctx.token, payload, ctx.request_id)
        except ValueError as exc:
            raise RcaError(422, f"nháp không hợp lệ: {exc}") from None
        except GatewayError as exc:
            raise RcaError(exc.status if exc.status in (401, 403, 429) else 502, exc.message) from None
        return {"proposal": res["proposal"], "id": res["id"], "expires_at": res.get("expires_at"), "summary": summary,
                "request": {"method": spec.method, "path": path, "body": body}}
