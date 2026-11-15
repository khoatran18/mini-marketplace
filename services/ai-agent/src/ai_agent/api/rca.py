"""Điều tra sự cố (RCA, ADR-46/47). Gateway gọi (header X-Agent-Token như /chat); chỉ role admin và operator.
  GET  /rca/runs[?incident_id=&limit=]    các phiên, mới nhất trước
  GET  /rca/runs/{id}                     một phiên: trạng thái, nháp, giả thuyết, bằng chứng (cắt ngắn), dòng thời gian, báo cáo
  POST /rca/runs {incident_id}            bấm "Điều tra": chạy nền, trả 202 + phiên (đang có phiên chưa xong thì trả phiên đó, 200)
  POST /rca/runs/{id}/recheck             "Kiểm tra lại": hỏi ops-service ngay thay vì chờ bộ theo dõi
  POST /rca/runs/{id}/proposal            CHỈ operator: xin gateway ký nháp để UI gọi POST /ops/proposals (người xác nhận bằng JWT của mình)
`holder["rca"]` là RcaService (None = RCA tắt: 503)."""
from __future__ import annotations

from fastapi import APIRouter, Request
from fastapi.responses import JSONResponse
from pydantic import BaseModel, Field

from ai_agent.api.auth import AgentIdentity, AuthError, parse_agent_token
from ai_agent.rca.service import RcaError
from ai_agent.runtime.context import RunContext

_ROLES = ("admin", "operator")


class StartBody(BaseModel):
    incident_id: int = Field(gt=0)


def _err(status: int, msg: str, **extra) -> JSONResponse:
    return JSONResponse({"error": msg, **extra}, status_code=status)


def create_router(holder: dict) -> APIRouter:
    router = APIRouter(prefix="/rca")

    def guard(request: Request) -> tuple[AgentIdentity, object] | JSONResponse:
        try:
            ident = parse_agent_token(request.headers.get("x-agent-token", ""), request.headers.get("x-agent-run-id", ""))
        except AuthError as exc:
            return _err(401, str(exc))
        if ident.role not in _ROLES:
            return _err(403, "chỉ admin và operator")
        svc = holder.get("rca")
        if svc is None:
            return _err(503, "điều tra sự cố đang tắt")
        return ident, svc

    @router.get("/runs")
    async def list_runs(request: Request):
        g = guard(request)
        if isinstance(g, JSONResponse):
            return g
        _, svc = g
        raw, lim = request.query_params.get("incident_id"), request.query_params.get("limit")
        if (raw and not raw.isdigit()) or (lim and not lim.isdigit()):
            return _err(422, "tham số không hợp lệ")
        return {"items": await svc.runs.recent(int(raw) if raw else None, max(1, min(int(lim or 30), 100)))}

    @router.get("/runs/{run_id}")
    async def one(run_id: int, request: Request):
        g = guard(request)
        if isinstance(g, JSONResponse):
            return g
        try:
            return await g[1].view(run_id)
        except RcaError as exc:
            return _err(exc.status, exc.message)

    @router.post("/runs")
    async def start(body: StartBody, request: Request):
        g = guard(request)
        if isinstance(g, JSONResponse):
            return g
        ident, svc = g
        try:
            run, created = await svc.start_manual(body.incident_id, ident.user_id)
        except RcaError as exc:
            return _err(exc.status, exc.message)
        return JSONResponse({"run": run, "created": created}, status_code=202 if created else 200)

    @router.post("/runs/{run_id}/recheck")
    async def recheck(run_id: int, request: Request):
        g = guard(request)
        if isinstance(g, JSONResponse):
            return g
        try:
            return {"run": await g[1].recheck(run_id)}
        except RcaError as exc:
            return _err(exc.status, exc.message)

    @router.post("/runs/{run_id}/proposal")
    async def proposal(run_id: int, request: Request):
        g = guard(request)
        if isinstance(g, JSONResponse):
            return g
        ident, svc = g
        if ident.role != "operator":
            return _err(403, "chỉ operator xác nhận được nháp (admin chỉ xem)")
        ctx = RunContext(token=request.headers.get("x-agent-token", ""), run_id=ident.run_id, request_id=request.headers.get("x-request-id", ""),
                         user_id=ident.user_id, role=ident.role)
        try:
            return await svc.draft_proposal(run_id, ctx)
        except RcaError as exc:
            return _err(exc.status, exc.message)

    return router
