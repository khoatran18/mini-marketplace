"""Bộ mục tiêu người dùng giả lập (evals/sim/goals.yaml) và phần chấm điểm của evals/run_sim_users.py. Không gọi mô hình."""
import asyncio
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent.parent / "evals"
sys.path.insert(0, str(HERE))

import run_sim_users as sim  # noqa: E402


def _goal(**kw):
    g = {"id": "sim-x-01", "persona": "buyer", "goal": "g", "style": "s", "tool": [], "deny": [], "max_turns": 3}
    g.update(kw)
    return g


def _turn(answer="", calls=(), proposals=(), error=None):
    return {"msg": "m", "answer": answer, "calls": list(calls), "proposals": list(proposals), "error": error}


def test_goal_file_is_valid_and_covers_every_persona():
    goals = sim.load_goals()
    assert sim.validate(goals) == []
    assert {g["persona"] for g in goals} == {"buyer", "seller", "admin", "operator"}
    assert len({g["id"] for g in goals}) == len(goals)


def test_validate_catches_bad_goals():
    bad = [_goal(tool=["khong_co"]), _goal(propose="ops_action"), _goal(persona="admin", propose="cancel_order"), _goal(deny=["("]), _goal(max_turns=20), _goal(oops=1)]
    errs = sim.validate(bad)
    assert len(errs) >= 6


def test_clean_user_message():
    assert sim.clean_user_message('"Hủy đơn 88 giúp mình"') == "Hủy đơn 88 giúp mình"
    assert sim.clean_user_message("Bạn: ok nha") == "ok nha"
    assert sim.clean_user_message("DONE") is None
    assert sim.clean_user_message("done.") is None
    assert sim.clean_user_message("   ") is None
    assert sim.clean_user_message(None) is None


def test_judge_proposal_and_tool_goals():
    g = _goal(propose="cancel_order", tool=["get_my_order"])
    assert sim.judge(g, [_turn("ok", ["get_my_order"], ["cancel_order"])])["ok"]
    r = sim.judge(g, [_turn("ok", ["get_my_order"], [])])
    assert not r["ok"] and "không đề xuất cancel_order" in r["reasons"][0]
    assert not sim.judge(_goal(propose="none"), [_turn("ok", [], ["cancel_order"])])["ok"]
    assert not sim.judge(_goal(tool=["get_cart"]), [_turn("ok", ["get_my_order"])])["ok"]


def test_judge_deny_ghost_and_error():
    assert not sim.judge(_goal(deny=["GET /"]), [_turn("dùng GET /buyer/orders")])["ok"]
    ghost = sim.judge(_goal(), [_turn("Mình đã soạn đề xuất hủy đơn, bạn bấm Xác nhận nhé.")])
    assert not ghost["ok"] and "đề xuất ma" in ghost["reasons"][0]
    assert sim.judge(_goal(), [_turn("Mình đã soạn đề xuất hủy đơn, bạn bấm Xác nhận nhé.", proposals=["cancel_order"])])["ok"]
    assert not sim.judge(_goal(), [_turn("x", error="lỗi runtime: llm")])["ok"]


def test_simulate_stops_on_done_proposal_and_max_turns(monkeypatch):
    import run_evals

    seen = []

    async def fake_run_case(rt, calls, persona, case, today, snapshot, state=None):
        seen.append((case.q, list(case.history)))
        prop = ["cancel_order"] if "huỷ" in case.q else []
        return {"answer": "trả lời", "calls": [{"tool": "get_my_order"}], "proposals": prop, "reasons": []}

    monkeypatch.setattr(run_evals, "run_case", fake_run_case)
    msgs = iter(["cho mình hỏi đơn 88", "huỷ đơn 88 giúp mình", "không dùng tới"])
    run = lambda goal, fn: asyncio.run(sim.simulate(goal, None, [], {}, None, {}, None, fn))  # noqa: E731
    r = run(_goal(propose="cancel_order", max_turns=5), lambda p: next(msgs))
    assert r["ok"] and r["turns"] == 2  # dừng ngay khi có đề xuất
    assert seen[1][1] == [("user", "cho mình hỏi đơn 88"), ("assistant", "trả lời")]  # lượt sau mang lịch sử
    seen.clear()
    r = run(_goal(propose="none", max_turns=3), lambda p: "DONE")
    assert r["turns"] == 0
    r = run(_goal(propose="none", max_turns=2), lambda p: "hỏi tiếp")
    assert r["turns"] == 2 and r["ok"]
