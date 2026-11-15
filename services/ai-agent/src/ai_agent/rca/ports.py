"""Hai cổng của đồ thị: Brain (phần dùng mô hình) và OpsPort (ops-service). Đồ thị không biết LLM hay HTTP, nên test bằng bản giả."""
from __future__ import annotations

from typing import Any, Protocol


class Brain(Protocol):
    async def collect(self, state: dict[str, Any]) -> dict[str, Any]:
        """-> {evidence: [{source,args,summary}], queries: int, model_calls: int, playbook: str|None}"""

    async def hypothesize(self, state: dict[str, Any]) -> dict[str, Any]:
        """-> {hypotheses: [{cause, confidence, evidence_ids, refuted_by}], model_calls: int}"""

    async def choose_action(self, state: dict[str, Any], catalog: list[dict[str, Any]]) -> dict[str, Any]:
        """-> {action_id, params, rationale, evidence_ids, model_calls}; action_id rỗng = không có hành động phù hợp"""

    async def verify(self, state: dict[str, Any]) -> dict[str, Any]:
        """-> {recovered: bool, note: str, evidence: [...], queries: int, model_calls: int}"""

    async def report(self, state: dict[str, Any]) -> dict[str, Any]:
        """-> {summary: str, ready_to_close: bool, reason: str, model_calls: int}"""


class OpsPort(Protocol):
    async def list_actions(self) -> list[dict[str, Any]]:
        """Sổ hành động của ops-service (GET /ops/actions): id, risk, mode, params[]."""

    async def pending_proposals(self, incident_id: int) -> int:
        """Số đề xuất đang PENDING của sự cố."""
