# Lộ trình xây agent (buyer, seller, dev, admin, vòng lặp RCA)

> **Trạng thái: kế hoạch cho phần chủ dự án tự xây (tầng LLM).** Không có dòng code agent nào trong repo. Tài liệu này gom lại: nền đã có, những gì cần **chốt trước khi viết code**, kiến trúc đề xuất, cấu trúc thư mục, thứ tự xây từng bước (mỗi bước có tiêu chí xong), và cách đo. Khái niệm (agent, tool, human-in-the-loop) xem [02_AGENT_ARCHITECTURE.md](02_AGENT_ARCHITECTURE.md) §0; danh mục tính năng theo persona: [06_AGENT_FEATURES.md](06_AGENT_FEATURES.md); điều tra sự cố: [07_COROOT_AND_INCIDENT_AGENT.md](07_COROOT_AND_INCIDENT_AGENT.md); an toàn token và xác nhận hành động: ADR-40, ADR-42 trong [../decisions/01_ADR_LOG.md](../decisions/01_ADR_LOG.md).
> Khi tài liệu này khác file cũ hơn trong `docs/ai/`, tài liệu này đúng **ở các điểm đã ghi rõ** ở §1.3 (chủ yếu: giao thức gateway ↔ agent là HTTP SSE chứ không phải gRPC stream, và token agent riêng chứ không chuyển JWT người dùng).

## 0. Tóm tắt một trang

- **Một dịch vụ `ai-agent`** (Python, FastAPI) với **bốn persona** chọn theo vai trò trong token: `buyer`, `seller`, `admin`, `dev`. Dùng chung một lõi (vòng lặp gọi tool, an toàn, ghi log lượt chat), mỗi persona chỉ khác prompt, bộ tool và chính sách.
- **Tool = REST của gateway** (không MCP trước mắt, không truy cập DB trực tiếp). Quyền nằm ở gateway, không ở prompt. Agent giữ token ngắn hạn do gateway cấp (ADR-42), không giữ JWT người dùng lâu dài.
- **Hành động ghi chỉ là đề xuất.** Người dùng bấm xác nhận, UI gọi route thật bằng token thật kèm gói đã ký; DB ghi `agent_run_id` để phân biệt (đã có hạ tầng).
- **Vòng lặp RCA (dev/admin)** chạy trên nền ops-service đã có: cảnh báo → incident → điều tra (chỉ đọc, truy vấn có tên) → giả thuyết xếp hạng → **đề xuất** hành động (catalog đóng) → người duyệt → thực thi → **kiểm tra lại**. Agent không có quyền duyệt hay thực thi.
- **Thứ tự xây:** lõi + khung kiểm thử (0) → buyer chỉ đọc (1) → seller chỉ đọc (2) → admin chỉ đọc (3) → dev RCA chỉ đọc (4) → hành động ghi có xác nhận (5) → vòng lặp RCA khép kín có duyệt (6) → vận hành (7). Mỗi bước phát hành được độc lập, có cờ tắt.

## 1. Đã có gì, còn thiếu gì

### 1.1 Đã có (chỉ việc dùng)

| Nền | Ở đâu | Agent dùng để |
|---|---|---|
| Luồng chat SSE, cờ `FEATURE_AI_CHAT` / `FEATURE_AI_OPS`, `GET /config/features` | gateway `handler/ai.go`, ADR-40 | UI ↔ agent; gateway chuyển tiếp `POST AI_SERVICE_URL/chat` |
| Token agent ngắn hạn (`X-Agent-Token`, `jti` = run id), khoá xoay `AGENT_TOKEN_KEYS` | gateway `internal/agent`, ADR-42 | Danh tính của agent khi gọi gateway |
| `POST /ai/proposals` + header `X-Agent-Proposal` + cột `agent_run_id` (order, payment, product) | gateway, `shared/reqctx`, migrations | Hành động ghi có người xác nhận, phân biệt nguồn trong DB |
| REST đọc cho buyer/seller/admin (đơn, sản phẩm, tồn kho, báo cáo analytics có `as_of`/`source`) | [../design/04_API_REFERENCE.md](../design/04_API_REFERENCE.md) | Tool đọc |
| ops-service: incident từ cảnh báo, `investigate`, `queries` có tên, `outbox/<svc>/dead`, đề xuất → duyệt → thực thi → xác nhận đã chạy, audit log chỉ ghi thêm, catalog hành động đóng | `services/ops-service`, ADR-38/39/40 | Nền của vòng lặp RCA; agent dev dùng `OPS_AGENT_TOKEN` (đọc + tạo đề xuất, không duyệt/thực thi) |
| Playbook YAML cho 10 nhóm sự cố, có kiểm tra khớp `alerts.yml` | `docs/playbooks/` | Nguồn sự thật (ground truth) và bộ đề kiểm tra RCA |
| Metrics, log (Loki), trace (Tempo), dashboard, `X-Request-ID` xuyên service | `deploy/`, ADR-33/36 | Nguồn bằng chứng cho RCA |
| Bộ dữ liệu giả + backfill/reconcile ClickHouse | `scripts/datagen`, `docs/dataset/` | Dữ liệu cho hỏi đáp số liệu và đánh giá |
| Vai trò `dev`, giao diện console, bong bóng chat (đang là bản xem trước, `NEXT_PUBLIC_AI_UI_PREVIEW`) | frontend | Chỗ gắn UI thật |

### 1.2 Còn thiếu (làm cùng lúc với agent, không phải trước)

| Thiếu | Cần cho | Ghi chú |
|---|---|---|
| Danh sách trắng route đọc cho token agent theo role (ADR-42 mục 3–4) và `GET /ai/tools` | Bước 1–4 | Hiện gateway gửi cả `Authorization` người dùng lẫn `X-Agent-Token`; chỉ khi làm xong mục này mới bỏ `Authorization` |
| Kho lượt chat theo `run_id` (nội dung, tool đã gọi, token, độ trễ) | Mọi bước, bắt buộc để tra lại | Thuộc `ai-agent`; bảng riêng trong Postgres, thời hạn giữ + che dữ liệu nhạy cảm (ADR-42 mục 9) |
| Rate limit theo `sub` (người dùng) thay vì IP cho `/ai/*` | Bước 1 | Hiện theo IP |
| Giao diện thật cho sự kiện SSE (`token`, `tool_call`, `card`, `action_proposal`, `error`, `done`) và nút xác nhận | Bước 1, 5 | Hợp đồng sự kiện: [05_AI_DRAFT_ORIGINAL.md](05_AI_DRAFT_ORIGINAL.md) §3 |
| Tìm kiếm ngữ nghĩa / gợi ý cho buyer (tool `search` hiện chỉ là tìm theo từ khoá) | Buyer nâng cao | Không chặn bước 1 |
| Tool 🆕 trong [06_AGENT_FEATURES.md](06_AGENT_FEATURES.md) (mỗi persona có vài tool chưa có REST) | Từng tính năng | Thêm REST ở gateway rồi mới thêm tool; đừng cho agent đi đường tắt |
| Môi trường đánh giá (bộ câu hỏi vàng, kịch bản sự cố tái hiện được) | Bước 0, 4, 6 | §8 |

### 1.3 Chỗ khác với tài liệu cũ

1. [02_AGENT_ARCHITECTURE.md](02_AGENT_ARCHITECTURE.md) §7 vẽ gateway → agent bằng gRPC stream. Thực tế đã làm (ADR-40) là **HTTP + SSE**: `POST AI_SERVICE_URL/chat`, body JSON ≤ 64 KB, trả `text/event-stream`. Giữ như đã làm.
2. Tài liệu cũ ghi "REST kèm JWT của user". Theo ADR-42 agent dùng **token agent ngắn hạn do gateway cấp**; JWT người dùng không được giao cho agent về lâu dài.
3. 06_AGENT_FEATURES gọi ba persona (`shopper`, `seller`, `admin`). Ở đây tách thêm `dev` vì vai trò `dev` đã có và có quyền khác (chỉ vận hành, không có dữ liệu kinh doanh).

## 2. Những thứ cần chốt TRƯỚC khi viết code

Mỗi mục là một câu hỏi cần một đáp án viết ra (ghi thành ADR khi chốt). Cột "đề xuất" là điểm xuất phát, không phải quyết định.

### 2.1 Phạm vi và rủi ro

| # | Câu hỏi | Đề xuất |
|---|---|---|
| D1 | Phase đầu agent làm được gì? | **Chỉ đọc + giải thích.** Ghi (đặt/huỷ/hoàn tiền) từ bước 5, RCA khép kín từ bước 6 |
| D2 | Hành động ghi nào được đề xuất? | Danh sách đóng `proposable` (hiện 5 route). Thêm từng cái, mỗi cái có test |
| D3 | Dữ liệu nào **không bao giờ** vào prompt? | Mật khẩu/băm, token, số thẻ (không tồn tại, thanh toán mô phỏng), `IP_HASH_SECRET`, secret. Số điện thoại/địa chỉ: che hoặc chỉ khi cần |
| D4 | Agent có được đọc dữ liệu cá nhân của người dùng khác không? | Không. Phạm vi lấy từ `sub` trong token, không từ tham số do mô hình điền |
| D5 | Khi mô hình sai hoặc không chắc? | Nói "không chắc" kèm lý do, đưa nguồn; số liệu luôn có `as_of`. Không bịa; không có tool thì từ chối |
| D6 | Ai chịu trách nhiệm hành động đã xác nhận? | Người xác nhận (ghi `agent_run_id` + user). Agent không bao giờ là bên duyệt |

### 2.2 Mô hình và chi phí

| # | Câu hỏi | Đề xuất |
|---|---|---|
| M1 | Nhà cung cấp/mô hình nào cho từng persona? | Khai báo nhiều **profile LLM** và ánh xạ persona/role → profile trong yaml (§13), nên đổi nhà cung cấp chỉ sửa cấu hình. Bắt đầu với một profile. Buyer/seller: mô hình nhỏ-nhanh; admin/dev RCA: mô hình mạnh hơn. Chốt tên mô hình sau khi chạy bộ đánh giá §8, đừng chốt theo cảm tính |
| M6 | Thư viện | LangChain: `create_agent` + middleware cho buyer/seller/admin, `langchain-core` cho mô hình; **LangGraph** chỉ cho RCA (bước 6); **LangSmith không bật** (§14) |
| M2 | Gọi có cấu trúc (tool calling) hay tự parse? | Tool calling gốc của nhà cung cấp; schema sinh từ một nguồn duy nhất (§4.3) |
| M3 | Ngân sách | Trần token/lượt, trần lượt/người/giờ, trần chi phí/ngày toàn hệ thống; vượt thì dừng và báo, không âm thầm hạ chất lượng |
| M4 | Dữ liệu gửi ra ngoài | Nếu dùng API ngoài: đọc điều khoản về lưu dữ liệu, che dữ liệu nhạy cảm trước khi gửi. Dữ liệu vận hành (log) có thể chứa dữ liệu người dùng: lọc trước khi đưa vào prompt RCA |
| M5 | Có cần bộ nhớ dài hạn không? | Không phase đầu. Chỉ giữ lịch sử trong một cuộc chat (cắt theo token); tóm tắt khi dài |

### 2.3 Giao thức và dữ liệu

| # | Câu hỏi | Đề xuất |
|---|---|---|
| P1 | Hợp đồng gateway ↔ agent | `POST /chat` (SSE) như ADR-40: header `X-Agent-Token`, `X-Agent-Run-ID`, `X-Request-ID`; body `{message, conversation_id?, page_context?}`. Viết thành OpenAPI trong `ai-agent/contracts/` |
| P2 | Sự kiện SSE | `start` (conversation_id, run_id), `token`, `tool_call`, `tool_result` (rút gọn), `card`, `citation`, `action_proposal`, `error`, `done` — mỗi sự kiện có `run_id` |
| P3 | Lưu hội thoại ở đâu | Postgres riêng của `ai-agent`: `conversations` (user_id = `sub`), `chat_runs`, `chat_messages`, `tool_calls`; khoá theo `run_id` = `jti`. `conversation_id` do **`ai-agent` sinh** (UUID) ở tin đầu và trả cho UI trong sự kiện `start`; mỗi lần kiểm `conversation.user_id == sub` (sai → 404); mỗi cuộc chat chỉ chạy một lượt tại một thời điểm (lượt thứ hai → 409); `client_message_id` duy nhất để bỏ tin gửi trùng (mạng lỗi) |
| P4 | Thời hạn giữ + ai xem | Đề xuất 30 ngày, chỉ admin xem, che số điện thoại/địa chỉ trước khi lưu (mở trong ADR-42) |
| P5 | Ngôn ngữ | Tiếng Việt mặc định, trả lời theo ngôn ngữ người hỏi; tên tool/field giữ tiếng Anh |

### 2.4 Đánh giá

| # | Câu hỏi | Đề xuất |
|---|---|---|
| E1 | "Tốt" nghĩa là gì cho từng persona? | Buyer/seller: đúng số liệu và đúng quyền; admin: đúng số liệu; dev: top-3 nguyên nhân chứa nguyên nhân thật, đề xuất đúng catalog (§7.5) |
| E2 | Ngưỡng cho phép phát hành | Viết số cụ thể trước khi chạy (ví dụ "đúng quyền 100%", "đúng số liệu ≥ 95% trên bộ vàng") |
| E3 | Cách chấm | Kiểm tra tự động ở chỗ có đáp án cứng (số, route đã gọi, từ chối đúng); người chấm cho văn bản tự do; **không** dùng chính mô hình để chấm khi chưa kiểm chứng với người |

## 3. Kiến trúc đề xuất

### 3.1 Bức tranh chung

```mermaid
flowchart LR
  UI["Frontend<br/>chat + thẻ + nút xác nhận"] -->|"POST /ai/chat (JWT người dùng)"| GW["api-gateway"]
  GW -->|"POST /chat (SSE)<br/>X-Agent-Token, run id"| AG["ai-agent (Python)"]
  AG -->|"REST bằng token agent<br/>chỉ route trong danh sách trắng"| GW
  AG -->|"POST /ai/proposals"| GW
  GW --> SV["order/product/user/payment/analytics<br/>(gRPC)"]
  AG -->|"OPS_AGENT_TOKEN<br/>đọc + tạo đề xuất"| OPS["ops-service"]
  AG --> LLM["nhà cung cấp mô hình"]
  AG --> DB[("Postgres ai-agent<br/>kho lượt chat")]
  UI -.->|"người dùng bấm xác nhận:<br/>route thật + X-Agent-Proposal<br/>(token thật)"| GW
```

Điểm cần nhớ: agent **không** có đường trực tiếp tới service nghiệp vụ hay DB nghiệp vụ; nó đi qua gateway như một client có quyền hẹp. Riêng ops, nó nói chuyện với ops-service bằng token riêng của ops (`OPS_AGENT_TOKEN`, ADR-40), không qua `/ai`.

### 3.2 Thành phần bên trong `ai-agent`

```mermaid
flowchart TB
  API["api/ (FastAPI: /chat SSE, /health)"] --> RUN["runtime/ vòng lặp lượt chat"]
  RUN --> PERS["personas/ (prompt + bộ tool + chính sách)"]
  RUN --> LLMP["llm/ (lớp bọc nhà cung cấp, đếm token)"]
  RUN --> TOOLS["tools/ (gateway client, ops client, schema)"]
  RUN --> GUARD["guard/ (giới hạn vòng, timeout, ngân sách, lọc đầu vào/ra)"]
  RUN --> STORE["store/ (kho lượt chat)"]
  TOOLS --> GWC["gateway REST"]
  TOOLS --> OPSC["ops-service REST"]
  RCA["rca/ (vòng lặp điều tra)"] --> RUN
  RCA --> OPSC
```

Nguyên tắc: `runtime` không biết persona nào cụ thể; persona là dữ liệu (prompt, danh sách tool, hằng số giới hạn). Thêm persona = thêm một thư mục, không sửa lõi.

### 3.3 Vòng lặp một lượt chat

1. Gateway xác thực người dùng, cấp token agent, chuyển tới `/chat`.
2. `runtime` chọn persona theo `role` **trong token** (không theo lời người dùng), nạp prompt + schema tool.
3. Lặp tối đa 4 vòng (RCA: tối đa theo §7): gọi mô hình → nếu có `tool_call`, **kiểm tra tool có trong bộ của persona**, kiểm tra tham số theo schema, gọi gateway, cắt kết quả cho vừa ngân sách token, đưa lại mô hình.
4. Câu trả lời cuối: stream `token`, kèm `citation` (tool nào, `as_of`).
5. Nếu cần ghi: gọi `POST /ai/proposals`, gửi `action_proposal` cho UI. **Dừng ở đây**, không tự gọi route ghi.
6. Ghi kho lượt chat; trả `done`.

### 3.4 Dùng framework hay tự viết

Phase 0–3 (một vòng lặp tool tuyến tính, tối đa 4 vòng): dùng `create_agent` của LangChain kèm middleware giới hạn (§14); tự viết chỉ phần chuyển SSE và lưu lịch sử. Cân nhắc LangGraph khi vào bước 6 (RCA có nhánh, quay lại, dừng chờ người duyệt, lưu trạng thái giữa chừng): xem [02_AGENT_ARCHITECTURE.md](02_AGENT_ARCHITECTURE.md) §4. Quan sát/đánh giá: bắt đầu từ chính kho lượt chat; thêm công cụ ngoài (LangSmith, Langfuse…) chỉ khi kho này không đủ.

## 4. Cấu trúc thư mục đề xuất

```
services/ai-agent/                     (Python 3.12, FastAPI; Dockerfile build từ gốc repo như các service khác)
├── pyproject.toml                     (phụ thuộc cố định phiên bản; ruff, mypy, pytest)
├── Dockerfile
├── README.md                          (cách chạy cục bộ, biến môi trường)
├── configs/
│   ├── config.dev.yaml                (profile LLM, role→profile, giới hạn; KHÔNG chứa secret)
│   ├── config.prod.yaml
│   └── .env.example                   (tên biến secret; giá trị thật ở deploy/.env)
├── contracts/
│   ├── chat.openapi.yaml              (POST /chat + sự kiện SSE: nguồn sự thật cho gateway và UI)
│   └── tools/                         (schema tool: sinh từ bảng role→tool của gateway, xem §4.3)
├── src/ai_agent/
│   ├── main.py                        (khởi tạo app, đọc cấu hình, kiểm tra biến bắt buộc)
│   ├── config/
│   │   ├── loader.py                  (nạp .env → configs/config.{APP_ENV}.yaml → thay ${VAR} → Config; che secret; §13)
│   │   └── logging.py                 (log JSON có run_id, query_context)
│   ├── api/
│   │   ├── chat.py                    (SSE, đọc X-Agent-Token / X-Agent-Run-ID)
│   │   ├── health.py                  (/health, /ready, /metrics)
│   │   └── auth.py                    (đọc token agent để biết role/sub; KHÔNG ký, KHÔNG tin header role)
│   ├── runtime/
│   │   ├── agent_factory.py           (dựng create_agent cho từng persona: model + tools + middleware; §14)
│   │   ├── sse_adapter.py             (đổi stream của LangChain → sự kiện SSE của mình)
│   │   ├── events.py                  (kiểu sự kiện SSE)
│   │   └── context.py                 (run_id, user, role, ngân sách còn lại)
│   ├── personas/
│   │   ├── base.py                    (giao diện Persona: prompt, tools, policy)
│   │   ├── buyer/        {prompt.md, tools.py, policy.py}
│   │   ├── seller/       {prompt.md, tools.py, policy.py}
│   │   ├── admin/        {prompt.md, tools.py, policy.py}
│   │   └── dev/          {prompt.md, tools.py, policy.py}
│   ├── tools/
│   │   ├── gateway.py                 (client REST: timeout 5 s, retry chỉ cho GET, ghi lại mọi lời gọi)
│   │   ├── ops.py                     (client ops-service bằng OPS_AGENT_TOKEN)
│   │   ├── registry.py                (tên tool → hàm + schema + persona được phép)
│   │   └── redact.py                  (che dữ liệu nhạy cảm trong kết quả tool)
│   ├── llm/                           (profile + role + factory, §13)
│   │   ├── registry.py                (LLM_PROVIDERS: tên provider → hàm build(profile); @register_llm)
│   │   ├── factory.py                 (get_llm(cfg, profile), get_llm_for_role(cfg, role))
│   │   ├── providers/
│   │   │   ├── __init__.py            (import từng provider để tự đăng ký)
│   │   │   ├── _common.py             (require, model_id: từ chối giá trị mẫu)
│   │   │   ├── openai.py              (OpenAI và API tương thích qua base_url: DeepSeek, proxy)
│   │   │   ├── anthropic.py | google.py | ollama.py
│   │   │   └── fake.py                (mô hình giả có kịch bản, cho test an toàn và CI)
│   │   ├── metrics.py                 (đo E2E, token vào/ra, lỗi mỗi lần gọi → log/JSONL)
│   │   └── budget.py                  (đếm token, trần chi phí theo lượt/ngày)
│   ├── guard/
│   │   ├── middleware.py              (middleware tự viết: ngân sách token, kiểm tool/tham số, che dữ liệu, ghi run)
│   │   ├── limits.py                  (cấu hình ModelCallLimit/ToolCallLimit, 60 s/lượt, 5 s/tool, kích thước kết quả)
│   │   ├── untrusted.py               (đánh dấu dữ liệu không tin cậy trong prompt)
│   │   └── proposals.py               (tạo đề xuất qua /ai/proposals; không có đường gọi route ghi)
│   ├── store/
│   │   ├── models.py                  (chat_runs, chat_messages, tool_calls)
│   │   ├── repo.py
│   │   └── migrations/                (Alembic hoặc SQL thuần theo đúng cách repo hiện làm)
│   ├── rca/                           (bước 6; chỉ dev/admin)
│   │   ├── state.py                   (trạng thái điều tra: bằng chứng, giả thuyết, hành động)
│   │   ├── graph.py                   (các nút và cạnh, §7)
│   │   ├── evidence.py                (gom bằng chứng: queries của ops, Prometheus, Loki, Tempo)
│   │   ├── playbooks.py               (nạp docs/playbooks/*.yml)
│   │   └── verify.py                  (kiểm tra lại sau khi chạy hành động)
│   └── observability/                 (metrics Prometheus, log JSON có run_id, trace OTel)
├── evals/                             (harness kiểm thử, §8A)
│   ├── golden/{buyer,seller,admin,dev}.jsonl   (câu hỏi + đáp án/route kỳ vọng)
│   ├── scenarios/                     (kịch bản sự cố tái hiện: §8.2)
│   ├── run_evals.py                   (chạy bộ vàng, in báo cáo, thoát lỗi nếu dưới ngưỡng)
│   ├── replay.py                      (chạy lại một run_id đã lưu)
│   └── fixtures/                      (dữ liệu giả cố định, sinh từ scripts/datagen với seed)
└── tests/
    ├── unit/                          (guard, redact, registry, budget: không gọi mô hình)
    ├── contract/                      (khớp OpenAPI; tool khớp bảng role→tool của gateway)
    └── safety/                        (prompt injection, vượt quyền, vòng lặp vô hạn)
```

Phía còn lại (đã có hoặc cần thêm, không nằm trong `ai-agent`):

```
services/api-gateway/internal/agent/    (đã có: khoá, token, gói đề xuất)
services/api-gateway/internal/handler/ai.go, proposals.go   (đã có)
services/api-gateway/internal/handler/ai_tools.go            (CẦN THÊM: GET /ai/tools từ bảng role→tool)
services/api-gateway/internal/agent/scopes.go                (CẦN THÊM: danh sách trắng route theo role, một nguồn sự thật)
apps/frontend/components/ai/                                 (đã có ChatWidget; cần: thẻ kết quả, nút xác nhận, hiển thị tool đang chạy)
deploy/services.yml                                          (thêm service ai-agent + Postgres riêng hoặc schema riêng)
docs/ai/ , docs/playbooks/                                   (đã có)
```

### 4.3 Một nguồn sự thật cho tool

Bảng role → tool → (method, route, tham số) nằm **một chỗ** trong gateway (`scopes.go`). Gateway dùng nó để cho phép/chặn route với token agent; `GET /ai/tools` trả cùng bảng cho `ai-agent`; `ai-agent` sinh schema tool từ đó (hoặc kiểm tra khớp trong `tests/contract`). Không viết tay hai bảng. `router_test.go` có test: mọi tool trỏ tới route thật và đúng role.

## 5. Thứ tự xây, từng bước

Mỗi bước: **làm gì**, **xong khi nào**. Không sang bước sau khi bước trước chưa đạt tiêu chí.

### Bước 0. Lõi và khung kiểm thử (1–2 tuần)
- Dựng `ai-agent` rỗng chạy được trong Swarm (`/health`, `/ready`, `/metrics`, log JSON có `run_id`), Postgres riêng + migration, kho lượt chat.
- `runtime/loop.py` với **mô hình giả (fake provider)** trả lời cố định; `guard/limits.py`; `tools/gateway.py`; `registry.py`.
- Gateway: `GET /ai/tools` và `scopes.go` (danh sách trắng), bỏ `Authorization` người dùng khỏi lời gọi tới agent, rate limit theo `sub`.
- `evals/run_evals.py` và `llm/fake.py` (mô hình giả có kịch bản) chạy được; `tests/safety/` có các ca đầu tiên (§8A).
- **Xong khi:** gọi `POST /ai/chat` đi hết đường UI → gateway → agent (mô hình giả) → gateway → trả SSE; kho có bản ghi theo `run_id`; token agent bị 401/403 đúng ở các route cấm (test của ADR-42 mục "Kiểm").

### Bước 1. Buyer, chỉ đọc (2–3 tuần)
- Tool: tìm sản phẩm, chi tiết, danh mục, giỏ hàng (và tổng tiền), đơn của chính mình, trạng thái đơn. Hỏi chính sách (trả/huỷ) từ tài liệu tĩnh trước, RAG sau.
- Prompt buyer; chống prompt injection trong mô tả sản phẩm/review (`untrusted.py`).
- **Xong khi:** bộ vàng buyer ≥ ngưỡng đã chốt (E2); không thấy đơn người khác (test safety); UI hiển thị thẻ sản phẩm/đơn.

### Bước 2. Seller, chỉ đọc
- Tool: sản phẩm của shop, tồn kho/sổ kho, hàng sắp hết, báo cáo analytics của shop, hộp thư đơn của shop.
- Gợi ý nhập hàng chỉ ở dạng **tư vấn bằng chữ** (chưa có đề xuất ghi).
- **Xong khi:** đúng số liệu trên bộ vàng; hỏi dữ liệu shop khác → từ chối/403; mỗi con số có `as_of`.

### Bước 3. Admin, chỉ đọc
- Tool: báo cáo nền tảng, đơn/thanh toán toàn hệ thống, `data-health`, danh mục. **Không** có tool tới `/admin/accounts` (không bao giờ: agent không quản lý tài khoản).
- **Xong khi:** đúng số liệu; không tool nào chạm route quản lý tài khoản (test hợp đồng).

### Bước 4. Dev, chỉ đọc + RCA một lượt
- Persona `dev`: dùng `OPS_AGENT_TOKEN` tới ops-service: `/incidents`, `/incidents/{id}/investigate`, `/queries/{name}`, `/outbox/{svc}/dead`; thêm đọc Prometheus/Loki/Tempo **qua truy vấn có tên** (không cho mô hình tự viết PromQL/LogQL tuỳ ý ở phase này).
- Đầu ra: báo cáo gồm bằng chứng (liên kết/truy vấn), top-3 giả thuyết có độ tin cậy, bước kiểm tra tiếp theo. Chưa đề xuất hành động.
- **Xong khi:** chạy lại bộ kịch bản sự cố (§8.2) đạt ngưỡng E2 cho "top-3 chứa nguyên nhân thật"; mọi bằng chứng truy ngược được tới một lời gọi tool.

### Bước 5. Hành động ghi có xác nhận (buyer, seller, admin)
- Mở dần từng hành động trong danh sách `proposable`: huỷ đơn của chính mình (buyer) trước, rồi seller huỷ đơn shop, admin hoàn tiền. Mỗi hành động: tool đề xuất (gọi `/ai/proposals`) + thẻ xác nhận trên UI + test.
- UI: thẻ hiển thị `summary`, tham số thật (số tiền, mã đơn), nút xác nhận/bỏ; sau xác nhận gọi route thật với `X-Agent-Proposal`; xử lý 401/403/409/503 (§ADR-42).
- **Xong khi:** thử tấn công bộ test safety: sửa tham số sau khi ký, dùng lại gói, gói của người khác, bỏ header (chạy nhưng không có `agent_run_id`) đều cho kết quả đúng; truy vấn `agent_run_id` ra đúng hành động.

### Bước 6. Vòng lặp RCA khép kín (dev, admin)
Chi tiết §7. Thêm: tạo đề xuất vận hành tại ops-service, chờ người duyệt, theo dõi thực thi, kiểm tra lại, đóng incident.
- **Xong khi:** trên kịch bản có hành động an toàn (ví dụ outbox kẹt, `docker service update --force`), vòng lặp đi hết từ cảnh báo đến đóng incident với một người duyệt; agent **không** có đường tự duyệt/tự chạy (test 403); kiểm tra lại phát hiện được trường hợp hành động không hiệu quả và không tự đóng incident.

### Bước 7. Vận hành
- Dashboard Grafana cho agent (số lượt, độ trễ, token, chi phí, tỉ lệ lỗi tool, tỉ lệ bị chặn), cảnh báo (chi phí vượt trần, lỗi nhà cung cấp, kho lượt chat đầy), runbook mục mới, thời hạn xoá kho lượt chat, quy trình xoay khoá (đã có §18).
- Công tắc tắt từng persona, đường lui không-AI.
- **Xong khi:** diễn tập tắt/bật cờ, hết ngân sách, nhà cung cấp lỗi: hệ thống vẫn dùng được không AI.

## 6. Chi tiết từng persona

| | buyer | seller | admin | dev |
|---|---|---|---|---|
| Mục tiêu | tìm hàng, theo dõi đơn, hiểu chính sách | số liệu shop, tồn kho, xử lý đơn | số liệu nền tảng, khiếu nại, kiểm duyệt | điều tra và xử lý sự cố |
| Dữ liệu đọc | sản phẩm công khai, giỏ + đơn của mình | shop của mình (từ `sub`) | toàn nền tảng (không tài khoản) | vận hành (incident, metric, log, trace), **không** dữ liệu kinh doanh |
| Ghi (đề xuất) | huỷ đơn mình, yêu cầu trả hàng | huỷ đơn shop, (sau) điều chỉnh tồn kho | huỷ đơn, hoàn tiền, (sau) khoá sản phẩm | đề xuất vận hành tại ops-service |
| Nguồn token | token agent (ADR-42) | token agent | token agent | `OPS_AGENT_TOKEN` cho ops + token agent cho `system/health` |
| Cấm tuyệt đối | thanh toán, đổi mật khẩu, địa chỉ người khác | dữ liệu shop khác, tiền | quản lý tài khoản, đổi quyền | duyệt/thực thi đề xuất, đọc dữ liệu khách hàng |
| Giới hạn riêng | 4 vòng | 4 vòng | 4 vòng | RCA: §7 |
| Đo chính | đúng số liệu + đúng quyền | đúng số liệu + đúng quyền | đúng số liệu | top-3 nguyên nhân, đúng catalog hành động |

Quy tắc chung đã chốt ở [06_AGENT_FEATURES.md](06_AGENT_FEATURES.md) §1 (persona theo role trong token, dữ liệu ngoài là không tin cậy, không nhập dữ liệu thanh toán, `as_of`/`source`, giới hạn cứng) vẫn đúng và là điều kiện của mọi bước.

## 7. Vòng lặp RCA đề xuất (dev/admin)

### 7.1 Hình dạng

```mermaid
flowchart TB
  A["Cảnh báo Prometheus"] --> B["ops-service: tạo/gộp incident"]
  B --> C["agent: nhận incident (poll hoặc webhook)"]
  C --> D["Thu thập bằng chứng<br/>(queries có tên, metric, log, trace)"]
  D --> E["Sinh giả thuyết + xếp hạng<br/>(đối chiếu playbook)"]
  E --> F{"Đủ chắc?"}
  F -- "không" --> G["Truy vấn thêm (có trần số vòng)"] --> D
  F -- "có" --> H["Đề xuất hành động<br/>(chỉ trong catalog đóng)"]
  H --> I["ops-service: proposal chờ duyệt"]
  I --> J{"Người duyệt"}
  J -- "từ chối" --> K["Ghi lý do, báo cáo, dừng"]
  J -- "duyệt" --> L["Thực thi (ops-service executor, không phải agent)"]
  L --> M["Agent kiểm tra lại: cảnh báo hết? chỉ số hồi phục?"]
  M --> N{"Hồi phục?"}
  N -- "có" --> O["Báo cáo + đóng incident (chờ người xác nhận đóng)"]
  N -- "không" --> P["Báo người, đưa lại bước giả thuyết (tối đa N lần)"]
```

### 7.2 Nút, trạng thái, giới hạn

| Nút | Làm gì | Giới hạn |
|---|---|---|
| `collect` | chạy các truy vấn **có tên** theo playbook của cảnh báo; thêm truy vấn theo gợi ý của mô hình **nếu nằm trong danh mục** | tối đa 12 truy vấn/lượt, 5 s/truy vấn, kết quả cắt theo token |
| `hypothesize` | mô hình xếp hạng nguyên nhân dựa trên bằng chứng + playbook; mỗi giả thuyết **phải trích bằng chứng** | tối đa 3 giả thuyết, mỗi cái có độ tin cậy và "điều gì sẽ bác bỏ nó" |
| `decide` | đủ chắc (ngưỡng viết rõ) thì đề xuất; không thì quay lại `collect` | tối đa 3 vòng `collect` |
| `propose` | tạo proposal tại ops-service với `action` trong catalog, tham số theo schema của catalog | không có `action` ngoài catalog; mô hình không viết lệnh shell/SQL tự do |
| `wait_human` | dừng, lưu trạng thái (cần checkpoint), chờ duyệt | hết hạn `OPS_PROPOSAL_TTL_MIN` thì thành `EXPIRED`, báo người |
| `verify` | sau khi báo đã thực thi, chờ cửa sổ ổn định (ví dụ 2–5 phút), kiểm lại đúng truy vấn đã chứng minh sự cố | không hồi phục thì **không** đóng, tăng bậc rủi ro, báo người |
| `report` | báo cáo cuối: dòng thời gian, bằng chứng, nguyên nhân, hành động, kết quả, việc cần làm tiếp | luôn tạo, kể cả khi thất bại |

### 7.3 Phân quyền trong vòng lặp

- Agent: đọc, tạo đề xuất, ghi chú vào incident, báo cáo. **Không** duyệt, không thực thi (ops-service trả 403 với `OPS_AGENT_TOKEN`).
- Người tạo đề xuất không tự duyệt được (luật đã có); agent cần người duyệt khác vai.
- Bậc rủi ro của hành động lấy từ catalog (`observe`/`low`/`medium`/`high`...), không để mô hình tự gán. Hành động `low` có thể cho tự động duyệt **chỉ khi chủ hệ thống bật rõ ràng** và đã có số liệu chất lượng (§8) cho loại hành động đó.

### 7.4 Điều khiển lặp (tránh tự gây thêm sự cố)

- Mỗi incident có **ngân sách**: tổng số lượt mô hình, số truy vấn, tổng thời gian. Hết thì dừng và báo người.
- Không đề xuất lại hành động đã thất bại cho cùng incident mà không có bằng chứng mới.
- Tối đa 2 đề xuất đang chờ cho một incident; không tạo đề xuất mới khi đang có cái đang thực thi.
- Khi nhiều cảnh báo cùng lúc (xem `docs/playbooks/multiple-faults.yml`): xử lý theo nguyên nhân gốc trước (ví dụ DB chậm trước, rồi mới nhìn lỗi ở gateway), không đề xuất song song nhiều hành động tác động lẫn nhau.
- Nếu chính hệ quan sát hỏng (`observability-down`): báo "không đủ bằng chứng", không suy đoán.

### 7.5 Kiểm chứng đã có thể làm ngay

Playbook YAML là đáp án chuẩn: cảnh báo → nguyên nhân → hành động an toàn. Dùng làm: (1) tập đề, (2) ground truth cho "top-3 chứa nguyên nhân thật", (3) kiểm tra hành động đề xuất nằm trong catalog. Chi tiết metric và dạng một bộ dữ liệu RCA: [07_COROOT_AND_INCIDENT_AGENT.md](07_COROOT_AND_INCIDENT_AGENT.md) §6–§7. Coroot là tuỳ chọn (nguồn tín hiệu thêm), không bắt buộc cho bước 4–6.

## 8. Đánh giá và kiểm thử

### 8.1 Ba tầng

1. **Test thường (không gọi mô hình):** guard, redact, registry khớp bảng của gateway, hợp đồng OpenAPI, giới hạn vòng/thời gian/kích thước. Chạy trong CI.
2. **Test an toàn (mô hình giả cố tình làm sai):** mô hình đòi tool ngoài bộ persona, tham số lạ, vòng vô hạn, tool trả văn bản chứa lệnh giả, đề xuất route ngoài danh sách. Hệ thống phải chặn ở tầng guard/gateway, **không** dựa vào việc mô hình ngoan. Chạy trong CI.
3. **Bộ vàng (gọi mô hình thật, có chi phí):** `evals/golden/*.jsonl` chạy theo yêu cầu/mỗi tuần, có báo cáo so với ngưỡng E2. Cố định dữ liệu bằng `scripts/datagen` với seed để đáp án ổn định.

### 8.2 Kịch bản sự cố tái hiện (cho RCA)

Mỗi kịch bản: cách gây lỗi (ví dụ dừng một service, chặn Kafka, làm đầy outbox, bơm tải), cảnh báo mong đợi, nguyên nhân thật, hành động đúng, cách kiểm tra phục hồi, nguồn: playbook tương ứng. Làm trên **môi trường thử, không phải Swarm đang dùng thật**, và có lệnh dọn dẹp. Bắt đầu từ ba kịch bản rẻ và rõ: dịch vụ chết, outbox kẹt, consumer DLQ tăng.

### 8.3 Số cần theo dõi liên tục

Tỉ lệ đúng quyền (mục tiêu 100%), tỉ lệ số liệu đúng, tỉ lệ từ chối đúng, số tool call/lượt, độ trễ lượt (p50/p95), token và chi phí/lượt, tỉ lệ hành động đề xuất được xác nhận/bị bỏ (đề xuất tệ thì bị bỏ nhiều), tỉ lệ RCA top-3 đúng, thời gian từ cảnh báo đến báo cáo.

## 8A. Harness: cái gì cần dựng, cái gì không

"Harness" ở đây có hai phần. Cả hai đã nằm trong cấu trúc §4; mục này chốt ranh giới và thứ tự.

### 8A.1 Harness chạy agent (runtime), tự viết mỏng
Gồm `runtime/`, `guard/`, `tools/registry.py`, `store/`:
- vòng lặp gọi tool, giới hạn 4 vòng, 60 s/lượt, 5 s/tool, ngân sách token;
- kiểm tra tool có thuộc bộ của persona, kiểm tra tham số theo schema trước khi gọi;
- cắt kết quả tool cho vừa ngân sách, che dữ liệu nhạy cảm (`redact.py`);
- ghi kho lượt chat theo `run_id`, stream sự kiện SSE;
- đường chạy duy nhất: mọi lời gọi mô hình và tool đều đi qua đây, không có đường tắt.

Phase 0–3 chỉ là vòng lặp tuyến tính (vài trăm dòng). Cân nhắc LangGraph chỉ ở bước 6, khi RCA cần nhánh, checkpoint và dừng chờ người duyệt.

### 8A.2 Harness kiểm thử (eval), dựng TRƯỚC khi nối mô hình thật
| Thành phần | Việc | Ở đâu | Chi phí |
|---|---|---|---|
| **Fake provider** | mô hình giả có kịch bản: trả lời cố định, đòi tool ngoài bộ persona, tham số lạ, lặp vô hạn, làm theo lệnh giả trong kết quả tool, đề xuất route ngoài danh sách | `llm/fake.py` | 0, chạy trong CI |
| **Test an toàn** | dùng fake provider, khẳng định guard/gateway chặn đúng (không dựa vào việc mô hình ngoan) | `tests/safety/` | 0 |
| **Bộ vàng** | câu hỏi + đáp án/route kỳ vọng, chạy mô hình thật, báo cáo so với ngưỡng E2, dữ liệu cố định bằng seed | `evals/golden/`, `run_evals.py` | có, chạy theo yêu cầu/hằng tuần |
| **Kịch bản sự cố** | tái hiện lỗi trên môi trường thử, ground truth từ `docs/playbooks/` (§8.2) | `evals/scenarios/` | có, chạy theo yêu cầu |
| **Replay** | chạy lại một `run_id` đã lưu (cùng đầu vào, cùng kết quả tool đã ghi) để gỡ lỗi và so sánh khi đổi prompt/mô hình | `evals/replay.py` | thấp (có thể dùng kết quả tool đã ghi, chỉ gọi lại mô hình) |

Thiếu phần này thì không biết mỗi lần chỉnh prompt hay đổi mô hình là tốt lên hay tệ đi; đây là phần dễ bị bỏ sót nhất.

### 8A.3 Không làm
Harness tổng quát hỗ trợ mọi framework và mọi nhà cung cấp. Chỉ cần registry + factory ở `llm/` (§13) đủ để đổi nhà cung cấp (M1). Công cụ quan sát bên ngoài (LangSmith, Langfuse…) chỉ thêm khi kho lượt chat không đủ.

### 8A.4 Thứ tự
Fake provider + test an toàn + khung `run_evals.py` thuộc **bước 0** (xong trước khi nối mô hình thật). Bộ vàng buyer viết ở bước 0–1. Kịch bản sự cố và replay thuộc **bước 4**. Mỗi bước sau thêm bộ vàng/kịch bản của persona đó rồi mới được coi là xong.

## 9. An toàn (tóm tắt các điều không thương lượng)

1. Quyền nằm ở gateway/ops-service, không ở prompt. Đổi prompt không bao giờ mở thêm quyền.
2. Không có tool nào cho agent: tự thực thi route ghi, duyệt/thực thi đề xuất ops, quản lý tài khoản, đổi mật khẩu, chạy lệnh/SQL tự do, tải URL tuỳ ý.
3. Mọi văn bản từ tool, sản phẩm, review, log, tin nhắn là **không tin cậy**: đánh dấu trong prompt, không bao giờ coi là chỉ thị. Log (Loki) có thể chứa dữ liệu do người dùng điền: coi như nguy cơ injection cho agent dev.
4. Secret không vào prompt, log chat, hay kho lượt chat. `redact.py` chạy cả chiều vào (kết quả tool) lẫn chiều ra (câu trả lời).
5. Đề xuất ghi: gói ký, hạn ngắn, dùng một lần, gắn chặt tham số (đã có). Mô hình không bao giờ thấy hay tạo khoá.
6. Giới hạn cứng ở server cho vòng, thời gian, token, tiền; vượt thì dừng.
7. Dấu vết đầy đủ theo `run_id`: gateway log, bảng sự kiện (`agent_run_id`), kho lượt chat, audit log của ops.
8. Công tắc tắt (từng persona), đường lui không-AI.

## 10. Rủi ro và việc cần quyết sớm

| Rủi ro | Giảm bằng |
|---|---|
| Mô hình bịa số liệu | Tool trả `as_of`/`source`; câu trả lời bắt buộc trích; bộ vàng đo; từ chối khi không có tool |
| Prompt injection qua mô tả/review/log | Khối không tin cậy, không có tool tự do, test safety, đề xuất vẫn phải qua người |
| Chi phí vượt kiểm soát | Trần token/lượt/ngày, rate limit theo người dùng, dừng cứng |
| RCA đề xuất sai, gây thêm sự cố | Catalog đóng, bậc rủi ro từ catalog, người duyệt, `verify`, không đóng khi chưa hồi phục |
| Lộ dữ liệu qua kho lượt chat/nhà cung cấp | Che dữ liệu, thời hạn giữ, chỉ admin xem, đọc điều khoản nhà cung cấp |
| Phụ thuộc một nhà cung cấp | Lớp `llm/provider.py`; đường lui không-AI |
| Phình phạm vi (làm cả bốn persona cùng lúc) | Theo thứ tự §5; mỗi bước có tiêu chí xong |
| Tài khoản `dev` bị chiếm → dùng agent để đọc vận hành | Persona dev chỉ chạm dữ liệu vận hành; đề xuất vẫn cần admin duyệt; khoá/xoá tài khoản dev đã có ở `/admin/accounts` |

## 11. Danh sách việc bước đầu (có thể bắt tay ngay)

- [ ] Chốt D1–D6, M1–M5, P1–P5, E1–E3 và ghi ADR-43 (kiến trúc `ai-agent`) khi đã chốt.
- [ ] Gateway: `scopes.go` (danh sách trắng route đọc theo role) + `GET /ai/tools` + test; bỏ `Authorization` người dùng khi gọi agent; rate limit theo `sub`.
- [ ] Dựng `services/ai-agent` theo §4, mô hình giả, kho lượt chat, deploy vào Swarm (cờ tắt mặc định).
- [ ] Dựng harness kiểm thử (§8A.2): fake provider có kịch bản, `tests/safety/`, khung `run_evals.py`.
- [ ] Viết `evals/golden/buyer.jsonl` (20–30 câu) và `tests/safety/` trước khi nối mô hình thật.
- [ ] Nối mô hình thật cho buyer, chạy bộ vàng, chỉnh prompt/tool tới khi đạt ngưỡng.
- [ ] Lặp lại cho seller, admin, dev theo §5.

## 12. Đầu việc theo service và các bước (bổ sung sau trao đổi 2026-11-14)

### 12.1 Quyết định đã chốt trong trao đổi
1. Stream hai chặng: mô hình → `ai-agent` (`stream: true`) → gateway → trình duyệt (SSE). Tool call cũng stream từng mảnh; ghép theo `index` rồi mới chạy tool. DeepSeek hỗ trợ cả hai (tài liệu chính thức `api-docs.deepseek.com/api/create-chat-completion`); chế độ suy luận không hỗ trợ `tool_choice` kiểu `required`/chỉ định tên; **đọc thêm trang Function Calling trước khi cố định vòng lặp tool**, tên mô hình theo danh sách hiện hành.
2. `run_id` (= `jti` token agent) do **gateway** cấp mỗi lần nhận `POST /ai/chat` (một lượt). `conversation_id` do **`ai-agent`** cấp (cả cuộc chat). `proposal id` do gateway cấp khi ký gói; chống dùng lại bằng Redis `SET NX` lúc xác nhận.
3. Token agent chứa danh tính (`sub`, `username`, `role`, `pwd_version`) do gateway điền và ký; agent đọc được nhưng không sửa được. Gateway truy vấn theo `sub` trong token, bỏ qua mọi id người dùng do mô hình điền.
4. Chat chưa có chống gửi trùng: hai request = hai `run_id` = hai lần gọi mô hình. Xử lý bằng `client_message_id` (P3).
5. **UI không tin `summary`** của đề xuất (do agent viết): phải tự lấy chi tiết (đơn, số tiền) từ `method`/`path` đã ký để hiện cho người dùng xác nhận.
6. Khi agent gọi tool, UI hiện nhãn thân thiện ("Đang tra cứu đơn…") từ sự kiện `tool_call`, không hiện tham số/kết quả thô.

### 12.2 Việc theo service

**api-gateway (hạ tầng, làm trước, không cần LLM)**
- [x] (2026-11-14, chỉ role buyer; seller/admin/dev thêm cùng persona) `internal/agent/scopes.go`: bảng role → tool → (method, route), một nguồn sự thật; middleware nhận token agent ở route đọc theo bảng; từ chối ở `/auth/*`, route ghi, `/ai/chat`.
- [x] `GET /ai/tools` trả cùng bảng cho `ai-agent`.
- [x] (khi có `AGENT_TOKEN_KEYS`) Ngừng gửi `Authorization` của người dùng sang `ai-agent` sau khi route đọc nhận token agent.
- [x] Rate limit `/ai/*` theo `sub` (fail-closed) và thu hồi token agent theo `pwd_version`. Còn: giới hạn đồng thời (2 lượt/người).
- [x] Test: bảng tool trỏ route thật và đúng role (`router_test.go`); token agent 403/401 ở route cấm.

**ai-agent (bạn tự xây, theo §4–§5)**
- [ ] Bước 0: khung, `/health` `/ready` `/metrics`, Postgres riêng, bảng `conversations`, `chat_runs`, `chat_messages`, `tool_calls`.
- [ ] `POST /chat` (SSE): sự kiện `start`, `token`, `tool_call`, `tool_result`, `card`, `citation`, `action_proposal`, `error`, `done`; luôn kết thúc bằng `done`/`error`; dừng khi client ngắt.
- [ ] Sinh `conversation_id`; kiểm quyền sở hữu mỗi lần; một lượt tại một thời điểm; bỏ tin trùng theo `client_message_id`.
- [ ] `config/` + `llm/` theo §13 (profile, role, registry, factory, provider `fake`, `metrics.py`, `budget.py`); ghép tool call stream theo `index`.
- [ ] `tools/redact.py`, `guard/`, chính sách tĩnh trong `policies/buyer/*.md` (B-4).
- [ ] Persona buyer (chỉ đọc), bộ vàng + test an toàn (§8A).

**frontend**
- [ ] Đọc stream bằng `fetch` (không `EventSource`); hiển thị `tool_call` dạng nhãn; thẻ `card`; lưu `conversation_id`, sinh `client_message_id`.
- [ ] Thẻ xác nhận: hiện chi tiết lấy từ `path` đã ký (không từ `summary`), gửi lại request kèm `X-Agent-Proposal`, xử lý 401/403/409/503.

**deploy**
- [ ] `services.yml`: service `ai-agent`, Postgres hoặc schema riêng, `AI_SERVICE_URL`, biến của agent (khoá API mô hình). `AGENT_TOKEN_KEYS` chỉ đặt cho gateway, **không** cho `ai-agent`.
- [ ] Runbook: mục vận hành agent (cờ tắt, ngân sách, xoá kho chat theo hạn giữ).

### 12.3 Thứ tự gợi ý cho buyer
1. Gateway: `scopes.go` + `GET /ai/tools` + bỏ `Authorization` (mình có thể làm, thuộc hạ tầng).
2. Chốt B-1…B-12, viết nội dung chính sách (B-4) và 20–30 câu vàng (B-12).
3. `ai-agent` khung + fake provider + test an toàn.
4. Nối mô hình thật, chạy bộ vàng, chỉnh tới khi đạt ngưỡng.
5. Frontend: tool_call, thẻ, `conversation_id`.
6. Bật `FEATURE_AI_CHAT` cho nhóm nhỏ, theo dõi, rồi mở rộng.

## 13. Cấu hình và LLM profile (mẫu lấy từ dự án `time-aware-agentic-memory`)

Tham chiếu: `src/tam/config/` và `src/tam/llm/` của dự án đó (`/home/tmkhoa-1812/CodingSpace/HUST/DATN/TimeAwareAgenticMemory/time-aware-agentic-memory`, README trong từng thư mục). Mình giữ ý tưởng, bỏ phần chỉ dành cho luận văn (cache SQLite, structured output của TAM).

### 13.1 Cấu hình: yaml cho cấu trúc, env cho secret
Cập nhật điều đã nói ở mục "mỗi service một file config": `ai-agent` dùng **yaml theo môi trường** cho phần không bí mật, **biến môi trường** cho secret.
- `configs/config.{APP_ENV}.yaml` chứa: profile LLM, ánh xạ role → profile, giới hạn (vòng, thời gian, token), cờ tính năng. Chỗ cần secret ghi `${DEEPSEEK_API_KEY:-}`.
- Loader: nạp `.env` → chọn file theo `APP_ENV` → thay `${VAR}` (bắt buộc) / `${VAR:-mặc_định}` → trả `Config` (đọc bằng `cfg.llm.profiles.x`). Thiếu biến bắt buộc thì lỗi nêu rõ khoá.
- **Che secret:** khoá tên chứa `key/secret/token/password` bị che thành `********` khi in hay ghi `config.resolved.yaml`. Không bao giờ log cấu hình chưa che.
- Giá trị thật của secret nằm ở `deploy/.env` / Swarm secret, truyền vào container qua `services.yml` (như các service Go). Yaml được đóng vào image hoặc mount chỉ đọc.
- `AGENT_TOKEN_KEYS` không xuất hiện ở đây (chỉ gateway giữ).

### 13.2 Hai tầng: profile và role
```yaml
llm:
  profiles:                         # tên dễ nhớ → cách kết nối một mô hình cụ thể
    deepseek_flash:
      provider: "openai"            # API tương thích OpenAI, trỏ base_url sang DeepSeek
      model_id: "deepseek-flash"    # chốt theo danh sách hiện hành trên api-docs.deepseek.com
      api_key: "${DEEPSEEK_API_KEY:-}"
      base_url: "https://api.deepseek.com"
      temperature: 0
      timeout: 60
      extra_body: {thinking: {type: "disabled"}}   # tuỳ chọn: tắt suy luận cho rẻ và nhanh
      stream_usage: true            # streaming trả kèm token vào/ra (để đo và áp trần)
      limits: {rpm: 60, tpm: 200000}   # đọc từ tài liệu nhà cung cấp, không suy từ log
    claude_haiku: {provider: "anthropic", model_id: "...", api_key: "${ANTHROPIC_API_KEY:-}"}
    local_ollama: {provider: "ollama", model_id: "<điền>", base_url: "${OLLAMA_URL:-http://localhost:11434}"}
    fake:         {provider: "fake"}               # CI và test an toàn, không gọi mạng
  roles:                            # chỗ dùng → tên profile
    buyer:    "deepseek_flash"
    seller:   "deepseek_flash"
    admin:    "deepseek_flash"
    dev_rca:  "deepseek_flash"      # RCA có thể trỏ sang mô hình mạnh hơn
    summarizer: "deepseek_flash"    # tóm tắt lịch sử chat khi dài
    judge:    "claude_haiku"        # chấm bộ vàng (chỉ chạy eval)
```
- **Profile** = một mô hình đã cấu hình xong (nhà cung cấp, model, khoá, tham số). Nhiều profile có thể cùng nhà cung cấp (ví dụ hai mô hình DeepSeek, hay DeepSeek trực tiếp và qua proxy).
- **Role** = chỗ trong hệ thống cần một LLM (mỗi persona, RCA, tóm tắt, judge). Đổi mô hình của một persona = đổi một dòng trong `roles`, không đụng code.
- Nhờ hai tầng này, **nhiều bên dùng chung một factory** nhưng mỗi bên có profile riêng: persona nào dùng mô hình nào, môi trường nào (dev dùng `fake`/mô hình rẻ, prod dùng mô hình thật), eval dùng judge khác.

### 13.3 Registry + factory (pattern)
- `llm/registry.py`: dict `LLM_PROVIDERS` (tên provider → **hàm** `build(profile)`); decorator `@register_llm("openai")` ghi hàm vào dict lúc file provider được import (chưa tạo mô hình nào). Đăng ký trùng tên thì lỗi.
- `llm/providers/*.py`: mỗi file một hàm `build(profile) -> chat model` gắn `@register_llm("...")`. Thêm nhà cung cấp mới = một file + một dòng import trong `providers/__init__.py` + một profile trong yaml; **factory không phải sửa**.
- `llm/factory.py`: `get_llm(cfg, profile_name)` tra profile, tra provider trong dict, gọi `build`, bọc lỗi cấu hình kèm tên profile; `get_llm_for_role(cfg, role)` tra `llm.roles[role]` rồi gọi `get_llm`.
- **Lỗi cấu hình chỉ báo khi profile thực sự được dùng** (api_key để `${VAR:-}` thì thiếu chỉ lỗi ở profile đó), nên có thể khai báo sẵn nhiều profile mà chưa có khoá.
- `_common.require()` báo thiếu/rỗng khoá; `model_id()` từ chối giá trị mẫu kiểu `<điền model id>` còn sót.
- **Tiêm phụ thuộc:** `runtime/loop.py` và các persona **nhận mô hình qua tham số**, không tự gọi factory. Chỉ chỗ lắp ráp (`main.py`) gọi `get_llm_for_role`. Test truyền `fake`. Đây là nền của harness kiểm thử ở §8A.
- Kết quả `build` là `BaseChatModel` của `langchain-core` (đã chốt, lý do ở §14): có sẵn `bind_tools`, `stream`/`astream`, nên persona không phụ thuộc SDK của nhà cung cấp.

### 13.4 Những chi tiết đáng mang theo từ dự án mẫu
| Chi tiết | Vì sao hữu ích cho `ai-agent` |
|---|---|
| `extra_body` (profile OpenAI-tương thích) | Gửi tham số riêng của nhà cung cấp, ví dụ bật/tắt thinking của DeepSeek |
| `stream_usage: true` | Qua `base_url` tuỳ chỉnh, streaming mặc định không trả `usage`; cần để đếm token và áp trần ngân sách (M3) |
| `default_headers`, `timeout` | Một số nhà cung cấp đòi User-Agent riêng; timeout riêng cho mỗi profile để kết nối treo không treo cả lượt |
| Cờ riêng cho profile (ví dụ `structured_output: false`) | Nhà cung cấp không hỗ trợ tính năng nào thì tắt ở profile, không sửa code |
| Đo mỗi lần gọi (`metrics.py`) | Ghi E2E, token vào/ra, token cache, lỗi kèm mã HTTP, theo `run_id`; cơ sở cho dashboard §5 bước 7 và `evals` |
| Đo streaming: TTFT (thời gian tới mẩu đầu), TPOT, tokens/s | Chỉ số cảm nhận của người dùng với chat stream |
| `limits` theo profile (RPM/TPM) | Lấy từ tài liệu nhà cung cấp, dùng để rate limit và quyết định xếp hàng |
| `profile` → `cache_model` | Chỉ cần khi có cache kết quả; `ai-agent` chat thì **không** cache câu trả lời của người dùng (riêng tư), chỉ cache trong `evals` |

### 13.5 Việc thêm vào checklist
- [ ] `config/loader.py` + `configs/config.dev.yaml`, `config.prod.yaml` (không secret), `.env.example`.
- [ ] `llm/registry.py`, `factory.py`, `providers/{openai,anthropic,fake}.py` (DeepSeek qua provider `openai` + `base_url`).
- [ ] Test: factory báo đúng tên profile khi thiếu khoá; role không tồn tại thì lỗi liệt kê role hiện có; `fake` dùng được trong CI; che secret khi in cấu hình.
- [ ] `metrics.py` ghi mỗi lần gọi mô hình kèm `run_id`; dashboard đọc từ đó.

## 14. Chọn thư viện: langchain-core, LangGraph, LangSmith (đã chốt 2026-11-14)

### 14.1 Mỗi thứ là gì, làm gì
| Thành phần | Làm gì | Không làm gì |
|---|---|---|
| **langchain-core** (+ gói provider như `langchain-openai`, `langchain-anthropic`) | Giao diện chung cho mô hình chat (`BaseChatModel`): `invoke`, **`stream`/`astream`** (trả `AIMessageChunk`, tool call đến từng mảnh, cộng các chunk lại để ghép đủ tên + tham số), `bind_tools` (khai báo tool theo schema), kiểu tin nhắn, callback đo lường. DeepSeek dùng `ChatOpenAI` + `base_url` | Không tự quyết vòng lặp, không giữ trạng thái, không lưu gì |
| **LangGraph** | Điều phối workflow dạng **đồ thị có trạng thái**: nút, cạnh có điều kiện, quay lại, **checkpoint** (lưu trạng thái giữa chừng, chạy tiếp sau), **interrupt** (dừng chờ người rồi tiếp tục), nhiều chế độ stream | Không thay mô hình, không thay quyền/guard; thêm khái niệm phải học |
| **LangSmith** | Nền tảng **quan sát và đánh giá**: ghi trace từng bước (prompt, tool, token, độ trễ), tập dữ liệu, chạy eval, so sánh phiên bản prompt | Không cần cho chạy; là dịch vụ ngoài (SaaS, bản tự host trả phí) nhận nội dung hội thoại |

Câu hỏi "LangChain có stream không?": có. `llm.stream(...)` / `astream(...)` trả từng chunk chữ; tool call cũng là chunk (`tool_call_chunks`) và phải ghép trước khi chạy. Qua `base_url` tuỳ chỉnh cần đặt `stream_usage: true` để streaming trả token (§13.4).

### 14.2 Lựa chọn
1. **Dùng LangChain (`langchain` + `langchain-core` + gói provider) cho lớp mô hình VÀ vòng lặp agent của buyer/seller/admin: dựng bằng `create_agent` + middleware** (đổi quyết định ngày 2026-11-14 theo yêu cầu chủ dự án; trước đó là vòng lặp tự viết). Lý do: đã quen, có sẵn stream, `bind_tools`, giới hạn gọi, tóm tắt, che PII, retry; đổi nhà cung cấp chỉ đổi profile (§13). Ghim phiên bản (API đổi nhanh: tên stream, middleware). **Mọi lời gọi tool vẫn đi qua một hàm bọc của mình** (kiểm quyền, kiểm tham số, che dữ liệu, ghi log), vì quyền thật ở gateway và điều này không phụ thuộc phiên bản thư viện.
2. **Tự viết chỉ những phần LangChain không có:** bộ chuyển sự kiện sang SSE của mình (§14.4), nạp/lưu lịch sử từ Postgres của mình, ngân sách token, kiểm tra sở hữu cuộc chat. Không dùng `HumanInTheLoopMiddleware` cho buyer/seller/admin (bước duyệt nằm ngoài agent, ADR-42).
3. **LangGraph cho RCA (bước 6) và dev (bước 4 nếu RCA nhiều nhánh):** cần nhánh (đủ chắc / cần thêm bằng chứng), quay lại có trần, **dừng chờ người duyệt rồi chạy tiếp, có thể sau nhiều giờ** (checkpoint + interrupt khớp đúng việc này), nên đáng dùng. Checkpoint lưu trong Postgres của `ai-agent`. Quyết định chốt ở đầu bước 6 sau khi thử vẽ đồ thị RCA; nếu hoá ra chỉ là vòng lặp có điều kiện thì không dùng.
   **Vì sao buyer/seller/admin không cần checkpoint dù có "phê duyệt":** với họ, bước duyệt nằm **ngoài agent**: agent phát `action_proposal` rồi kết thúc lượt; người dùng bấm xác nhận trên UI và UI gọi route thật bằng token thật (ADR-42); nói tiếp thì là lượt chat mới, lịch sử lấy từ kho chat. Không có gì để "dừng rồi chạy tiếp". RCA khác: agent phải sống qua lúc chờ admin khác duyệt (có thể nhiều giờ) rồi chạy `verify`, nên cần checkpoint + interrupt (hoặc cách thay thế: mỗi giai đoạn là một lượt chạy riêng, trạng thái điều tra lưu ở incident của ops-service, ops-service gọi agent khi proposal được duyệt/thực thi). Dùng LangGraph cho cả bốn persona vẫn hợp lệ nếu muốn một cách viết thống nhất; đổi lại là code dài hơn vòng lặp tuyến tính, guard vẫn là code của mình, và phải học thêm khái niệm chưa cần. Quyết định hiện tại (tự viết cho ba persona đầu) có thể đổi.
4. **LangSmith: không bật.** Trace chứa nội dung chat, tên, địa chỉ, đơn hàng của khách và gửi ra dịch vụ ngoài (M4, P4). Đã có kho lượt chat (`run_id`), `metrics.py`, `evals/` tự viết và Grafana. Nếu muốn dùng trong lúc phát triển thì chỉ với **dữ liệu giả** (`scripts/datagen`), và bảo đảm tracing tắt ở môi trường thật (kiểm biến `LANGSMITH_TRACING`/`LANGCHAIN_TRACING_V2` không được đặt trong `deploy/.env`; thêm kiểm tra lúc khởi động). Muốn có giao diện xem trace tự host thì cân nhắc công cụ mã nguồn mở khác sau.

### 14.3 Việc thêm vào checklist
- [ ] Ghim phiên bản `langchain`, `langchain-core`, `langchain-openai` (và provider khác nếu dùng) trong `pyproject.toml`.
- [ ] Test: ghép tool call stream theo `index` thành tham số đủ, kể cả tham số bị cắt giữa chunk.
- [ ] Khởi động `ai-agent` báo lỗi nếu biến tracing của LangSmith được đặt ở prod.
- [ ] Đầu bước 6: thử vẽ đồ thị RCA rồi chốt dùng LangGraph hay vòng lặp tay.

### 14.4 Dùng `create_agent` cho buyer/seller/admin: khớp yêu cầu và điểm vướng
Đối chiếu với tài liệu LangChain hiện hành (`docs.langchain.com/oss/python/langchain/agents` và `/middleware/built-in`, đọc 2026-11-14). **Không có điểm chặn**; có vài chỗ cần tự viết hoặc phải thử trước.

| Yêu cầu của mình | Dùng gì trong LangChain | Ghi chú |
|---|---|---|
| Tối đa 4 vòng gọi mô hình | `ModelCallLimitMiddleware(run_limit=4, exit_behavior=...)` | thử `end` vs `error`: người dùng thấy gì khi chạm trần |
| Giới hạn số lần gọi tool | `ToolCallLimitMiddleware(run_limit=..., tool_name=...)` | |
| Mỗi persona một bộ tool đóng | một agent cho mỗi persona (`tools=` cố định khi dựng) | mô hình không gọi được tool ngoài danh sách |
| Truyền token agent, `sub`, `run_id` cho tool | `context_schema` + `runtime.context` | token không vào prompt |
| Che dữ liệu nhạy cảm | `PIIMiddleware` (email, ip, url, thẻ; mẫu tự định nghĩa cho số điện thoại VN, địa chỉ) | **bật `apply_to_tool_results=True`** (mặc định tắt) |
| Lịch sử dài | `SummarizationMiddleware` | |
| Lỗi tạm thời của mô hình/tool | `ModelRetryMiddleware`, `ToolRetryMiddleware`, `ModelFallbackMiddleware` | fallback = profile khác (§13) |
| Tool theo schema, kiểm tham số | công cụ định nghĩa bằng Pydantic | kiểm thêm ở hàm bọc |
| Timeout 5 s/tool, 60 s/lượt | đặt trong hàm bọc (httpx timeout) và `asyncio.timeout` quanh cả lượt | không có sẵn |
| Tắt tracing LangSmith | không đặt biến tracing; kiểm lúc khởi động (§14.3) | |

**Chỗ cần tự viết hoặc thử (spike ở bước 0, làm trước khi nối mô hình thật).** Cột "LangChain có" theo tài liệu đọc ngày 2026-11-14 (trang agents, streaming, middleware tuỳ biến, middleware dựng sẵn); chỗ chưa tài liệu hoá ghi rõ.

| # | Việc | Vì sao phải tự viết | LangChain có | LangChain thiếu |
|---|---|---|---|---|
| 1 | Chuyển stream → SSE của mình (`start`, `token`, `tool_call`, `tool_result`, `card`, `citation`, `action_proposal`, `done`) | Tên và hình dạng sự kiện là hợp đồng giữa mình, gateway và UI, không phải của thư viện | `stream_mode` `messages` (chữ + `tool_call_chunks`), `updates` (tool call hoàn chỉnh, `ToolMessage`), `custom`; `stream_events(version="v3")` có phép chiếu `tool_calls`; `get_stream_writer()` phát sự kiện tuỳ biến từ trong tool | không có định dạng SSE, không có sự kiện `card`/`action_proposal`/`start`; phải ghép và đổi tên; kiểu stream đổi theo phiên bản (`v2`/`v3`) |
| 2 | Lịch sử cuộc chat | Mình cần bảng riêng: kiểm sở hữu theo `sub`, tra theo `run_id`, thời hạn giữ, che dữ liệu | `checkpointer` + `thread_id` nhớ hội thoại | không kiểm "cuộc chat này của ai", không tra theo `run_id`, lưu cấu trúc nội bộ khó truy vấn; dùng cả hai sẽ trùng và lệch → **không dùng checkpointer** |
| 3 | `card` cần dữ liệu đầy đủ, mô hình chỉ thấy bản đã che | Hai bản của cùng một kết quả tool đi hai hướng khác nhau | `wrap_tool_call` (bọc mỗi lần gọi tool, trả `ToolMessage`/`Command`), `get_stream_writer()` để phát `card`; `PIIMiddleware(apply_to_tool_results=True)` che bản đưa cho mô hình | không có sẵn "gửi bản đầy đủ cho UI, bản che cho mô hình"; mẫu số điện thoại/địa chỉ Việt Nam phải tự định nghĩa |
| 4 | **Trần token** (mỗi lượt, mỗi người, mỗi ngày) | Chi phí là rủi ro thật (M3); số lần gọi ≠ số token | `ModelCallLimitMiddleware`/`ToolCallLimitMiddleware` (đếm lần gọi); `max_tokens` của mô hình (trần **một câu trả lời**); `usage_metadata` có trên `AIMessage` sau mỗi lần gọi; hook `after_model` + `jump_to: "end"` để dừng sớm | **không có middleware trần token** (diễn đàn LangChain có đề xuất `TokenBudgetMiddleware`, chưa phải tính năng có sẵn); không có bộ đếm theo người dùng/ngày (cần Redis hoặc Postgres của mình); streaming qua `base_url` cần `stream_usage: true` mới có `usage` |
| 5 | Timeout 5 s mỗi tool, 60 s mỗi lượt | Quy tắc vận hành của mình | `ToolRetryMiddleware`/`ModelRetryMiddleware` (thử lại, backoff) | không có timeout theo tool hay theo lượt; đặt `httpx` timeout trong hàm bọc và `asyncio.timeout` quanh cả lượt |
| 6 | Kiểm tool/tham số/quyền | Quyền thật ở gateway, nhưng cần chặn sớm và ghi log | tool khai báo Pydantic (kiểm tham số); mỗi persona một danh sách tool cố định; `wrap_tool_call` | không biết route nào của gateway cho role nào: bảng đó là `scopes.go` của mình |
| 7 | Ghi kho lượt chat, đo mỗi lần gọi, `run_id` | Kho và `agent_run_id` là của mình | callback, `before_agent`/`after_agent`, `wrap_model_call`; `usage_metadata` | không có lưu trữ; `runtime.context` (truyền `run_id`) có trong `create_agent` nhưng trang middleware tuỳ biến **chưa tài liệu hoá** cách đọc: xem tài liệu tham chiếu API |
| 8 | DeepSeek + tool + chế độ suy luận | Phụ thuộc nhà cung cấp | `ChatOpenAI` + `base_url`, gói `langchain-deepseek` | chưa kiểm chứng việc chuyển lại phần suy luận giữa các vòng tool; thử, hoặc tắt thinking bằng `extra_body` |
| 9 | Mô hình giả có tool call | Test an toàn không gọi mạng (§8A) | lớp mô hình giả có sẵn trong langchain-core | phải xác nhận `bind_tools` và `tool_calls` theo kịch bản chạy được; không thì viết lớp giả |
| 10 | Một lượt tại một thời điểm, `client_message_id`, `conversation_id` | Quy tắc của dịch vụ | không có | ngoài phạm vi của thư viện |

Phiên bản: các tên (`stream_events`, `context_schema`, middleware) còn đổi; ghim phiên bản và đọc changelog khi nâng.

**Không đổi:** quyền ở gateway, token agent, gói đề xuất ký, một lượt tại một thời điểm cho mỗi cuộc chat, `client_message_id`: tất cả vẫn là code của mình ngoài LangChain.

### 14.5 Rate limit nhiều lớp cho `/ai/*` (đã chốt hướng, chưa làm)
Hiện `/ai/*` giới hạn theo IP (`AI_RATE_LIMIT_PER_MINUTE`, khoá Redis theo `ClientIP()`), cùng middleware với route khác và **cho qua khi Redis lỗi**. Không đủ cho AI: nhiều người chung IP bị chặn lẫn nhau, một người đổi IP thì lách được, và đếm request không đếm chi phí.

| Lớp | Giới hạn (số liệu gợi ý, chỉnh khi đo) | Ở đâu |
|---|---|---|
| IP thô | ~60/phút, trước xác thực | gateway (đã có kiểu này) |
| Người dùng (`sub`) | ~10 tin/phút, khoá `rate_limit:ai:user:<id>`, đặt **sau** bước xác thực | gateway |
| Đồng thời | 1 lượt chạy mỗi cuộc chat, 2 lượt mỗi người | gateway (Redis) + `ai-agent` |
| Token theo người dùng/ngày | trần token/ngày | `ai-agent` (có `usage_metadata`) |
| Chi phí toàn hệ thống/ngày | trần cứng; vượt thì tắt tính năng kèm thông báo | `ai-agent` + cờ `FEATURE_AI_CHAT` |
| `POST /ai/proposals` | theo `sub` trong token agent, thấp (~5/lượt) | gateway |

Sửa so với middleware hiện có: tuyến AI **từ chối khi Redis lỗi** (fail closed) vì đây là nơi tiền bị tiêu; khoá theo `sub` thay vì IP ở các lớp người dùng. Việc nằm ở gateway (hạ tầng), làm cùng `scopes.go`.

### 14.6 Tách hai kết quả của một tool: bản đầy đủ cho UI, bản rút gọn cho mô hình (đã chốt)
Mỗi kết quả tool đi hai hướng. Hàm bọc tool (`wrap_tool_call`) gọi gateway bằng token agent, nhận **kết quả gốc**, rồi:
- **`card_view(gốc)`** → sự kiện SSE `card` → UI vẽ thẻ (mã, trạng thái, tổng tiền, tên hàng, ảnh…). **Không đi qua mô hình.**
- **`model_view(gốc)`** → `ToolMessage` đưa vào ngữ cảnh mô hình: bỏ địa chỉ, số điện thoại, tên người nhận, ảnh; che phần còn nhạy cảm; cắt còn N dòng; thêm `as_of`/`source`.

```
mô hình đòi tool → hàm bọc → gateway (token agent) → KẾT QUẢ GỐC
                                  ├─ card_view  → SSE `card` → UI (không qua mô hình)
                                  └─ model_view → ToolMessage → mô hình → SSE `token` → UI
```

**Khi nào che (bốn điểm):** (1) ở hàm bọc, ngay sau khi gateway trả và **trước** khi vào ngữ cảnh mô hình; (2) quét câu trả lời cuối của mô hình (`PIIMiddleware` với `apply_to_output`); (3) trước khi lưu DB (chỉ lưu bản đã che trong `tool_calls`); (4) trước khi ghi log.

| Dữ liệu | Đi tới | Không đi tới |
|---|---|---|
| Kết quả gốc | bộ nhớ hàm bọc trong lượt chạy, rồi `card_view` | DeepSeek, DB chat, log |
| `card_view` | SSE `card` → UI | mô hình |
| `model_view` | ngữ cảnh mô hình, DB chat | UI |

Hỏi trực tiếp thông tin cá nhân ("giao về địa chỉ nào?"): mô hình không thấy địa chỉ nên gọi tool hiển thị, địa chỉ hiện trên **card**, câu trả lời chỉ dẫn người dùng nhìn card.
Lợi ích: (a) dữ liệu cá nhân không rời hệ thống; (b) số tiền/trạng thái trên card lấy thẳng từ server nên mô hình không bịa được (khi chữ khác card thì card đúng); (c) prompt injection không lấy được dữ liệu mà mô hình chưa từng thấy.
**Lưu trữ:** chỉ lưu tham chiếu của card (loại + id), không lưu nội dung đầy đủ; mở lại cuộc chat cũ thì UI gọi API thường bằng token thật để lấy dữ liệu mới.
**Cài đặt:** mỗi tool có hai hàm ánh xạ (`card_view`, `model_view`) ở `tools/views.py`. Cơ chế LangChain có thể dùng: tool `response_format="content_and_artifact"` (`content` cho mô hình, `artifact` đi kèm `ToolMessage` nhưng không gửi cho mô hình; **chưa kiểm chứng trong tài liệu hiện hành**, thử ở spike bước 0). Dự phòng: `get_stream_writer()` phát `card` từ trong tool.

### 14.7 Chốt: ba tầng (LangChain / bọc của mình / ngoài thư viện)
| Tầng | Gồm | Ai viết |
|---|---|---|
| Thư viện | `create_agent` (vòng lặp, chạy trên LangGraph), `langchain-core` + `langchain-openai` (mô hình, stream, `bind_tools`), middleware dựng sẵn (call limit, PII, tóm tắt, retry, fallback) | LangChain |
| Bọc và mở rộng bằng hook của thư viện | middleware tuỳ biến (`before_agent`, `after_model`, `wrap_tool_call`), hàm bọc tool, `tools/views.py`, trần token, timeout, bộ chuyển SSE | mình, **dựa trên** thư viện |
| Ngoài thư viện | lịch sử chat ở Postgres + kiểm sở hữu, `conversation_id`/`client_message_id`, một lượt tại một thời điểm, kho lượt chat, `scopes.go` và quyền ở gateway, rate limit, ADR-42 | mình |

## 15. Hành động ghi mà agent được đề xuất, theo persona (ghi 2026-11-14)

Danh sách đề xuất được (`proposable` trong `services/api-gateway/internal/agent/proposal.go`) là danh sách đóng: route nào không có ở đây thì `/ai/proposals` trả 403. Mỗi route trong danh sách phải gắn middleware `confirm` (test `TestEveryProposableRouteHasTheConfirmGate`). Quyền thật vẫn do route kiểm bằng JWT người dùng lúc xác nhận.

### 15.1 Đã có trong danh sách (8 route)
| Persona | Hành động | Route |
|---|---|---|
| buyer | huỷ đơn của mình | `POST /orders/:id/cancel` |
| buyer | yêu cầu trả hàng | `POST /orders/:id/return` |
| seller | huỷ đơn của shop | `POST /seller/orders/:id/cancel` |
| seller | chấp nhận trả hàng | `POST /seller/orders/:id/return/approve` |
| seller | từ chối trả hàng | `POST /seller/orders/:id/return/reject` |
| seller | điều chỉnh tồn kho (`delta`, `reason`) | `POST /seller/products/:id/inventory/adjust` |
| admin | huỷ đơn | `POST /admin/orders/:id/cancel` |
| admin | hoàn tiền | `POST /admin/payments/:id/refund` |

Chưa có agent nào dùng (agent chưa xây); đây là phần gateway đã sẵn sàng.

### 15.2 Cân nhắc, chưa làm
| Route | Quyết định hiện tại | Lý do / khi nào xem lại |
|---|---|---|
| `PATCH /products/:id/status` (seller ẩn/hiện sản phẩm), `PATCH /admin/products/:id/status` | Để sau | cần thêm method PATCH vào danh sách và thẻ xác nhận hiện đúng trạng thái đích |
| `POST /seller/orders/:id/ship`, `.../deliver` | Không đề xuất; seller bấm tay | thao tác thường ngày, nhiều đơn; bắt xác nhận từng đơn gây phiền. Xem lại nếu muốn "gửi hàng loạt" |
| `POST /admin/orders/:id/return/approve|reject`, `/admin/orders/:id/deliver` | Chưa | thêm khi làm admin ghi (bước 5) |
| `POST/PUT /products` (tạo, sửa sản phẩm) | Không | body lớn nhiều trường, người dùng khó xem hết trước khi xác nhận |
| buyer: đặt hàng, thanh toán, sửa giỏ, địa chỉ, `confirm-received` | Không cho buyer v1 | liên quan tiền hoặc dữ liệu cá nhân; xem lại sau khi buyer đọc ổn |
| `/admin/accounts/*`, `/auth/*`, `/admin/dev/*` | **Không bao giờ** | quản lý tài khoản, mật khẩu, công cụ giả lập |

Mỗi hành động thêm mới cần: một dòng trong `proposable`, `confirm` trên route, dòng trong bảng test, thẻ xác nhận trên UI (hiện chi tiết lấy từ `path` đã ký), và kiểm tra service đó ghi `agent_run_id` vào outbox (order, payment, product đã có).

## 16. Trạng thái xây `services/ai-agent` (2026-11-14)

Đã dựng (xem `services/ai-agent/README.md`): config yaml + loader; LLM profile/role/registry/factory + provider `openai` (DeepSeek, OpenCode qua `base_url`), `anthropic`, `fake` (có kịch bản); persona buyer, seller; tool sinh từ `GET /ai/tools`; hàm bọc tool tách card/model_view; `POST /chat` SSE; giới hạn vòng/tool/thời gian/token; kho trong bộ nhớ; Dockerfile + service trong `deploy/services.yml` + `scripts/build-images.sh ai-agent`. Phía gateway: bảng route đọc của seller (`seller_admin`, `seller_employee`) ở `agent/scopes.go`, route đọc seller nhận token agent.

**Đã kiểm (Python 3.10, langchain-core 1.6.9):** 36 test chạy qua: config/env/che secret, factory + lỗi nêu tên profile, mô hình giả (invoke/stream/ghép tool call), tool (schema cấm tham số lạ, không thoát được route, che dữ liệu, card đầy đủ vs bản mô hình, lỗi → văn bản), token agent, kho (sở hữu/bận/trùng), bộ chuyển SSE, cả lượt chat bằng agent giả (thứ tự sự kiện, token/timeout/hết hạn, lưu kho).

**CHƯA kiểm** (sandbox không có mạng, env chỉ có langchain-core): (1) `create_agent` + `ModelCallLimitMiddleware`/`ToolCallLimitMiddleware` (tên, tham số, dạng `astream(stream_mode=["messages","updates"])`, tên nút `model`); (2) lớp FastAPI (`tests/test_api.py` tự bỏ qua nếu thiếu fastapi); (3) provider `openai`/`anthropic` mới chỉ dựng đối tượng, chưa gọi mạng; (4) streaming của DeepSeek/OpenCode. Chạy `pytest` ở môi trường Python 3.12 đủ requirements và sửa theo kết quả trước khi nối mô hình thật. Nếu `create_agent` khác giả định, chỉ cần sửa `runtime/agent_factory.py`, `guard/limits.py`, `runtime/sse_adapter.py` (đã tách riêng).

**OpenCode (cổng mô hình):** chưa xác minh streaming của endpoint chat/completions. Tìm được: server của opencode phát sự kiện qua SSE `GET /event` (SDK cộng đồng, dành cho phiên làm việc của opencode, không phải API mô hình); endpoint Zen `/v1/responses` stream thiếu nhiều sự kiện vòng đời (issue anomalyco/opencode #40171, đóng "not planned"). Dùng endpoint **chat/completions** (provider `openai` + `base_url`) thay vì `/v1/responses`, và tự thử một request `stream: true` bằng khoá của bạn trước khi dựa vào nó. Hồ sơ `opencode_zen` trong yaml để trống `model_id` cho tới khi bạn điền.

**Việc tiếp theo:** chạy test thật ở 3.12; kho Postgres; `/metrics`; tool đề xuất hành động (bước 5); bộ vàng buyer/seller (`evals/`); chọn mô hình theo bộ vàng.

### 16.1 Môi trường kiểm thử đã dùng (ghi để không nhầm là đã kiểm xong)
- Mới chạy trên **Python 3.10.21** (conda env `base_python_3.10`: `langchain-core 1.6.9`, `langchain-openai 1.7.0`, `langchain-anthropic 1.7.5`, `httpx 0.28.1`, `pydantic 2.14.0`, `PyYAML 6.0.3`), bằng một runner tạm vì không có pytest. Không có env 3.12 trên máy, sandbox soạn code không có mạng (DNS tới pypi.org lỗi), và mạng của máy dev đang yếu nên chưa dựng được 3.12.
- Code cố ý không dùng tính năng chỉ có từ 3.11 trở lên trong các đường đã test, nên chạy được ở 3.10; `pyproject.toml` và Dockerfile đặt 3.12 (đích triển khai). `langchain` 1.x yêu cầu Python ≥ 3.10.
- Nếu tải cả env 3.12 quá nặng: dùng luôn env 3.10 và chỉ cài phần còn thiếu (nhỏ hơn nhiều): `pip install "langchain>=1.0,<2" langgraph fastapi "uvicorn[standard]" pytest pytest-asyncio`, rồi `cd services/ai-agent && pytest`. Hoặc tải wheel ở nơi có mạng tốt (`pip download -r requirements-dev.txt -d wheels/`) rồi cài offline (`pip install --no-index --find-links wheels/ -r requirements-dev.txt`).
- Phiên bản `==` trong `requirements.txt` là bản đã import và chạy được; dòng `~=` (`langchain`, `fastapi`, `uvicorn`, `pytest`) chưa được kiểm. Cài xong chạy `pip freeze > requirements.lock.txt`, commit, và Dockerfile sẽ tự dùng file khoá.

### 16.2 Checklist chạy lần đầu (khi đã có env đủ gói)
1. `cd services/ai-agent && pytest`. Kỳ vọng 36 test cũ đạt + `tests/test_api.py` (đang tự bỏ qua) chạy. Ca lỗi nhiều khả năng nhất: tên nút/dạng stream của `create_agent`, đối số `ModelCallLimitMiddleware`/`ToolCallLimitMiddleware`.
2. Chạy thử không cần model thật: `APP_ENV=dev PYTHONPATH=src uvicorn ai_agent.main:create_app --factory --port 8090`, rồi `POST /chat` với một token agent do gateway cấp (hoặc dùng `tests/helpers.agent_token` để tạo token thử và mock gateway). Ở dev, `llm.roles.buyer/seller` là `fake_*`.
3. Viết thêm test cho `create_agent` thật với model giả (spike ở bước 0): `ScriptedChatModel.bind_tools` có chạy trong `create_agent` không; `astream(stream_mode=["messages","updates"])` có ra đúng `(mode, data)` và tên nút `model` không; `ModelCallLimit` khi chạm trần có kết thúc êm không. Kết quả ghi vào §14.4.
4. Nối gateway thật trong dev: `FEATURE_AI_CHAT=true`, `AI_SERVICE_URL`, `AGENT_TOKEN_KEYS`; đăng nhập buyer, `POST /ai/chat`, xem đủ sự kiện `start`, `tool_call`, `card`, `token`, `done`; thử token hết hạn và tài khoản đổi mật khẩu giữa chừng.
5. Chỉ sau đó mới gán `llm.roles.buyer` sang profile thật (`deepseek_flash`), thử 1 request `stream: true` và kiểm `usage` có về không (`stream_usage`). Chưa có API model nào được gọi tới giờ.
6. Lỗi/khác biệt tìm được ghi vào đây và vào `docs/planning/01_OPEN_ISSUES.md` (V8).

### 16.3 Quyết định đã làm khi dựng (không có trong các mục trên)
- **ai-agent không kiểm chữ ký token agent** (không có khoá). Bảo vệ bằng mạng nội bộ (không publish cổng) và vì mọi lời gọi tool quay lại gateway kèm token đó (gateway kiểm chữ ký). Header `X-Agent-Run-ID` phải khớp `jti`.
- **Tool lấy từ gateway** (`GET /ai/tools`, cache theo role 60 s), không khai báo bộ tool trong code agent. Persona chỉ là prompt + tên role LLM.
- **Hàng đợi sự kiện chung** cho một lượt: token từ model và `card`/`tool_call` từ hàm bọc tool cùng vào một `asyncio.Queue`; không dùng `get_stream_writer` (tránh phụ thuộc API chưa kiểm). Hàm bọc đọc ngữ cảnh lượt qua contextvar nên token agent không vào prompt.
- **Kho lịch sử** hiện chỉ lưu chữ người dùng + câu trả lời cuối (không lưu kết quả tool): lượt sau model phải gọi lại tool nếu cần. Đổi khi làm kho Postgres.
- **Trần token** tính ở bộ chuyển SSE (cộng `usage_metadata` từng chunk), không bằng middleware (chưa có sẵn).
- **Prod từ chối khởi động nếu có biến tracing LangSmith**; dev chỉ cảnh báo.
- **Seller dùng chung bộ tool cho `seller_admin` và `seller_employee`**; hai role cùng một persona `seller`. Tách nếu cần quyền hẹp hơn cho nhân viên.


### 16.4 OpenCode và đổi tên route vận hành (2026-11-14)
- **Đổi route:** nhóm vận hành chuyển từ `/admin/ops/*` và `/admin/system/health` sang `/ops/*` và `/ops/system/health` (vẫn admin hoặc dev, ADR-40). `/admin/analytics/data-health` giữ nguyên chỗ cũ, vẫn mở cho dev. Gateway test pass; UI (`lib/api.ts`) và tài liệu đã đổi theo; log phiên cũ giữ nguyên.
- **Profile `opencode_deepseek_flash`** (dev, prod): OpenCode Go, `base_url` mặc định `https://opencode.ai/inference/go/openai/v1`, `model_id: deepseek-v4.1-flash` (theo [docs OpenCode Go](https://opencode.ai/docs/go/)). Docs yêu cầu User-Agent riêng và header `x-opencode-session` ổn định cho mỗi cuộc hội thoại: provider `openai` nay hỗ trợ khoá `session_header`, giá trị lấy từ `conversation_id` của lượt đang chạy lúc gửi request (hook httpx, đọc contextvar), nên một model dùng chung vẫn đúng phiên.
- **Streaming: ĐÃ KIỂM (2026-11-14, curl với key thật, 3 request nhỏ).** Kết quả ở đoạn "Kết quả kiểm" bên dưới. Đoạn cũ:  Docs OpenCode (Go và Zen) không nói gì về `stream`, `stream_options` hay `tools`. Chỉ có bằng chứng gián tiếp: các endpoint `chat/completions` này dùng gói `@ai-sdk/openai-compatible` mà chính OpenCode gọi để stream. Muốn chắc: một request `curl` với `"stream": true, "stream_options": {"include_usage": true}` và một request có `tools`. Chưa gọi API nào.
- Test mới (header phiên theo conversation) đã viết trong `tests/test_config_llm.py` nhưng **chưa chạy**.

**Kết quả kiểm (`POST .../go/openai/v1/chat/completions`, model `deepseek-v4.1-flash`):**
| Phép thử | Kết quả |
|---|---|
| `stream: true` + `stream_options.include_usage` | HTTP 200, SSE `chat.completion.chunk`, kết thúc `finish_reason: stop` và `data: [DONE]`; chunk cuối có `usage` (prompt/completion/total, `reasoning_tokens`). TTFB ~1,8 s, tổng ~2,4 s cho 60 token |
| `stream: true` + `tools` | HTTP 200, `tool_calls` stream từng mảnh (`name`, rồi `arguments` theo từng ký tự JSON); LangChain ghép được qua `tool_call_chunks` |
| Mặc định (thinking bật) | stream thêm `delta.reasoning_content`, chiếm phần lớn token ra (37/46 trong phép thử); `max_tokens` nhỏ dễ bị suy luận ăn hết |
| `"thinking": {"type": "disabled"}` (top-level, = `extra_body` của langchain-openai) | `reasoning_tokens: 0`, không có `reasoning_content` |
| Thiếu `x-opencode-session` | HTTP 400 `MissingSessionID` (đúng như docs) |
Hệ quả: profile bật `extra_body: thinking disabled` (dev, prod). Chưa kiểm qua `ChatOpenAI` của langchain-openai và chưa kiểm chất lượng gọi tool của model khi tắt thinking (nên đo bằng bộ câu hỏi mẫu). `usage.cost`/`opencode.list_cost` có trong chunk cuối: có thể dùng đo chi phí.

### 16.5 Kiểm trên Python 3.12 và UI chat (2026-11-14)
- **Môi trường:** venv 3.12 dựng bằng `python3.12 -m venv --without-pip` rồi `pip --python <venv> install -r requirements-dev.txt` (hệ thống không có ensurepip). Phiên bản thực tế: langchain 1.4.4, langgraph 1.2.22, fastapi 0.162.7, pydantic 2.14.0. `requirements.lock.txt` sinh từ venv này (Dockerfile dùng nếu có).
- **Test:** 45 đạt trên 3.12 (gồm test chạy `create_agent` + middleware giới hạn thật với mô hình giả có kịch bản, `tests/test_api.py` qua FastAPI, test header phiên). `helpers.cfg()` ép role LLM về mô hình giả để test không phụ thuộc key/mạng.
- **Lượt thật (OpenCode `deepseek-v4.1-flash`, thinking tắt, gateway giả):** buyer "đơn gần đây": `tool_call` → `card` → `token` từng mẩu → `done` (2586 vào / 139 ra token), token đầu ~2,8 s sau khi gửi; seller "đếm đơn theo trạng thái" tương tự (~3,2 s tới khi tool chạy, xong ~6,9 s).
- **Phát hiện:** trên Python 3.10, `agent.astream(stream_mode="messages")` trả MỘT message hoàn chỉnh (không stream) và `usage_metadata` không vào được `pump` (token = 0, trần token không áp được). Trên 3.12 stream từng mẩu và đếm token đúng. Vì vậy 3.12 là bắt buộc, không chỉ là mong muốn (`pyproject.toml`, Dockerfile đã nhắm 3.12).
- **UI:** `lib/ai-chat.ts` có `sseTransport` (fetch + ReadableStream, `parseSse` xử lý sự kiện bị cắt giữa chừng, `\r\n`, nhiều dòng `data:`), `ChatProvider` giữ `conversation_id`, áp từng sự kiện (token, tool_call → trạng thái "Đang tra cứu…", card, citation, error), `ChatCards` vẽ thẻ dữ liệu chung (bảng hoặc cặp khoá/giá trị, tiền theo VND). `AI_BACKEND_READY` nay luôn bật (chỉ hiện khi cờ gateway bật), `NEXT_PUBLIC_AI_UI_PREVIEW=true` vẫn cho bản giả. `tsc` và `next lint` sạch; `parseSse` kiểm bằng script rời. Chưa thử trên trình duyệt. Chưa có thẻ xác nhận đề xuất (chưa có tool đề xuất).

### 16.6 Lỗi thấy khi dùng thử trên Swarm (2026-11-14) và cách sửa
- **Lộ đường dẫn API:** UI hiện "Nguồn: GET /search". Sửa: sự kiện `citation` chỉ còn `{tool, label}` với `label` là tên thân thiện ("Tìm kiếm sản phẩm", `events.SOURCE_LABELS`); route không còn đi ra ngoài ai-agent. UI hiện nhãn, không hiện route.
- **Hỏi "điện thoại dưới 20 triệu" ra khung trả lời trống:** không có điện thoại nào dưới 20 triệu, model tìm 3 lần không ra rồi hết `max_model_calls` (4) khi chưa kịp viết chữ. Sửa: `max_model_calls` 6; câu trả lời dự phòng khi model kết thúc mà không có chữ ("Mình chưa tổng hợp xong…"); `prompt.md` buyer có mục "Cách tìm sản phẩm" (giá bằng VND nguyên, chỉ đặt bộ lọc khi người dùng nói rõ, không có kết quả thì thử tối đa một lần nới điều kiện rồi trả lời, dùng `sort` thay vì tự so sánh "rẻ nhất"). UI ẩn thẻ rỗng (tìm không ra) vì câu trả lời đã nói.
- **Chưa đặt/huỷ đơn qua agent:** đúng thiết kế phase đầu (chỉ đọc, D1). Huỷ đơn cần tool đề xuất (bước 5); đặt đơn chưa nằm trong kế hoạch v1.
- Ghi chú: log ai-agent cố ý không chứa nội dung chat (chỉ tên tool, token, độ trễ), kho chat nằm trong bộ nhớ, nên không có "lịch sử chat" để xem lại sau; muốn xem phải thêm kho Postgres (§5 bước 0) hoặc bật log nội dung ở dev.

### 16.7 Đề xuất hành động và kho chat Postgres (2026-11-14)
- **Trước đó chưa có tool tạo đề xuất ở ai-agent và chưa có thẻ xác nhận ở UI** (chỉ có `POST /ai/proposals` và cổng xác nhận ở gateway), nên chat không có nút Xác nhận. Nay có:
  - Gateway: `agent.DefaultActions` (role -> hành động: tên, route, tham số `path`/`body`, mẫu câu tóm tắt) là nguồn duy nhất; `GET /ai/tools` trả thêm `actions`; `POST /ai/proposals` kiểm cả vai trò (`AllowsAction`): buyer không đề xuất được route seller/admin. Có hàng cho admin (chưa có persona) để danh sách proposable và cổng xác nhận vẫn được test.
  - ai-agent: tool `propose_<hành động>` (`tools/propose.py`). Tool CHỈ xin gateway ký rồi phát `action_proposal` `{proposal, id, expires_at, summary, request{method,path,body}}`; token đề xuất không qua mô hình; câu tóm tắt do mã điền từ tham số đã kiểm (không phải mô hình viết); tối đa 2 đề xuất/lượt; lỗi gateway trả "[lỗi công cụ]"; 401 dừng lượt.
  - UI: `ChatProposals` (thẻ "Đề xuất cần bạn xác nhận": Xác nhận, Bỏ qua, đếm ngược). Xác nhận = `confirmProposal` gọi route thật bằng JWT người dùng + `X-Agent-Proposal` (CORS đã cho header này). Lỗi 401/403 hiện "hết hạn/đã dùng, hãy yêu cầu lại".
  - Kiểm: gateway test, ai-agent test (tham số, tóm tắt, sự kiện, trần, lỗi, cả lượt với `create_agent` thật), một lượt thật với OpenCode: "Huỷ giúp tôi đơn 1, tôi đặt nhầm" ra đúng đề xuất `POST /orders/1/cancel`; "Trả hàng cho tôi" (thiếu mã đơn) thì hỏi lại. Chưa thử xác nhận thật trên Swarm/trình duyệt.
- **Kho chat Postgres** (`store/postgres.py`, `store.backend: postgres`, dev và prod): bảng `ai_conversations`, `ai_messages`, `ai_runs` (cùng cụm Postgres, tự tạo lúc khởi động, khoá advisory nhiều bản sao). Lưu chữ người dùng và câu trả lời cuối đã che email/số điện thoại, trạng thái/số tool/token và tham chiếu card/đề xuất mỗi lượt; KHÔNG lưu kết quả tool, token agent, token đề xuất. Giữ 30 ngày (`retention_days`), dọn mỗi giờ. Lượt treo quá 180 s không khoá cuộc chat. Kết nối lỗi lúc khởi động thì dịch vụ không lên. 9 test cần `TEST_POSTGRES_DSN` (mỗi test tạo DB riêng), kiểm trên Postgres 16 thật: 64 đạt.
- Chưa có: giao diện/API cho admin xem lại hội thoại (hiện xem bằng SQL), mở lại cuộc chat cũ trên UI, chặn đồng thời theo người dùng.

### 16.8 Persona admin và dev, chỉ đọc (2026-11-14)
- **Gateway** (`agent/scopes.go`): `DefaultScopes["admin"]` gồm đọc toàn sàn (`/admin/orders`, `/admin/orders/:id`, `/admin/payments`, `/admin/payments/:id`, `/admin/categories`, 8 báo cáo `/admin/analytics/*`) cộng bộ ops. `DefaultScopes["dev"]` chỉ có bộ ops: `/ops/incidents`, `/ops/incidents/:id`, `/ops/audit`, `/ops/outbox/:service/dead`, `/ops/system/health`, `/admin/analytics/data-health`. Dev không có dữ liệu kinh doanh (cùng luật với route). Không có `/admin/accounts`, `/auth/*`, `/users/*`.
- **Route:** các GET trên chuyển sang nhóm `agentOr(authenticated)` (`opsAgentRead`, `adminAgentRead`) để token agent đi qua; POST/PUT/DELETE vẫn ở nhóm cũ chỉ nhận JWT người dùng. Các route `/ops` ghi (`investigate`, `proposals/*`, `queries/:name`) không nằm trong bảng.
- **Chỉ đọc thật sự:** `readOnlyRoles = {admin, dev}`: `GET /ai/tools` trả `actions: []` (agent không có tool `propose_`) và `/ai/proposals` từ chối (`CanPropose`). Hàng `DefaultActions["admin"]` giữ lại để test cổng xác nhận. Muốn admin được đề xuất sau này: bỏ role khỏi `readOnlyRoles`.
- **ai-agent:** persona `admin` và `dev` (`personas/admin|dev/prompt.md`, role LLM `admin`/`dev` trong `llm.roles`, dev và prod), nhãn tool thân thiện. Prompt nói rõ chỉ đọc, dev không có dữ liệu kinh doanh.
- **UI/cấu hình:** `FEATURE_AI_OPS=true` bật nút trợ lý ở mọi trang `/admin` cho admin và dev (cùng cờ `ai_ops`). Gợi ý câu hỏi của admin/dev đổi cho khớp tool có thật.
- **Seller:** nền tảng có hai role `seller_admin` (chủ shop) và `seller_employee` (nhân viên); agent cho cả hai cùng một persona `seller` và cùng bộ tool, không phân biệt.
- Chưa có: chạy thử thật với admin/dev trên Swarm.

### 16.9 Số liệu hệ thống, đề xuất vận hành, chính sách (2026-11-14)
- **Số liệu Prometheus/Loki cho admin và dev:** tool `ops_metrics_query` = `GET /ops/queries/:name?service=&window=&request_id=`. Route mới `OpsHandler.Query` đổi query string thành body `{"params":{...}}` rồi gọi `POST /queries/:name` của ops-service (đây là kho truy vấn có tên, cố định, chỉ đọc, ops-service kiểm enum/regex từng tham số, cắt kết quả 50 chuỗi/100 dòng log và ghi audit mỗi lần chạy với `X-Actor` là `admin:<id>` hoặc `dev:<id>`). Agent KHÔNG viết PromQL/LogQL. Danh sách tên nằm ở `agent.opsQueryNames` và phải khớp `services/ops-service/internal/queries/queries.go` (24 truy vấn; đã đối chiếu tay, chưa có test tự động vì hai module tách nhau). Hỏi "service nào chậm nhất" -> `grpc_latency_p95` và `gateway_latency_p95` (prompt hướng dẫn).
- **Đề xuất vận hành:** `ops_list_proposals`, `ops_get_proposal` (chỉ GET; duyệt/chạy vẫn là việc người dùng ở trang Hệ thống). Các GET này chuyển sang nhóm `opsAgentRead`.
- **Chính sách:** `personas/buyer/policy.md` (trước chỉ có TODO) và `personas/seller/policy.md` (prompt seller có thêm mục `{policy}`). Nội dung lấy từ mặc định thật của order-service (`ORDER_PAYMENT_TTL_MIN` 15, `RETURN_WINDOW_DAYS` 7, `AUTO_DELIVER_DAYS` 7, `SHIPPING_FLAT_FEE` 30000, `FREE_SHIP_OVER` 500000) và máy trạng thái đơn. Đã thay bằng tool `get_shop_rules`, xem §16.11 (không còn phải sửa tay khi đổi biến). Chỗ code chưa chứng minh (ngày giao dự kiến, hoàn tiền khi huỷ đơn đã trả online) ghi là "chưa có thông tin" để agent không hứa.
- Test: `TestOpsQueryTurnsTheQueryStringIntoTheUpstreamBody`, mở rộng `TestAdminAndDevAgentScopes`, `test_personas.py` (policy thật, nhãn tool).

### 16.10 Kiểm tra khi khởi động và một luồng chat mỗi người dùng (2026-11-14)
- **Khởi động:** `FEATURE_AI_CHAT` hoặc `FEATURE_AI_OPS` bật mà `AI_SERVICE_URL` rỗng hoặc `AGENT_TOKEN_KEYS` rỗng thì gateway KHÔNG chạy (`config.checkAIConfig`, lỗi nêu tên biến). Trước đây chỉ lộ ra khi có người chat (503, hoặc agent không có token). Đổi hành vi: bản triển khai bật cờ AI mà không có khoá sẽ không lên nữa; tắt cờ hoặc điền khoá.
- **Một luồng mỗi người dùng:** `handler.RedisSlots` (SET NX theo `ai_slot:<user_id>`, giá trị ngẫu nhiên, TTL = thời lượng tối đa của luồng + 10 s, xoá bằng script chỉ khi còn đúng giá trị). Luồng thứ hai: 409 `busy`, UI hiện "Trợ lý đang xử lý câu hỏi trước của bạn". Redis lỗi: 503, không cho qua (mỗi luồng tốn tiền gọi mô hình). Giới hạn này chung cho mọi replica gateway; ai-agent vẫn tự chặn trùng theo cuộc hội thoại.
- Chưa có: trần token mỗi ngày mỗi người dùng.
- Test: `TestCheckAIConfigStopsAnAIFeatureThatCannotWork`, `TestRedisSlotsAllowOneHolderPerUserAndReleaseOnlyOwn` (miniredis, gồm TTL), `TestChatRefusesASecondStreamAndFreesTheSlotAfterwards`.

### 16.11 Quy tắc đơn hàng lấy từ order-service (2026-11-14)
- **Vì sao:** số liệu chính sách (hạn thanh toán, ngày trả hàng, ngày tự "đã giao", phí ship, mức miễn phí) từng được chép tay vào `policy.md`; đổi biến môi trường là agent nói sai. Giờ chỉ còn một nguồn: cấu hình đang chạy của order-service.
- **API:** gRPC `OrderService.GetShopRules` (proto mới `ShopRules`, đã sinh lại `.pb.go` bằng `scripts/protogen/gen.sh order-service`, chỉ thêm) -> gateway `GET /shop/rules` (công khai, chỉ đọc, `Cache-Control: public, max-age=60`) trả `{payment_ttl_minutes, return_window_days, auto_deliver_days, shipping_flat_fee, free_ship_over}` (VND nguyên; 0 ở `auto_deliver_days`/`free_ship_over` là "không bao giờ").
- **Tool:** `get_shop_rules` cho buyer và seller (`agent/scopes.go`). `policy.md` của hai persona không còn con số nào, chỉ nêu luật không đổi và dặn lấy số bằng tool; test `test_policy_numbers_come_from_the_tool_not_from_the_text` chặn việc chép số lại.
- **Hai điều trước đây "chưa có thông tin" đã tra code:** (1) huỷ đơn đã trả online phát `order.refund_requested` hoàn TOÀN BỘ `total_price` (gồm phí ship) cho payment-service, hoàn xong bất đồng bộ (`order-service/internal/repository/orders.go:324`); duyệt trả hàng cũng hoàn toàn bộ, chỉ với đơn online; COD không có gì để hoàn. Đã ghi vào policy. (2) Ngày giao dự kiến: không có trường nào trong order-service, giữ "chưa có thông tin".
- Test: `TestSettingsRulesAreWholeUnitsFromTheEnvironment`, `TestDefaultRulesMatchTheDocumentedDefaults` (order-service), `/shop/rules` nằm trong `TestPublicRoutesNeedNoToken`, tool nằm trong kiểm tra "mọi hàng bảng là route thật".


### 16.12 Không lộ chi tiết nội bộ cho buyer/seller, bảng trong chat, smoke test (2026-11-14)
- **Lỗi thấy khi chạy thật:** (1) hỏi "bạn đã gọi API nào" thì buyer/seller trả ra được tên công cụ và đường dẫn, vì kết quả tool đưa cho mô hình có `source: "GET /orders/:id"` và mô hình biết tên tool; (2) bảng markdown hiện thành dấu `|` thô và `**\`ready\`**` hiện dấu backtick; (3) persona admin/dev quên bật trong `config.*.yaml` nên chat trả 403 "vai trò này chưa được hỗ trợ".
- **Nguyên tắc:** biết tên route KHÔNG phải ranh giới bảo mật. Ranh giới thật là bảng `agent/scopes.go` ở gateway (token buyer gọi route admin vẫn 403). Phần này chỉ để người dùng cuối không thấy chi tiết kỹ thuật và để giảm bề mặt bị dò. Admin/dev cố ý vẫn thấy route (họ cần nó để đối chiếu).
- **Ba lớp (`Persona.hide_internals` = true cho buyer, seller):** (a) `tools/views.py`: mô hình chỉ thấy nhãn nguồn ("Đơn hàng của bạn"), không thấy `GET /…`; (b) prompt quy tắc 6 dặn không nói về công cụ/API/đường dẫn và nêu câu trả lời mẫu; (c) `guard/leak.py` `LeakFilter` lọc TRÊN LUỒNG token (giữ đuôi 48 ký tự để bắt tên bị cắt giữa hai mẩu) và cả bản lưu vào kho: che "METHOD /path", đường dẫn chỉ có ở API (`/ai/`, `/ops/`, `/admin/`, `/auth/`, `/users/`, `/shop/rules`, có `:id`) và tên mọi tool của role (+ `propose_*`). Trang web như `/cart` không bị đụng. Đánh đổi: văn bản trễ tới 48 ký tự so với mô hình; tên tool vẫn nằm trong ngữ cảnh mô hình (function calling cần), nên lớp (c) là chốt chặn cuối chứ không thay cho (a), (b).
- **Dữ liệu nhạy cảm trong text tự do (dòng log Loki, audit):** `redact.mask_text` che thêm JWT, `Bearer …`, `password=…`/`token: …`, IPv4 (đã có email, số điện thoại). Không bắt được tên người hay địa chỉ viết thành chữ: dữ liệu như vậy phải bị bỏ theo tên khoá hoặc không cấp cho tool.
- **Frontend:** `ChatMarkdown` thêm bảng (`| a | b |` + dòng `|---|`, cuộn ngang khi hẹp), tiêu đề `#`, in đậm chứa mã. Vẫn chỉ dựng React element, không HTML thô.
- **Canh lệch cấu hình:** `test_every_persona_is_enabled_and_has_an_llm_role_in_every_config`; `scripts/smoke-ai.sh` đăng nhập từng role, hỏi một câu và kiểm tra có `event: done` (gọi LLM thật; tài khoản qua biến `SMOKE_*`).
- Test: `tests/test_guard_leak.py` (lọc, luồng bị cắt từng 1–7 ký tự, buyer bị che còn admin thì không, mask chuỗi nhạy cảm), `tests/test_api.py` (admin/dev được nhận).

### 16.13 Gọi tool tiết kiệm: sắp xếp theo tồn kho và rút gọn danh sách (2026-11-14)
- **Triệu chứng:** seller hỏi "sản phẩm nào của tôi tồn nhiều nhất" thì agent tải cả danh mục. **Hai nguyên nhân:** (1) API chỉ sắp xếp theo `relevance|price|newest|best_selling`, không có tồn kho, nên mô hình phải lật từng trang rồi tự so; (2) mỗi sản phẩm trong danh sách mang cả `description`, `attributes`, `image_urls`... nên 20 sản phẩm vượt `tool_result_max_chars` (6000), kết quả bị cắt và mô hình lật tiếp (trần `max_tool_calls` = 8).
- **Sửa:** product-service thêm `sort=stock_desc|stock_asc` (`inventory` = tồn khả dụng; CHỈ cho chủ store, route công khai trả 403 vì số tồn của người khác bị che "20+" và xếp hạng sẽ làm lộ thứ tự thật; test `TestSearchSortsByAvailableStock`, chạy với Postgres thật); gateway `list_my_products` có tham số `sort` (enum), `in_stock`, mô tả `page_size` và lời dặn dùng sắp xếp + `page_size` 5 cho câu hỏi top-N; `redact` bỏ các trường nặng (`description`, `attributes`, `image_urls`, `tags`...) trong DANH SÁCH nhiều dòng (một mục như `get_product` vẫn đủ, thẻ UI vẫn đầy đủ); prompt seller thêm mục "Cách dùng công cụ (tiết kiệm)".
- **Quy tắc chung khi thêm tool:** câu hỏi "nhất/top N" phải trả lời được bằng MỘT lần gọi có sắp xếp phía server, không bằng việc mô hình tải hết rồi so. Thiếu tham số sắp xếp ở API thì thêm ở API, đừng dạy mô hình lật trang.
