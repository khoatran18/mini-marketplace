"""Kiểm nháp đề xuất theo sổ hành động của ops-service TRƯỚC khi đưa cho người. Ops-service vẫn kiểm lại (lệch thì 400), đây chỉ để
agent không đưa người một nháp chắc chắn bị từ chối. Luật phản chiếu `services/ops-service/internal/actions/catalog.go`."""
from __future__ import annotations

import re
from datetime import date
from typing import Any

ESCALATE = "escalate_to_human"
_DATE = re.compile(r"^\d{4}-\d{2}-\d{2}$")


def _int(v: Any) -> bool:
    return isinstance(v, int) and not isinstance(v, bool)


def check_params(action: dict[str, Any], params: dict[str, Any]) -> str | None:
    """None nếu hợp lệ, ngược lại lý do bằng tiếng Việt."""
    spec = {p["name"]: p for p in action.get("params") or []}
    for k in params:
        if k not in spec:
            return f"tham số lạ: {k}"
    for name, p in spec.items():
        if p.get("required") and name not in params:
            return f"thiếu tham số bắt buộc: {name}"
    for name, v in params.items():
        p = spec[name]
        t = p.get("type")
        if t == "enum" and v not in (p.get("enum") or []):
            return f"{name} phải thuộc {p.get('enum')}"
        if t == "int":
            if not _int(v) or v < p.get("min", v) or v > p.get("max", v):
                return f"{name} phải là số nguyên trong [{p.get('min')}, {p.get('max')}]"
        if t == "int_list":
            if not isinstance(v, list) or not v or not all(_int(i) for i in v) or len(v) > p.get("max", 100):
                return f"{name} phải là danh sách số nguyên (1-{p.get('max', 100)} phần tử)"
        if t == "date":
            if not isinstance(v, str) or not _DATE.match(v):
                return f"{name} phải có dạng YYYY-MM-DD"
            try:
                date.fromisoformat(v)
            except ValueError:
                return f"{name} không phải ngày hợp lệ"
    if action.get("id") == "requeue_outbox" and ("ids" in params) == ("limit" in params):
        return "cần đúng một trong ids hoặc limit"
    if action.get("id") == "run_backfill":
        a, b = date.fromisoformat(params["from"]), date.fromisoformat(params["to"])
        if b < a or (b - a).days > 31:
            return "khoảng ngày không hợp lệ (to >= from, tối đa 31 ngày)"
    return None
