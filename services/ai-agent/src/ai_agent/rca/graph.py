"""Đồ thị RCA (docs/ai/11_RCA_BUILD_LOG.md §16.26):

collect -> hypothesize -> decide --(more)--> collect
                                  \\-(propose)-> propose -> wait_confirm -> wait_approval -> verify -> report
                                  \\-(stop)----> report                                         \\-(retry)-> collect

Hai bước chờ dùng interrupt(): đồ thị DỪNG, trạng thái nằm trong checkpoint, không có tiến trình nào chạy cho tới khi bên ngoài
(bộ theo dõi / nút "Kiểm tra lại") gọi Command(resume={...}). Sự kiện resume:
  wait_confirm : {"event": "confirmed", "proposal_id": N} | {"event": "dismissed"} | {"event": "expired"}
  wait_approval: {"event": "executed"} | {"event": "rejected"} | {"event": "expired"}
Agent không có đường nào để duyệt, chạy hay ghi nhận kết quả: những việc đó chỉ người làm được (ADR-45)."""
from __future__ import annotations

import json
import time
from typing import Any, Callable

from langgraph.graph import END, START, StateGraph
from langgraph.types import interrupt

from ai_agent.config.logging import get_logger
from ai_agent.rca.budget import Budget
from ai_agent.rca.conclusion import interim_conclusion
from ai_agent.rca.events import NullEvents
from ai_agent.rca.grounding import hypothesis_supported, ungrounded_params
from ai_agent.rca.ports import Brain, OpsPort
from ai_agent.rca.state import RcaState
from ai_agent.rca.validate import ESCALATE, check_params

logger = get_logger(__name__)


def _tok(state: Any, r: dict[str, Any]) -> dict[str, int]:
    """Cộng token của một bước (`r`) vào tổng của phiên trong state."""
    return {"input_tokens": int(state.get("input_tokens", 0)) + int(r.get("input_tokens", 0)),
            "output_tokens": int(state.get("output_tokens", 0)) + int(r.get("output_tokens", 0))}

LOW_CONFIDENCE = 0.4  # dưới mức này không dám đề xuất hành động thật: chuyển cho người


def _why(exc: BaseException) -> str:
    """Lý do lỗi cho người đọc: nhà cung cấp quá tải / hết hạn mức / 400... (llm/resilience.py), không thì tên lớp lỗi."""
    from ai_agent.llm.resilience import ErrorKind, LlmUnavailable, classify, label

    kind = exc.kind if isinstance(exc, LlmUnavailable) else classify(exc)
    return label(kind) if kind is not ErrorKind.UNKNOWN else f"lỗi {type(exc).__name__}"


def _note(node: str, note: str) -> list[dict[str, Any]]:
    return [{"node": node, "note": note}]


def _key(action_id: str, params: dict[str, Any]) -> str:
    return action_id + json.dumps(params, sort_keys=True, ensure_ascii=False)


_NODE_TITLE = {"collect": "Thu bằng chứng", "hypothesize": "Lập giả thuyết", "decide": "Quyết định", "propose": "Soạn nháp đề xuất",
               "wait_confirm": "Chờ xác nhận", "wait_approval": "Chờ duyệt", "verify": "Kiểm tra lại", "report": "Báo cáo"}


def build_graph(brain: Brain, ops: OpsPort, budget: Budget, *, checkpointer: Any = None, clock: Callable[[], float] = time.time, events: Any = None):
    events = events or NullEvents()

    def traced(name: str, fn):
        """Mỗi bước xong thì ghi vào nhật ký: dòng thời gian của bước + phần kết quả đáng xem. Bước dừng ở interrupt() không chạy xong nên không ghi ở đây
        (runner ghi sự kiện chờ đúng một lần)."""
        async def run(state):
            out = await fn(state)
            for t in out.get("timeline") or []:
                await events.emit("step", _NODE_TITLE.get(name, name), {"node": name, "note": t.get("note", "")})
            if name == "hypothesize" and out.get("hypotheses"):
                await events.emit("hypotheses", "Giả thuyết", {"items": out["hypotheses"]})
            elif name == "propose" and out.get("draft"):
                await events.emit("draft", "Nháp đề xuất", {"draft": out["draft"]})
            elif name == "report" and out.get("report"):
                await events.emit("report", "Báo cáo", {"report": out["report"], "outcome": out.get("outcome")})
            return out

        return run

    def over(state: dict[str, Any]) -> str | None:
        return budget.exceeded(state, clock())

    async def collect(state: RcaState) -> dict[str, Any]:
        rnd = state.get("round", 0) + 1
        out: dict[str, Any] = {"round": rnd, "started_at": state.get("started_at") or clock()}
        full = {**state, **out}
        limit = over(full)
        if limit:
            return {**out, "timeline": _note("collect", f"hết ngân sách ({limit}): không thu thêm")}
        try:
            r = await brain.collect(full)
        except Exception as exc:  # noqa: BLE001 - lỗi mô hình/tool không được làm sập phiên
            logger.warning("rca collect failed: %s: %s", type(exc).__name__, str(exc)[:300])
            return {**out, "abort": "error", "error_reason": _why(exc), "timeline": _note("collect", f"lỗi khi thu bằng chứng: {_why(exc)}")}
        have = {(e["source"], json.dumps(e.get("args"), sort_keys=True)) for e in state.get("evidence", [])}
        merged = list(state.get("evidence", []))
        for e in r.get("evidence", []):
            k = (e["source"], json.dumps(e.get("args"), sort_keys=True))
            if k in have:
                continue
            have.add(k)
            merged.append({"id": f"e{len(merged) + 1}", "source": e["source"], "args": e.get("args") or {}, "summary": str(e.get("summary", ""))[:1200]})
        out.update(evidence=merged, queries=state.get("queries", 0) + int(r.get("queries", 0)),
                   model_calls=state.get("model_calls", 0) + int(r.get("model_calls", 0)), **_tok(state, r))
        if r.get("playbook"):
            out["incident"] = {**state["incident"], "playbook": r["playbook"]}
        out["timeline"] = _note("collect", f"vòng {rnd}: {len(merged)} bằng chứng ({int(r.get('queries', 0))} truy vấn mới)")
        return out

    async def hypothesize(state: RcaState) -> dict[str, Any]:
        if not state.get("evidence"):
            return {"hypotheses": [], "timeline": _note("hypothesize", "chưa có bằng chứng nên chưa có giả thuyết")}
        if over(state) and state.get("hypotheses"):
            return {"timeline": _note("hypothesize", "hết ngân sách: giữ giả thuyết hiện có")}
        try:
            r = await brain.hypothesize(dict(state))
        except Exception as exc:  # noqa: BLE001
            logger.warning("rca hypothesize failed: %s: %s", type(exc).__name__, str(exc)[:300])
            return {"abort": "error", "error_reason": _why(exc), "timeline": _note("hypothesize", f"lỗi khi lập giả thuyết: {_why(exc)}")}
        ids = {e["id"] for e in state["evidence"]}
        hyps: list[dict[str, Any]] = []
        for h in r.get("hypotheses", []):
            conf = max(0.0, min(1.0, float(h.get("confidence", 0))))
            cited = [i for i in h.get("evidence_ids", []) if i in ids]
            bogus = [i for i in h.get("evidence_ids", []) if i not in ids]  # mã bằng chứng không tồn tại: mô hình tự nghĩ ra
            cause = str(h.get("cause", ""))
            grounded = hypothesis_supported(cause, [e for e in state["evidence"] if e["id"] in cited]) if cited else False
            if not cited or grounded is False:
                conf = min(conf, 0.3)  # không dẫn được bằng chứng nào, hoặc bằng chứng dẫn không nhắc gì tới điều nó khẳng định: không được tin cao
            hyps.append({"cause": cause, "confidence": conf, "evidence_ids": cited, "refuted_by": str(h.get("refuted_by", "")),
                         "grounded": grounded, **({"bogus_refs": bogus} if bogus else {})})
        hyps = sorted(hyps, key=lambda h: -h["confidence"])[: budget.max_hypotheses]
        for n, h in enumerate(hyps, start=1):
            h["id"] = f"h{n}"
        top = f"{hyps[0]['cause']} ({hyps[0]['confidence']:.0%})" if hyps else "không có"
        weak = sum(1 for h in hyps if h["grounded"] is False)
        bogus_n = sum(len(h.get("bogus_refs", [])) for h in hyps)
        extra = (f"; {weak} giả thuyết không được bằng chứng đã dẫn ủng hộ (độ tin cậy bị hạ)" if weak else "") + (f"; {bogus_n} mã bằng chứng không tồn tại bị bỏ" if bogus_n else "")
        return {"hypotheses": hyps, "model_calls": state.get("model_calls", 0) + int(r.get("model_calls", 0)), **_tok(state, r),
                "timeline": _note("hypothesize", f"{len(hyps)} giả thuyết; hàng đầu: {top}{extra}")}

    async def decide(state: RcaState) -> dict[str, Any]:
        hyps = state.get("hypotheses", [])
        top = hyps[0]["confidence"] if hyps else 0.0
        limit = over(state)
        if top >= budget.confident_at:
            d, why = "propose", f"giả thuyết hàng đầu đủ tin cậy ({top:.0%})"
        elif limit or state.get("round", 0) >= state.get("round_limit", budget.max_rounds):
            d, why = ("propose", "hết vòng hoặc ngân sách: đề xuất theo những gì có") if state.get("evidence") else ("stop", "không thu được bằng chứng nào")
        else:
            d, why = "more", f"độ tin cậy {top:.0%} chưa đủ: thu thêm bằng chứng"
        out: dict[str, Any] = {"decision": d, "timeline": _note("decide", why)}
        if top < budget.confident_at and d != "more":
            out["stop_reason"] = limit or "rounds"  # để kết luận tạm nói rõ vì sao dừng
        if d == "stop":
            out["outcome"] = "budget" if limit else "no_evidence"
        return out

    async def propose(state: RcaState) -> dict[str, Any]:
        incident_id = int(state["incident"]["id"])
        if await ops.pending_proposals(incident_id) >= budget.max_pending_proposals:
            return {"draft": None, "outcome": "blocked", "timeline": _note("propose", "đã có đủ đề xuất đang chờ cho sự cố này: không tạo thêm")}
        catalog = await ops.list_actions()
        by_id = {a["id"]: a for a in catalog}
        top = (state.get("hypotheses") or [{}])[0]
        reason = ""
        draft: dict[str, Any] | None = None
        if top.get("confidence", 0) < LOW_CONFIDENCE:
            reason = "độ tin cậy thấp, chưa đủ để đề xuất hành động thật"
        else:
            try:
                r = await brain.choose_action(dict(state), catalog)
            except Exception as exc:  # noqa: BLE001
                r, reason = {}, f"lỗi khi chọn hành động: {_why(exc)}"
            out_calls = int(r.get("model_calls", 0))
            aid, params = str(r.get("action_id") or ""), dict(r.get("params") or {})
            if not aid:
                reason = reason or "không có hành động nào trong sổ khớp"
            elif aid not in by_id:
                reason = f"hành động {aid!r} không có trong sổ"
            elif aid != ESCALATE and (bad := check_params(by_id[aid], params)):
                reason = f"tham số không hợp lệ: {bad}"
            elif aid != ESCALATE and (loose := ungrounded_params(params, state.get("evidence", []), state["incident"])):
                reason = f"tham số {', '.join(loose)} không có trong bằng chứng hay thông tin sự cố (mô hình không được tự chọn đích tác động)"
            elif any(_key(f["action_id"], f["params"]) == _key(aid, params) and len(state.get("evidence", [])) <= f["evidence_count"] for f in state.get("failed", [])):
                reason = "hành động này đã thất bại/bị từ chối và chưa có bằng chứng mới"
            else:
                draft = {"action_id": aid, "params": params, "rationale": str(r.get("rationale", ""))[:1000],
                         "evidence_ids": [i for i in r.get("evidence_ids", []) if i in {e["id"] for e in state.get("evidence", [])}][:20],
                         "risk": by_id[aid].get("risk"), "mode": by_id[aid].get("mode"), "escalated": aid == ESCALATE}
            state = {**state, "model_calls": state.get("model_calls", 0) + out_calls, **_tok(state, r)}
        if draft is None:
            if ESCALATE not in by_id:
                return {"draft": None, "outcome": "blocked", "model_calls": state.get("model_calls", 0), **_tok(state, {}), "timeline": _note("propose", f"{reason}; sổ không có escalate_to_human")}
            ev = [e["id"] for e in state.get("evidence", [])][:20]
            draft = {"action_id": ESCALATE, "params": {}, "rationale": f"Chuyển cho người: {reason}.\n{interim_conclusion(state, reason)}", "evidence_ids": ev,
                     "risk": by_id[ESCALATE].get("risk"), "mode": by_id[ESCALATE].get("mode"), "escalated": True}
        return {"draft": draft, "model_calls": state.get("model_calls", 0), **_tok(state, {}),
                "timeline": _note("propose", f"nháp: {draft['action_id']}" + (f" ({reason})" if draft["escalated"] and reason else ""))}

    async def wait_confirm(state: RcaState) -> dict[str, Any]:
        ev = interrupt({"waiting": "confirm", "draft": state["draft"]}) or {}
        kind = ev.get("event")
        if kind == "confirmed" and ev.get("proposal_id"):
            return {"proposal_id": int(ev["proposal_id"]), "timeline": _note("wait_confirm", f"operator đã xác nhận, đề xuất #{ev['proposal_id']} chờ duyệt")}
        out = {"outcome": "expired" if kind == "expired" else "dismissed"}
        return {**out, "timeline": _note("wait_confirm", "hết hạn chờ xác nhận" if kind == "expired" else "operator không xác nhận")}

    async def wait_approval(state: RcaState) -> dict[str, Any]:
        ev = interrupt({"waiting": "approval", "proposal_id": state["proposal_id"]}) or {}
        kind = ev.get("event")
        if kind == "executed":
            return {"timeline": _note("wait_approval", "đề xuất đã được duyệt và thực hiện")}
        failed = [*state.get("failed", []), {"action_id": state["draft"]["action_id"], "params": state["draft"]["params"], "evidence_count": len(state.get("evidence", []))}]
        return {"outcome": "expired" if kind == "expired" else "rejected", "failed": failed,
                "timeline": _note("wait_approval", {"expired": "hết hạn chờ duyệt", "failed": "người duyệt đã chạy nhưng hành động thất bại"}.get(kind, "đề xuất bị từ chối"))}

    async def verify(state: RcaState) -> dict[str, Any]:
        attempts = state.get("verify_attempts", 0) + 1
        try:
            r = await brain.verify(dict(state))
        except Exception as exc:  # noqa: BLE001
            logger.warning("rca verify failed: %s: %s", type(exc).__name__, str(exc)[:300])
            return {"verify_attempts": attempts, "outcome": "error", "error_reason": _why(exc), "timeline": _note("verify", f"lỗi khi kiểm tra lại: {_why(exc)}")}
        out: dict[str, Any] = {"verify_attempts": attempts, "model_calls": state.get("model_calls", 0) + int(r.get("model_calls", 0)), **_tok(state, r),
                               "queries": state.get("queries", 0) + int(r.get("queries", 0))}
        if r.get("recovered"):
            return {**out, "outcome": "recovered", "timeline": _note("verify", f"đã hồi phục: {r.get('note', '')}")}
        failed = [*state.get("failed", []), {"action_id": state["draft"]["action_id"], "params": state["draft"]["params"], "evidence_count": len(state.get("evidence", []))}]
        out.update(failed=failed, outcome="not_recovered")
        if attempts <= budget.verify_retries and not over({**state, **out}):
            out.update(round_limit=state.get("round", 0) + 1, draft=None, proposal_id=None, decision="more", outcome="",
                       timeline=_note("verify", f"chưa hồi phục ({r.get('note', '')}): điều tra thêm một vòng"))
        else:
            out["timeline"] = _note("verify", f"chưa hồi phục ({r.get('note', '')}): báo người")
        return out

    async def report(state: RcaState) -> dict[str, Any]:
        outcome = state.get("outcome") or ("escalated" if (state.get("draft") or {}).get("escalated") else "error")
        if state.get("abort") and not state.get("outcome"):
            outcome = "error"
        s = {**state, "outcome": outcome}
        try:
            r = await brain.report(s)
        except Exception:  # noqa: BLE001 - báo cáo không có LLM vẫn phải có
            r = {"summary": f"Phiên kết thúc: {outcome}.\n{interim_conclusion(s)}", "ready_to_close": False, "reason": "không lập được báo cáo tự động", "model_calls": 0}
        ready = bool(r.get("ready_to_close")) and outcome == "recovered"  # chỉ khi verify xác nhận hồi phục
        rep = {"summary": str(r.get("summary", "")), "ready_to_close": ready,
               "reason": str(r.get("reason", "")) if ready or not r.get("ready_to_close") else "chưa có xác nhận hồi phục từ bước kiểm tra lại: người quyết định"}
        if outcome == "error" and state.get("error_reason"):
            rep["error_reason"] = str(state["error_reason"])  # vì sao phiên lỗi (hiện cho người, không cần mở log)
        return {"outcome": outcome, "report": rep, "model_calls": state.get("model_calls", 0) + int(r.get("model_calls", 0)), **_tok(state, r),
                "timeline": _note("report", f"kết thúc: {outcome}; đủ điều kiện đóng: {'có' if ready else 'chưa'}")}

    g = StateGraph(RcaState)
    for name, fn in (("collect", collect), ("hypothesize", hypothesize), ("decide", decide), ("propose", propose),
                     ("wait_confirm", wait_confirm), ("wait_approval", wait_approval), ("verify", verify), ("report", report)):
        g.add_node(name, traced(name, fn))
    g.add_edge(START, "collect")
    g.add_conditional_edges("collect", lambda s: "report" if s.get("abort") else "hypothesize", {"report": "report", "hypothesize": "hypothesize"})
    g.add_conditional_edges("hypothesize", lambda s: "report" if s.get("abort") else "decide", {"report": "report", "decide": "decide"})
    g.add_conditional_edges("decide", lambda s: s["decision"], {"more": "collect", "propose": "propose", "stop": "report"})
    g.add_conditional_edges("propose", lambda s: "wait_confirm" if s.get("draft") else "report", {"wait_confirm": "wait_confirm", "report": "report"})
    g.add_conditional_edges("wait_confirm", lambda s: "wait_approval" if s.get("proposal_id") and not s["draft"]["escalated"] else "report",
                            {"wait_approval": "wait_approval", "report": "report"})
    g.add_conditional_edges("wait_approval", lambda s: "report" if s.get("outcome") in ("rejected", "expired") else "verify", {"verify": "verify", "report": "report"})
    g.add_conditional_edges("verify", lambda s: "collect" if s.get("decision") == "more" and s.get("draft") is None else "report",
                            {"collect": "collect", "report": "report"})
    g.add_edge("report", END)
    return g.compile(checkpointer=checkpointer)
