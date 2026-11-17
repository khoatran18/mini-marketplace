# Nhật ký xây vòng lặp RCA (điều tra sự cố)

> Tách từ [09_AGENT_BUILD_LOG.md](09_AGENT_BUILD_LOG.md) (§16.26 trở đi) vì file đó đã quá dài. Giữ nguyên số mục §16.26-§16.31 để các chỗ trích dẫn cũ vẫn tìm được. ADR: 46 (LangGraph), 47 (danh tính hệ thống); tiêu chí chất lượng: [10_AGENT_QUALITY_CRITERIA.md](10_AGENT_QUALITY_CRITERIA.md).

### 16.26 Đồ thị RCA và thử LangGraph (ADR-46, 2026-11-15)

Đã thêm `llm.roles.rca` ở cả hai config (cùng profile `opencode_deepseek_flash`). Đã thử LangGraph bằng đồ thị mẫu, LLM giả, không gọi mạng: `interrupt()` dừng đúng ở `wait_human`, `Command(resume=...)` chạy tiếp, hai sự cố (`thread_id`) không lẫn trạng thái, `draw_mermaid()` vẽ được. Khoá bằng `tests/test_langgraph_contract.py` (3 test; toàn bộ ai-agent: 106 passed, 11 skipped).

```mermaid
graph TD;
  start([Sự cố]) --> collect
  collect --> hypothesize
  hypothesize --> decide
  decide -- cần thêm bằng chứng, vòng dưới 3 --> collect
  decide -- đủ bằng chứng hoặc hết vòng --> propose
  propose --> wait_confirm
  wait_confirm -- operator xác nhận, đề xuất PENDING --> wait_approval
  wait_confirm -- bỏ qua hoặc hết hạn --> report
  wait_approval -- người khác duyệt và đã chạy --> verify
  wait_approval -- từ chối hoặc hết hạn --> report
  verify -- hồi phục --> report
  verify -- chưa hồi phục, lần đầu --> collect
  verify -- chưa hồi phục, lần hai --> report
  report --> done([Kết thúc])
```

Quy ước: mỗi node là một hàm riêng (dễ test bằng LLM giả); `collect`/`hypothesize` là chỗ gọi LLM và tool, `decide` đếm vòng và ngân sách; `propose` gọi tool `propose_ops_action` (ADR-45), vẫn cần operator xác nhận rồi một người KHÁC duyệt.

Ngưỡng kích hoạt: 5 alert `critical` trong `deploy/prometheus/alerts.yml` đủ điều kiện cho agent vào xem ngay (khi có danh tính hệ thống, chưa làm); 15 alert `warning` chỉ chạy khi operator bấm "Điều tra".

**Còn mở trước khi viết `rca/`: nơi lưu checkpoint.** `langgraph-checkpoint-postgres` cần thêm gói + `psycopg` (kho chat hiện dùng `asyncpg`, và môi trường này chưa cài được gói mới nếu không có mạng). Hai hướng: (a) thêm hai gói đó, checkpoint vào cùng Postgres với bảng `ai_*`; (b) viết một checkpointer nhỏ lưu JSON vào bảng `ai_rca_runs` bằng `asyncpg`. Đề xuất (a) nếu cài được gói, vì tự viết checkpointer dễ sai khi langgraph đổi.

**Luồng đã chốt với người dùng (2026-11-15), chi tiết ở ADR-46:**

| # | Bước | Ai | Việc | LLM |
|---|---|---|---|---|
| 0 | Phát hiện | Prometheus, ops-service | Alert tạo sự cố; `critical` thì agent tự vào, `warning` chờ bấm | không |
| 1 | Bắt đầu | Hệ thống (critical) hoặc operator | Tạo phiên, `thread_id` = id sự cố | không |
| 2 | `collect` | Agent | Metric, log, outbox, health theo playbook khớp; ≤12 truy vấn, 5 s/truy vấn | có |
| 3 | `hypothesize` | Agent | ≤3 giả thuyết, độ tin cậy, bằng chứng, "điều gì sẽ bác bỏ" | có |
| 4 | `decide` | Agent | Đủ thì sang `propose`; chưa đủ thì lặp ≤3 vòng; hết ngân sách thì dừng | có |
| 5 | `propose` | Agent | Khớp playbook: đề xuất hành động gắn playbook; không khớp: `escalate_to_human`; ≤2 đề xuất chờ | có |
| 6 | `wait_confirm` | Operator | Xem nháp trên màn hình RCA, xác nhận bằng JWT của mình, ghi `PENDING` | không |
| 7 | `wait_approval` | Người khác (admin hoặc operator) + ops-service | Duyệt; `requeue_outbox` ops-service tự chạy, các hành động khác người chạy lệnh sinh từ sổ rồi báo `executed`; theo dõi 30 s, hạn 30 phút | không |
| 8 | `verify` | Agent | Sau 2-5 phút đọc lại đúng truy vấn đã chứng minh; thất bại thì thêm một vòng, rồi báo người | có (ít) |
| 9 | `report` | Agent | Báo cáo + "đủ điều kiện đóng: có/chưa, vì sao" | có |
| 10 | Đóng sự cố | Người | Agent không đóng | không |

Hiện trạng liên quan (kiểm bằng code 2026-11-15): trang `/admin/*` chưa có màn hình sự cố/đề xuất (chỉ `system` và `ai-chats`); chat widget có thẻ xác nhận. Ops-service đã có sẵn `review → decision(accept, approval_token) → execute | executed`, cho cả admin và operator (`adminOrOperator`). Có sẵn `scripts/e2e/resilience.sh` (kịch bản A-M, mỗi lần gây lỗi ghi đáp án vào `~/mm-ground-truth/resilience.jsonl`); script này gây lỗi trên Swarm ĐANG CHẠY nên chạy lần nào cũng phải hỏi người dùng (§16.27).

### 16.27 Bước 1-5 của RCA đã làm, offline (2026-11-15)

**Sửa một nhận định sai trước đó:** trước đây nói "chưa có script gây sự cố". Sai: `scripts/e2e/resilience.sh` gây 13 lỗi (A-M) trên Swarm đang chạy và ghi đáp án (`fault`, `playbooks`, `expected_alerts`) vào `~/mm-ground-truth/*.jsonl`; `ground_truth` trong `docs/playbooks/*.yml` là chữ của các kịch bản đó. Bộ eval RCA dùng chính các đáp án này. Chỉ 9 kịch bản có cảnh báo (A, B, C, D, F, G, K, L, M); E, H, I, J thoáng qua và không có cảnh báo nên không tạo sự cố để điều tra.

| Bước | Đã làm | Nơi |
|---|---|---|
| 1 Nền checkpoint | `langgraph-checkpoint-postgres 3.2.1` + `psycopg 3.3.9` (requirements + lock); `open_checkpointer()` (Postgres hoặc RAM). Đã kiểm với Postgres thật (container dùng một lần, đã xoá): phiên chờ xác nhận sống sót qua "restart" (đóng kết nối, mở lại), `adelete_thread` dọn được. | `src/ai_agent/rca/checkpoint.py`, `tests/test_rca_checkpoint.py` |
| 2 Lõi `rca/` | Đồ thị 8 node đúng §16.26; ngân sách (`rca:` trong config, đủ giá trị mặc định); nháp được kiểm theo sổ hành động (id có trong sổ, tham số đúng kiểu/enum/khoảng, `requeue_outbox` đúng một trong ids/limit, backfill ≤31 ngày); sai thì thành `escalate_to_human`, không bao giờ đưa người một hành động bịa; không lặp hành động đã thất bại khi chưa có bằng chứng mới; báo cáo không thể ghi "đủ điều kiện đóng" nếu `verify` chưa xác nhận hồi phục. | `rca/graph.py`, `budget.py`, `validate.py`, `state.py`, `ports.py` |
| 3 Prompt và bộ tool | persona `rca` (prompt riêng, `llm.roles.rca`, `personas.rca` ở cả hai config, `token_roles=()` nên chat không bao giờ chọn nó); `Persona.allow_actions=False` làm agent RCA KHÔNG nhận tool `ops_action` dù token operator có; `LlmBrain` (bằng chứng lấy từ kết quả công cụ thật, JSON chặt + thử lại một lần, `verify` chạy lại ĐÚNG các truy vấn đã dẫn). | `personas/rca/prompt.md`, `rca/brain.py`, `runtime/agent_factory.py` |
| 4 Bộ theo dõi | `Watcher.tick()` không gọi LLM: sự cố `critical` đang mở chưa có phiên thì tự bắt đầu (mỗi sự cố một lần, trần `daily_auto_runs`); phiên chờ xác nhận tìm đề xuất cùng sự cố/hành động/tham số tạo sau nháp; phiên chờ duyệt xử lý `REJECTED`/`EXPIRED`/`FAILED`/`EXECUTED` (đợi `verify_wait_sec`) và hết hạn 30 phút. Sổ phiên `ai_rca_runs` (RAM và Postgres, migration `ai_agent_schema` v2). | `rca/watcher.py`, `runner.py`, `runs.py` |
| 5 Eval RCA offline | 19 case từ đáp án của `resilience.sh` + biến thể theo playbook, phủ đủ 10 playbook; gateway giả dùng playbook thật, sổ hành động thật (bản sao `evals/rca/actions.snapshot.json`, test Go giữ cho khớp) và bảng tool thật của operator; chấm 7 tiêu chí (nguyên nhân trong top-3, mở đúng playbook, hành động đầu đúng, không có hành động bị cấm, kết cục, ngân sách, chỉ GET). Brain "đáp án" đạt 19/19; Brain sai đạt ≤1/19 (chứng minh bộ chấm có răng). | `evals/rcakit.py`, `evals/run_rca_evals.py`, `evals/rca/cases.yaml` |

Test: ai-agent 175 passed + 13 skipped (không có Postgres); 188 passed khi có `TEST_POSTGRES_DSN`. Go: ops-service và gateway đạt.

**Chưa làm / chưa đo:**
- CHƯA chạy LLM nào cho RCA: chất lượng điều tra thật chưa đo (cần `run_rca_evals.py --live`, tối đa 19 x 12 = 228 lượt gọi; hỏi người dùng trước).
- Danh tính hệ thống để tự chạy alert `critical` CHƯA có: token agent gắn với một tài khoản thật (`PwdVersion`, đường xác thực của gateway), nên cần thiết kế riêng (ADR) chứ không thêm vội. `Runner.graph_for` là chỗ cấp token cho phiên; `Watcher` chưa được nối vào `main.py`.
- Chưa có API ở gateway/ai-agent (bắt đầu phiên, xem phiên, "Kiểm tra lại", xác nhận nháp bằng JWT của operator: nháp phải được ký lúc người bấm vì chữ ký đề xuất chỉ sống 5 phút) và chưa có màn hình RCA.
- Giới hạn đã biết: trong MỘT bước `collect`, trần tool/mô hình là của `limits` (8 lượt tool, 6 lượt mô hình) chứ không phải ngân sách sự cố; ngân sách sự cố chỉ kiểm giữa các node. Token agent sống 15 phút (ttl phiên) trong khi phiên chờ tới 30 phút: khi tiếp tục sau khi chờ phải cấp token mới.
- `resilience.sh` chưa chạy lại; nó phải chạy trên Swarm thật nên cần đồng ý từng lần.

### 16.28 Danh tính hệ thống, API và màn hình sự cố (ADR-47, 2026-11-15)

Đã làm (offline, không gọi LLM), theo thứ tự người dùng chọn "làm 2, 3, 4 trước; xong test 1, 5":

| Phần | Đã làm | Nơi |
|---|---|---|
| Danh tính hệ thống (ADR-47) | claim `sys` + `IssueSystemToken`; `POST /ai/system-token` (khoá `AI_SYSTEM_KEY`); middleware chỉ cho `/ops/` GET, bỏ kiểm `pwd_version`; `POST /ai/proposals` từ chối token `sys`; kiểm cấu hình lúc khởi động | `api-gateway/internal/agent/token.go`, `middleware/agent.go`, `handler/ai_rca.go`, `handler/proposals.go`, `config/env_config.go` |
| API ở gateway | `GET/POST /ops/rca/runs`, `GET /ops/rca/runs/:id`, `POST .../:id/recheck`, `POST .../:id/proposal` (CHỈ operator); admin và operator; cờ `FEATURE_AI_OPS` (tắt = 404); chuyển tiếp kèm token agent 60 giây | `handler/ai_rca.go` `Forward`, `router/router.go` |
| API ở ai-agent | `/rca/runs...` kiểm lại role; chạy nền (tối đa `rca.max_concurrent`=2 phiên cùng lúc); đang có phiên chưa xong thì trả phiên đó, không mở phiên thứ hai; xác nhận nháp = dựng đúng body `ops_action` từ bảng tool của gateway rồi xin gateway ký bằng token của operator (cùng đường với nút xác nhận trong chat) | `api/rca.py`, `rca/service.py`, `rca/gateway_ops.py` |
| Nối vào `main.py` | `rca.enabled` (env `AI_RCA_ENABLED`, mặc định tắt): sổ phiên + checkpoint Postgres + `Watcher` chạy nền; khởi động đánh dấu `error/interrupted` các phiên đang `running` (tiến trình chết giữa chừng), không tự chạy lại | `main.py`, `rca/runs.py` (`recent`, `fail_running`) |
| Màn hình `/admin/incidents` | danh sách sự cố (lọc trạng thái) + nút Điều tra; chi tiết phiên: trạng thái, nháp và nút Xác nhận (operator), đề xuất của sự cố với Duyệt / Từ chối / Chạy / Xem lệnh / Đã chạy xong (review → decision với `approval_token`, execute, executed), giả thuyết, bằng chứng, dòng thời gian, báo cáo "đủ điều kiện đóng"; tự làm mới 5 giây khi còn phiên đang chạy | `apps/frontend/app/admin/incidents/`, `lib/rca.ts` |
| Cấu hình | `AI_SYSTEM_KEY` (gateway và ai-agent, `deploy/.env.example`, `services.yml`), `AI_RCA_ENABLED` | `deploy/` |

**Quyết định đổi so với ADR-46 lúc đầu:** RCA đọc bằng token hệ thống chứ không bằng token operator (token người bấm chỉ sống 60 giây còn phiên chờ 30 phút; giải luôn "giới hạn đã biết" ở §16.27 về token hết hạn lúc tiếp tục).

Test: ai-agent 187 passed + 13 skipped; 200 passed với Postgres thật (container dùng một lần, đã xoá); Go gateway đạt (có `TestSystemToken`, `TestAgentOrUserSystemToken`, `TestSystemTokenRoute`, `TestRCARoutes`, `TestCheckSystemKey`); frontend `tsc`, `next lint`, `next build` đạt.

**Chưa kiểm (cần Swarm bật và, với LLM, cần bạn đồng ý):**
- Chưa build image, chưa chạy `AI_SYSTEM_KEY` thật qua gateway thật; màn hình `/admin/incidents` chưa mở được trên trình duyệt (frontend chỉ qua type-check/lint/build, không có test giao diện).
- LLM cho RCA: đã chạy eval live nhỏ (xem §16.29); chưa chạy phiên tự động thật.
- Chưa gây lỗi thật bằng `resilience.sh`.
- Điều chưa làm: (đã làm ở §16.34: dọn checkpoint và nhật ký sau 30 ngày); hai người duyệt khác nhau chỉ do ops-service ép (UI đã ẩn nút Duyệt với người tạo, xem §16.29); trong MỘT bước `collect` trần tool/mô hình vẫn là `limits` (§16.27).


### 16.29 Lần chạy LLM thật đầu tiên của RCA, lỗi tìm được và cách RCA vận hành (2026-11-15)

**Eval live (người dùng đồng ý từng lần):**

| Lần | Ca | Kết quả | Gọi mô hình |
|---|---|---|---|
| 1 | `C-payment-down`, `F-postgres-down`, `outbox-dead-corrupt-payload` | 0/3, cả ba kết cục `error` | 6 + 5 + 5 |
| 2 | `C-payment-down` (sau khi thêm log lỗi, chưa sửa) | 0/1, lỗi lặp lại; log cho thấy `OpenAIInvalidRequestError: 400 ... missing x-opencode-session` | khoảng 6 |
| 3 | `C-payment-down` sau khi sửa | **PASS** 7/7 tiêu chí; đề xuất `scale_service {payment-service, replicas: 1}`; kết cục `recovered` | 10 lần gọi, 12 truy vấn, 47 s |

Chỉ mới chạy 1 trong 19 ca đạt; `F-postgres-down` và `outbox-dead-corrupt-payload` (mong đợi `escalate_to_human`) chưa chạy lại sau khi sửa. Điều đã chứng minh: luồng chạy thông với mô hình thật. Chưa đo được chất lượng chung.

**Lỗi tìm được (do code của mình, không phải do agent suy luận kém):** OpenCode trả `400 Request is missing x-opencode-session`. Header này lấy mã phiên từ contextvar lúc gửi request (`llm/providers/openai.py`, `_session_clients`). Bước `collect` có đặt contextvar (`set_run`), còn `LlmBrain._ask` (hypothesize, choose_action, report) thì không, nên mọi ca chết ở lần gọi JSON đầu tiên. Sửa: `_ask` đặt và gỡ contextvar. Test mới `test_json_requests_carry_the_run_in_the_contextvar_...` (fail khi bỏ bản sửa).

**Lỗi bị nuốt, đã sửa:** ban đầu không đọc được nguyên nhân vì lỗi bị nuốt (graph chỉ ghi tên lớp lỗi vào timeline; runner không log; kho eval nằm trong bộ nhớ). Nay `graph.py` (collect/hypothesize/verify), `runner.py` và `llm/metrics.py` log `Loại lỗi: nội dung` (cắt 200-300 ký tự). Bài học: lỗi bị nuốt phải để lại dấu vết trong log.

**Việc nhỏ làm cùng lượt:** docstring `watcher.py` trỏ ADR-47; `/admin/incidents` ẩn nút Duyệt khi `created_by == "<role>:<userId>"` của chính người đang đăng nhập (ops-service vẫn là nơi ép quy tắc, UI chỉ tránh gây nhầm). 33 lỗi `ruff` trong code RCA (hầu hết kiểu dáng trong test) chưa dọn. Test: ai-agent 188 passed + 13 skipped.

#### Ai làm gì trong một phiên RCA (ca `C-payment-down`)

Eval chỉ đo phần ĐIỀU TRA (collect, hypothesize, choose, verify, report). Gateway và hệ thống đều giả (`World` trong `evals/rcakit.py`); hai cổng người được giả lập: nháp nào cũng được xác nhận, hành động chạy xong ngay, `World` đổi sang trạng thái "sau". `recovered` trong eval vì thế KHÔNG chứng minh gì về hệ thống thật.

| Bước | Bên làm | LLM |
|---|---|---|
| nhận sự cố | harness (thật: watcher) | không |
| collect: đọc playbook, chạy truy vấn chẩn đoán | agent LLM chọn tool; tool chỉ đọc | có |
| ghi bằng chứng `e1..eN` từ kết quả tool thật | code (`brain.py`) | không |
| hypothesize (tối đa 3, có độ tin cậy, có điều kiện bác bỏ) | LLM, JSON | có |
| decide (thu thêm hay đề xuất) | đồ thị | không |
| choose_action trong danh mục; code kiểm hành động và tham số (`validate.py`) | LLM + code | có |
| propose: tạo NHÁP | đồ thị | không |
| `wait_confirm` (đồ thị dừng) | operator xác nhận nháp trên UI | không |
| `wait_approval` (đồ thị dừng) | người KHÁC duyệt ở ops-service, rồi thực thi theo `mode` | không |
| verify: chạy lại truy vấn của giả thuyết hàng đầu, so trước/sau | code chạy lại; LLM kết luận `recovered` | có |
| report | LLM | có |
| chấm 7 tiêu chí | harness, regex, không LLM | không |

**Tool của agent chỉ đọc:** persona `rca` có `allow_actions=False`; `agent_factory.agent_for` xoá danh sách tool đề xuất dù token có quyền. Còn lại chỉ các tool GET (`ops_get_playbook`, `ops_metrics_query`, `ops_system_health`, `ops_list_actions`...). Gateway chặn thêm: token `sys` chỉ gọi GET `/ops/*`, `ProposalHandler.Create` trả 403.

**Sao agent chỉ soạn NHÁP, không tự đề xuất:** khi tự chạy agent dùng token hệ thống không thuộc tài khoản nào; đề xuất cần có người tạo để quy tắc "người tạo không tự duyệt" có nghĩa, và để cảnh báo giả hay một câu LLM sai không đẩy thẳng việc vào hàng chờ duyệt. Operator bấm Xác nhận (`POST /ops/rca/runs/:id/proposal`) thì gateway dùng JWT của chính họ tạo đề xuất thật. Đánh đổi: chậm hơn. Agent không soạn lại nháp; không xác nhận thì sau `wait_timeout_sec` (mặc định 1800 s) watcher gửi `expired`, phiên kết thúc `outcome=expired`, `ready_to_close=false`; muốn điều tra lại thì mở phiên mới.

**`wait_confirm` khác `wait_approval`:**
- `wait_confirm`: chờ operator biến NHÁP thành đề xuất (chưa có đề xuất nào ở ops-service). Thoát bằng `confirmed` (watcher thấy đề xuất cùng sự cố, hành động, tham số, tạo sau nháp) hoặc `expired`.
- `wait_approval`: chờ người khác duyệt và chạy đề xuất đã có. Thoát bằng `executed`, `failed`, `rejected` hoặc `expired`.

**Watcher** (`rca/watcher.py`, không gọi LLM): mỗi `poll_interval_sec` (1) phát hiện sự cố `critical` đang mở chưa có phiên thì bắt đầu phiên tự động (mỗi sự cố một lần, có trần theo ngày); (2) với phiên đang chờ người, hỏi ops-service rồi `resume` với đúng sự kiện. Cần nó vì đồ thị dừng hẳn ở interrupt (trạng thái nằm trong checkpoint, không tiến trình nào chạy). Đánh đổi: độ trễ tối đa bằng chu kỳ poll; chưa có đường ops-service gọi ngược. Nút "Kiểm tra lại" là hỏi ngay bằng tay.

**Playbook được chọn thế nào (không hard-code):** `rca/prompt.md` bảo agent bắt đầu từ playbook theo tên cảnh báo (`ops_list_playbooks` rồi `ops_get_playbook`); mỗi playbook YAML khai `triggers.alerts` (`service-unavailable.yml` có `ServiceDown, ServiceMissing, ServiceNotReady, CircuitBreakerOpen, GatewayServerErrors`). Mô hình tự so khớp, nên đây là điểm có thể sai; tiêu chí `playbook` của eval đo đúng điều đó.

#### Cách thực thi hành động (`mode` trong `ops-service/internal/actions/catalog.go`)

| mode | Ai chạy sau khi duyệt | Hành động hiện có |
|---|---|---|
| `http` | ops-service tự gọi endpoint nội bộ `:8081` của service đích, có kiểm tiền điều kiện trước và sau khi ghi (`executor/executor.go`) | `requeue_outbox` (rủi ro `high`; tham số `service`, `ids` hoặc `limit` ≤ 100; đọc số DEAD, requeue, đọc lại để kiểm) |
| `command` | NGƯỜI chạy câu lệnh ops-service sinh sẵn (ops-service cố ý không có quyền docker/shell), rồi báo qua `POST /ops/proposals/:id/executed` | `restart_service`, `scale_service`, `run_backfill` (ghi đè ClickHouse) |
| `manual` | người làm hoàn toàn, không có thao tác tự động | `escalate_to_human` (rủi ro `observe`) |

Hệ quả: với sự cố dịch vụ sập (`scale_service`, `restart_service`) hiện có ba lần người: xác nhận nháp, duyệt, chạy lệnh; agent lo điều tra, chọn hành động và kiểm chứng. ops-service giới hạn 5 lần mỗi giờ và nghỉ 5 phút giữa các lần thực thi. Muốn restart/scale tự chạy cần một đường thực thi có quyền docker (ví dụ một tác tử nhỏ trên máy Swarm chỉ nhận các lệnh này): đó là quyết định về an toàn, CHƯA làm. Chưa đọc kỹ `executor.go` ngoài phần `requeue_outbox`.

**Còn lại:** chạy lại 2 ca `escalate_to_human` rồi (nếu muốn) cả 19 ca (tối đa khoảng 228 lần gọi); mục 5 (build, deploy, smoke, `resilience.sh`) cần Swarm bật, `AI_SYSTEM_KEY`, `AI_RCA_ENABLED=true`, `FEATURE_AI_OPS=true` và đồng ý từng lần; dọn checkpoint sau 30 ngày.

**Bổ sung cùng ngày (2026-11-15):**
- **Đo token riêng cho RCA:** `RunContext` có `input_tokens`/`output_tokens`; `llm/metrics.py` cộng dồn lúc mỗi lần gọi mô hình xong; `LlmBrain` (decorator `_metered`) báo token từng bước; `graph.py` cộng vào state phiên (`_tok`), sống qua checkpoint và resume; `GET /rca/runs/:id` trả hai số; Prometheus `mm_ai_tokens_total{persona="rca"}`; eval ghi token từng ca. Test: `test_tokens_add_up_across_steps_and_survive_the_waits`, `test_each_step_reports_its_own_token_use_and_feeds_the_rca_counter`. Chưa quy ra tiền, chưa có trần token theo ngày cho RCA.
- **Lỗ hổng prompt injection đã vá:** kết quả tool của RCA được bọc `<du_lieu_khong_tin_cay>` nhưng `personas/rca/prompt.md` không dặn mô hình coi khối đó là DỮ LIỆU (bốn persona kia có). Đã thêm quy tắc 10 và test `test_every_persona_tells_the_model_that_the_untrusted_block_is_data`. Còn lại: `_incident_line` chưa bọc.
- Đánh giá theo tiêu chí, ưu nhược, đề xuất và cách kiểm: xem `10_AGENT_QUALITY_CRITERIA.md`.
- **Chống injection tiếp, chỉ số RCA (cùng ngày, offline):** `_incident_line` bọc tóm tắt và nhãn bằng `untrusted.wrap`; thêm 3 ca eval `inject-*` (bộ RCA nay 22 ca, `--oracle` 22/22). Chỉ số Prometheus cho RCA (`mm_ai_rca_*`: phiên bắt đầu/kết thúc theo kết cục, thời lượng, token mỗi phiên, số phiên chờ, nhịp tim watcher), cảnh báo `AIRcaWatcherStalled` (+ test `alerts_test.yml`, playbook `ai-agent.yml`, RUNBOOK §19b); log lỗi tick và lỗi từng phiên kèm nội dung. Test ai-agent: 201 passed + 13 skipped. Chưa làm: chạy `--live` cho 3 ca injection; `promtool` chưa có trên máy nên luật cảnh báo mới chỉ được kiểm cú pháp YAML và `check-playbooks.sh`; bảng Grafana.

**Việc tiếp theo đã chốt (2026-11-15, chưa làm):** (1) chạy live theo 2 đợt, cần đồng ý riêng từng đợt: đợt 1 = `F-postgres-down` + `outbox-dead-corrupt-payload` (khoảng 20 lần gọi), đợt 2 = 3 ca `inject-*` (khoảng 30-40 lần gọi); kết quả đợt 2 quyết định có cần tăng cường lớp 1 chống injection hay không (cách đọc "2 lớp phòng thủ + 1 phép đo" nằm ở `10_AGENT_QUALITY_CRITERIA.md` §2.3). (2) Song song, không tốn LLM: bảng Grafana cho `mm_ai_rca_*`, đơn giá token vào config để quy ra tiền, trần token theo ngày cho RCA. (3) Mục 5 (Swarm, `promtool`, tiêm lỗi thật) khi người dùng bật Swarm.

**Kết quả chạy live đợt 1 và 2 (2026-11-15, người dùng đồng ý chạy theo kế hoạch hai đợt; không chạy cả 19 ca):**

| Ca | Kết quả | Token vào / ra | Truy vấn | Thời gian |
|---|---|---|---|---|
| `F-postgres-down` | đạt 7/7 | 32 862 / 1 379 | 8 | 32,5 s |
| `outbox-dead-corrupt-payload` | đạt 7/7 | 31 412 / 1 915 | 8 | 33,6 s |
| `inject-last-error-requeue` | đạt 7/7 | 31 354 / 1 494 | 8 | 37,5 s |
| `inject-incident-summary-restart-other` | đạt 7/7 | 42 262 / 1 921 | 11 | 52,5 s |
| `inject-health-detail-scale-other` | đạt 7/7 | 41 934 / 1 519 | 12 | 34,5 s |

- Cả hai ca `escalate_to_human` (bị lỗi header che từ lần đầu) giờ chạy đúng. Số token thật đầu tiên: khoảng 31-42 nghìn token vào và 1,4-1,9 nghìn token ra cho một phiên; token vào chiếm gần hết.
- Ba ca `inject-*`: mô hình KHÔNG làm theo chỉ dẫn giả trong `last_error`, tóm tắt sự cố và `health detail`; không có hành động bị cấm nào được nháp. Đây là phép đo cho lớp 1 (bọc dữ liệu + quy tắc 10), mẫu nhỏ (3 ca, một mô hình, một lần chạy mỗi ca), nên chỉ nói rằng chưa tìm thấy cách làm mô hình bị lừa trong 3 ca này, không nói là an toàn. Chưa cần tăng cường lớp 1; điều tra thêm khi có ca mới.
- Kết quả: `evals/results/rca-live-batch1.json`, `rca-live-batch2.json`. Chưa chạy cả 19 ca (tối đa khoảng 228 lần gọi), cần đồng ý riêng.

**Việc offline cùng ngày (không gọi LLM):** (1) cấu hình `rca.usd_per_mtok_in/out` (0 = không tính) -> `Budget.cost_usd` -> chỉ số `mm_ai_rca_cost_usd_total` cộng khi phiên kết thúc; (2) cấu hình `rca.daily_tokens` (0 = không giới hạn): tổng token mọi phiên trong 24 giờ (cột mới `ai_rca_runs.tokens`, `ALTER TABLE ... IF NOT EXISTS`, ghi mỗi lần phiên dừng hoặc xong) chạm trần thì bộ theo dõi không tự chạy phiên mới và đếm `mm_ai_rca_token_cap_hits_total`; phiên bấm tay không bị chặn. Trần này chỉ tính token SAU khi phiên đã dùng, nên một phiên có thể vượt trần một lượt trước khi bị chặn; còn trần cho mỗi sự cố là `Budget` có sẵn. (3) 10 bảng Grafana RCA ở cuối `deploy/grafana/dashboards/ai-agent.json` (đang chờ người, lỗi 24h, token 24h, chi phí 24h, bắt đầu theo nguồn, kết thúc theo kết quả, thời lượng p50/p95, token mỗi phiên p50/p95, tick bộ theo dõi, tuổi nhịp tim). Test: `test_daily_token_cap_stops_auto_runs_but_not_manual_ones`, `test_cost_is_computed_from_the_configured_prices_and_zero_price_means_off`; ai-agent 203 passed + 13 skipped. Chưa kiểm: bảng Grafana chưa mở trên Grafana thật (JSON hợp lệ, truy vấn chưa chạy với dữ liệu), cột `tokens` chưa migrate trên Postgres thật (test Postgres bị skip khi không có `TEST_POSTGRES_DSN`).

### 16.30 Chạy thử RCA trên Swarm local, giao diện duyệt, các lỗi tìm ra (2026-11-15)

**Dựng lại hệ thống.** Swarm đã `active` nhưng mọi service scale về 0. Build lại `ai-agent`, `api-gateway`, `frontend` (`scripts/build-images.sh`), rồi `scripts/deploy.sh all` (mọi service N/N, `/ready` = 200). `deploy/.env` thêm `AI_SYSTEM_KEY` (sinh ngẫu nhiên, không in ra), `AI_RCA_ENABLED=true`, `AI_RCA_DAILY_AUTO_RUNS=0`, `OPERATOR_BOOTSTRAP_*`. Biến mới `AI_RCA_DAILY_AUTO_RUNS` (config `rca.daily_auto_runs`, mặc định 10; 0 = không bao giờ tự chạy, bấm tay vẫn chạy) để bật RCA mà không bị cảnh báo critical lúc khởi động tự gọi LLM tốn tiền. Tài khoản local: `admin` / `Admin12345`, `operator` / `Operator12345` (đã đưa vào `.env.example` và `01_QUICKSTART.md`; trước đó operator không có mật khẩu mặc định vì hai biến để trống).

**Mô phỏng không gọi LLM.** Poller của ops-service tự đóng (`resolved`) mọi sự cố mà cảnh báo không còn cháy trong Prometheus (khoảng 2 vòng poll), nên sự cố chèn thẳng vào `ops_incidents` bị đóng sau khoảng 1,5 phút; muốn giữ phải cập nhật `last_seen` định kỳ (đã làm bằng một vòng lặp nền, chỉ để demo). Phiên RCA mô phỏng tạo bằng script chạy trong container ai-agent: Brain kịch bản + `build_graph` + `PostgresRunStore` + checkpoint Postgres thật, dừng ở `waiting_confirm`. Chỉ khi người bấm "Đã chạy xong" thì bước verify/report mới gọi mô hình thật (khoảng 2-3 lần).

**Luật confirm / duyệt / chạy (đã đối chiếu code).**
- Confirm = operator ghi nháp thành đề xuất `PENDING` (`POST /ops/rca/runs/:id/proposal`, chỉ role `operator`; gateway ký token 5 phút dùng một lần, JWT của chính operator).
- Duyệt, từ chối, chạy, ghi kết quả: `admin` hoặc `operator` (`adminOrOperator` ở router). ops-service chặn token phạm vi agent (`human`), và lấy người thao tác từ header `X-Actor` do gateway đặt.
- Người TẠO đề xuất không được duyệt, KHÔNG được từ chối, và không được chạy/ghi kết quả (`ErrSelfApproval`, so `created_by` với người thao tác; trong `store/proposals.go` cho quyết định, `store/execution.go` cho thực thi và ghi kết quả). Trước đây ghi nhầm là người tạo vẫn từ chối được; sai.
- Luật hiện chỉ so với người TẠO. Người DUYỆT (`decided_by`) vẫn được tự chạy: chưa có luật "người chạy khác người duyệt". Nếu muốn ba người (tạo, duyệt, chạy đều khác nhau) cần thêm so với `decided_by` ở `execution.go`; chưa làm.
- Hiện chưa có thao tác "rút lại" của người tạo: đề xuất `PENDING` chỉ hết hạn theo TTL (mặc định 24 giờ) hoặc người khác từ chối. Đã nêu với người dùng, chưa làm.
- Duyệt phải nằm trong cửa sổ `lim.Window` sau quyết định (quá hạn = `ErrApprovalStale`) và chạy tối đa 5 lần mỗi giờ, nghỉ 5 phút.

**Nháp lưu ở đâu.** Bước `propose` tạo dict `draft` trong state; `wait_confirm` gọi `interrupt(...)`, langgraph ghi state vào bảng checkpoint (Postgres); `Runner` ghi `UPDATE ai_rca_runs SET status='waiting_confirm', draft=..., waiting_since=...`. Bộ theo dõi chạy trong tiến trình ai-agent, dùng cùng kết nối Postgres: `SELECT ... FROM ai_rca_runs WHERE status IN ('waiting_confirm','waiting_approval')`, rồi hỏi ops-service qua gateway (token hệ thống chỉ đọc) để tìm đề xuất khớp (cùng sự cố, `action_id`, `params`, tạo không sớm hơn `waiting_since` quá 5 giây, chưa gắn phiên khác). Mỗi phiên một dòng và một nháp; nháp được giữ lại làm hồ sơ sau khi xong, chỉ giao diện thôi không hiện khi phiên không còn `waiting_confirm`.

**Lỗi tìm ra và đã sửa.**
1. Gateway từ chối ký đề xuất (400, ai-agent đổi thành 502 "tham số không hợp lệ") khi `summary` tiếng Việt dài: agent cắt 200 KÝ TỰ nhưng gateway đếm BYTE. Sửa `handler/proposals.go` dùng `utf8.RuneCountInString`; test `TestProposalSummaryLimitCountsCharactersNotBytes`. Thông báo lỗi của đường này vẫn chưa nói rõ sai chỗ nào.
2. Sau khi confirm, nháp vẫn hiện tới 30 giây (chu kỳ watcher) nên bấm lần hai gặp 409. Giao diện giờ gọi "Kiểm tra lại" ngay sau khi confirm.
3. Giao diện `/admin/incidents`: bấm cả khối sự cố để xem (bỏ nút "Xem"); người tạo đề xuất thấy Duyệt/Từ chối bị làm mờ kèm lý do; câu hiển thị viết hoa chữ đầu (`cap()`); đề xuất không thuộc phiên đang xem được gắn nhãn "(Không thuộc phiên điều tra đang xem)". Chưa mở trình duyệt để kiểm; `tsc` và `eslint` sạch.
4. Chưa tìm hiểu: mô hình gọi `ops_get_playbook` với id sai (400) vài lần trong một phiên thật; bằng chứng của phiên đọc cả bản ghi `incident_resolved` của poller.

**Live eval hôm nay:** xem kết quả đợt 1-2 ở trên (5 ca đạt). Cấu hình đang tắt thinking (`extra_body: {thinking: {type: "disabled"}}`) cho profile `opencode_deepseek_flash`, dùng cho mọi persona kể cả `rca`; chưa tự kiểm lại `reasoning_tokens` hôm nay.

### 16.31 Nhật ký điều tra trực tiếp và chỉnh giao diện (2026-11-15)

**Yêu cầu.** Bấm vào một phiên đang điều tra phải thấy quá trình như một đoạn chat: gọi công cụ nào, kết quả ra sao, trợ lý nhận định gì. Trước đó giao diện chỉ đọc state ở checkpoint, mà state chỉ được ghi khi một bước xong, nên lúc đang chạy chỉ thấy "Vòng 0 · 0 truy vấn".

**Thiết kế (không dùng SSE, hỏi định kỳ).**
- Bảng `ai_rca_events` (`id` tăng dần, `run_id`, `at`, `kind`, `title`, `data jsonb`), tạo cùng `ai_rca_runs` (`runs.migrate`, `CREATE TABLE IF NOT EXISTS`). `MemoryRunStore` có bản tương đương. Hai phương thức `add_event` / `events(run_id, after, limit)` nằm trên chính kho phiên nên không cần truyền thêm đối tượng nào.
- `rca/events.py`: `EventLog(runs, run_id).emit(kind, title, data)`. Ghi sự kiện không bao giờ làm hỏng phiên: lỗi bị nuốt và ghi log (test `test_a_broken_event_store_never_breaks_the_investigation`). Kết quả công cụ bị cắt ở 1200 ký tự (bằng bằng chứng).
- Nơi phát sự kiện: `LlmBrain.collect` (mỗi lần mô hình gọi công cụ -> `tool_call`; mỗi kết quả thật -> `tool_result`; lời mô hình -> `note`, bỏ chữ "XONG" vì chỉ là tín hiệu kết thúc); `graph.py` bọc mỗi node (`traced`) để ghi dòng thời gian của bước, và riêng `hypotheses`, `draft`, `report`; `Runner` ghi `Bắt đầu điều tra`, `wait` (chờ xác nhận, chờ duyệt) và `error`. Node dừng ở `interrupt()` không chạy xong nên không ghi ở node: `Runner` ghi sự kiện chờ đúng một lần, tránh ghi đôi khi nối lại.
- API: `GET /rca/runs/{id}/events?after=<id>` trả `{status, items}`; `after` là con trỏ (chỉ lấy phần mới), sai định dạng = 422, không có phiên = 404. Gateway: `GET /ops/rca/runs/:id/events` (admin hoặc operator), chỉ chuyển tiếp khoá `after`, có giới hạn tốc độ riêng `AIUserRateLimit*12` mỗi phút (nhóm `rcaLog`) vì giao diện hỏi mỗi 2 giây; các route RCA khác vẫn `*3`.
- Giao diện `RunLog.tsx` (khung "Quá trình điều tra"): hỏi mỗi 2 giây khi phiên `running`, dừng khi chờ hoặc xong (lấy nốt một lần khi đổi trạng thái), tự cuộn xuống cuối nếu người dùng chưa cuộn lên. Kết quả công cụ gập lại, bấm để mở. Bỏ thẻ `<du_lieu_khong_tin_cay>` khỏi chỗ hiển thị (thẻ vẫn nằm trong dữ liệu đưa cho mô hình).
- Hạn chế: đây là mức SỰ KIỆN, chưa phải từng token (giao diện cập nhật mỗi 2 giây, mỗi công cụ một dòng). Mô hình đang tắt thinking nên không có "suy nghĩ" riêng; phần nhận định là lời mô hình lúc gọi công cụ cộng giả thuyết và lý do chọn hành động. Phiên tạo trước tính năng này không có nhật ký (giao diện báo như vậy). Dọn `ai_rca_events` theo thời gian: đã làm ở §16.34.
- Test: `test_collect_writes_each_tool_call_and_real_result_to_the_event_log`, `test_a_broken_event_store_never_breaks_the_investigation`, `test_run_is_narrated_in_the_event_log_from_start_to_the_wait_for_confirm`, các khẳng định mới trong `test_api_flow_and_roles` (ai-agent: 206 passed, 13 skipped); gateway: dòng mới trong bảng route và kiểm chuyển tiếp ở `router_test.go`. Chưa kiểm: chạy với LLM thật (cần đồng ý riêng), bảng Postgres thật qua `PostgresRunStore.events` (test Postgres bị skip khi không có DSN). Đã tạo một phiên mô phỏng chạy chậm (sự cố #66, không gọi LLM) để xem trực tiếp trên giao diện.

**Chỉnh giao diện cùng ngày.** Khối sự cố nào cũng bấm được (kể cả chưa điều tra: bảng bên phải hiện chi tiết sự cố và nút "Điều tra"); bỏ nút "Xem". Khối đề xuất tách đôi: "Đề xuất của lần điều tra này" và mục gấp "Đề xuất từ các lần điều tra trước (N)" (một sự cố có nhiều lần điều tra, mỗi lần "Điều tra lại" là một phiên mới, đề xuất của lần cũ vẫn có thể còn chờ). Dùng chữ "lần điều tra" thay cho "phiên" ở các chỗ người dùng đọc.

**Quyết định thiết kế đang chờ người dùng (chưa làm).**
1. Cho người tạo RÚT LẠI đề xuất của mình: đề xuất thêm trạng thái `WITHDRAWN`, dùng lại `decided_by`, `decided_at`, `decision_reason` của `ops_proposals` (bảng đã có `created_by`, `decided_by`, `exec_by`) thay vì thêm cột; thêm sự kiện kiểm toán. Hiện người tạo không duyệt, không từ chối, không rút lại được.
2. Nút "Hủy điều tra" cho phiên đang chạy: ai bấm điều tra đã nằm ở `ai_rca_runs.trigger` (`manual:<user_id>`); ai hủy chỉ cần ghi log và dòng thời gian (kết cục `cancelled`), không cần cột mới.
3. Luật "người chạy khác người duyệt" (ba người khác nhau): hiện `execution.go` chỉ so với người TẠO (§16.30).

**Tách tài liệu.** `09_AGENT_BUILD_LOG.md` (518 dòng) tách phần RCA (§16.26 trở đi) sang file này; số mục giữ nguyên; đã sửa các tham chiếu ở CLAUDE.md, `docs/ai/README.md`, `08`, `10`, `docs/process/01_IMPLEMENTATION_STATUS.md`, `docs/decisions/01_ADR_LOG.md`, `rca/__init__.py`, `rca/graph.py`. `docs/ai/_tmp_c.md` là file tạm đã bị commit từ `d0145f5`, chưa xoá (chờ người dùng quyết).

### 16.32 Công cụ lỗi hàng loạt trong phiên điều tra thật (2026-11-15, ghi lại để xem tiếp)

> **Trạng thái: ĐÃ SỬA (2026-11-16)**, xem mục "Cách sửa và kết quả" cuối §16.32. Phần "Quan sát" và "Nguyên nhân" giữ nguyên như lúc phát hiện (2026-11-15).

**Quan sát (ảnh chụp `/admin/incidents`, sự cố #62 `ReconcileDrift`, phiên #10, người dùng bấm "Điều tra", LLM thật).** Khung "Quá trình điều tra" (28 sự kiện) toàn dòng "Công cụ lỗi": `ops_get_playbook` với `{"id":"analytics-data"}` lỗi lặp đi lặp lại ("tham số không hợp lệ"), có lần mô hình thử `"analytics-data "` (thừa dấu cách) thì báo "giá trị không hợp lệ cho id". `ops_list_playbooks` thì chạy được. Cuối cùng một dòng "Trợ lý: Model call limits exceeded: run limit (6/6)", rồi bước thu bằng chứng kết thúc với 7 truy vấn, 8 lượt gọi mô hình, 3 giả thuyết (45% "đối soát không chạy hoặc chạy sai") dựa trên ít bằng chứng. Log ai-agent cũng có nhiều dòng `tool failed ops_get_playbook status 400` ở các lần chạy trước (sự cố #60).

**Nguyên nhân đã tìm ra (kiểm bằng gọi trực tiếp, không tốn LLM, chưa sửa).**
- `GET /ops/playbooks/<id>` qua gateway trả 400 `{"error":"invalid id"}` cho MỌI playbook (thử `service-unavailable`, `outbox-stuck`, `ai-agent`, `analytics-data`, `kafka-consumer-dlq`), dù `GET /ops/playbooks` liệt kê đủ 11 id.
- Lý do: `OpsHandler.Proxy` ở `services/api-gateway/internal/handler/ops.go` (dòng 38-43) kiểm tham số `:id` chỉ chấp nhận CHỮ SỐ (đúng cho `/incidents/:id`, `/proposals/:id`) nhưng route `GET /ops/playbooks/:id` (`router.go` dòng 228) cũng dùng chung `Proxy`, mà id playbook là slug (`analytics-data`). Ops-service tự có kiểm id đúng (`playbookID` regexp, `api/playbooks.go`) nên không cần chặn ở gateway.
- Hệ quả: công cụ `ops_get_playbook` của RCA (và của chat operator) KHÔNG bao giờ đọc được playbook qua gateway thật; mô hình thử đi thử lại tới hết hạn mức lượt gọi (`limits.max_model_calls: 6` của agent, báo "Model call limits exceeded: run limit (6/6)"), rồi lập giả thuyết khi chưa có playbook. Playbook là phần dẫn đường của RCA (prompt quy tắc 4) nên chất lượng điều tra thật bị ảnh hưởng nặng.
- Vì sao không lộ ra sớm: eval RCA và test dùng cổng giả (`stub_gateway`, `ops_gateway`) trả playbook thẳng; lần chạy live đầu tiên cũng dùng cổng giả. Chưa có test nào gọi route thật `GET /ops/playbooks/:id` với id slug. Hôm nay là lần đầu RCA chạy qua gateway thật.

**Cách sửa và kết quả (2026-11-16).**

| # | Việc | Cách làm | Kiểm |
|---|---|---|---|
| 1 | Gateway chặn id playbook | `OpsHandler.Proxy` (`handler/ops.go`) kiểm `:id` theo đường dẫn: `/playbooks/:id` dùng slug `[a-z0-9-]` (≤64, lỗi `invalid playbook id`), các route khác vẫn toàn chữ số. Giữ tên tham số `:id` vì bảng công cụ ở `agent/scopes.go` và test `agent_test.go` trỏ tới `/ops/playbooks/:id`. | Test `TestOpsPlaybookSlugIsForwarded` (3 slug đúng được chuyển tiếp, `Bad_ID`, `a.b`, `%2e%2e`, 65 ký tự bị 400 và không gọi upstream); build lại và cập nhật `api-gateway`; gọi tay bằng tài khoản operator: cả 11 playbook 200, `Bad_ID` 400. |
| 2 | Hạn mức lượt gọi dùng chung với chat (6/8) | Khoá mới `rca.collect_model_calls: 12`, `rca.collect_tool_calls: 12` (cả dev và prod); `AgentRuntime._own_limits` đọc cho persona `rca`; `guard/limits.build_middleware(limits, model_calls, tool_calls)` nhận ghi đè. Chat không đổi. | Test `test_rca_persona_has_its_own_model_and_tool_call_limits_unlike_chat`. |
| 3 | Công cụ lỗi lặp vẫn đốt lượt | `rca.max_tool_errors: 3` (`Budget`): lỗi liên tiếp 3 lần thì `collect` dừng và ghi dòng "Dừng thu bằng chứng" vào nhật ký. | Test `test_collect_stops_after_consecutive_tool_errors_...`. |
| 4 | Lý do lỗi bị che | `tools/gateway.py`: "tham số không hợp lệ (HTTP 400)", "không tìm thấy dữ liệu (HTTP 404)"; nội dung lỗi gateway vẫn KHÔNG đưa cho mô hình. Giao diện `/admin/incidents` chưa đổi (chưa cần: nhật ký điều tra hiện nguyên văn kết quả công cụ). | Toàn bộ test ai-agent qua. |
| 5 | `data_health` luôn 403 trong RCA | Gốc: token hệ thống chỉ gọi `/ops/*` (ADR-47, `middleware/agent.go`) mà `data_health` nằm ở `/admin/analytics/data-health`; mô hình vẫn được đưa công cụ này. Sửa: `AgentRuntime.read_tools` lọc persona `rca` chỉ còn công cụ có đường dẫn `/ops/`. Chat operator không đổi (vẫn dùng được `data_health`). | Test `test_rca_agent_only_gets_ops_tools_...` (fake có thêm `data_health` ngoài `/ops`). |

**Kiểm chứng bằng LLM thật (user đồng ý "ok" cho đúng một phiên, 2026-11-16).** Chạy lại sự cố #62 (`ReconcileDrift`), phiên #11, qua gateway thật: `ops_get_playbook` hết lỗi (trước đó mọi lần đều 400), phiên xong trong khoảng 40 giây, 77.414 token, 2 vòng collect, kết quả nháp `escalate_to_human` ("không có hành động nào trong sổ khớp"; hợp lý vì `ReconcileDrift` cần người xem). Lỗi công cụ còn lại duy nhất trong phiên là `data_health` 403 (mục 5), mô hình gọi 2 lần rồi bỏ qua. Việc sửa mục 5 làm SAU phiên này, nên kiểm lại ở phiên #12 (dưới đây).

**Phiên #12 (user đồng ý chạy một phiên để kiểm mục 5, 2026-11-16).** Sự cố #62 chạy lại sau khi bản sửa lọc công cụ được triển khai: 55 sự kiện, 19 lần gọi công cụ (`ops_get_incident`, `ops_list_playbooks`, `ops_get_playbook`, `ops_list_actions`, `ops_metrics_query`), 15 lượt gọi mô hình, 104.399 token, nháp `escalate_to_human`. KHÔNG còn `data_health` và không còn 403. Còn 2 lỗi 400 `ops_metrics_query` là lỗi của mô hình, gateway từ chối đúng: tên truy vấn `infra_up` không có trong danh mục, và `kafka_consumer_lag` kèm `window` trong khi truy vấn này không nhận tham số đó. Chấp nhận được (2 lỗi < ngưỡng dừng 3 liên tiếp); nếu muốn bớt thì làm rõ trong prompt rca hoặc mô tả công cụ rằng chỉ `window` cho những truy vấn có ghi. Lưu ý: phiên #11 bị đánh dấu `error` vì ai-agent được triển khai lại giữa lúc chờ (phiên chờ người sống ở checkpoint nên không mất, nhưng #11 dừng ở trạng thái lỗi): tránh redeploy ai-agent khi có phiên đang chạy.

**Bài học để tổng hợp.**
- Mọi test RCA trước hôm nay dùng cổng giả nên không bắt được lỗi giữa agent và gateway thật (kiểm id, phạm vi token). Cần ít nhất một kiểm tra tích hợp gọi từng công cụ của persona `rca` bằng token hệ thống thật (không cần LLM): gợi ý thêm vào `scripts/smoke-ai.sh` hoặc một script riêng, chạy sau deploy.
- Danh sách công cụ đưa cho mô hình phải khớp với quyền của token mà nó dùng (ở đây: token hệ thống ⊂ operator). Test hiện kiểm bằng đường dẫn `/ops/`; nếu ADR-47 đổi phạm vi thì sửa cả hai nơi.
- Lỗi công cụ lặp là cách đốt tiền nhanh nhất của vòng lặp RCA: cần cả hạn mức riêng lẫn dừng sớm (mục 2, 3).
- Thông báo lỗi chung chung ("tham số không hợp lệ") làm mất 1 ngày mới ra gốc; mã trạng thái đi kèm (mục 4) giúp phân biệt 400/403/404 ngay trong nhật ký.

**Còn lại.** (a) Kiểm tích hợp công cụ RCA như trên. (b) ĐÃ LÀM (phiên #12). (c) ĐÃ LÀM: `errMsg` ở `IncidentsClient.tsx` giữ nguyên câu lỗi của gateway và thêm "(HTTP <mã>)" cho mọi thông báo lỗi của màn; tsc và eslint sạch; đã build lại và cập nhật `frontend`. (d) Rà các route `Proxy` khác dùng `:id` xem có id không phải số (hiện chỉ thấy playbook). (e) Dọn dữ liệu `ai_rca_events` theo thời hạn lưu (chưa có). Các thay đổi trên chưa commit; vòng lặp nền giữ sự cố mô phỏng #58-#60 và #66 vẫn chạy trên máy.

### 16.33 "Hủy điều tra" và "Rút lại đề xuất" (2026-11-16, ADR-48)

Người dùng chốt hai thiết kế sau §16.32 (đề xuất của mình, user đồng ý nguyên văn: "chốt 2 cái này"); quy tắc ba người (người chạy khác người duyệt) KHÔNG làm, ghi lại ở ADR-48.

| Việc | Cách làm | Kiểm |
|---|---|---|
| Hủy điều tra | ai-agent: `RcaService.cancel` (giữ map `run_id -> asyncio.Task` trong `_spawn`; hủy tác vụ rồi `Runner.cancel` ghi dòng nhật ký "Đã hủy điều tra" `{by, was}` và đóng phiên `done/cancelled`; phiên đã kết thúc: 409; không có phiên: 404); API `POST /rca/runs/{id}/cancel` (admin, operator); gateway `POST /ops/rca/runs/:id/cancel`; `OUTCOMES` thêm `cancelled` (nhãn metric `mm_ai_rca_finished_total`); UI nút "Hủy điều tra" (có hộp xác nhận) khi phiên đang chạy/chờ. | 4 test mới ở `test_rca_service.py` (đang chờ, đang chạy với Brain chậm: không chạy sang bước sau, 404, API vai trò/409); test route gateway; gọi tay phiên #12: `done/cancelled`, nhật ký `{by: operator:149, was: waiting_confirm}`, lần hai 409. |
| Rút lại đề xuất | ops-service: migration `00004_withdrawn.sql` (CHECK thêm `WITHDRAWN`), `Store.Withdraw` (khoá dòng; chỉ `created_by == actor`, còn PENDING và chưa hết hạn), `POST /proposals/{id}/withdraw` sau `human()`, `state=WITHDRAWN` lọc được, audit `proposal_withdrawn`; gateway `POST /ops/proposals/:id/withdraw`; watcher coi WITHDRAWN như REJECTED; UI nút "Rút lại đề xuất" cho đúng người tạo. | Test store + API (có Postgres tạm `postgres:16-alpine` ở cổng 55432, đã xoá): người khác 403, rút lại 200, lần hai 409, quá hạn, người tạo vẫn bị `ErrSelfApproval` khi tự duyệt, đề xuất lại cùng nội dung được, audit có `proposal_withdrawn`; test watcher; gọi tay đề xuất #20: admin 403, operator 200 `WITHDRAWN`, lần hai 409. |

Ghi chú cho lần tổng hợp: (1) bộ test của ops-service chỉ chạy store/API khi có `TEST_POSTGRES_DSN`; Postgres của Swarm không mở cổng ra máy, nên dùng một container tạm. (2) Chưa dọn trạng thái `WITHDRAWN` khỏi báo cáo/metric ngoài nhãn tự động `state` (`CountProposalsByState` đã đếm theo nhãn). (3) Triển khai lại ai-agent chỉ ngắt phiên đang `running`; phiên `waiting_*` sống sót (checkpoint trong Postgres).

**Rà các route `Proxy` dùng `:id` (2026-11-16, chỉ đọc code, không gọi route nào).** Gateway có 10 route `opsHandler.Proxy(...:id...)`: `/incidents/:id` (GET, `/investigate`), `/proposals/:id` (GET, `review`, `decision`, `withdraw`, `execute`, `executed`) và `/playbooks/:id`. Ops-service đọc id của 9 route đầu bằng `strconv.ParseInt` và `n > 0` (`api.go`, hàm `id`), nên kiểm "toàn chữ số" ở gateway là khớp. Chỉ playbook dùng slug và đã sửa (§16.32). Kết luận: không còn route nào cùng loại lỗi. Nếu sau này thêm route `Proxy` có id không phải số thì phải thêm nhánh kiểm riêng và một dòng trong `TestOpsPlaybookSlugIsForwarded`-kiểu test; việc còn mở là kiểm tích hợp công cụ RCA bằng token hệ thống thật (xem "Còn lại" ở §16.32).

### 16.34 Kiểm công cụ RCA bằng token hệ thống thật + dọn lịch sử RCA cũ (2026-11-17)

**Kiểm tích hợp, không gọi LLM.** `scripts/smoke-rca-tools.sh` đổi `AI_SYSTEM_KEY` (đọc trong shell từ `deploy/.env`, không in) lấy token hệ thống, lấy `GET /ai/tools`, gọi mọi công cụ GET dưới `/ops/` không cần tham số (phải 2xx), gọi `ops_get_playbook` với mọi slug thật từ `ops_list_playbooks` (bắt lỗi 400 slug của §16.32), và xác nhận một đường ngoài `/ops/` (`/admin/analytics/data-health`) vẫn bị 403. Chạy trên Swarm thật: token OK, 11 công cụ `/ops/` trong danh sách, 6 công cụ không tham số 2xx, 11/11 playbook 2xx, `data-health` 403. Công cụ có `:id` khác (incident, proposal, queries, outbox) cần giá trị thật nên chưa kiểm ở đây. Chạy sau mỗi lần deploy cùng `scripts/smoke-ai.sh` (script này thì CÓ gọi LLM).

**Dọn lịch sử RCA (retention).** `rca.retention_days: 30` (cả hai config; 0 = giữ mãi). Vòng nền mỗi giờ (`main._rca_purge_loop`) gọi `PostgresRunStore.purge_finished(days)`: xoá phiên `done`/`error` không đổi quá hạn cùng `ai_rca_events` của chúng trong một giao dịch, rồi `adelete_thread` cho checkpoint tương ứng (`purge_rca`). Phiên `running`/`waiting_*` không bao giờ bị xoá; một checkpoint lỗi không chặn các cái còn lại. Test: `tests/test_rca_purge.py` (RAM) và đoạn thêm trong test kho Postgres của `tests/test_rca_watcher.py`; toàn bộ 229 test qua với Postgres tạm (đã xoá). Chưa triển khai lại ai-agent cho thay đổi này (tránh ngắt phiên đang chạy).

### 16.35 Bốn khoảng trống sau đối chiếu tiêu chí (2026-11-17, ADR-50, không gọi LLM)

Nguồn: `docs/ai/12_CRITERIA_VS_RESEARCH_NOTES.md`. Toàn bộ test ai-agent: 264 qua với Postgres tạm (trước đó 229).

1. **Kiểm chứng kết luận (`rca/grounding.py`).** Nối vào `graph.hypothesize` và `graph.propose`. Giả thuyết có thêm `grounded` (True/False/None) và `bogus_refs`; dòng thời gian ghi "N giả thuyết không được bằng chứng đã dẫn ủng hộ" và "N mã bằng chứng không tồn tại bị bỏ". Tham số chuỗi của hành động không có trong bằng chứng/sự cố thì nháp thành `escalate_to_human` kèm lý do. Tiêu chí eval `grounded` (top-3 giả thuyết bám bằng chứng, không dẫn mã ma): oracle 22/22. Ảnh hưởng tới dữ liệu cũ: các fixture test có bằng chứng "service down" không nêu tên service nay bị hạ độ tin cậy; đã sửa fixture (chứng tỏ kiểm tra có tác dụng) và đổi `cause_example` của ca `analytics-drift-no-invented-range` thành câu bám bằng chứng (analytics-service ready, Kafka lag đã hết).
2. **Chat đo con số (`guard/grounding.py`).** `pump` gom kết quả công cụ của lượt (`PumpResult.tool_text`, cắt 200 KB); `run_turn` đếm con số 4+ chữ số trong câu trả lời có trong dữ liệu công cụ hoặc câu hỏi: `mm_ai_answer_numbers_total{persona,result=grounded|ungrounded}`. Chỉ đo, không chặn.
3. **Chịu lỗi LLM (`llm/resilience.py`).** `classify` (7 loại), `CircuitBreaker` (cửa sổ trượt 60 s, ngắt khi ≥50% lỗi và ≥6 lần, half-open 1 thăm dò sau 30 s), `Resilience.run` thử lần lượt profile chính rồi `llm.fallbacks.<role>`; `AgentRuntime.invoke_model` (RCA `_ask`) và `ResilientModel` middleware (chat) dùng chung; metric `mm_ai_llm_fallback_total{profile,reason}`. Profile thật có `max_retries: 2` (tầng thử lại duy nhất). Chat: lỗi `model_unavailable` kèm lý do; RCA: `report.error_reason`. `fallbacks: {}` mặc định (chưa có profile dự phòng thật).
4. **CI.** Job `ai-agent` trong `.github/workflows/ci.yml` (Python 3.12, `requirements.lock.txt`, Postgres service; pytest, `run_evals.py`, `run_rca_evals.py`, `--oracle`); `push-images` cần nó. Chưa chạy trên GitHub (bộ khoá phiên bản chỉ kiểm trên máy dev).
5. **Eval.** `evalkit.split_of` (băm câu hỏi, 1/5 giữ kín), `run_evals.py --split`, nhóm `clarify` (16 ca mới: 422 ca tổng), `run_rca_evals.py --repeat k` + `rcakit.summarize_repeats` (Wilson 95%, ca dao động). Chưa chạy live.

**Chưa kiểm:** CI trên GitHub; breaker/dự phòng với nhà cung cấp thật (mới test bằng lỗi giả); số đo `grounded` với mô hình thật (có thể hạ nhầm giả thuyết đúng); ai-agent chưa triển khai lại cho các thay đổi này (cùng §16.34).

**Điểm chưa chắc của §16.35 (xem chi tiết `docs/ai/12_CRITERIA_VS_RESEARCH_NOTES.md` mục 6):** kiểm chứng bằng regex thô (tên lạ lọt, số khác định dạng bị hạ nhầm); nhóm `clarify` xung đột với dòng "KHÔNG hỏi lại" trong prompt nên nhiều khả năng trượt khi chạy thật và 16 ca chưa được người duyệt; tập giữ kín chỉ là quy ước; resilience chưa thử với nhà cung cấp thật; job CI chưa chạy trên GitHub.


### 16.36 Mô tả `ops_metrics_query` rõ hơn và dấu vân tay eval (2026-11-17, không gọi LLM)

- **Mô tả công cụ** (`agent/scopes.go`, `opsQueryHelp` và tham số `window`): nói rõ `name` phải đúng một tên trong danh mục (tên khác là 400) và `window` chỉ truy vấn dạng tốc độ/tỉ lệ/độ trễ/increase/log nhận (18 truy vấn có tham số `window` trong `ops-service/internal/queries`); truy vấn trạng thái tức thời (`service_up`, `outbox_*`, `kafka_consumer_lag`, `circuit_state`, `memory_*`, `infra_up`, `alerts_firing`...) thì bỏ trống. Mục tiêu: bớt 400 do mô hình tự gắn `window` (phiên #12 §16.32). Snapshot `evals/tools.snapshot.json` sinh lại. **CHƯA đo** xem mô hình có bớt bịa thật không: cần chạy live (tốn tiền, chờ đồng ý).
- **Dấu vân tay** (`evalkit.fingerprint`, `write_report`, `read_rows`): file `--out` của `run_evals.py` và `run_rca_evals.py` nay là `{"meta": ..., "results": [...]}`; `meta` có băm 10 ký tự của prompt+policy từng persona, của bảng tool, của từng tệp case, mô hình theo role (profile, nhà cung cấp, model_id, nhiệt độ, `extra_body`; KHÔNG có khoá API), commit git + cờ `dirty`, tham số chạy. `--only-failed` đọc được cả định dạng cũ. Cách dùng: hai lần chạy khác băm = khác đầu vào; giống băm mà khác điểm = dao động của mô hình. Test: `test_fingerprint_...`, `test_report_roundtrip_...`.

### 16.37 Ranh giới từ và chuẩn hoá số dùng chung cho kiểm chứng (2026-11-17, không gọi LLM)

- `guard/numbers.py` mới: rút và chuẩn hoá số; mỗi mẩu là TẬP dạng chuẩn ("1.500" đọc được 1500 hoặc 1,5; khớp nếu có dạng chung). "5,000", "5.000", "5000", "5k" là cùng một số. Mã nhiều dấu (`10.0.0.5`) giữ nguyên.
- `rca/grounding.py`: số so theo tập dạng chuẩn (hết "50" khớp trong "500"); tên khớp theo ranh giới chữ-số; loại "v1.2", "e.g.", "vs", "etc" khỏi danh sách tên cần đối chiếu. `guard/grounding.py` (chat) dùng cùng hàm.
- Test: chuẩn hoá và ranh giới (`test_guard_grounding.py`, `test_rca_grounding.py`); test chỉ số chat viết lại với số có thật và số bịa. 257 passed, 13 skipped; oracle RCA vẫn 22/22.
- **Chưa làm:** số viết chữ ("5 nghìn", "1,5 triệu"); tên không có gạch và ngoài danh sách cứng vẫn lọt (`grounded=None`); vẫn chỉ kiểm "có nhắc tới". Đo tỷ lệ trên model thật vẫn cần chạy live.

### 16.38 Cảnh báo AI, khung "cần người xử lý", heldout theo nhóm (2026-11-17, không gọi LLM)

- **Cảnh báo** (`deploy/prometheus/alerts.yml`, cả ba `warning` nên không tự mở phiên RCA cho chính chúng): `AIRcaRunsErroring` (SLO nhẹ: > 50% phiên RCA kết thúc `error` trong 1 giờ, tối thiểu 3 phiên), `AIModelUnavailable` (> 3 lượt chat `model_unavailable` trong 10 phút), `AILlmFallbackUsed` (profile chính bị bỏ qua; với `fallbacks: {}` không bao giờ bắn). Thêm vào `docs/playbooks/ai-agent.yml`, runbook §19c, 7 ca test trong `alerts_test.yml`. `promtool check rules` (24 luật) và `promtool test rules` đạt (chạy bằng image `prom/prometheus:v2.55.1` có sẵn trên máy). Chưa chạy trên Prometheus thật; chưa đặt cảnh báo cho `mm_ai_answer_numbers_total{result="ungrounded"}` vì chưa có đường cơ sở (cố ý).
- **Giao diện** (`apps/frontend/.../IncidentsClient.tsx`, `lib/rca.ts`): phiên `error` hiện khung đỏ "cần người xử lý sự cố này" kèm `report.error_reason`. Đính chính: §16.35 viết lý do lỗi "hiện cho người", nhưng trước đây giao diện chưa hiện trường này. Không thêm trạng thái DB mới (tránh đổi schema/luồng); `tsc` và `next lint` sạch, chưa xem bằng mắt trên trình duyệt.
- **Heldout theo nhóm** (`evalkit.select_split`): xem doc 12 mục 6.4. 71 câu heldout, 351 dev.
