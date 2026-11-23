"""Phần dùng mô hình của RCA (cổng `Brain`). Mỗi bước một yêu cầu riêng; bước cần JSON thì parse chặt bằng pydantic, sai thì thử lại MỘT lần.

- collect: chạy agent chỉ-đọc (persona rca, allow_actions=False); bằng chứng lấy từ KẾT QUẢ CÔNG CỤ THẬT, không từ lời mô hình tả.
- verify: chạy lại ĐÚNG các truy vấn đã dẫn làm bằng chứng cho giả thuyết hàng đầu (xác định, không để mô hình chọn truy vấn), rồi hỏi mô hình
  một câu so sánh trước/sau.
Mọi lời gọi tool đi qua `tools/registry.make_tool` (cùng đường với chat) nên giới hạn, che dữ liệu và nhật ký như nhau."""
from __future__ import annotations

import functools
import json
import re
from typing import Any

from langchain_core.messages import AIMessage, HumanMessage, SystemMessage, ToolMessage
from pydantic import BaseModel, Field, ValidationError

from ai_agent import metrics
from ai_agent.rca.events import RESULT_CHARS, NullEvents
from ai_agent.config.logging import get_logger
from ai_agent.guard import untrusted
from ai_agent.personas.base import Persona
from ai_agent.rca.budget import Budget
from ai_agent.runtime.context import RunContext, reset_run, set_run
from ai_agent.runtime.sse_adapter import text_of

logger = get_logger(__name__)
TOOL_ERROR = "[lỗi công cụ]"


class _Hyp(BaseModel):
    cause: str
    confidence: float = Field(ge=0, le=1)
    evidence_ids: list[str] = []
    refuted_by: str = ""


class _Hyps(BaseModel):
    hypotheses: list[_Hyp]


class _Choice(BaseModel):
    action_id: str = ""
    params: dict[str, Any] = {}
    rationale: str = ""
    evidence_ids: list[str] = []


class _Verdict(BaseModel):
    recovered: bool
    note: str = ""


class _Report(BaseModel):
    summary: str
    ready_to_close: bool = False
    reason: str = ""


def parse_json(text: str, model: type[BaseModel]) -> BaseModel:
    """Lấy đối tượng JSON đầu tiên trong `text` (mô hình đôi khi thêm chữ hoặc khối mã) và kiểm theo `model`."""
    m = re.search(r"\{.*\}", text, flags=re.S)
    if not m:
        raise ValueError("không có JSON")
    return model.model_validate(json.loads(m.group(0)))


def _evidence_lines(evidence: list[dict[str, Any]], limit: int = 40) -> str:
    return "\n".join(f"[{e['id']}] {e['source']} {json.dumps(e.get('args') or {}, ensure_ascii=False)} -> {e['summary']}" for e in evidence[-limit:]) or "(chưa có)"


def _incident_line(inc: dict[str, Any]) -> str:
    # tóm tắt và nhãn do hệ thống khác sinh ra: bọc như mọi dữ liệu không tin cậy (prompt rca, quy tắc 10)
    return (f"Sự cố #{inc.get('id')}: cảnh báo {inc.get('alert_name')} (mức {inc.get('severity')}), thành phần {inc.get('component')}. "
            + untrusted.wrap(f"Tóm tắt: {inc.get('summary')}. Nhãn: {json.dumps(inc.get('labels') or {}, ensure_ascii=False)}"))


def _metered(fn):
    """Đo token mô hình dùng trong MỘT bước của Brain: thêm `input_tokens`/`output_tokens` vào kết quả (đồ thị cộng dồn vào state)
    và tăng `mm_ai_tokens_total{persona="rca"}`. Bước lỗi giữa chừng vẫn được đếm vào Prometheus (tiền đã tiêu) nhưng không vào state."""
    @functools.wraps(fn)
    async def wrapper(self, *a, **kw):
        i0, o0 = self.ctx.input_tokens, self.ctx.output_tokens
        try:
            r = await fn(self, *a, **kw)
        finally:
            di, do = self.ctx.input_tokens - i0, self.ctx.output_tokens - o0
            metrics.TOKENS.labels("rca", "input").inc(di)
            metrics.TOKENS.labels("rca", "output").inc(do)
        return {**r, "input_tokens": di, "output_tokens": do}
    return wrapper


class LlmBrain:
    def __init__(self, rt: Any, persona: Persona, ctx: RunContext, budget: Budget, events: Any = None) -> None:
        self.rt, self.persona, self.ctx, self.budget = rt, persona, ctx, budget
        self.events = events or NullEvents()

    # ---- chung ----
    async def _ask(self, stage: str, body: str, model_cls: type[BaseModel]) -> tuple[BaseModel, int]:
        """Một yêu cầu JSON tới mô hình `llm.roles.rca`; sai định dạng thì thử lại một lần. Trả (kết quả, số lượt gọi)."""
        msgs = [SystemMessage(self.persona.prompt), HumanMessage(f"{stage}\n\n{body}\n\nChỉ trả JSON.")]
        calls = 0
        tok = set_run(self.ctx)  # nhà cung cấp đòi mã phiên (x-opencode-session) lấy từ contextvar lúc gửi request
        try:
            for attempt in range(2):
                calls += 1
                ai = await self.rt.invoke_model(self.persona.llm_role, msgs)
                try:
                    return parse_json(text_of(ai.content), model_cls), calls
                except (ValueError, ValidationError, json.JSONDecodeError):
                    msgs += [ai, HumanMessage("Phản hồi chưa đúng định dạng. Trả lại đúng MỘT đối tượng JSON theo mẫu, không thêm chữ.")]
        finally:
            reset_run(tok)
        raise ValueError(f"mô hình không trả JSON hợp lệ ở bước {stage[:20]!r}")

    async def _tools(self) -> dict[str, Any]:
        specs, _ = await self.rt.specs_for(self.ctx)
        return {t.name: t for t in self.rt.read_tools(self.persona, specs)}

    # ---- các bước ----
    @_metered
    async def collect(self, state: dict[str, Any]) -> dict[str, Any]:
        remaining = max(1, self.budget.max_queries - state.get("queries", 0))
        known = state.get("evidence", [])
        first = not known
        task = (f"{_incident_line(state['incident'])}\n\nBƯỚC THU BẰNG CHỨNG (vòng {state.get('round', 1)}). Bằng chứng đã có:\n{_evidence_lines(known)}\n\n"
                + ("Bắt đầu bằng playbook khớp với tên cảnh báo (ops_list_playbooks, ops_get_playbook), rồi chạy các truy vấn chẩn đoán trong đó. "
                   if first else
                   "Giả thuyết hiện có: " + json.dumps([{k: h[k] for k in ("cause", "confidence", "refuted_by")} for h in state.get("hypotheses", [])], ensure_ascii=False)
                   + ". Thu THÊM bằng chứng để xác nhận hoặc bác bỏ chúng; đừng lặp lại truy vấn đã có.")
                + f" Tối đa {min(remaining, int(self.rt.cfg.limits.max_tool_calls))} lần gọi công cụ. Xong thì trả lời đúng một chữ: XONG.")
        tok = set_run(self.ctx)
        try:
            agent = await self.rt.agent_for(self.persona, self.ctx)
            pending: dict[str, dict[str, Any]] = {}
            evidence: list[dict[str, Any]] = []
            model_calls, playbook, err_run, stopped, ok_calls = 0, None, 0, False, 0
            async for mode, data in agent.astream({"messages": [HumanMessage(task)]}, stream_mode=["updates"], config={"recursion_limit": 40}):
                if mode != "updates":
                    continue
                for upd in (data or {}).values():
                    for m in (upd or {}).get("messages", []) if isinstance(upd, dict) else []:
                        if isinstance(m, AIMessage):
                            model_calls += 1
                            said = text_of(m.content).strip()
                            if said and said.upper().strip(". ") != "XONG":  # "XONG" chỉ là tín hiệu kết thúc, không phải nhận định
                                await self.events.emit("note", "Mô hình", {"text": said[:RESULT_CHARS]})
                            for tc in m.tool_calls:
                                pending[tc["id"]] = tc
                                await self.events.emit("tool_call", tc["name"], {"tool": tc["name"], "args": tc["args"]})
                        elif isinstance(m, ToolMessage):
                            tc = pending.get(m.tool_call_id)
                            text = text_of(m.content)
                            await self.events.emit("tool_result", (tc or {}).get("name", "công cụ"),
                                                   {"tool": (tc or {}).get("name"), "text": text[:RESULT_CHARS], "error": text.startswith(TOOL_ERROR)})
                            err_run = err_run + 1 if text.startswith(TOOL_ERROR) else 0
                            if self.budget.max_tool_errors and err_run >= self.budget.max_tool_errors:
                                stopped = True
                                await self.events.emit("note", "Dừng thu bằng chứng", {"text": f"Công cụ lỗi liên tiếp {err_run} lần, dừng để không đốt thêm lượt gọi."})
                                break
                            if tc and not text.startswith(TOOL_ERROR):
                                ok_calls += 1
                                evidence.append({"source": tc["name"], "args": tc["args"], "summary": text})
                                if tc["name"] == "ops_get_playbook":
                                    playbook = str(tc["args"].get("id") or "") or playbook
                    if stopped:
                        break
                if stopped:
                    break
            # ngân sách truy vấn chỉ tính lần gọi THÀNH CÔNG: lần lỗi 400 do sai tham số không được ăn ngân sách (chúng vẫn bị chặn bởi trần
            # lỗi liên tiếp max_tool_errors và trần cứng limits.max_tool_calls của ctx)
            return {"evidence": evidence, "queries": ok_calls, "model_calls": model_calls, "playbook": playbook}
        finally:
            reset_run(tok)

    @_metered
    async def hypothesize(self, state: dict[str, Any]) -> dict[str, Any]:
        body = (f"{_incident_line(state['incident'])}\n\nBằng chứng:\n{_evidence_lines(state['evidence'])}\n\n"
                f"Nêu tối đa {self.budget.max_hypotheses} giả thuyết về nguyên nhân gốc, xếp theo độ tin cậy giảm dần. Mẫu: "
                '{"hypotheses":[{"cause":"...","confidence":0.0,"evidence_ids":["e1"],"refuted_by":"điều gì sẽ bác bỏ giả thuyết này"}]}')
        res, calls = await self._ask("BƯỚC LẬP GIẢ THUYẾT.", body, _Hyps)
        return {"hypotheses": [h.model_dump() for h in res.hypotheses], "model_calls": calls}

    @_metered
    async def choose_action(self, state: dict[str, Any], catalog: list[dict[str, Any]]) -> dict[str, Any]:
        catalog_view = [{"id": a["id"], "risk": a.get("risk"), "mode": a.get("mode"), "title": a.get("title"), "playbook": a.get("playbook"),
               "params": [{k: p[k] for k in ("name", "type", "required", "enum", "min", "max") if k in p} for p in a.get("params") or []]} for a in catalog]
        body = (f"{_incident_line(state['incident'])}\n\nGiả thuyết:\n{json.dumps(state.get('hypotheses', []), ensure_ascii=False)}\n\n"
                f"Bằng chứng:\n{_evidence_lines(state['evidence'])}\n\nSổ hành động (chỉ được chọn trong đây):\n{json.dumps(catalog_view, ensure_ascii=False)}\n\n"
                f"Hành động ĐÃ thất bại hoặc bị từ chối (đừng lặp lại khi chưa có bằng chứng mới): {json.dumps(state.get('failed', []), ensure_ascii=False)}\n\n"
                'Chọn MỘT hành động hợp với playbook và bằng chứng. Không có hành động nào phù hợp thì action_id để trống. Mẫu: '
                '{"action_id":"restart_service","params":{"service":"payment-service"},"rationale":"vì sao, dẫn bằng chứng","evidence_ids":["e1"]}')
        res, calls = await self._ask("BƯỚC CHỌN HÀNH ĐỘNG.", body, _Choice)
        return {**res.model_dump(), "model_calls": calls}

    @_metered
    async def verify(self, state: dict[str, Any]) -> dict[str, Any]:
        """Chạy lại các truy vấn đã dẫn cho giả thuyết hàng đầu (xác định), rồi so trước/sau."""
        evid = {e["id"]: e for e in state["evidence"]}
        top = (state.get("hypotheses") or [{}])[0]
        keys = [evid[i] for i in top.get("evidence_ids", []) if i in evid][:5] or state["evidence"][:3]
        tools = await self._tools()
        tok = set_run(self.ctx)
        try:
            after, queries = [], 0
            for e in keys:
                tool = tools.get(e["source"])
                if tool is None:
                    continue
                queries += 1
                after.append({"source": e["source"], "args": e.get("args") or {}, "before": e["summary"][:600], "after": str(await tool.ainvoke(e.get("args") or {}))[:600]})
        finally:
            reset_run(tok)
        if not after:
            return {"recovered": False, "note": "không chạy lại được truy vấn nào", "queries": 0, "model_calls": 0}
        d = state.get("draft") or {}
        body = (f"{_incident_line(state['incident'])}\n\nĐã thực hiện: {d.get('action_id')} {json.dumps(d.get('params'), ensure_ascii=False)}.\n"
                f"So sánh TRƯỚC và SAU của các truy vấn đã chứng minh sự cố:\n{json.dumps(after, ensure_ascii=False)}\n\n"
                'Hệ thống đã hồi phục chưa? Chỉ nói "recovered": true khi số liệu SAU cho thấy rõ sự cố hết. Mẫu: {"recovered":false,"note":"lý do ngắn"}')
        res, calls = await self._ask("BƯỚC KIỂM TRA LẠI SAU HÀNH ĐỘNG.", body, _Verdict)
        return {"recovered": res.recovered, "note": res.note, "queries": queries, "model_calls": calls}

    @_metered
    async def report(self, state: dict[str, Any]) -> dict[str, Any]:
        body = (f"{_incident_line(state['incident'])}\n\nKết cục: {state.get('outcome')}. Giả thuyết: {json.dumps(state.get('hypotheses', []), ensure_ascii=False)}\n"
                f"Hành động: {json.dumps(state.get('draft'), ensure_ascii=False)}\nBằng chứng:\n{_evidence_lines(state.get('evidence', []), 20)}\n\n"
                'Viết báo cáo ngắn cho người: chuyện gì xảy ra, bằng chứng, giả thuyết chọn, đã làm gì, kết quả. "ready_to_close" chỉ true khi đã xác nhận hồi phục. Mẫu: '
                '{"summary":"...","ready_to_close":false,"reason":"vì sao đủ/chưa đủ để đóng"}')
        res, calls = await self._ask("BƯỚC BÁO CÁO.", body, _Report)
        return {**res.model_dump(), "model_calls": calls}
