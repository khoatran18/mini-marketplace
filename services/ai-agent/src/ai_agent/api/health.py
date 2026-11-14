from __future__ import annotations

from fastapi import APIRouter


def create_router(personas) -> APIRouter:
    router = APIRouter()

    @router.get("/health")
    @router.get("/healthz")
    async def health():
        return {"status": "ok"}

    @router.get("/ready")
    @router.get("/readyz")
    async def ready():
        return {"status": "ok", "personas": sorted(personas)}

    return router
