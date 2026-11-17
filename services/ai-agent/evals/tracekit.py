"""Ghi lại ĐẦY ĐỦ mỗi lần chạy eval live để đọc lại và so sánh sau này (không cần chạy lại, không tốn tiền thêm).

Một lần chạy tạo một thư mục `evals/results/runs/<giờ UTC>-<nhãn>/`:
  meta.json     dấu vân tay (băm prompt/tool/case), cấu hình đã che khoá, phiên bản thư viện, system prompt đầy đủ của từng persona, tham số chạy
  cases.jsonl   MỘT DÒNG MỖI CA, ghi ngay sau khi ca xong (chết giữa chừng vẫn còn phần đã chạy): câu hỏi, kết quả chấm và lý do, câu trả lời,
                `trace` = từng lần gọi mô hình (messages vào, trả lời ra, tool_calls mô hình chọn, token vào/ra, độ trễ, finish_reason),
                từng lần gọi tool (tên, tham số, kết quả đầy đủ, lỗi, độ trễ), số liệu Prometheus của ai-agent thay đổi trong ca đó
  summary.json  đạt/tổng theo persona và nhóm, token và số lần gọi mô hình/tool (tổng, trung bình, lớn nhất), thời gian, danh sách ca trượt
  run.log       toàn bộ những gì in ra màn hình
Không ghi khoá API: cấu hình đi qua `Config.to_dict(masked=True)`, biến môi trường chỉ ghi TÊN. System prompt chỉ ghi một lần ở meta.json
(mỗi lần gọi mô hình tham chiếu bằng băm), còn lại mọi message đều ghi nguyên văn.

Cơ chế: một callback của LangChain gắn qua `register_configure_hook` (ContextVar), nên mọi lần gọi mô hình/tool trong lượt chat đều đi qua nó mà
không sửa mã chạy thật."""
from __future__ import annotations

import hashlib
import json
import sys
import time
from contextvars import ContextVar
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from langchain_core.callbacks import BaseCallbackHandler
from langchain_core.tracers.context import register_configure_hook

MAX_TOOL_OUTPUT = 50_000  # ký tự: kết quả tool dài hơn thì cắt (ghi rõ đã cắt)

_CURRENT: ContextVar[Any | None] = ContextVar("eval_tracer", default=None)
register_configure_hook(_CURRENT, inheritable=True)


def _h(text: str) -> str:
    return hashlib.sha1(text.encode("utf-8")).hexdigest()[:10]


def _text(content: Any) -> str:
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        return "".join(b.get("text", "") if isinstance(b, dict) else str(b) for b in content)
    return str(content)


def _msg(m: Any, systems: dict[str, str]) -> dict[str, Any]:
    kind = getattr(m, "type", "?")
    content = _text(getattr(m, "content", ""))
    if kind == "system":
        ref = _h(content)
        systems[ref] = content
        return {"role": "system", "ref": ref, "chars": len(content)}
    out: dict[str, Any] = {"role": {"human": "user", "ai": "assistant"}.get(kind, kind), "content": content}
    calls = getattr(m, "tool_calls", None)
    if calls:
        out["tool_calls"] = [{"name": c.get("name"), "args": c.get("args")} for c in calls]
    if kind == "tool":
        out["tool_name"] = getattr(m, "name", None)
        out["tool_call_id"] = getattr(m, "tool_call_id", None)
    return out


class Tracer(BaseCallbackHandler):
    """Gom các lần gọi mô hình và tool của MỘT ca. Chạy trong cùng vòng lặp (run_inline) nên thứ tự đúng."""

    run_inline = True

    def __init__(self) -> None:
        self.model_calls: list[dict[str, Any]] = []
        self.tool_calls: list[dict[str, Any]] = []
        self.errors: list[dict[str, Any]] = []
        self.systems: dict[str, str] = {}
        self._open: dict[Any, dict[str, Any]] = {}

    # --- mô hình
    def on_chat_model_start(self, serialized, messages, *, run_id, invocation_params=None, **kw):  # noqa: ANN001
        flat = [m for batch in messages for m in batch]
        tools = ((invocation_params or {}).get("tools") or [])
        names = [(t.get("function") or t).get("name") for t in tools if isinstance(t, dict)]
        self._open[run_id] = {"n": len(self.model_calls) + 1, "t0": time.monotonic(), "messages": [_msg(m, self.systems) for m in flat],
                              "tools_offered": names, "model": (invocation_params or {}).get("model") or (invocation_params or {}).get("model_name")}

    def on_llm_end(self, response, *, run_id, **kw):  # noqa: ANN001
        rec = self._open.pop(run_id, None)
        if rec is None:
            return
        gen = response.generations[0][0] if response.generations and response.generations[0] else None
        msg = getattr(gen, "message", None)
        usage = dict(getattr(msg, "usage_metadata", None) or {})
        meta = dict(getattr(msg, "response_metadata", None) or {})
        rec.update(seconds=round(time.monotonic() - rec.pop("t0"), 3),
                   output={"content": _text(getattr(msg, "content", "") if msg is not None else (gen.text if gen else "")),
                           "tool_calls": [{"name": c.get("name"), "args": c.get("args")} for c in (getattr(msg, "tool_calls", None) or [])]},
                   usage={"input_tokens": usage.get("input_tokens"), "output_tokens": usage.get("output_tokens"), "total_tokens": usage.get("total_tokens"),
                          "details": {k: v for k, v in usage.items() if k.endswith("_details")} or None},
                   finish_reason=meta.get("finish_reason") or meta.get("stop_reason"), model=meta.get("model_name") or meta.get("model") or rec.get("model"))
        self.model_calls.append(rec)

    def on_llm_error(self, error, *, run_id, **kw):  # noqa: ANN001
        rec = self._open.pop(run_id, {})
        info = {"where": "model", "n": rec.get("n"), "error": type(error).__name__, "message": str(error)[:500], "status_code": getattr(error, "status_code", None),
                "seconds": round(time.monotonic() - rec["t0"], 3) if "t0" in rec else None}
        self.errors.append(info)
        if rec:  # lần gọi lỗi vẫn là một lần gọi (đã gửi request): ghi để số lần gọi khớp hoá đơn
            rec.pop("t0", None)
            rec.update(error=info["error"], message=info["message"])
            self.model_calls.append(rec)

    # --- tool
    def on_tool_start(self, serialized, input_str, *, run_id, inputs=None, **kw):  # noqa: ANN001
        self._open[run_id] = {"name": (serialized or {}).get("name") or kw.get("name"), "args": inputs if inputs is not None else input_str, "t0": time.monotonic()}

    def on_tool_end(self, output, *, run_id, **kw):  # noqa: ANN001
        rec = self._open.pop(run_id, None)
        if rec is None:
            return
        text = _text(getattr(output, "content", output))
        rec.update(seconds=round(time.monotonic() - rec.pop("t0"), 3), output=text[:MAX_TOOL_OUTPUT], output_chars=len(text), truncated=len(text) > MAX_TOOL_OUTPUT)
        self.tool_calls.append(rec)

    def on_tool_error(self, error, *, run_id, **kw):  # noqa: ANN001
        rec = self._open.pop(run_id, {})
        rec.update(seconds=round(time.monotonic() - rec.pop("t0", time.monotonic()), 3), error=type(error).__name__, message=str(error)[:500])
        self.tool_calls.append(rec)
        self.errors.append({"where": "tool", "name": rec.get("name"), "error": rec["error"], "message": rec["message"]})

    # --- kết quả
    def summary(self) -> dict[str, Any]:
        tin = sum((c.get("usage") or {}).get("input_tokens") or 0 for c in self.model_calls)
        tout = sum((c.get("usage") or {}).get("output_tokens") or 0 for c in self.model_calls)
        return {"model_calls": self.model_calls, "tool_calls": self.tool_calls, "errors": self.errors,
                "totals": {"model_calls": len(self.model_calls), "tool_calls": len(self.tool_calls), "input_tokens": tin, "output_tokens": tout,
                           "total_tokens": tin + tout, "model_seconds": round(sum(c.get("seconds") or 0 for c in self.model_calls), 3),
                           "tool_seconds": round(sum(c.get("seconds") or 0 for c in self.tool_calls), 3)}}


class traced:  # noqa: N801 - dùng như context manager: `with traced() as t:`
    def __enter__(self) -> Tracer:
        self.tracer = Tracer()
        self._tok = _CURRENT.set(self.tracer)
        return self.tracer

    def __exit__(self, *exc: Any) -> None:
        _CURRENT.reset(self._tok)


def metrics_snapshot() -> dict[str, float]:
    """Giá trị hiện tại của mọi bộ đếm/histogram `mm_ai_*` trong tiến trình (để lấy hiệu trước/sau mỗi ca)."""
    from ai_agent import metrics

    snap: dict[str, float] = {}
    for fam in metrics.REGISTRY.collect():
        for s in fam.samples:
            if s.name.endswith(("_total", "_sum", "_count")) and s.name.startswith("mm_ai_"):
                snap[s.name + "{" + ",".join(f"{k}={v}" for k, v in sorted(s.labels.items())) + "}"] = s.value
    return snap


def metrics_delta(before: dict[str, float], after: dict[str, float]) -> dict[str, float]:
    return {k: round(v - before.get(k, 0.0), 6) for k, v in after.items() if round(v - before.get(k, 0.0), 6) != 0}


class Tee:
    """Ghi mọi thứ in ra màn hình vào run.log nữa."""

    def __init__(self, stream: Any, path: Path) -> None:
        self.stream, self.file = stream, path.open("a", encoding="utf-8")

    def write(self, s: str) -> int:
        self.file.write(s)
        self.file.flush()
        return self.stream.write(s)

    def flush(self) -> None:
        self.stream.flush()

    def close(self) -> None:
        self.file.close()


class RunLog:
    def __init__(self, root: Path, label: str) -> None:
        self.dir = root / f"{datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')}-{label}"
        self.dir.mkdir(parents=True, exist_ok=False)
        self.systems: dict[str, dict[str, str]] = {}
        self._cases = (self.dir / "cases.jsonl").open("a", encoding="utf-8")
        self.tee = Tee(sys.stdout, self.dir / "run.log")
        self._stdout = sys.stdout
        sys.stdout = self.tee  # type: ignore[assignment]

    def write_meta(self, meta: dict[str, Any]) -> None:
        (self.dir / "meta.json").write_text(json.dumps(meta, ensure_ascii=False, indent=1, default=str), encoding="utf-8")

    def add_systems(self, persona: str, systems: dict[str, str]) -> None:
        self.systems.setdefault(persona, {}).update(systems)

    def append(self, row: dict[str, Any]) -> None:
        self._cases.write(json.dumps(row, ensure_ascii=False, default=str) + "\n")
        self._cases.flush()

    def finish(self, summary: dict[str, Any]) -> None:
        (self.dir / "summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=1, default=str), encoding="utf-8")
        meta_path = self.dir / "meta.json"
        meta = json.loads(meta_path.read_text(encoding="utf-8")) if meta_path.exists() else {}
        meta["system_prompts"] = self.systems
        meta["finished_at"] = datetime.now(timezone.utc).isoformat(timespec="seconds")
        meta_path.write_text(json.dumps(meta, ensure_ascii=False, indent=1, default=str), encoding="utf-8")
        self._cases.close()
        sys.stdout = self._stdout
        self.tee.close()


def summarize(rows: list[dict[str, Any]]) -> dict[str, Any]:
    """Tổng hợp từ các hàng đã chạy (mỗi hàng có `trace.totals`)."""
    def stats(vals: list[float]) -> dict[str, float]:
        return {"sum": round(sum(vals), 3), "mean": round(sum(vals) / len(vals), 3) if vals else 0, "max": max(vals) if vals else 0}

    by: dict[str, dict[str, Any]] = {}
    for r in rows:
        key = f"{r['id'].split('-')[0]}/{r['g']}"
        b = by.setdefault(key, {"ok": 0, "total": 0})
        b["total"] += 1
        b["ok"] += 1 if r["ok"] else 0
    totals = [(r.get("trace") or {}).get("totals") or {} for r in rows]
    col = lambda k: [t.get(k) or 0 for t in totals]  # noqa: E731
    reasons: dict[str, int] = {}
    for r in rows:
        for why in r.get("reasons") or []:
            reasons[why.split(":")[0][:80]] = reasons.get(why.split(":")[0][:80], 0) + 1
    return {
        "cases": len(rows), "ok": sum(1 for r in rows if r["ok"]),
        "by_persona_group": by,
        "model_calls": stats(col("model_calls")), "tool_calls": stats(col("tool_calls")),
        "input_tokens": stats(col("input_tokens")), "output_tokens": stats(col("output_tokens")), "total_tokens": stats(col("total_tokens")),
        "model_seconds": stats(col("model_seconds")), "case_seconds": stats([r.get("seconds") or 0 for r in rows]),
        "runtime_errors": sum(1 for r in rows if any("lỗi runtime" in w for w in r.get("reasons") or [])),
        "model_calls_histogram": {str(n): sum(1 for v in col("model_calls") if v == n) for n in sorted(set(col("model_calls")))},
        "failure_reasons": dict(sorted(reasons.items(), key=lambda kv: -kv[1])),
        "failed": [{"id": r["id"], "g": r["g"], "q": r["q"], "reasons": r["reasons"], "answer": (r.get("answer") or "")[:300]} for r in rows if not r["ok"]],
    }
