"""Bộ mục tiêu người dùng giả lập (evals/sim/goals.yaml) và phần chấm điểm của evals/sim.py. Không gọi mô hình."""
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


def test_min_turns_blocks_early_done_and_prompt_carries_behavior(monkeypatch):
    import run_evals

    async def fake_run_case(rt, calls, persona, case, today, snapshot, state=None):
        return {"answer": "ok", "calls": [], "proposals": [], "reasons": []}

    monkeypatch.setattr(run_evals, "run_case", fake_run_case)
    prompts = []

    def user(p):
        prompts.append(p)
        return "DONE" if "CHƯA được dừng" not in p else "tin tiếp"

    g = _goal(min_turns=2, max_turns=4, behavior="cãi lại khi bị từ chối", propose="none")
    r = asyncio.run(sim.simulate(g, None, [], {}, None, {}, None, user))
    assert r["turns"] == 2  # hai tin đầu bắt buộc, sau đó DONE được chấp nhận
    assert all("cãi lại khi bị từ chối" in p for p in prompts)
    assert sim.validate([_goal(min_turns=5, max_turns=3)])  # min_turns > max_turns là lỗi


def test_user_model_uses_replay_cache(tmp_path, monkeypatch):
    """Model đóng vai người dùng cũng đi qua cache phát lại: lần 2 không gọi mạng; chế độ ro thì trượt là lỗi."""
    import io
    import json

    import pytest

    from ai_agent.llm import replay_cache

    calls = []

    class Resp(io.BytesIO):
        def __enter__(self):
            return self

        def __exit__(self, *a):
            return False

    def fake_urlopen(req, timeout=0):
        calls.append(1)
        return Resp(json.dumps({"choices": [{"message": {"content": f"câu {len(calls)}"}}]}).encode())

    monkeypatch.setenv("OPENCODE_API_KEY", "x")
    monkeypatch.setattr(sim.urllib.request, "urlopen", fake_urlopen)
    monkeypatch.setenv("AI_LLM_CACHE_DIR", str(tmp_path))
    monkeypatch.setenv("AI_LLM_CACHE_MODE", "rw")
    monkeypatch.setattr(replay_cache, "_shared", None)
    assert sim.call_user_model("https://x.test/v1", "m", "hỏi gì đó") == "câu 1"
    monkeypatch.setattr(replay_cache, "_shared", None)  # tiến trình mới
    assert sim.call_user_model("https://x.test/v1", "m", "hỏi gì đó") == "câu 1" and len(calls) == 1
    monkeypatch.setenv("AI_LLM_CACHE_MODE", "ro")
    monkeypatch.setattr(replay_cache, "_shared", None)
    with pytest.raises(replay_cache.CacheMiss):
        sim.call_user_model("https://x.test/v1", "m", "lời nhắc khác")
    assert len(calls) == 1
