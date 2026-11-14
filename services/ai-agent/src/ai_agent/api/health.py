from __future__ import annotations

import os

from fastapi import APIRouter, Response

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
    async def prom():
        body, ctype = metrics.render()
        return Response(content=body, media_type=ctype)

    return router
