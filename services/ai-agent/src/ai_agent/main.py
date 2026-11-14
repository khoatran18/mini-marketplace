"""Lắp ráp dịch vụ. Chạy: uvicorn ai_agent.main:create_app --factory --host 0.0.0.0 --port 8090"""
from __future__ import annotations

import asyncio
from contextlib import asynccontextmanager

from fastapi import FastAPI

from ai_agent.api import chat as chat_api
from ai_agent.api import health as health_api
from ai_agent.config import Config, load_config
from ai_agent.config.logging import get_logger, setup_logging
from ai_agent.guard.startup import check_no_tracing
from ai_agent.personas import load_personas
from ai_agent.runtime.agent_factory import AgentRuntime
from ai_agent.store import MemoryStore
from ai_agent.tools.gateway import GatewayClient

logger = get_logger(__name__)


class _LazyStore:
    """Cầu nối: router nhận kho ngay lúc dựng app, còn kết nối Postgres chỉ có sau khi lifespan chạy."""

    def __init__(self, holder: dict) -> None:
        self._h = holder

    def __getattr__(self, name: str):
        st = self._h["store"]
        if st is None:
            raise RuntimeError("kho chat chưa sẵn sàng")
        return getattr(st, name)


async def _purge_loop(store, every_sec: float = 3600.0) -> None:
    while True:
        try:
            await store.purge()
        except Exception as exc:  # noqa: BLE001 - dọn dẹp lỗi không được làm sập dịch vụ
            logger.warning("purge failed", extra={"error": type(exc).__name__})
        await asyncio.sleep(every_sec)


def create_app(cfg: Config | None = None, runtime: AgentRuntime | None = None, store=None) -> FastAPI:
    cfg = cfg or load_config()
    setup_logging(str(cfg.app.get("log_level", "INFO")))
    found = check_no_tracing(str(cfg.app.env))
    if found:
        logger.warning("tracing env vars are set (ignored in dev, fatal in prod)", extra={"vars": found})
    personas = load_personas()
    gateway = runtime.gateway if runtime else GatewayClient(str(cfg.gateway.base_url), float(cfg.gateway.timeout_sec))
    rt = runtime or AgentRuntime(cfg, gateway)
    backend = str(cfg.store.backend)
    if store is None and backend not in ("memory", "postgres"):
        raise ValueError(f"store.backend={backend!r} chưa hỗ trợ (hiện có: memory, postgres)")
    holder = {"store": store or (MemoryStore() if backend == "memory" else None)}

    @asynccontextmanager
    async def lifespan(_: FastAPI):
        purge_task = None
        if holder["store"] is None:  # postgres: kết nối lúc khởi động; lỗi thì dịch vụ không lên (không âm thầm mất lịch sử)
            from ai_agent.store.postgres import PostgresStore

            pg = cfg.store.postgres
            holder["store"] = await PostgresStore.connect(
                host=str(pg.host), port=int(pg.port), user=str(pg.user), password=str(pg.password or ""), database=str(pg.database),
                retention_days=int(cfg.store.retention_days))
            logger.info("chat store ready", extra={"backend": "postgres", "retention_days": int(cfg.store.retention_days)})
        if hasattr(holder["store"], "purge"):
            purge_task = asyncio.create_task(_purge_loop(holder["store"]))
        try:
            yield
        finally:
            if purge_task:
                purge_task.cancel()
                await asyncio.gather(purge_task, return_exceptions=True)
            if hasattr(holder["store"], "aclose"):
                await holder["store"].aclose()
            await gateway.aclose()

    app = FastAPI(title="ai-agent", lifespan=lifespan, docs_url=None, redoc_url=None, openapi_url=None)
    app.include_router(health_api.create_router(personas))
    app.include_router(chat_api.create_router(rt, _LazyStore(holder), personas))
    logger.info("ai-agent ready", extra={"env": str(cfg.app.env), "personas": sorted(personas)})
    return app
