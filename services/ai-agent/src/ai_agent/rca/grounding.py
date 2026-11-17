"""Kiểm xác định (không LLM) rằng kết luận của mô hình bám vào bằng chứng đã thu (ADR-49, docs/ai/12 mục 1).

Bằng chứng do HỆ THỐNG cấp mã (`e1..eN` trong `graph.collect`, lấy từ kết quả công cụ thật); mô hình chỉ được TRÍCH mã. Hai kiểm tra thêm ở đây:
  1. giả thuyết: những thứ có thể đối chiếu máy móc trong câu nguyên nhân (tên có gạch như `payment-service`, `kafka_consumer_lag`, tên thành phần
     như postgres/kafka, con số) phải xuất hiện ít nhất một trong số bằng chứng nó dẫn. Không thì giả thuyết bị coi là "không được ủng hộ".
  2. nháp hành động: mọi tham số chuỗi (tên service...) phải có mặt trong bằng chứng hoặc thông tin sự cố; mô hình không được tự "nghĩ ra" đích tác động.
Chủ ý bảo thủ: câu nguyên nhân không có gì để đối chiếu thì KHÔNG bị chê (grounded = None). Đây là kiểm tra cần, chưa đủ: bằng chứng có chứa tên
service chưa chứng minh nó ủng hộ nguyên nhân; việc đó vẫn cần người xem (dòng thời gian hiển thị bằng chứng đã dẫn)."""
from __future__ import annotations

import json
import re
from typing import Any

from ai_agent.guard import numbers

_IDENT = re.compile(r"[A-Za-z][A-Za-z0-9]*(?:[-_.][A-Za-z0-9]+)+")
_NOT_NAME = re.compile(r"^(?:v\d+(?:\.\d+)+|e\.g|i\.e|vs|etc)$", re.I)  # "v1.2", "e.g." trông như tên có dấu nhưng không phải thành phần hệ thống
_DATE = re.compile(r"^\d{4}-\d{2}-\d{2}$")
_COMPONENTS = ("postgres", "kafka", "redis", "clickhouse", "traefik", "prometheus", "grafana", "outbox", "loki", "tempo", "vector")


def _text(e: dict[str, Any]) -> str:
    return f"{e.get('source', '')} {json.dumps(e.get('args') or {}, ensure_ascii=False)} {e.get('summary', '')}".lower()


def _has_name(name: str, hay: str) -> bool:
    """Khớp theo ranh giới chữ và số: "kafka" khớp trong "kafka_consumer_lag" nhưng không khớp trong "kafkaesque"."""
    return re.search(rf"(?<![a-z0-9]){re.escape(name)}(?![a-z0-9])", hay) is not None


def _split(cause: str) -> tuple[list[str], list[frozenset[str]]]:
    names = [m.group(0).lower() for m in _IDENT.finditer(cause) if not _NOT_NAME.match(m.group(0))] + [c for c in _COMPONENTS if _has_name(c, cause.lower())]
    nums = list(dict.fromkeys(numbers.extract(cause, min_digits=2)))
    return list(dict.fromkeys(names)), nums


def checkable(cause: str) -> list[str]:
    """Các mẩu trong câu nguyên nhân có thể đối chiếu máy móc với bằng chứng."""
    names, nums = _split(cause)
    return names + ["/".join(sorted(n)) for n in nums]


def hypothesis_supported(cause: str, cited: list[dict[str, Any]]) -> bool | None:
    """True/False nếu có gì để đối chiếu; None nếu câu nguyên nhân không có mẩu nào đối chiếu được.
    Tên: ít nhất MỘT tên phải có trong bằng chứng dẫn. Con số: MỌI con số phải có (số là thứ mô hình hay bịa nhất)."""
    names, nums = _split(cause)
    if not names and not nums:
        return None
    hay = " ".join(_text(e) for e in cited)
    have = numbers.pool(hay)
    return (not names or any(_has_name(t, hay) for t in names)) and all(n & have for n in nums)


def ungrounded_params(params: dict[str, Any], evidence: list[dict[str, Any]], incident: dict[str, Any]) -> list[str]:
    """Tên các tham số chuỗi (≥3 ký tự, không phải ngày) không có trong bằng chứng lẫn thông tin sự cố."""
    hay = (" ".join(_text(e) for e in evidence) + " " + json.dumps(incident, ensure_ascii=False).lower())
    bad = []
    for k, v in params.items():
        if isinstance(v, str) and len(v) >= 3 and not _DATE.match(v) and v.lower() not in hay:
            bad.append(k)
    return bad
