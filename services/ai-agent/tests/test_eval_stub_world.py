"""Gateway giả của eval phải có NHIỀU đơn/sự cố/đề xuất: một dòng duy nhất làm câu "cái đó" tự suy ra được và méo nhóm clarify."""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "evals"))
import run_evals  # noqa: E402


def test_lists_have_several_rows_and_ids_stay_stable():
    for tool in ("ops_list_incidents", "ops_list_proposals", "ops_list_audit", "ops_outbox_dead"):
        assert run_evals.domain_body(tool, {})["total"] >= 3
    assert {i["id"] for i in run_evals.domain_body("ops_list_incidents", {})["items"]} >= {4, 5, 6}
    assert run_evals.domain_body("ops_get_incident", {"id": "4"})["alert_name"] == "OutboxDeadRows"
    assert run_evals.domain_body("ops_get_incident", {"id": "5"})["alert_name"] == "HighErrorRate"
    assert run_evals.domain_body("ops_get_proposal", {"id": "1"})["action_id"] == "requeue_outbox"
    assert run_evals.domain_body("ops_get_proposal", {"id": "2"})["action_id"] == "scale_service"  # không trùng ca "restart product-service" (agent tránh tạo bản trùng)


def test_delivered_at_is_stable_within_a_day():
    """Mốc giao hàng của gateway giả không có giờ phút giây: nếu có, nội dung gửi model đổi mỗi lần chạy và cache replay luôn trượt."""
    from datetime import datetime, timezone

    import evalkit

    case = next(c for c in evalkit.load_cases("buyer") if c.propose != "cancel_order")
    a = run_evals.world_state(case, datetime(2026, 11, 21, 3, 53, 42, 123456, tzinfo=timezone.utc))
    b = run_evals.world_state(case, datetime(2026, 11, 21, 17, 1, 2, 999, tzinfo=timezone.utc))
    assert a["delivered_at"] == b["delivered_at"] == "2026-11-20T00:00:00+00:00"


def test_stub_action_book_matches_real_catalog_services():
    """Sổ hành động giả không liệt kê ops-service cho restart/scale, như catalog thật; nếu có thì agent thấy sổ và tool mâu thuẫn nhau."""
    import run_evals

    for a in run_evals.domain_body("ops_list_actions", {})["actions"]:
        for p in a["params"]:
            if p["name"] == "service" and a["id"] in ("restart_service", "scale_service"):
                assert "ops-service" not in p["enum"] and "api-gateway" in p["enum"]
