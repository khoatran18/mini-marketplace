"""Trạng thái phiên RCA. Chỉ chứa kiểu đơn giản (dict/list/str/số) để checkpointer lưu được; bằng chứng lớn nên lưu tóm tắt."""
from __future__ import annotations

import operator
from typing import Annotated, Any, TypedDict

OUTCOMES = ("recovered", "not_recovered", "rejected", "expired", "dismissed", "escalated", "budget", "no_evidence", "blocked", "error", "cancelled")


class RcaState(TypedDict, total=False):
    incident: dict[str, Any]      # id, alert_name, severity, component, summary, labels
    started_at: float             # giây epoch lúc bắt đầu (tính ngân sách thời gian)
    round: int
    round_limit: int              # trần vòng hiện tại; verify thất bại thì nới thêm đúng một vòng
    model_calls: int
    input_tokens: int             # token vào/ra đã dùng của cả phiên (cộng dồn qua mọi bước, sống qua checkpoint và resume)
    output_tokens: int
    queries: int
    evidence: list[dict[str, Any]]      # {id, source, args, summary}
    hypotheses: list[dict[str, Any]]    # {id, cause, confidence, evidence_ids, refuted_by}
    decision: str                 # more | propose | stop
    draft: dict[str, Any] | None  # {action_id, params, rationale, evidence_ids, risk, mode, escalated}
    failed: list[dict[str, Any]]  # hành động đã thất bại/bị từ chối: {action_id, params, evidence_count}
    proposal_id: int | None
    verify_attempts: int
    outcome: str
    abort: str                    # có giá trị = dừng thẳng tới report
    report: dict[str, Any] | None
    timeline: Annotated[list[dict[str, Any]], operator.add]   # {node, note}: màn hình RCA hiển thị
