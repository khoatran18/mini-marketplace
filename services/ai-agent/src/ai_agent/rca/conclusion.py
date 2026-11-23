"""Kết luận tạm của một phiên RCA khi không đủ tin cậy để đề xuất hành động (hết ngân sách, chuyển cho người).

Không gọi mô hình: ghép từ giả thuyết và bằng chứng đã có, để người vận hành luôn thấy phiên đã đi tới đâu, vì sao dừng và nên xem gì tiếp,
thay vì chỉ thấy "chuyển cho người: không có hành động nào khớp"."""
from __future__ import annotations

import json
from typing import Any

STOP_WHY = {"model_calls": "hết lượt gọi mô hình", "queries": "hết lượt truy vấn", "seconds": "hết thời gian", "rounds": "hết số vòng điều tra"}
_CAUSE_MAX, _OTHER_MAX, _EVID_MAX = 320, 140, 4


def _cut(text: str, n: int) -> str:
    text = " ".join(str(text).split())
    return text if len(text) <= n else text[: n - 1] + "…"


def _evidence_line(e: dict[str, Any]) -> str:
    args = json.dumps(e.get("args") or {}, ensure_ascii=False, separators=(",", ":"))
    return f"{e.get('id', '?')} {e.get('source', '')}{'' if args == '{}' else ' ' + _cut(args, 60)}"


def interim_conclusion(state: dict[str, Any], reason: str = "") -> str:
    """Văn bản ngắn (nhiều dòng): giả thuyết hàng đầu + độ tin cậy, bằng chứng dẫn, giả thuyết khác, lý do dừng, việc người nên làm."""
    hyps = state.get("hypotheses") or []
    ev = {e.get("id"): e for e in state.get("evidence", [])}
    stop = STOP_WHY.get(str(state.get("stop_reason") or ""), "")
    if not hyps:
        base = "Kết luận tạm: chưa lập được giả thuyết nào" + (f" ({stop})" if stop else "") + f"; đã thu {len(ev)} bằng chứng."
        return base + (f" {reason}." if reason else "")
    top = hyps[0]
    lines = [f"Kết luận tạm: {_cut(top.get('cause', ''), _CAUSE_MAX)} (độ tin cậy {float(top.get('confidence', 0)):.0%}, chưa đủ để đề xuất hành động"
             + (f"; dừng vì {stop}" if stop else "") + ")."]
    cited = [ev[i] for i in top.get("evidence_ids", []) if i in ev][:_EVID_MAX]
    if cited:
        lines.append("Bằng chứng: " + "; ".join(_evidence_line(e) for e in cited) + ".")
    if top.get("refuted_by"):
        lines.append(f"Chưa khớp: {_cut(top['refuted_by'], 200)}")
    others = [f"{_cut(h.get('cause', ''), _OTHER_MAX)} ({float(h.get('confidence', 0)):.0%})" for h in hyps[1:]]
    if others:
        lines.append("Giả thuyết khác: " + " | ".join(others) + ".")
    lines.append(f"Đã thu {len(ev)} bằng chứng. Người nên kiểm tra trực tiếp theo giả thuyết hàng đầu" + (f" ({reason})" if reason else "") + ".")
    return "\n".join(lines)
