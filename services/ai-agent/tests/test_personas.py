"""Persona theo role trong token: admin và operator có persona riêng, chỉ đọc; không role nào rơi về persona của role khác."""
from ai_agent.personas import load_personas, persona_for_role
from ai_agent.runtime import events

from test_chat_flow import collect, make


def test_every_gateway_role_has_exactly_one_persona():
    personas = load_personas()
    for role, name in {"buyer": "buyer", "seller_admin": "seller", "seller_employee": "seller", "admin": "admin", "operator": "operator"}.items():
        assert persona_for_role(personas, role).name == name
    assert persona_for_role(personas, "nobody") is None


def test_admin_prompt_is_read_only_and_operator_may_only_propose_an_ops_action():
    personas = load_personas()
    p = personas["admin"].prompt
    assert "CHỈ ĐỌC" in p and "propose_" not in p and "{policy}" not in p
    # operator (ADR-45): one proposal tool, ops_action; it never approves, runs or records a result
    o = personas["operator"].prompt
    assert "propose_ops_action" in o and "{policy}" not in o
    assert o.count("propose_") == o.count("propose_ops_action")
    assert "NGƯỜI KHÁC" in o and "không có công cụ để duyệt" in o
    assert "không có dữ liệu kinh doanh" in o


async def test_admin_turn_runs_with_the_admin_persona():
    rt, store, personas = make()
    evs, _ = await collect(rt, store, personas["admin"], role="admin")
    assert evs[0]["event"] == events.START and evs[-1]["event"] == events.DONE


def test_admin_tool_labels_are_friendly():
    assert events.label_for("admin_analytics_funnel") == "Đang xem báo cáo toàn sàn…"
    assert events.source_label("ops_list_incidents") == "Sự cố"
    assert "/" not in events.source_label("admin_analytics_traffic")


def test_policy_numbers_come_from_the_tool_not_from_the_text():
    personas = load_personas()
    for name in ("buyer", "seller"):
        p = personas[name].prompt
        assert "{policy}" not in p and "chưa có tài liệu chính sách" not in p and "TODO" not in p
        assert "get_shop_rules" in p
        # no hard-coded rule values: they would go stale when the order-service settings change
        for stale in ("30.000", "500.000", "15 phút", "7 ngày"):
            assert stale not in p, f"{name}: {stale!r} must come from get_shop_rules"


def test_ops_tools_have_friendly_labels():
    for tool in ("ops_metrics_query", "ops_list_proposals", "ops_get_proposal", "get_shop_rules"):
        assert events.label_for(tool) != "Đang tra cứu…" and "/" not in events.source_label(tool)


def test_every_persona_is_enabled_and_has_an_llm_role_in_every_config():
    """Persona nằm ở code, config.*.yaml và bảng quyền của gateway: quên một nơi thì chỉ lộ khi chạy thật ("vai trò này chưa được hỗ trợ")."""
    from helpers import CONFIG_DIR
    from ai_agent.config import load_config

    for env in ("dev", "prod"):
        c = load_config(env, CONFIG_DIR)
        for name, persona in load_personas().items():
            assert c.personas.get(name, {}).get("enabled") is True, f"{env}: persona {name} chưa bật trong personas:"
            assert persona.llm_role in c._data["llm"]["roles"], f"{env}: llm.roles thiếu {persona.llm_role}"


def test_prompts_tell_agents_to_filter_instead_of_paging():
    p = load_personas()
    for name in ("buyer", "seller", "admin"):
        assert "MỘT lần gọi" in p[name].prompt, name
