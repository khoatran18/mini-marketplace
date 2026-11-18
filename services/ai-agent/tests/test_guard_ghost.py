"""Đề xuất ma (guard/ghost.py): câu trả lời nói đã soạn đề xuất / bảo bấm Xác nhận mà lượt không tạo đề xuất nào.
Phần phát hiện là xác định (không gọi mô hình); ctx.proposals là nguồn sự thật có đề xuất thật hay không."""
import pytest

from ai_agent import metrics
from ai_agent.config import Config
from ai_agent.guard import ghost
from ai_agent.runtime import events

from helpers import cfg
from test_chat_flow import collect, make

# Câu thật do hy4-flash-free viết khi không gọi tool đề xuất (log eval 2026-11-18), cộng vài biến thể không dấu / in đậm
GHOSTS = [
    "Đơn 45 đã thanh toán, đủ điều kiện huỷ.\n\nTôi đã chuẩn bị đề xuất huỷ đơn 45 (lý do: khách yêu cầu). Bạn kiểm tra và bấm **Xác nhận** để hoàn tất nhé.",
    "Tôi đã soạn đề xuất huỷ; bạn kiểm tra và bấm **Xác nhận** nhé.",
    "Đề xuất điều chỉnh +100 (lý do: kiểm kê). Bạn kiểm tra lại và bấm nút Xác nhận.",
    "Da soan de xuat huy don, bam nut xac nhan nhe",
    "Mình vừa soạn xong đề xuất trả hàng, hãy nhấn Xác nhận.",
    "Đề xuất đã sẵn sàng bên dưới.",
]
HONEST = [
    "Mình chưa tạo được đề xuất nào.",
    "Nếu bạn muốn, mình sẽ soạn đề xuất hủy đơn.",
    "Mình có thể soạn đề xuất cho bạn, bạn nói rõ lý do nhé.",
    "Đơn 45 đã giao, không thể huỷ. Bạn có thể yêu cầu trả hàng.",
    "Đơn của bạn đang ở trạng thái PAID.",
    "Bạn cần bấm nút Thanh toán để tiếp tục.",
    "Tôi kiểm tra thêm các cảnh báo đang firing để xác nhận.",
    "Bước kiểm tra tiếp theo: service_ready để xác nhận service sẵn sàng nhận request.",
    # giải thích cách làm việc chung (không phải khẳng định đã tạo)
    "Với huỷ đơn hoặc trả hàng, mình chỉ soạn đề xuất để bạn kiểm tra và bấm Xác nhận.",
    "Mình không thể tự áp dụng: mình chỉ gửi đề xuất để bạn kiểm tra và bấm Xác nhận trên giao diện.",
    "",
]


@pytest.mark.parametrize("text", GHOSTS)
def test_claims_are_detected(text):
    assert ghost.claims_proposal(text)


@pytest.mark.parametrize("text", HONEST)
def test_denials_conditions_and_process_explanations_are_not_claims(text):
    assert not ghost.claims_proposal(text)


def test_a_real_proposal_or_a_persona_without_proposals_is_never_a_ghost():
    claim = GHOSTS[0]
    assert ghost.is_ghost(claim, 0, can_propose=True)
    assert not ghost.is_ghost(claim, 1, can_propose=True)      # đề xuất thật đã tạo
    assert not ghost.is_ghost(claim, 0, can_propose=False)     # admin / RCA không có tool đề xuất


def test_mode_falls_back_to_log_never_to_a_notice_by_accident():
    assert [ghost.mode_of(v) for v in ("off", "LOG", "notice", None, "bogus")] == ["off", "log", "notice", "notice", "log"]


def _with_script(script, mode=None):
    d = cfg().to_dict(masked=False)
    for p in ("fake_buyer", "fake_seller"):
        d["llm"]["profiles"][p]["script"] = script
    if mode:
        d["limits"]["ghost_proposal"] = mode
    return Config(d)


def count(persona, mode):
    return metrics.GHOST_PROPOSALS.labels(persona, mode)._value.get()


async def run(script, mode=None, persona="buyer", role="buyer"):
    rt, store, personas = make(config=_with_script(script, mode))
    evs, turn = await collect(rt, store, personas[persona], role=role)
    answer = "".join(e["data"]["text"] for e in evs if e["event"] == events.TOKEN)
    return evs, answer, store.conversations[turn.conversation_id]["messages"][-1]


async def test_ghost_proposal_gets_a_notice_a_metric_and_the_stored_answer_matches():
    before = count("buyer", "notice")
    evs, answer, stored = await run([{"content": GHOSTS[0]}])
    assert ghost.NOTICE.strip() in answer and answer.startswith("Đơn 45") and evs[-1]["event"] == events.DONE
    assert stored == ("assistant", answer)                                        # lịch sử khớp với màn hình
    assert not any(e["event"] == events.ACTION_PROPOSAL for e in evs)
    assert count("buyer", "notice") - before == 1


async def test_log_mode_counts_but_leaves_the_answer_alone():
    before = count("buyer", "log")
    _, answer, _ = await run([{"content": GHOSTS[0]}], mode="log")
    assert ghost.NOTICE.strip() not in answer and "bấm **Xác nhận**" in answer
    assert count("buyer", "log") - before == 1


async def test_off_mode_does_nothing():
    b = (count("buyer", "log"), count("buyer", "notice"))
    _, answer, _ = await run([{"content": GHOSTS[0]}], mode="off")
    assert ghost.NOTICE.strip() not in answer and (count("buyer", "log"), count("buyer", "notice")) == b


async def test_a_turn_that_really_created_a_proposal_is_left_alone():
    before = count("buyer", "notice")
    script = [{"tool_calls": [{"name": "propose_cancel_order", "args": {"id": 12, "reason": "đặt nhầm"}}]}, {"content": GHOSTS[0]}]
    evs, answer, _ = await run(script)
    assert any(e["event"] == events.ACTION_PROPOSAL for e in evs)
    assert ghost.NOTICE.strip() not in answer and count("buyer", "notice") == before


async def test_a_failed_proposal_tool_still_counts_as_no_proposal():
    """Gateway từ chối đề xuất (tool trả lỗi, không có sự kiện action_proposal) mà mô hình vẫn nói đã soạn: vẫn là đề xuất ma."""
    from helpers import mock_gateway
    from ai_agent.runtime.agent_factory import AgentRuntime
    from ai_agent.store import MemoryStore
    from ai_agent.personas import load_personas
    from helpers import fake_builder
    rt = AgentRuntime(_with_script([{"tool_calls": [{"name": "propose_cancel_order", "args": {"id": 12}}]}, {"content": GHOSTS[1]}]),
                      mock_gateway(status=500), agent_builder=fake_builder)
    store, personas = MemoryStore(), load_personas()
    evs, _ = await collect(rt, store, personas["buyer"])
    answer = "".join(e["data"]["text"] for e in evs if e["event"] == events.TOKEN)
    assert not any(e["event"] == events.ACTION_PROPOSAL for e in evs) and ghost.NOTICE.strip() in answer


async def test_an_honest_answer_without_a_claim_is_not_touched():
    _, answer, _ = await run([{"content": "Đơn đã giao nên không huỷ được. Bạn có thể yêu cầu trả hàng nhé."}])
    assert ghost.NOTICE.strip() not in answer
