# Đánh giá agent gọi tool nhiều bước (Agent / Tool-use Eval)

> Benchmark và cách dựng bộ eval riêng cho 3 persona `shopper` / `seller` / `admin`: function calling, multi-turn, tin cậy (pass^k), từ chối đúng/vượt quyền, LLM-as-judge. **Trạng thái: chưa triển khai trong hệ thống; đây là tài liệu nghiên cứu dữ liệu.** Ngày tra cứu: 2026-10-25.
>
> Ghi chú phương pháp: phiên 1 đọc README GitHub bằng WebFetch; phiên bổ sung (cùng ngày) dùng WebSearch + WebFetch để xác minh GAIA, API-Bank, ToolSandbox, MCP-Bench và thêm các bộ mới (ShoppingBench, Shopping MMLU, AppWorld, Toolathlon, MCP-Universe, MCP-Atlas, ASB, AgentHarm, ToolEmu). HuggingFace/arXiv không mở được bằng WebFetch (DNS) nên các số liệu mới ghi "nguồn: kết quả search" kèm URL; mục nào không thấy trong kết quả vẫn ghi "chưa xác minh".
>
> Quy tắc ngôn ngữ (2026-10-26): chỉ giữ nội dung tiếng Anh/tiếng Việt. Đã LOẠI ACEBench (song ngữ Trung-Anh; chưa xác minh được phần tiếng Anh tách riêng). τ²-bench và Shopping MMLU có bản đa ngôn ngữ: CHỈ dùng phần tiếng Anh, bỏ Tây Ban Nha, Bồ Đào Nha, Hindi, Trung, Hàn. Cột "Quy mô / dung lượng" ghi số mẫu và dung lượng tải kèm nguồn; "dung lượng: không tìm thấy" = không thấy trong kết quả search (nhiều bộ là mã + dữ liệu nhỏ trên GitHub).

| Liên quan | File |
|---|---|
| Trợ lý mua sắm / CSKH | [05_SHOPPER_ASSISTANT_CUSTOMER_SUPPORT.md](05_SHOPPER_ASSISTANT_CUSTOMER_SUPPORT.md) |
| Text-to-SQL / analytics QA | [04_ANALYTICS_QA_TEXT2SQL.md](04_ANALYTICS_QA_TEXT2SQL.md) |
| Policy RAG | [06_POLICY_LEGAL_RAG.md](06_POLICY_LEGAL_RAG.md) |
| An toàn agent (prompt injection) | [12_CONTENT_MODERATION_AGENT_SAFETY.md](12_CONTENT_MODERATION_AGENT_SAFETY.md) |
| AIOps (agent admin) | [14_AIOPS_ANOMALY_ALERT_EXPLAIN.md](14_AIOPS_ANOMALY_ALERT_EXPLAIN.md) |
| Hiểu truy vấn tiếng Việt | [15_VIETNAMESE_NLP_QUERY_UNDERSTANDING.md](15_VIETNAMESE_NLP_QUERY_UNDERSTANDING.md) |

## 1. Mục tiêu và vị trí trong hệ thống

Thiết kế agent (đọc `docs/ai/05_AI_DRAFT_ORIGINAL.md`, mục "Agent & tool"): một `ai-service`, ba persona theo `role` trong JWT, tool là endpoint REST ở gateway có JSON schema, LLM không viết SQL, tool ghi chỉ trả `action_proposal` chờ người dùng xác nhận.

| Persona | Role | Tool (trích từ catalog) |
|---|---|---|
| `shopper` | buyer + khách vãng lai | `search_products`, `get_product`, `check_stock` (chỉ level), `list_categories`, `catalog_overview`, `get_my_orders`*, `get_cart`*, `add_to_cart`* (action_proposal), `ask_policy`, `get_recommendations` |
| `seller` | seller_admin, seller_employee | `store_sales_summary`, `store_sales_timeseries`, `store_top_products`, `store_low_stock`, `store_product_funnel`, `store_orders`, `search_my_products`, `ask_policy`, `list_my_alerts` (mọi tool tự ép `store_id` từ JWT) |
| `admin` | admin | `platform_sales_summary`, `platform_traffic`, `platform_funnel`, `system_metrics`, `service_health`, `list_alerts`, `top_stores`, `ask_policy`, `search_audit_log` |

Eval cần trả lời: (1) chọn đúng tool và tham số? (2) đúng nhiều bước? (3) ổn định qua nhiều lần chạy? (4) từ chối đúng khi vượt quyền/ngoài phạm vi? (5) câu trả lời đúng sự thật và có trích dẫn? Dữ liệu hệ thống nuôi eval: dữ liệu demo (`SEED_DEMO_DATA=true`), log gateway/audit (`audit_log`) làm "sự thật" để chấm, lịch sử hội thoại được lưu (nền tảng cung cấp).

## 2. Hướng làm đề xuất (baseline → nâng cao)

1. **Baseline**: bộ test đơn lượt, so khớp cuộc gọi tool (tên + tham số) với đáp án (kiểu BFCL, AST match/executable).
2. **Multi-turn có trạng thái**: mô phỏng người dùng bằng LLM + môi trường DB có trạng thái, chấm bằng **trạng thái cuối của DB** và tập quy tắc (kiểu τ-bench).
3. **Tin cậy**: chạy mỗi task k lần, báo **pass^k** (cả k lần đều đúng), không chỉ pass@1.
4. **Hàng rào quyền**: bộ đối kháng riêng cho role (xem mục 5).
5. **LLM-as-judge** cho phần văn bản tự do (trả lời có căn cứ, trích dẫn chính sách), hiệu chỉnh với nhãn người; ưu tiên chấm quyết định (kiểm tra tool call, DB) hơn chấm bằng judge.
6. **CI**: chạy bộ nhỏ mỗi PR (promptfoo/Inspect/DeepEval), bộ lớn theo đêm.

## 3. Metric đánh giá

- **Tool selection accuracy**, **argument accuracy** (exact/AST), **executable success**.
- **Task success** (trạng thái cuối đúng), **pass@1** và **pass^k** (τ-bench; trên leaderboard τ-bench điểm giảm rõ khi k tăng, ví dụ airline hàng đầu từ 0.460 pass^1 xuống 0.225 pass^4 theo README).
- **Refusal precision/recall**: từ chối đúng (vượt quyền, ngoài phạm vi) và không từ chối thừa.
- **Unauthorized-call rate** (tool ngoài allowlist/ownership) — mục tiêu 0, đo bằng log gateway.
- **Groundedness / citation accuracy** cho `ask_policy`; **hallucination rate** số liệu (so với kết quả tool).
- Chi phí: số lượt, token, độ trễ p95, số lần gọi tool thừa.
- Độ tin cậy judge: Cohen's kappa judge-vs-người.

## 4. Bộ dữ liệu

### Bảng tổng hợp

| # | Bộ / công cụ | Nhóm | Quy mô / dung lượng (theo nguồn đã tra) | License | Phù hợp |
|---|---|---|---|---|---|
| 1 | [τ-bench / τ²-bench (Sierra)](https://github.com/sierra-research/tau-bench) | c | CHỈ dùng tiếng Anh: retail, airline, telecom, banking...; 50 task cố định/domain/ngôn ngữ (README); theo evalscope qua search: airline 50, retail 114, telecom 114 task (tổng 278 mẫu); dung lượng: không tìm thấy (nhỏ, JSON trong repo) | MIT | ★★★★★ |
| 2 | [BFCL (Berkeley Function Calling Leaderboard)](https://gorilla.cs.berkeley.edu/leaderboard) | c | Live / Non-Live / Multi-turn; V4 thêm web search, memory, format sensitivity; số mẫu: EvalScope ghi 5,106 mẫu cho BFCL-v4 theo subset (có thể chưa đủ), một aggregator ghi V3 = 1,000 test case (chưa đối chiếu); dung lượng: không tìm thấy | Apache-2.0 (dữ liệu/leaderboard) | ★★★★★ |
| 3 | [WebShop](https://github.com/princeton-nlp/WebShop) | b/c | ~1.18M sản phẩm, 12,087 chỉ dẫn (README); có bản nhỏ 1,000 sản phẩm `items_shuffle_1000.json` (mirror HF qua search); dung lượng bản đầy đủ: không tìm thấy | MIT | ★★★★ |
| 4 | [WebArena](https://github.com/web-arena-x/webarena) | c | 812 task, có site Shopping + Shopping Admin; dung lượng: không tìm thấy (cần tự host site, ảnh Docker lớn, chưa xác minh) | Apache-2.0 | ★★★ |
| 5 | [MCPMark](https://github.com/eval-sys/mcpmark) | c | 127 task (suite chuẩn), 5 dịch vụ MCP; dung lượng: không tìm thấy | Apache-2.0 | ★★★★ |
| 6 | [MCP-Bench (Accenture)](https://github.com/Accenture/mcp-bench) | c | 28 MCP server; số task/tool và dung lượng: không tìm thấy | Apache-2.0 | ★★★ |
| 7 | [ToolSandbox (Apple)](https://github.com/apple/ToolSandbox) | c | 6 nhóm kịch bản (đơn/đa tool, đơn/đa lượt, phụ thuộc trạng thái, canonicalization, thiếu thông tin); tổng số kịch bản không nêu trong README; theo paper qua search, số kịch bản theo nhóm (chồng lấn): 152 single tool call, 656 multiple tool call, 584 single user turn, 224 multiple user turn, 192 state dependency, 472 canonicalization, 224 insufficient information (một tin tức ghi gần 2,000 kịch bản, chưa xác minh); dung lượng: không tìm thấy | Repo có LICENSE nhưng nội dung chưa đọc (chưa xác minh) | ★★★★ |
| 8 | [NESTFUL (IBM)](https://github.com/IBM/NESTFUL) | c | 1,861 ví dụ gọi API lồng nhau (`nestful_data.jsonl`, README); dung lượng: không tìm thấy (nhỏ) | Apache-2.0 | ★★★ |
| 9 | [ToolBench / ToolLLM](https://github.com/OpenBMB/ToolBench) | c | 3,451 tool, 16,464 API, 126,486 instance, 469,585 lần gọi (README); dung lượng: không tìm thấy | Apache-2.0 (dữ liệu: chỉ nghiên cứu) | ★★ |
| 10 | [AgentBench](https://github.com/THUDM/AgentBench) | c | 8 môi trường (OS, DB, KG, DCG, LTP, ALFWorld, WebShop, Mind2Web); số mẫu tổng và dung lượng: không tìm thấy | Apache-2.0 | ★★ |
| 11 | [API-Bank](https://github.com/AlibabaResearch/DAMO-ConvAI) | c | Đánh giá: 73 API tool, 314 hội thoại, 753 lời gọi API; train: 1,888 hội thoại/2,138 API (đã xác minh, nguồn: arXiv 2304.08244 qua search); dung lượng: không tìm thấy | Chưa xác minh (nằm trong repo DAMO-ConvAI) | ★★★ |
| 12 | [AgentDojo, InjecAgent (an toàn)](https://github.com/ethz-spylab/agentdojo) | c | AgentDojo 97 task + 629 test case bảo mật; InjecAgent 1,054 test case (xem mục 4(c)); dung lượng: không tìm thấy | MIT | ★★★★★ (an toàn) |
| 13 | Framework: [promptfoo](https://github.com/promptfoo/promptfoo), [DeepEval](https://github.com/confident-ai/deepeval), [Ragas](https://github.com/explodinggradients/ragas), [Inspect AI](https://github.com/UKGovernmentBEIS/inspect_ai), LangSmith | công cụ | — | MIT / Apache-2.0 / MIT | công cụ |
| 14 | [GAIA](https://arxiv.org/abs/2311.12983) | c | 466 câu hỏi (300 câu giữ đáp án cho leaderboard); người 92% vs GPT-4+plugins 15% (paper); dung lượng: không tìm thấy (câu hỏi + file đính kèm) | Chưa xác minh (xem thẻ HF gaia-benchmark/GAIA; có thể gated) | ★★ |
| 15 | [ShoppingBench (Alibaba)](https://arxiv.org/pdf/2508.04266) | b/c | Môi trường mô phỏng >2.5M sản phẩm thật; 17 agent được đánh giá; số task và dung lượng: không tìm thấy | Chưa xác minh | ★★★★ |
| 16 | [Shopping MMLU (Amazon)](https://arxiv.org/html/2410.20745v2) | b/c | CHỈ dùng phần tiếng Anh (bỏ nhánh đa ngôn ngữ): 57 tác vụ, 20,799 câu hỏi toàn bộ (số riêng phần tiếng Anh không tìm thấy); không phải môi trường agent; dung lượng: không tìm thấy | Chưa xác minh (dữ liệu công khai trên GitHub) | ★★★ |
| 17 | [AppWorld (AI2)](https://github.com/StonyBrookNLP/appworld) | c | 9 app, 457 API, 750 task (nguồn thứ cấp); dung lượng: không tìm thấy | Chưa xác minh | ★★★★ |
| 18 | [Toolathlon (Tool Decathlon)](https://arxiv.org/abs/2510.25726) | c | 108 task, 32 ứng dụng, 604 tool; Claude-4.5-Sonnet 38.6%; dung lượng: không tìm thấy | Chưa xác minh | ★★★ |
| 19 | [MCP-Universe (Salesforce)](https://arxiv.org/pdf/2508.14704) | c | 6 domain, 11 MCP server, 231 task (v1); dung lượng: không tìm thấy | Chưa xác minh | ★★★ |
| 20 | [MCP-Atlas (Scale AI)](https://labs.scale.com/papers/mcpatlas) | c | 36 MCP server, 220 tool, 1,000 task (500 công khai); dung lượng: không tìm thấy | Chưa xác minh | ★★★ |
| 21 | [ASB - Agent Security Bench](https://github.com/agiresearch/ASB) | c (an toàn) | 10 kịch bản (có e-commerce), >400 tool, 27 kiểu tấn công/phòng thủ; dung lượng: không tìm thấy | Mâu thuẫn giữa nguồn (README không nêu; một port ghi MIT): chưa xác minh | ★★★★ |
| 22 | [AgentHarm (UK AISI)](https://huggingface.co/datasets/ai-safety-institute/AgentHarm) | c (an toàn) | <1K mẫu; có tập harmful/benign; dung lượng: không tìm thấy (nhỏ) | Thẻ HF ghi license "other" | ★★★ |
| 23 | [ToolEmu](https://github.com/ryoungj/toolemu) | c (an toàn) | 36 toolkit, 144 test case (theo bảng của một survey); dung lượng: không tìm thấy | Chưa xác minh | ★★★ |

Ghi chú: các bộ trong bảng đều dùng phần tiếng Anh (ACEBench song ngữ Anh-Trung đã bị loại); chủ dự án xác nhận không cần bộ tiếng Việt. Kiểm tra tiếng Việt dùng bộ eval nội bộ (mục 5). Số liệu của dòng 14-23 và API-Bank lấy từ kết quả WebSearch (URL ở mục 4(c) và mục 8).

### (a) Tiếng Việt / Việt Nam
Ghi chú trung tính: dự án dùng các bộ tiếng Anh để kiểm phương pháp/chọn mô hình; kết luận về tiếng Việt dựa trên bộ eval nội bộ (mục 5). τ²-bench có chế độ đa ngôn ngữ (English, Spanish, Portuguese, Hindi, Chinese, Korean) nhưng không có tiếng Việt; theo quy tắc ngôn ngữ chỉ dùng bản tiếng Anh, bỏ các bản dịch khác (chỉ tham khảo cách dịch/ghép ngôn ngữ vào bộ task).

### (b) TMĐT / miền liên quan

**τ-bench (Sierra Research)** — https://github.com/sierra-research/tau-bench : mô phỏng hội thoại user (LLM) ↔ agent có API tool và chính sách miền; domain airline + retail; leaderboard Pass^1..Pass^4; MIT. README cảnh báo task trong repo không còn được cập nhật, khuyên dùng τ³-bench (thêm banking). **τ²-bench** — https://github.com/sierra-research/tau2-bench : domain mock, airline, retail, telecom, banking_knowledge, intake (>200 task); text-v1 (6 ngôn ngữ x 3 domain, 900 lượt gọi/1 trial; chỉ dùng ngôn ngữ English), voice; 50 task cố định mỗi domain/ngôn ngữ; lệnh `tau2 judges`; MIT, Python 3.12-3.13, cài bằng `uv`. **Quy mô / dung lượng**: 50 task cố định/domain (README); airline 50, retail 114, telecom 114 (evalscope qua search); dung lượng: không tìm thấy. **Phù hợp nhất để mô phỏng persona shopper** (retail: huỷ/đổi đơn theo chính sách). ★5. Hạn chế: tiếng Anh, tool khác catalog dự án; chính sách đặc thù Mỹ.

**WebShop** — https://github.com/princeton-nlp/WebShop : môi trường web thương mại mô phỏng, ~1.18M sản phẩm thật, 12,087 chỉ dẫn crowd-sourced, tìm-chọn-thuộc tính-mua; MIT. **Quy mô / dung lượng**: ~1.18M sản phẩm, 12,087 chỉ dẫn; bản nhỏ 1,000 sản phẩm (mirror HF qua search); dung lượng đầy đủ: không tìm thấy. Dùng như khuôn tác vụ mua hàng nhiều bước. ★4.

**WebArena** — https://github.com/web-arena-x/webarena : 812 ví dụ; website tự host (Shopping, Shopping Admin/CMS, Reddit, GitLab, Map, Wikipedia); Apache-2.0. Có site Shopping Admin gần persona admin/seller, nhưng agent thao tác trình duyệt, khác kiến trúc tool-API của dự án. ★3.

### (c) Benchmark / paper học thuật

**BFCL** — https://github.com/ShishirPatil/gorilla/tree/main/berkeley-function-call-leaderboard ; https://gorilla.cs.berkeley.edu/leaderboard : đánh giá gọi hàm có thực thi; nhóm Live (đơn lượt), Non-Live (đơn lượt), Multi-turn (V3, blog https://gorilla.cs.berkeley.edu/blogs/13_bfcl_v3_multi_turn.html); V4 thêm agentic web search, memory, format sensitivity; danh sách category trong `TEST_CATEGORIES.md`. **Quy mô / dung lượng**: xem bảng (EvalScope 5,106 mẫu cho V4 theo subset; dung lượng: không tìm thấy). Dữ liệu và leaderboard Apache-2.0 (license mã nguồn không nêu rõ trên trang). Leaderboard cập nhật cuối 2025-12-16 trên trang đã tra. Dùng: mẫu định dạng test, tính độ chính xác chọn tool/tham số, parallel/multiple calls. ★5.

**MCPMark** — https://github.com/eval-sys/mcpmark : stress-test dùng MCP trong sandbox cô lập, xác minh tự động; 127 task (suite chuẩn) + suite dễ 10 task/dịch vụ; dịch vụ Notion, GitHub, Filesystem, Postgres, Playwright; Apache-2.0. Postgres gần với backend dự án. ★4.

**MCP-Bench** — https://github.com/Accenture/mcp-bench : đánh giá phát hiện/chọn/dùng tool qua MCP trên 28 MCP server; có file task một, hai, ba server (README không nêu tổng số task/tool: chưa xác minh); Apache-2.0 (đã xác minh, nguồn: README qua WebFetch). ★3.

**ToolSandbox** — https://github.com/apple/ToolSandbox : thực thi tool có trạng thái, phụ thuộc ngầm giữa tool, user simulator tích hợp, chấm theo milestone (milestone tạo thành DAG, điểm tương đồng lấy trung bình nhân); 6 nhóm kịch bản: single/multiple tool call, single/multiple user turn, state dependency, canonicalization, insufficient information; user simulator dùng mô hình OpenAI; công cụ chạy trực tiếp trên máy chủ, không sandbox (README nêu). Tổng số kịch bản và license không có trong README (chưa xác minh; có file LICENSE trong repo). Theo paper qua search: 152/656/584/224/192/472/224 kịch bản theo 7 nhóm (chồng lấn); dung lượng: không tìm thấy. Nguồn: README qua WebFetch. Rất hợp ý tưởng "trạng thái cuối + milestone". ★4.

**NESTFUL** — https://github.com/IBM/NESTFUL ; arXiv 2409.03797 : 1,861 ví dụ (`nestful_data.jsonl`), chuỗi API lồng nhau (đầu ra bước này là đầu vào bước sau), có hàm thực thi; Apache-2.0. Liên quan chuỗi tool seller (summary → top products → low stock). ★3.

**ToolBench** — https://github.com/OpenBMB/ToolBench : 3,451 tool, 16,464 REST API (RapidAPI), 126,486 instance, 469,585 lần gọi thật; dữ liệu sinh bằng gpt-3.5-turbo-16k; Apache-2.0, chỉ nghiên cứu/giáo dục; có StableToolBench (2024-03). Quy mô lớn nhưng API đời thực không ổn định. ★2.

**AgentBench** — https://github.com/THUDM/AgentBench (ICLR 2024): 8 môi trường gốc (OS, DB, KG, Digital Card Game, Lateral Thinking Puzzles + ALFWorld, WebShop, Mind2Web), bản FC mới; Apache-2.0. Môi trường DB liên quan SQL. ★2.

**API-Bank** — https://arxiv.org/pdf/2304.08244 (EMNLP 2023); mã/dữ liệu trong https://github.com/AlibabaResearch/DAMO-ConvAI (thư mục `api-bank`). Hệ thống đánh giá chạy được với 73 API tool; 314 hội thoại dùng tool có 753 lời gọi API (đo lập kế hoạch, truy hồi, gọi API); tập huấn luyện 1,888 hội thoại từ 2,138 API, dùng huấn luyện Lynx (đã xác minh, nguồn: kết quả search về paper). License chưa xác minh. ★3.

**GAIA** — https://arxiv.org/abs/2311.12983 (ICLR 2024; Meta AI, Hugging Face, AutoGPT): 466 câu hỏi cần suy luận, đa phương thức, duyệt web, dùng tool; công bố câu hỏi, giữ đáp án của 300 câu cho leaderboard tại https://huggingface.co/gaia-benchmark ; người trả lời đúng 92% so với 15% của GPT-4 có plugin (nguồn: kết quả search). License chưa xác minh (không mở được thẻ HF). Là benchmark trợ lý đa dụng, ít liên quan nghiệp vụ TMĐT. ★2.

**ShoppingBench** — https://arxiv.org/pdf/2508.04266 (AAAI): benchmark agent mua sắm nhắm mục tiêu phức tạp (áp voucher, quản lý ngân sách, tìm nhiều sản phẩm của một người bán), môi trường mô phỏng với hơn 2.5 triệu sản phẩm thật, đánh giá 17 agent và một Qwen3-4B tinh chỉnh (nguồn: kết quả search). Gần persona shopper hơn WebShop. License chưa xác minh. ★4.

**Shopping MMLU** — https://arxiv.org/html/2410.20745v2 (Amazon, NeurIPS 2024; KDD Cup 2024): 57 tác vụ, 20,799 câu hỏi, 4 nhóm kỹ năng (hiểu khái niệm mua sắm, suy luận tri thức, hành vi người dùng, đa ngôn ngữ). Đánh giá kiến thức LLM trong miền mua sắm, không phải agent gọi tool. CHỈ dùng phần tiếng Anh, bỏ nhánh đa ngôn ngữ. License chưa xác minh. ★3.

**AppWorld** (AI2) — nguồn thứ cấp (alphaxiv/wizwand qua search): 9 ứng dụng (Amazon, Venmo, Spotify, Gmail, SimpleNote, Splitwise, FileSystem, Phone, Todoist), 457 API, 750 task từ 250 kịch bản, chia Test-N và Test-C. License chưa xác minh. Mô phỏng đa app có trạng thái, hợp ý tưởng chấm bằng trạng thái cuối. ★4.

**Toolathlon / Tool Decathlon** — https://arxiv.org/abs/2510.25726 (ICLR 2026): 32 ứng dụng, 604 tool (phần lớn qua MCP server), 108 task, trung bình khoảng 20 lượt gọi tool, mỗi task có script kiểm tra riêng; bản paper ghi Claude-4.5-Sonnet 38.6%, mô hình mở tốt nhất 20.1% (nguồn: kết quả search). License chưa xác minh. ★3.

**MCP-Universe** (Salesforce) — https://arxiv.org/pdf/2508.14704 (NeurIPS 2025): 6 domain, 11 MCP server, 231 task (v1); GPT-5 44.16%, Grok-4 33.33% theo bản gốc. **MCP-Atlas** (Scale AI) — https://labs.scale.com/papers/mcpatlas : 36 MCP server thật, 220 tool, 1,000 task đa bước (3-6 lần gọi tool), một nửa công khai, nửa giữ kín (nguồn: kết quả search). License hai bộ này chưa xác minh. ★3.

**Bộ an toàn/prompt-injection bổ sung** (chi tiết kỹ thuật ở [12](12_CONTENT_MODERATION_AGENT_SAFETY.md)):
- **AgentDojo** — https://arxiv.org/html/2406.13352v3 : 97 task thực tế, 629 test case bảo mật (nguồn: paper qua search). **InjecAgent** — https://github.com/uiuc-kang-lab/InjecAgent : 1,054 test case, 17 tool người dùng, 62 tool kẻ tấn công, dựng từ 330 tool của 36 toolkit (README qua search).
- **ASB (Agent Security Bench)** — https://arxiv.org/pdf/2410.02644 ; https://github.com/agiresearch/ASB (ICLR 2025): 10 kịch bản (gồm e-commerce, tài chính, xe tự lái), 10 agent, >400 tool, 27 kiểu tấn công/phòng thủ, 7 metric (bản v1 ghi 23 kiểu/8 metric; trích bản mới). License: nguồn mâu thuẫn, chưa xác minh. ★4.
- **AgentHarm** — https://huggingface.co/datasets/ai-safety-institute/AgentHarm : <1K mẫu, có tập harmful và benign, validation + test_public; license ghi "other" trên thẻ. **ToolEmu**: 36 toolkit, 144 test case (theo bảng của một survey), license chưa xác minh.
- **WildGuardMix** (AI2) — https://huggingface.co/datasets/allenai/wildguardmix : 92K mẫu (WildGuardTrain 86,759 + WildGuardTest 5K người gán nhãn), ODC-BY, gated (chấp nhận Responsible Use Guidelines); là bộ kiểm duyệt/jailbreak/từ chối, không phải bộ agent tool.

**An toàn agent**: AgentDojo, InjecAgent, BIPIA, R-Judge — chi tiết ở [12](12_CONTENT_MODERATION_AGENT_SAFETY.md); AgentDojo (MIT) là khuôn tốt nhất để thêm suite marketplace.

**Ghi chú BFCL (bổ sung)**: một trang EvalScope liệt kê 5,106 mẫu cho BFCL-v4 theo các subset (có thể chưa đủ toàn bộ); một wiki thứ cấp ghi dữ liệu/mã đánh giá/kết quả Apache-2.0; cần đối chiếu repo chính thức (nguồn: kết quả search).

### (d) Framework eval (mô tả ngắn, đã đọc README GitHub)
- **promptfoo** — https://github.com/promptfoo/promptfoo : CLI/thư viện đánh giá và red-team ứng dụng LLM; MIT. Hợp CI + red-team tool agent.
- **DeepEval** — https://github.com/confident-ai/deepeval : framework Python kiểu unit-test, metric LLM-as-judge; Apache-2.0.
- **Ragas** — https://github.com/explodinggradients/ragas : metric cho ứng dụng LLM/RAG và sinh dữ liệu test; Apache-2.0.
- **Inspect AI** — https://github.com/UKGovernmentBEIS/inspect_ai : framework eval của UK AI Security Institute; MIT.
- **LangSmith datasets**: dịch vụ thương mại; không đọc được trang trong phiên (chưa xác minh chi tiết).

## 5. Dữ liệu tự tạo / synthetic từ chính hệ thống: dựng bộ eval riêng cho 3 persona

**Nguyên tắc**: môi trường = gateway thật chạy trên dữ liệu seed cố định (`SEED_DEMO_DATA=true`, đặt snapshot Postgres/ClickHouse để tái lập); tool = đúng endpoint REST; chấm bằng (a) tool call trong log, (b) trạng thái cuối, (c) judge cho văn bản.

**Cấu trúc một case (JSON)**: `id`, `persona`, `jwt_role` + `store_id`/`user_id` giả lập, `conversation` (các lượt user, tiếng Việt, có biến thể không dấu/teencode), `expected_tool_calls` (tên + ràng buộc tham số, có cho phép nhiều thứ tự đúng), `forbidden_tool_calls`, `expected_final_state`, `answer_facts` (số liệu phải xuất hiện, tính từ DB), `must_refuse` (true/false), `tags` (nhóm).

**Nhóm case (đề xuất số lượng khởi điểm ~ 40-60/persona)**
- **Shopper**: tìm hàng với bộ lọc (khoảng giá, danh mục, tồn kho), hỏi tồn kho (đáp án chỉ `level`, không lộ số chính xác), "đơn của tôi" (chỉ đơn của user), thêm giỏ (phải ra `action_proposal`, không ghi trực tiếp), hỏi chính sách (có trích dẫn), khách vãng lai gọi tool cần đăng nhập (phải từ chối/yêu cầu đăng nhập).
- **Seller**: doanh thu theo kỳ, so kỳ trước, top sản phẩm, sắp hết hàng, funnel sản phẩm; số liệu đối chiếu với truy vấn ClickHouse trên `fact_order_items`/`fact_order_status` theo quy ước doanh thu của dự án (PAID online / DELIVERED COD trừ hoàn); hỏi shop khác (phải bị chặn — `store_id` ép từ JWT); thay đổi giá/huỷ đơn → `action_proposal`.
- **Admin**: tổng quan sàn, top shop, traffic/funnel, `system_metrics`/`service_health`/`list_alerts`, `search_audit_log`; câu hỏi RCA chéo (xem [14](14_AIOPS_ANOMALY_ALERT_EXPLAIN.md)).
- **Chuỗi nhiều bước**: ví dụ seller: "sản phẩm nào bán chạy nhưng sắp hết hàng?" → `store_top_products` rồi `store_low_stock` (kiểu NESTFUL).
- **Đối kháng/vượt quyền (negative set)**: buyer đòi gọi tool admin, đổi vai ("tôi là admin"), đưa `store_id` trong lời nhắn, prompt injection gián tiếp trong mô tả sản phẩm/review/tên shop, đòi xuất toàn bộ danh sách khách, yêu cầu bỏ qua xác nhận `action_proposal`, đòi số tồn chính xác. Mục tiêu: unauthorized-call rate = 0, rò rỉ = 0.
- **Ngoài phạm vi**: câu hỏi không liên quan (từ chối lịch sự); câu mơ hồ (hỏi lại thay vì đoán); đo từ chối thừa bằng các case hợp lệ gần biên.
- **Ổn định**: mỗi case chạy k=5-8 lần với nhiệt độ vận hành, báo pass^k; tách case "đúng sự thật số liệu" khỏi "văn phong".
- **Tiếng Việt**: mỗi case có 3 biến thể (chuẩn, không dấu, viết tắt/teencode) để đo độ bền (xem [15](15_VIETNAMESE_NLP_QUERY_UNDERSTANDING.md)).

**Cách sinh case**: viết tay 15-20 case/persona làm seed; dùng LLM tạo biến thể và hội thoại nhiều lượt, **người duyệt** từng case; đáp án số liệu luôn tính bằng SQL từ snapshot, không để LLM tạo đáp án. Tách bộ `dev` (điều chỉnh prompt) và `test` giữ kín (không đưa vào prompt/ví dụ few-shot).

**User simulator**: LLM đóng vai người dùng theo kịch bản (mục tiêu + ràng buộc + mức hợp tác) như τ-bench, giới hạn số lượt.

**Judge**: rubric ngắn (đúng sự thật so với `answer_facts`, có trích dẫn, đúng giọng/ngôn ngữ, không lộ dữ liệu), mô hình judge khác mô hình agent, đo kappa với 100 mẫu người chấm; judge chỉ bổ sung cho kiểm tra xác định.

**Tích hợp công cụ**: promptfoo hoặc Inspect AI chạy case (assert trên tool call và JSON), DeepEval/Ragas cho metric RAG của `ask_policy`; AgentDojo-style suite cho tấn công; CI chạy bộ nhỏ.

## 6. Lộ trình dùng dữ liệu đề xuất

1. **Tuần 1-2**: dựng harness + 20 case/persona (đơn lượt), chấm bằng tool call; dùng BFCL để chọn mô hình ban đầu và học định dạng.
2. **Tuần 3-4**: thêm hội thoại nhiều lượt + user simulator + pass^k, mô phỏng kiểu τ-bench retail; thêm negative set vượt quyền (bắt buộc trước khi mở cho người dùng thật).
3. **Tuần 5-6**: suite injection kiểu AgentDojo/InjecAgent trên tool dự án; judge cho `ask_policy`.
4. **Sau đó**: tham chiếu MCPMark/ToolSandbox nếu chuyển sang MCP hoặc cần chấm milestone; tham chiếu WebShop/WebArena nếu có agent thao tác UI.
5. Theo dõi hồi quy mỗi lần đổi model/prompt/tool schema.

## 7. Rủi ro, license, thiên lệch, lưu ý pháp lý/PII

- **Chuyển miền/ngôn ngữ**: benchmark công khai tiếng Anh, chính sách nước ngoài; điểm cao trên τ-bench không đảm bảo trên tiếng Việt.
- **LLM-as-judge**: thiên lệch dài dòng, tự ưa model cùng họ; hiệu chỉnh với người; không dùng làm cổng chặn duy nhất.
- **Nhiễm dữ liệu (contamination)**: benchmark công khai có thể đã nằm trong dữ liệu huấn luyện model; giữ bộ test nội bộ kín.
- **Bất ổn**: LLM không xác định; một lần chạy không đủ, dùng pass^k; kết quả τ-bench cho thấy suy giảm mạnh theo k.
- **License**: hầu hết MIT/Apache-2.0 (đã đọc README); ToolBench dữ liệu chỉ nghiên cứu/giáo dục; ToolSandbox, API-Bank, GAIA, ShoppingBench, AppWorld, Toolathlon, MCP-Universe, MCP-Atlas, ASB chưa xác minh license; AgentHarm ghi "other", WildGuardMix ODC-BY gated; kiểm tra điều khoản nhà cung cấp LLM khi dùng judge.
- **PII**: hội thoại và dữ liệu seed không chứa dữ liệu thật; nếu dùng log thật phải ẩn danh trước khi đưa cho judge bên ngoài. Môi trường eval cô lập (sandbox), không trỏ vào production, đặc biệt với tool ghi.
- Ngày tra cứu: leaderboard (BFCL cập nhật 2025-12-16) có thể đã đổi; kiểm tra lại trước khi trích số.

## 8. Nguồn tham khảo

- τ-bench: https://github.com/sierra-research/tau-bench ; τ²-bench: https://github.com/sierra-research/tau2-bench
- BFCL: https://gorilla.cs.berkeley.edu/leaderboard ; https://github.com/ShishirPatil/gorilla/tree/main/berkeley-function-call-leaderboard ; https://gorilla.cs.berkeley.edu/blogs/13_bfcl_v3_multi_turn.html ; https://gorilla.cs.berkeley.edu/blogs/17_bfcl_v4_prompt_variation.html
- WebShop: https://github.com/princeton-nlp/WebShop ; WebArena: https://github.com/web-arena-x/webarena
- MCPMark: https://github.com/eval-sys/mcpmark ; MCP-Bench: https://github.com/Accenture/mcp-bench
- ToolSandbox: https://github.com/apple/ToolSandbox ; NESTFUL: https://github.com/IBM/NESTFUL
- GAIA: https://arxiv.org/abs/2311.12983 ; ShoppingBench: https://arxiv.org/pdf/2508.04266 ; Shopping MMLU: https://arxiv.org/html/2410.20745v2 ; Toolathlon: https://arxiv.org/abs/2510.25726 ; MCP-Universe: https://arxiv.org/pdf/2508.14704 ; MCP-Atlas: https://labs.scale.com/papers/mcpatlas ; API-Bank paper: https://arxiv.org/pdf/2304.08244
- ASB: https://arxiv.org/pdf/2410.02644 ; AgentHarm: https://huggingface.co/datasets/ai-safety-institute/AgentHarm ; WildGuardMix: https://huggingface.co/datasets/allenai/wildguardmix ; AgentDojo paper: https://arxiv.org/html/2406.13352v3
- ToolBench: https://github.com/OpenBMB/ToolBench ; AgentBench: https://github.com/THUDM/AgentBench ; API-Bank: https://github.com/AlibabaResearch/DAMO-ConvAI
- AgentDojo: https://github.com/ethz-spylab/agentdojo ; InjecAgent: https://github.com/uiuc-kang-lab/InjecAgent
- Framework: https://github.com/promptfoo/promptfoo ; https://github.com/confident-ai/deepeval ; https://github.com/explodinggradients/ragas ; https://github.com/UKGovernmentBEIS/inspect_ai
- Tool catalog nội bộ: /home/user/mini-marketplace/docs/ai/05_AI_DRAFT_ORIGINAL.md
