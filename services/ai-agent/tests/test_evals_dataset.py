"""Bộ eval (evals/): kiểm tĩnh, KHÔNG gọi mô hình. Đổi tên tool/tham số ở gateway thì tools.snapshot.json lệch (test Go canh) và case ở đây báo lỗi."""
import sys
from datetime import date
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "evals"))
import evalkit  # noqa: E402

PERSONAS = list(evalkit.PERSONA_TOOL_ROLE)


@pytest.mark.parametrize("persona", PERSONAS)
def test_cases_are_valid_against_the_tool_table(persona):
    assert evalkit.validate(persona) == []


def test_total_size_is_enough_to_see_a_regression():
    total = sum(len(evalkit.load_cases(p)) for p in PERSONAS)
    assert total >= 380, total


def _case(**kw):
    base = dict(persona="buyer", id="t", g="filter", q="q", tool=["list_my_orders"], args={}, max=1, propose=None, deny=[], need=[])
    base.update(kw)
    return evalkit.Case(**base)


TODAY = date(2026, 11, 14)


def test_score_accepts_the_expected_single_call():
    c = _case(args={"status": "SHIPPED", "created_from": "@month_start"})
    r = evalkit.score(c, [{"tool": "list_my_orders", "args": {"status": "shipped", "created_from": "2026-11-01"}}], "ok", [], TODAY)
    assert r.ok, r.reasons


def test_score_rejects_paging_wrong_args_and_leaks():
    c = _case(args={"status": "SHIPPED"}, deny=["list_my_orders"])
    calls = [{"tool": "list_my_orders", "args": {"status": "PAID"}}, {"tool": "list_my_orders", "args": {"page": 2}}]
    r = evalkit.score(c, calls, "tôi gọi list_my_orders", [], TODAY)
    assert not r.ok and len(r.reasons) == 3


def test_score_none_and_proposals():
    refuse = _case(tool=["none"], args={})
    assert not evalkit.score(refuse, [{"tool": "get_cart", "args": {}}], "", [], TODAY).ok
    assert evalkit.score(refuse, [], "xin lỗi", [], TODAY).ok
    prop = _case(g="propose", tool=["get_my_order", "none"], propose="cancel_order", max=2)
    assert evalkit.score(prop, [], "đã tạo đề xuất", ["cancel_order"], TODAY).ok
    assert not evalkit.score(prop, [], "đã hủy", [], TODAY).ok


def test_date_tokens():
    assert evalkit.resolve_token("@month_start", TODAY) == "2026-11-01"
    assert evalkit.resolve_token("@last_month_end", TODAY) == "2026-10-31"
    assert evalkit.resolve_token("@week_start", TODAY) == "2026-11-09"  # thứ Hai
    assert evalkit.resolve_token("@days_ago_7", TODAY) == "2026-11-07"


async def test_runner_records_calls_and_scores_with_the_stub_gateway():
    """Cơ chế của run_evals.py (gateway giả dựng từ bảng tool thật, ghi lại lần gọi) chạy được với mô hình giả: không gọi LLM."""
    import run_evals
    from ai_agent.personas import load_personas
    from ai_agent.runtime.agent_factory import AgentRuntime
    from helpers import cfg, fake_builder

    snap = evalkit.load_snapshot()
    calls: list = []
    rt = AgentRuntime(cfg(), run_evals.stub_gateway(snap, calls), agent_builder=fake_builder)
    case = _case(q="Các đơn của tôi", tool=["list_my_orders"], args={"page_size": 3}, max=2)
    row = await run_evals.run_case(rt, calls, load_personas()["buyer"], case, TODAY, snap)
    assert row["calls"] and row["calls"][0]["tool"] == "list_my_orders" and row["calls"][0]["args"]["page_size"] == "3"
    assert row["ok"], row["reasons"]


def test_action_name_is_recovered_from_the_proposal_request():
    import run_evals
    snap = evalkit.load_snapshot()
    assert run_evals._action_name(snap, "buyer", {"method": "POST", "path": "/orders/12/cancel"}) == "cancel_order"


def test_every_tool_in_the_table_has_a_friendly_label_and_source():
    """Tool mới ở gateway mà thiếu nhãn tiếng Việt sẽ hiện "Đang tra cứu…" chung chung và nguồn trống: bắt ở đây."""
    from ai_agent.runtime import events
    snap = evalkit.load_snapshot()
    names = {t["name"] for role in ("buyer", "seller_admin", "admin", "operator") for t in snap["scopes"][role]}
    missing = sorted(n for n in names if n not in events.TOOL_LABELS and not n.startswith("admin_analytics_"))
    assert not missing, f"thiếu TOOL_LABELS: {missing}"
    missing = sorted(n for n in names if n not in events.SOURCE_LABELS and not n.startswith("admin_analytics_"))
    assert not missing, f"thiếu SOURCE_LABELS: {missing}"


def test_date_token_tolerance():
    from datetime import date

    import evalkit

    today = date(2026, 11, 15)
    assert evalkit._same("@days_ago_7~1", "2026-11-09", today)      # 6 ngày trước: vẫn đạt
    assert evalkit._same("@days_ago_7~1", "2026-11-08", today)
    assert not evalkit._same("@days_ago_7~1", "2026-11-05", today)
    assert not evalkit._same("@days_ago_7", "2026-11-09", today)    # không có ~N thì khớp đúng
