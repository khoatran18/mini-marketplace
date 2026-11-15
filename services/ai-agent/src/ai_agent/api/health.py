from __future__ import annotations

import hmac
import os

from fastapi import APIRouter, Request, Response

from ai_agent import metrics


def create_router(personas) -> APIRouter:
    router = APIRouter()

    @router.get("/health")
    @router.get("/healthz")
    async def health():
        return {"status": "ok"}

    @router.get("/ready")
    @router.get("/readyz")
    async def ready():
        return {"status": "ready", "personas": sorted(personas), "version": os.environ.get("SERVICE_VERSION", "dev")}

    @router.get("/metrics")
    async def prom(request: Request):
        # Cùng quy ước với các service Go: METRICS_TOKEN đặt thì /metrics cần "Authorization: Bearer <token>"; rỗng = mở (probe /health, /ready luôn mở).
        want = os.environ.get("METRICS_TOKEN", "").strip()
        if want:
            got = request.headers.get("authorization", "").removeprefix("Bearer ").strip()
            if not hmac.compare_digest(got.encode(), want.encode()):
                return Response(status_code=401, content="unauthorized")
        body, ctype = metrics.render()
        return Response(content=body, media_type=ctype)

    return router
