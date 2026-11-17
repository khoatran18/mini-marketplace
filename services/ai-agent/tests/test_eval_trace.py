"""Log đầy đủ của eval live (evals/tracekit.py): chạy một ca qua agent THẬT (create_agent) với mô hình giả có kịch bản, rồi kiểm cái đã ghi.
Không gọi mạng."""
import json
import sys
from datetime import date
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE.parent / "evals"))

import evalkit  # noqa: E402
import run_evals  # noqa: E402
import tracekit  # noqa: E402
from ai_agent.personas import load_personas  # noqa: E402
from ai_agent.runtime.agent_factory import AgentRuntime  # noqa: E402

from helpers import cfg  # noqa: E402


async def _run_one(case_q="Đơn của tôi đâu?"):
    c = cfg()
    snapshot = evalkit.load_snapshot()
    calls: list = []
    rt = AgentRuntime(c, run_evals.stub_gateway(snapshot, calls, {}))
    case = evalkit.Case(persona="buyer", id="buyer-001", g="lookup", q=case_q, tool=["list_my_orders"])
    return await run_evals.run_case(rt, calls, load_personas()["buyer"], case, date(2026, 11, 17), snapshot, state={})


async def test_a_case_records_prompts_outputs_tokens_and_tool_calls():
    row = await _run_one()
    t = row["trace"]
    assert row["ok"], row["reasons"]
    assert t["totals"]["model_calls"] == 2 and t["totals"]["tool_calls"] == 1          # kịch bản: gọi tool rồi trả lời
    first, second = t["model_calls"]
    assert first["output"]["tool_calls"][0]["name"] == "list_my_orders"                 # mô hình chọn tool nào
    assert "tools_offered" in first                                                     # (mô hình giả không đưa tool vào tham số: kiểm có tên ở test ChatOpenAI bên dưới)
    sysmsg = first["messages"][0]
    assert sysmsg["role"] == "system" and sysmsg["ref"] in row["systems"] and len(row["systems"][sysmsg["ref"]]) == sysmsg["chars"] > 100   # system prompt ghi một lần, tham chiếu bằng băm
    assert first["messages"][-1]["role"] == "user" and "Đơn của tôi đâu?" in first["messages"][-1]["content"]   # prompt vào mô hình, có dòng ngày
    assert any(m["role"] == "tool" and m["tool_name"] == "list_my_orders" for m in second["messages"])          # lần 2 mô hình thấy kết quả tool
    assert second["output"]["content"] and "đơn gần đây" in second["output"]["content"]                         # câu trả lời của mô hình
    tool = t["tool_calls"][0]
    assert tool["name"] == "list_my_orders" and tool["output"] and not tool["truncated"] and tool["seconds"] >= 0
    assert t["turn"]["status"] == "ok" and t["events"][0] == "start" and t["events"][-1] == "done"
    assert "mm_ai_runs_total{persona=buyer,status=ok}" in t["metrics_delta"]            # số liệu Prometheus đổi trong ca này


async def test_tokens_are_summed_when_the_model_reports_usage():
    t = tracekit.Tracer()
    t.model_calls = [{"usage": {"input_tokens": 100, "output_tokens": 20}, "seconds": 1.0}, {"usage": {"input_tokens": 150, "output_tokens": 30}, "seconds": 2.0}]
    tot = t.summary()["totals"]
    assert (tot["input_tokens"], tot["output_tokens"], tot["total_tokens"], tot["model_seconds"]) == (250, 50, 300, 3.0)


def test_run_log_writes_meta_jsonl_summary_and_log_without_secrets(tmp_path, monkeypatch, capsys):
    monkeypatch.setenv("OPENCODE_API_KEY", "SECRET-VALUE-123")
    log = tracekit.RunLog(tmp_path, "unit")
    log.write_meta({"a": 1})
    print("dòng in ra màn hình")
    rows = [{"id": "buyer-001", "g": "lookup", "q": "x", "ok": True, "reasons": [], "seconds": 1.0,
             "trace": {"totals": {"model_calls": 2, "tool_calls": 1, "input_tokens": 10, "output_tokens": 5, "total_tokens": 15, "model_seconds": 1.0}}},
            {"id": "buyer-002", "g": "clarify", "q": "y", "ok": False, "reasons": ["không được gọi tool nhưng gọi ['a']"], "seconds": 2.0, "answer": "z",
             "trace": {"totals": {"model_calls": 1, "tool_calls": 1, "input_tokens": 7, "output_tokens": 3, "total_tokens": 10, "model_seconds": 0.5}}}]
    for r in rows:
        log.append(r)
    log.add_systems("buyer", {"abc": "SYSTEM PROMPT"})
    log.finish(tracekit.summarize(rows))
    d = log.dir
    assert [json.loads(line)["id"] for line in (d / "cases.jsonl").read_text(encoding="utf-8").splitlines()] == ["buyer-001", "buyer-002"]
    s = json.loads((d / "summary.json").read_text(encoding="utf-8"))
    assert s["cases"] == 2 and s["ok"] == 1 and s["total_tokens"]["sum"] == 25 and s["model_calls"]["max"] == 2
    assert s["by_persona_group"]["buyer/clarify"] == {"ok": 0, "total": 1} and s["failed"][0]["id"] == "buyer-002"
    assert json.loads((d / "meta.json").read_text(encoding="utf-8"))["system_prompts"]["buyer"]["abc"] == "SYSTEM PROMPT"
    assert "dòng in ra màn hình" in (d / "run.log").read_text(encoding="utf-8")
    assert all("SECRET-VALUE-123" not in f.read_text(encoding="utf-8") for f in d.iterdir())


def test_load_env_reads_only_model_key_prefixes_and_never_overrides(tmp_path, monkeypatch):
    f = tmp_path / ".env"
    f.write_text("OPENCODE_API_KEY=from-file\nDEEPSEEK_API_KEY=\nPOSTGRES_PASSWORD=nope\nANTHROPIC_API_KEY=a-key\n", encoding="utf-8")
    for k in ("OPENCODE_API_KEY", "DEEPSEEK_API_KEY", "POSTGRES_PASSWORD", "ANTHROPIC_API_KEY"):
        monkeypatch.delenv(k, raising=False)
    monkeypatch.setenv("ANTHROPIC_API_KEY", "already-set")
    names = evalkit.load_env(f)
    import os
    assert names == ["OPENCODE_API_KEY"]                       # chỉ trả tên; không nạp biến rỗng, biến khác tiền tố, hay đè biến có sẵn
    assert os.environ["OPENCODE_API_KEY"] == "from-file" and os.environ["ANTHROPIC_API_KEY"] == "already-set" and "POSTGRES_PASSWORD" not in os.environ
    monkeypatch.delenv("OPENCODE_API_KEY")


async def test_real_chat_openai_class_reports_tokens_tools_offered_and_finish_reason():
    """Lớp ChatOpenAI THẬT (HTTP giả, không mạng): usage của nhà cung cấp phải vào log, kèm tool đã đưa cho mô hình."""
    import httpx
    from langchain_core.tools import tool
    from langchain_openai import ChatOpenAI

    @tool
    def list_my_orders(page_size: int = 3) -> str:
        """orders"""
        return "[]"

    def handler(req: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"id": "c1", "object": "chat.completion", "created": 1, "model": "deepseek-x", "usage": {"prompt_tokens": 321, "completion_tokens": 12, "total_tokens": 333},
                                         "choices": [{"index": 0, "finish_reason": "tool_calls", "message": {"role": "assistant", "content": None, "tool_calls": [
                                             {"id": "t1", "type": "function", "function": {"name": "list_my_orders", "arguments": "{\"page_size\": 3}"}}]}}]})

    model = ChatOpenAI(model="deepseek-x", api_key="k", base_url="http://x/v1", http_async_client=httpx.AsyncClient(transport=httpx.MockTransport(handler))).bind_tools([list_my_orders])
    with tracekit.traced() as t:
        await model.ainvoke([("system", "SYS"), ("user", "hi")])
    call = t.summary()["model_calls"][0]
    assert call["tools_offered"] == ["list_my_orders"] and call["finish_reason"] == "tool_calls" and call["model"] == "deepseek-x"
    assert (call["usage"]["input_tokens"], call["usage"]["output_tokens"], call["usage"]["total_tokens"]) == (321, 12, 333)
    assert call["output"]["tool_calls"] == [{"name": "list_my_orders", "args": {"page_size": 3}}]


async def test_a_failed_model_call_is_still_counted_with_its_error():
    import httpx
    from langchain_openai import ChatOpenAI

    model = ChatOpenAI(model="x", api_key="k", base_url="http://x/v1", max_retries=0,
                       http_async_client=httpx.AsyncClient(transport=httpx.MockTransport(lambda r: httpx.Response(429, json={"error": {"message": "rate limited"}}))))
    with tracekit.traced() as t:
        try:
            await model.ainvoke([("user", "hi")])
        except Exception:  # noqa: BLE001
            pass
    s = t.summary()
    assert s["totals"]["model_calls"] == 1 and s["errors"][0]["status_code"] == 429 and "RateLimit" in s["model_calls"][0]["error"]


def test_stub_gateway_returns_real_shaped_data_for_ops_tools_not_order_records():
    """Trước đây mọi tool không phải sản phẩm trả bản ghi kiểu đơn hàng (status PAID...), làm model operator thấy dữ liệu sai và gọi thêm nhiều lần."""
    snap = evalkit.load_snapshot()
    ops = [t["name"] for t in snap["scopes"]["operator"]]
    assert {"ops_list_incidents", "ops_metrics_query", "ops_system_health", "ops_list_actions"} <= set(ops)
    for tool in ops:
        body = run_evals.domain_body(tool, {"id": "4", "name": "service_up", "service": "order-service"})
        assert body is not None, f"{tool} chưa có dữ liệu giả đúng hình dạng"
        assert "PAID" not in json.dumps(body) and "delivered_at" not in body, tool
    assert run_evals.domain_body("ops_list_incidents", {})["items"][0]["alert_name"]
    assert run_evals.domain_body("ops_metrics_query", {"name": "logs_errors", "service": "order-service"})["untrusted"] is True
    assert len(run_evals.domain_body("ops_system_health", {})["services"]) == 8
    assert run_evals.domain_body("search_products", {}) is None                 # dạng chung vẫn dùng cho sản phẩm/đơn


async def test_stub_gateway_serves_the_new_shapes_over_http():
    from helpers import agent_token

    calls: list = []
    gw = run_evals.stub_gateway(evalkit.load_snapshot(), calls, {})
    tok = agent_token(1, "operator", "run-x", ttl=900)
    incidents = await gw.get(tok, "/ops/incidents")
    assert incidents["items"][0]["alert_name"] == "OutboxDeadRows" and "status" not in incidents["items"][0]
    health = await gw.get(tok, "/ops/system/health")
    assert health["status"] == "ready" and len(health["services"]) == 8
    metric = await gw.get(tok, "/ops/queries/service_up", {"service": "order-service"})
    assert metric["query"] == "service_up" and metric["series"][0]["labels"]["service"] == "order-service"
    assert [c["tool"] for c in calls][-3:] == ["ops_list_incidents", "ops_system_health", "ops_metrics_query"]
