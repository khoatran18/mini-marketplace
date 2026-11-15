"""Hợp đồng LangGraph mà rca/ dựa vào (ADR-46): đồ thị dừng ở interrupt(), lưu trạng thái qua checkpointer, chạy tiếp bằng Command(resume=...).
Đồ thị mẫu có cùng hình với đồ thị RCA; nâng phiên bản langgraph mà test này đỏ thì rca/ cũng hỏng."""
from typing import TypedDict

from langgraph.checkpoint.memory import MemorySaver
from langgraph.graph import END, START, StateGraph
from langgraph.types import Command, interrupt


class S(TypedDict, total=False):
    round: int
    decision: str
    approved: bool


def _build():
    g = StateGraph(S)
    g.add_node("collect", lambda s: {"round": s.get("round", 0) + 1})
    g.add_node("hypothesize", lambda s: {})
    g.add_node("decide", lambda s: {"decision": "propose" if s["round"] >= 2 else "more"})
    g.add_node("propose", lambda s: {})
    g.add_node("wait_human", lambda s: {"approved": interrupt({"waiting": "proposal"})})
    g.add_node("verify", lambda s: {})
    g.add_node("report", lambda s: {})
    g.add_edge(START, "collect")
    g.add_edge("collect", "hypothesize")
    g.add_edge("hypothesize", "decide")
    g.add_conditional_edges("decide", lambda s: s["decision"], {"propose": "propose", "more": "collect"})
    g.add_edge("propose", "wait_human")
    g.add_edge("wait_human", "verify")
    g.add_edge("verify", "report")
    g.add_edge("report", END)
    return g.compile(checkpointer=MemorySaver())


def test_graph_loops_then_pauses_at_wait_human_and_resumes():
    app = _build()
    cfg = {"configurable": {"thread_id": "inc-1"}}
    first = app.invoke({}, cfg)
    assert first["__interrupt__"][0].value == {"waiting": "proposal"}
    assert app.get_state(cfg).next == ("wait_human",)
    assert app.get_state(cfg).values["round"] == 2  # vòng lặp collect→decide chạy 2 lần trước khi đề xuất
    done = app.invoke(Command(resume=True), cfg)
    assert done["approved"] is True and "__interrupt__" not in done
    assert app.get_state(cfg).next == ()


def test_two_incidents_do_not_share_state():
    app = _build()
    a, b = {"configurable": {"thread_id": "a"}}, {"configurable": {"thread_id": "b"}}
    app.invoke({}, a)
    app.invoke({}, b)
    app.invoke(Command(resume=False), a)
    assert app.get_state(a).values["approved"] is False
    assert app.get_state(b).next == ("wait_human",)


def test_graph_can_be_drawn_for_the_docs():
    assert "wait_human" in _build().get_graph().draw_mermaid()
