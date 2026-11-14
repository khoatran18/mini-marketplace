"""Buyer/seller không được nghe agent kể đường dẫn API hay tên công cụ; admin/dev thì được. Cộng với lọc chuỗi nhạy cảm trong text tự do."""
import pytest

from ai_agent.config import Config
from ai_agent.guard.leak import REPLACEMENT, LeakFilter
from ai_agent.personas import load_personas
from ai_agent.runtime import events
from ai_agent.tools.redact import MASK, redact
from ai_agent.tools.spec import ToolSpec
from ai_agent.tools.views import model_view

from helpers import cfg
from test_chat_flow import collect, make

TOOLS = ["list_my_orders", "get_my_order", "propose_cancel_order"]


@pytest.mark.parametrize("text", [
    "Mình đã gọi GET /orders/:id để lấy đơn.",
    "Mình gọi list_my_orders rồi propose_cancel_order.",
    "Route /ops/system/health và /admin/orders đều dùng được.",
    "POST /seller/products/12/inventory/adjust",
])
def test_scrub_hides_routes_and_tool_names(text):
    out = LeakFilter(TOOLS).scrub(text)
    assert REPLACEMENT in out
    for leak in ("list_my_orders", "propose_cancel_order", "/orders/:id", "/ops/", "/admin/", "/seller/products"):
        assert leak not in out


def test_scrub_leaves_normal_text_and_storefront_pages_alone():
    t = "Đơn #12 đang giao. Bạn xem ở trang /cart hoặc mục Đơn hàng của tôi nhé; giá 20.000đ."
    assert LeakFilter(TOOLS).scrub(t) == t


def test_streaming_catches_a_name_split_across_chunks():
    f = LeakFilter(TOOLS)
    text = "Để trả lời mình đã dùng công cụ list_my_orders rồi GET /orders/:id để lấy dữ liệu cho bạn, kết quả như sau."
    for size in (1, 2, 3, 5, 7):
        f = LeakFilter(TOOLS)
        out = "".join(f.feed(text[i:i + size]) for i in range(0, len(text), size)) + f.flush()
        assert "list_my_orders" not in out and "/orders/:id" not in out, (size, out)
        assert out.startswith("Để trả lời mình đã dùng") and out.endswith("kết quả như sau.")


def test_model_view_hides_the_route_for_buyer_and_shows_it_for_ops():
    spec = ToolSpec("get_my_order", "GET", "/orders/:id", "one order")
    assert "/orders/:id" not in model_view(spec, {"id": 1}, 2000, 20, reveal_route=False)
    assert "Đơn hàng của bạn" in model_view(spec, {"id": 1}, 2000, 20, reveal_route=False)
    assert "GET /orders/:id" in model_view(spec, {"id": 1}, 2000, 20, reveal_route=True)


def test_persona_flags():
    p = load_personas()
    assert p["buyer"].hide_internals and p["seller"].hide_internals
    assert not p["admin"].hide_internals and not p["dev"].hide_internals
    for name in ("buyer", "seller"):
        assert "đường dẫn" in p[name].prompt and "bạn đã gọi API nào" in p[name].prompt


def _with_script(script):
    d = cfg().to_dict(masked=False)
    d["llm"]["profiles"]["fake_buyer"]["script"] = script
    d["llm"]["profiles"]["fake_seller"]["script"] = script
    return Config(d)


LEAKY = [{"content": "Mình đã gọi list_my_orders qua GET /orders để lấy đơn của bạn nhé."}]


async def test_buyer_answer_and_stored_answer_are_scrubbed():
    rt, store, personas = make(config=_with_script(LEAKY))
    evs, turn = await collect(rt, store, personas["buyer"])
    answer = "".join(e["data"]["text"] for e in evs if e["event"] == events.TOKEN)
    assert REPLACEMENT in answer and "list_my_orders" not in answer and "/orders" not in answer
    assert store.conversations[turn.conversation_id]["messages"][-1] == ("assistant", answer)


async def test_admin_answer_is_not_scrubbed():
    rt, store, personas = make(config=_with_script(LEAKY))
    evs, _ = await collect(rt, store, personas["admin"], role="admin")
    answer = "".join(e["data"]["text"] for e in evs if e["event"] == events.TOKEN)
    assert "list_my_orders" in answer and "GET /orders" in answer


def test_free_text_secrets_are_masked_before_reaching_the_model():
    jwt = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjMifQ.abcdefgh"
    data = {"message": f"login failed user a@b.com phone 0901234567 ip 10.1.2.3 Authorization: Bearer abc.def-123456 token={jwt} password=hunter2xx"}
    out = redact(data)["message"]
    for secret in ("a@b.com", "0901234567", "10.1.2.3", "abc.def-123456", jwt, "hunter2xx"):
        assert secret not in out, out
    assert MASK in out and out.startswith("login failed user")
