"""Lắp ráp dịch vụ. Chạy: uvicorn ai_agent.main:create_app --factory --host 0.0.0.0 --port 8090"""
from __future__ import annotations

import asyncio
from contextlib import AsyncExitStack, asynccontextmanager

from fastapi import FastAPI

from ai_agent.api import chat as chat_api
from ai_agent.api import conversations as conversations_api
from ai_agent.api import health as health_api
from ai_agent.api import rca as rca_api
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


async def purge_rca(runs, saver, days: int) -> int:
    """Xoá phiên RCA đã kết thúc quá `days` ngày, kèm nhật ký và checkpoint của nó. Phiên đang chờ xác nhận/duyệt được giữ nguyên."""
    threads = await runs.purge_finished(days)
    for t in threads:
        try:
            await saver.adelete_thread(t)
        except Exception as exc:  # noqa: BLE001 - một checkpoint lỗi không được chặn các cái còn lại
            logger.warning("rca checkpoint purge failed", extra={"error": type(exc).__name__})
    if threads:
        logger.info("rca history purged", extra={"runs": len(threads), "retention_days": days})
    return len(threads)


async def _rca_purge_loop(runs, saver, days: int, every_sec: float = 3600.0) -> None:
    while True:
        try:
            await purge_rca(runs, saver, days)
        except Exception as exc:  # noqa: BLE001
            logger.warning("rca purge failed", extra={"error": type(exc).__name__})
        await asyncio.sleep(every_sec)


def _truthy(v) -> bool:
    """Cờ trong yaml có thể là bool hoặc chuỗi từ biến môi trường ("false" không được tính là bật)."""
    return v is True or str(v).strip().lower() in ("1", "true", "yes", "on")


async def _start_rca(stack: AsyncExitStack, cfg: Config, rt, gateway, store, personas):
    """Dựng RCA (ADR-46/47). Postgres: sổ phiên + checkpoint bền. Memory: chỉ cho dev/test (mất khi restart)."""
    from ai_agent.rca.budget import Budget
    from ai_agent.rca.checkpoint import open_checkpointer
    from ai_agent.rca.runs import MemoryRunStore, PostgresRunStore
    from ai_agent.rca.service import RcaService

    rca = cfg.rca
    if hasattr(store, "_pool"):
        runs = PostgresRunStore(store._pool)
        await runs.migrate()
        saver = await stack.enter_async_context(open_checkpointer(cfg.store.postgres.to_dict(masked=False)))
    else:
        runs = MemoryRunStore()
        saver = await stack.enter_async_context(open_checkpointer())
    svc = RcaService(rt=rt, gateway=gateway, runs=runs, saver=saver, persona=personas["rca"], budget=Budget.from_config(cfg),
                     system_key=str(rca.get("system_key", "") or ""), max_concurrent=int(rca.get("max_concurrent", 2)))
    await svc.start()
    stack.push_async_callback(svc.stop)
    days = int(rca.get("retention_days", 30) or 0)
    if days > 0:
        task = asyncio.create_task(_rca_purge_loop(runs, saver, days))

        async def _stop_purge() -> None:
            task.cancel()
            await asyncio.gather(task, return_exceptions=True)

        stack.push_async_callback(_stop_purge)
    return svc


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
    holder = {"store": store or (MemoryStore() if backend == "memory" else None), "rca": None}
    rca_cfg = cfg.get("rca") if hasattr(cfg, "get") else None

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
        stack = AsyncExitStack()  # RCA: checkpoint + bộ theo dõi, đóng trước kho chat
        if rca_cfg is not None and _truthy(rca_cfg.get("enabled", False)):
            holder["rca"] = await _start_rca(stack, cfg, rt, gateway, holder["store"], personas)
        try:
            yield
        finally:
            await stack.aclose()
            holder["rca"] = None
            if purge_task:
                purge_task.cancel()
                await asyncio.gather(purge_task, return_exceptions=True)
            if hasattr(holder["store"], "aclose"):
                await holder["store"].aclose()
            await gateway.aclose()

    app = FastAPI(title="ai-agent", lifespan=lifespan, docs_url=None, redoc_url=None, openapi_url=None)
    app.include_router(health_api.create_router(personas))
    app.include_router(chat_api.create_router(rt, _LazyStore(holder), personas))
    app.include_router(conversations_api.create_router(_LazyStore(holder)))
    app.include_router(rca_api.create_router(holder))
    logger.info("ai-agent ready", extra={"env": str(cfg.app.env), "personas": sorted(personas)})
    return app
