# 12. Tiêu chí của một hệ thống agent (từ ghi chú nghiên cứu) so với `ai-agent` hiện tại

Ngày viết: 2026-11-17 (cập nhật cùng ngày sau khi xử lý bốn khoảng trống, mục 4). Nguồn tiêu chí: các bài trong `personal-research-knowledge/knowledge/ai/` (agent harness, agent memory, guardrails và HITL, eval, production eval và guardrail, circuit breaker cho agent, LangGraph và Reflexion, Deep Agents, ReAct, prompt và tool calling, MCP và A2A). Đây là ghi chú học tập từ các lab, KHÔNG phải chuẩn bắt buộc; mỗi mục được xét "có cần cho hệ thống này không" trước, rồi mới so với hiện trạng. Bổ sung cho `10_AGENT_QUALITY_CRITERIA.md` (tiêu chí tự đặt, kèm mức bằng chứng); bảng dưới dùng cùng nhãn bằng chứng [đo live] / [test] / [đọc code] / [chưa kiểm]. Về MCP, Evidence Gateway, multi-agent xem ADR-49 và `02_AGENT_ARCHITECTURE.md` §6.1.

Nhãn kết luận: **Có** (đã làm), **Một phần**, **Chưa** (cần, chưa làm), **Không cần** (có lý do), **Thay thế** (không làm theo kiểu của bài nhưng có cách khác).

## 1. Bảng tổng hợp

| # | Tiêu chí (nguồn) | Kết luận | Hiện trạng và bằng chứng | Cần không, vì sao |
|---|---|---|---|---|
| 1 | Vòng lặp ReAct có trần số vòng (chatbot-vs-react-agent) | **Có** | [test] `guard/limits.py`: `max_model_calls` 6, `max_tool_calls` 8; RCA có trần riêng 12/12 và dừng sớm sau 3 lỗi công cụ liên tiếp (§16.32) | Cần: chống model kẹt và tốn tiền |
| 2 | Tool lỗi trả chuỗi lỗi cho model, không làm sập (nt) | **Có** | [test] lỗi công cụ kèm mã HTTP và lời nhắn cho model (`tools/gateway.py`); UI hiện lỗi nguyên văn | Cần |
| 3 | Mô tả tool nói khi nào dùng, khi nào không, tham số, ranh giới xác nhận (prompt-tool-calling) | **Một phần** | [đọc code] mô tả trong `agent/scopes.go`; hành động ghi chỉ là đề xuất, người xác nhận ở UI. Còn 400 do model tự bịa (`infra_up`, tham số `window` không có) [đo live] §16.32 | Cần: đây là nguồn lỗi công cụ còn lại |
| 4 | Phân loại lỗi agent (wrong_tool, wrong_arg, wrong_boundary, unnecessary_tool, out_of_scope, missing_info) | **Có** (cập nhật 2026-11-17) | [test] 422 ca chia 7 nhóm `lookup/filter/variant/propose/adversarial/scope/clarify`; nhóm `clarify` mới (16 ca, 4 mỗi persona) = thiếu thông tin thì phải hỏi lại, không đoán, không gọi tool | Cần |
| 5 | Nhật ký phiên bản prompt/tool kèm giả thuyết (`version_log.csv`) | **Thay thế** | Nhật ký xây `09`/`11` ghi lỗi, nguyên nhân, cách sửa, kết quả; không có băm prompt/tool theo từng lần chạy eval | Nên nhẹ: ghi băm prompt vào file kết quả eval |
| 6 | Harness: `injection_guard` (dữ liệu là dữ liệu, không là lệnh) (agent-harness-arena, guardrails-hitl) | **Có** | [test] `guard/untrusted.py` bọc mọi kết quả công cụ (`tools/views.py`) và tóm tắt/nhãn sự cố RCA; prompt 4 persona dặn rõ; quyền thật ở gateway nên lệnh bị tiêm cũng không gọi được route ngoài vai | Cần. Chưa đo live tỷ lệ bị lừa (3 ca `inject-*`, tài liệu 10 §2.3) |
| 7 | Harness: `critic` (câu trả lời phải có trong bằng chứng đã đọc) và `citation_checker` | **Một phần** (cập nhật 2026-11-17) | RCA: `rca/grounding.py` kiểm xác định (không LLM): mã bằng chứng không tồn tại bị bỏ và báo, giả thuyết mà bằng chứng dẫn không nhắc tên service/thành phần/con số nó khẳng định bị hạ độ tin cậy xuống 0.3, tham số hành động (tên service...) phải có trong bằng chứng hoặc thông tin sự cố, không thì nháp thành `escalate_to_human`; tiêu chí eval `grounded` (22/22 với Brain đáp án). Chat: `guard/grounding.py` ĐO (không chặn) con số 4+ chữ số trong câu trả lời có trong kết quả công cụ không, chỉ số `mm_ai_answer_numbers_total`. Còn thiếu: chưa chứng minh bằng chứng THỰC SỰ ủng hộ (chỉ kiểm có nhắc tới); chat chưa chặn | Cần cho RCA; chat vừa phải |
| 8 | Harness: `budget_policy` (cắt phần thừa) | **Có** | [test] trần vòng/công cụ; `token_budget_per_turn` và theo persona; hạn mức token mỗi ngày mỗi người (429 `daily_token_limit`); RCA có ngân sách mỗi sự cố và trần phiên tự động mỗi ngày | Cần |
| 9 | Harness: `retry` ở dưới model (lớp bọc công cụ) | **Một phần** (cập nhật 2026-11-17) | `max_retries: 2` nay ghi rõ trong profile (SDK thử lại 408/409/429/5xx và lỗi kết nối, có backoff và tôn trọng Retry-After); `GatewayClient` vẫn không thử lại (công cụ GET, lỗi chủ yếu do model gọi sai nên thử lại vô ích); dừng sớm sau lỗi lặp (§16.32) | Đủ cho quy mô này |
| 10 | Sáu hook và thứ tự middleware | **Một phần** | Dùng middleware dựng sẵn của LangChain (`ModelCallLimit`, `ToolCallLimit`); các lớp khác (leak filter, wrap) nằm ở tầng stream/tools, không là middleware | Không cần đổi; chỉ cần khi thêm Verifier (nên làm thành `after_agent`) |
| 11 | Memory ngắn hạn | **Có** | [test] lịch sử lưu Postgres dạng che, giữ 30 ngày, nạp lại tối đa `max_history_messages: 20` | Cần |
| 12 | Memory bền (User.md) | **Không cần** (cố ý) | Không có hồ sơ người dùng giữ qua phiên; admin đọc được lịch sử chat nên không gom thêm dữ liệu cá nhân | Không: tăng rủi ro riêng tư, lợi ích thấp với hỏi đáp về shop |
| 13 | Nén hội thoại dài | **Chưa** (rủi ro thấp) | Chỉ cắt cứng 20 tin cũ; role `summarizer` có trong config nhưng ghi "chưa dùng" | Nên nếu hội thoại dài thật; hiện câu hỏi ngắn nên hoãn |
| 14 | Input guardrail (chặn injection trong tin người dùng, chủ đề ngoài phạm vi) | **Thay thế** | Không có bộ lọc đầu vào riêng: giới hạn `max_message_chars: 2000`; nhóm eval `scope`/`adversarial` kiểm model từ chối; an toàn dựa vào cấu trúc: agent chỉ đề xuất, danh tính chỉ từ JWT, scope ở gateway | Bộ lọc đầu vào bắt từ khoá dễ chặn nhầm và dễ lách; chưa cần nếu quyền đã ép ở gateway |
| 15 | Output guardrail (che PII, secret) | **Thay thế** + **Có** một phần | PII bị BỎ khỏi dữ liệu trước khi vào model (`tools/redact.py`) nên model không có gì để lộ; `guard/leak.py` che đường dẫn API và tên công cụ cho buyer/seller (lọc theo luồng); chat lưu dạng che. Chưa có quét secret trong câu trả lời | Che ở nguồn tốt hơn che ở đầu ra; thêm quét secret là rẻ, nên làm nếu có dữ liệu chứa khoá |
| 16 | Rate limit | **Có** | [đọc code] gateway: theo IP, theo user, riêng cho đề xuất và cho `system-token`; Redis lỗi thì từ chối (`fail closed`); một luồng chat mỗi user | Cần |
| 17 | Audit trail | **Có** | Lịch sử chat, `ai_rca_events` (từng bước), nhật ký kiểm toán chỉ-thêm của ops-service, admin mở chat của người khác đều bị ghi | Cần |
| 18 | Permission và egress (công cụ nào được gọi, dữ liệu nào ra) | **Có** | `scopes.go` là nguồn duy nhất; token `sys` chỉ GET `/ops/*`, RCA chỉ thấy công cụ `/ops/`; `scripts/smoke-rca-tools.sh` kiểm bằng token thật (§16.34) | Cần, đây là lớp mạnh nhất |
| 19 | HITL cho hành động rủi ro cao | **Có** | Agent chỉ đề xuất; người xác nhận bằng JWT của chính họ; ops: người tạo không tự duyệt, người khác duyệt, chỉ ops-service chạy (ADR-45/47/48) | Cần |
| 20 | Định tuyến theo độ tin cậy LLM tự báo | **Không cần** (cố ý) | RCA không dùng độ tin cậy làm cổng: mọi hành động đi qua người; `escalate_to_human` khi không đủ bằng chứng | Bài nghiên cứu cũng cảnh báo độ tin cậy tự báo không đáng tin; chỉ nên dùng khi đã hiệu chỉnh (tài liệu 10 §3) |
| 21 | Red team (tấn công có hệ thống) và "clean controls" (đo chặn nhầm) | **Một phần** | khoảng 68 ca `adversarial` (đếm theo dòng) trong 422 ca, chạy tĩnh với gateway giả; chưa red team live; chưa đo chặn nhầm của `leak.py` ngoài test đơn vị | Nên: chạy live một lần trước khi trình bày (cần đồng ý chi phí) |
| 22 | Golden dataset phân tầng | **Có** | [test] 422 ca viết tay, 4 persona, 7 nhóm (thêm `clarify`); RCA 22 ca (19 từ đáp án `resilience.sh` + 3 injection) | Cần |
| 23 | Tách tập dev/test, đóng băng trước khi chấm (Deep Agents) | **Một phần** (cập nhật 2026-11-17) | `evalkit.split_of`: 1/5 câu (theo băm của câu hỏi, ổn định khi thêm/đổi thứ tự) là tập giữ kín; `run_evals.py --split dev|heldout|all`. Quy ước: chỉnh prompt chỉ chạy `dev`, `heldout` chỉ khi chốt. Chưa chạy `heldout` live lần nào; chưa có cơ chế ép (chỉ là quy ước) | Cần |
| 24 | Metric RAG (faithfulness, context recall/precision) | **Không cần** | Không có kho vector và truy xuất văn bản; "truy xuất" là chọn công cụ và tham số, đã đo ở #4. Playbook RCA là tra cứu theo khoá | Chỉ cần nếu sau này thêm RAG trên tài liệu |
| 25 | LLM làm giám khảo, kiểm thiên lệch, Cohen κ | **Không cần** (hiện) | Chấm xác định bằng quy tắc (không tốn thêm tiền, lặp lại được). Đổi lại KHÔNG đo chất lượng câu chữ và độ đúng số liệu | Cần nếu muốn đo "câu trả lời có tốt không"; khi đó phải làm kèm kiểm κ với nhãn người |
| 26 | Cổng chất lượng trong CI (chặn deploy khi điểm tụt) | **Có** (cập nhật 2026-11-17, chưa chạy trên GitHub) | Job `ai-agent` trong `.github/workflows/ci.yml`: `pytest` (kèm Postgres thật), `run_evals.py` (kiểm tĩnh 422 ca theo bảng tool), `run_rca_evals.py` và `--oracle`; `push-images` cần job này. Không gọi LLM. Chưa có ngưỡng điểm live (eval live chạy tay) | Cần |
| 27 | Chạy lặp k lần, báo dao động | **Một phần** (cập nhật 2026-11-17) | `run_rca_evals.py --repeat k` + `rcakit.summarize_repeats`: tỷ lệ đạt, khoảng tin cậy Wilson 95%, ca luôn đạt/không bao giờ đạt, ca dao động. Đã test bằng dữ liệu giả; CHƯA chạy live (cần đồng ý chi phí) | Cần trước khi nêu con số chất lượng |
| 28 | Phân loại lỗi nhà cung cấp LLM (429 rate limit, 429 hết hạn mức, 5xx, 400) | **Có** (cập nhật 2026-11-17) | `llm/resilience.py: classify` ra 7 loại (rate_limit, quota, overloaded, timeout, auth, bad_request, unknown); phiên RCA lỗi ghi lý do vào `report.error_reason` và dòng thời gian; chat báo mã `model_unavailable` kèm lý do thay vì lỗi chung. [test] | Cần |
| 29 | Backoff có jitter, retry budget, một tầng retry | **Thay thế** (cập nhật 2026-11-17) | Đúng MỘT tầng thử lại: SDK (`max_retries`, backoff có jitter, Retry-After). Lớp `resilience` KHÔNG thử lại cùng profile (tránh retry storm nhiều tầng); thay vào đó breaker + dự phòng. Không làm retry budget riêng vì không có tầng thử lại thứ hai để kiềm | Đủ |
| 30 | Circuit breaker | **Có** (cập nhật 2026-11-17) | `CircuitBreaker` cửa sổ trượt theo tỷ lệ lỗi, half-open có giới hạn thăm dò, mỗi profile một breaker; ngắt thì lượt sau thất bại ngay (không chờ timeout 60 s) hoặc sang dự phòng; 400 không tính. Gateway→service vẫn có breaker riêng. [test] 21 test | Cần ở quy mô lớn hơn; rẻ nên có |
| 31 | Fallback ladder (model dự phòng, kiểm tương thích tool calling) | **Một phần** (cập nhật 2026-11-17) | Cơ chế có và đã test (`llm.fallbacks.<role>`, lỗi nhà cung cấp mới sang dự phòng, 400 thì không). Cấu hình mặc định `fallbacks: {}` (chưa chọn model dự phòng thật, cần khoá và tiền); chưa có kiểm tương thích tool calling tự động (người cấu hình phải chọn profile hỗ trợ tool) | Nên cấu hình khi chạy liên tục |
| 32 | Idempotency, checkpoint, resume sau crash | **Có** | [test] token đề xuất dùng một lần, hạn 5 phút; chỉ một đề xuất PENDING mỗi hành động (chỉ mục duy nhất); `Checkout` có khoá idempotent; checkpoint Postgres, phiên chờ sống sót qua deploy (§16.33) | Cần |
| 33 | Semantic cache, stale-if-error | **Không cần** | Dữ liệu shop đổi liên tục, cache trả cũ là trả sai; chi phí đã bị chặn bằng trần token | Không |
| 34 | Error budget, cảnh báo burn-rate đa cửa sổ, postmortem | **Một phần** | Cảnh báo ai-agent chết, watcher đứng (`AIRcaWatcherStalled`); chưa có SLO riêng cho AI, chưa burn-rate; postmortem dưới dạng nhật ký xây, chưa theo mẫu không đổ lỗi | Nên nhẹ: một SLO "phiên RCA không ra `error`" |
| 35 | Đồ thị có trạng thái, cạnh có điều kiện, checkpointer | **Có** | `rca/graph.py` (LangGraph), chờ người bằng trạng thái + watcher đánh thức, checkpoint Postgres | Cần |
| 36 | Dead letter khi không xử lý được | **Một phần** (cập nhật 2026-11-17) | Phiên hỏng ra `error`; nay kèm `report.error_reason` (vd "hạn mức mô hình đã hết") hiện cho người; chưa chuyển thành trạng thái "cần người xử lý" riêng | Nên |
| 37 | Reflexion (tự viết nhận xét lỗi rồi thử lại) | **Một phần** | Không có Reflexion; có `verify_retries: 1`: kiểm lại sau khi người chạy lệnh, nếu chưa khỏi thì thêm một vòng điều tra | Không cần thêm; giá trị thấp so với chi phí |
| 38 | Sandbox, khoá API nằm phía host | **Không cần** | Agent không chạy mã tuỳ ý; khoá model chỉ ở biến môi trường của ai-agent; token agent ngắn hạn, không vào log | Chỉ cần nếu thêm công cụ chạy mã |
| 39 | Multi-agent (Supervisor/Researcher/…), A2A, MCP, Evidence Gateway | **Không cần** (quyết định) | ADR-49: một agent + công cụ chọn lọc; lớp bằng chứng làm trong ai-agent. Bài nghiên cứu cũng nêu câu hỏi mở "multi-agent có đáng chi phí không" | Không, trừ khi đề bài yêu cầu |
| 40 | Trace từng lần dùng bằng chứng, `evidence_ref` do hệ thống cấp | **Một phần** (cập nhật 2026-11-17) | Mã `e1..eN` đã do HỆ THỐNG cấp từ kết quả công cụ thật (model chỉ trích dẫn); nay mã không tồn tại bị bỏ và ghi lại (`bogus_refs`); Verifier tất định = `rca/grounding.py` (mục 7). Chưa có kho bằng chứng riêng tách khỏi trạng thái RCA và chưa dùng cho chat (ADR-49 thứ tự 1-2 còn lại) | Nên làm tiếp nếu cần |

## 2. Giải thích kỹ năm mục chủ ý không làm

Với mỗi mục: nó để làm gì, bài nghiên cứu đòi gì, hệ thống mình có gì thay thế và vì sao không làm.

### 2.1 Memory bền kiểu `User.md` (#12)

- **Để làm gì:** agent nhớ thông tin ổn định về người dùng qua nhiều phiên (tên, nơi ở, nghề, cách trả lời họ thích, món yêu thích) để không hỏi lại và cá nhân hoá câu trả lời.
- **Bài đòi gì:** mỗi lượt trích "sự kiện ổn định" từ tin nhắn, ghi vào một tệp markdown cho mỗi người (làm sạch id người dùng để không trỏ ra ngoài thư mục), nạp vào prompt cùng tóm tắt và tin gần nhất; đo recall, token, tốc độ tăng của bộ nhớ.
- **Hệ thống mình có:** chỉ nhớ ngắn hạn trong một cuộc chat (tối đa 20 tin cũ nạp lại) và người dùng có thể mở lại chat cũ trong 30 ngày. Dữ liệu cá nhân (địa chỉ, số điện thoại, email) bị bỏ trước khi vào mô hình nên mô hình không có gì để "nhớ".
- **Vì sao không làm:** (1) tạo kho dữ liệu cá nhân thứ hai, mà admin đã đọc được lịch sử chat đã che; nhớ bền làm tăng bề mặt rò rỉ và nghĩa vụ xoá. (2) Lợi ích thấp: câu hỏi về shop (đơn, sản phẩm, tồn kho) lấy từ công cụ theo danh tính trong JWT, không cần "nhớ" người dùng. (3) Trí nhớ sai (mô hình trích sai sự kiện) sẽ lặp lại sai ở mọi phiên sau. Khi nào nên làm: nếu muốn gợi ý cá nhân hoá theo sở thích, thì lưu rõ ràng, có nút xem/xoá, và không dùng văn bản tự do do mô hình viết.

### 2.2 Định tuyến theo độ tin cậy do mô hình tự báo (#20)

- **Để làm gì:** giảm việc người phải duyệt: độ tin cậy cao thì tự làm, trung bình thì dừng chờ người (`interrupt`), thấp thì leo thang và hỏi câu cụ thể (ngưỡng ví dụ 72% và 58%).
- **Bài đòi gì:** mô hình trả kết quả có cấu trúc kèm độ tin cậy; hàm định tuyến chia ba nhánh; bảng audit ghi mọi quyết định; hai ví dụ phải đi hai nhánh khác nhau. Chính bài cũng ghi: độ tin cậy tự báo "không đáng tin tuyệt đối", ngưỡng phải đo trên dữ liệu thật và hành động rủi ro cao luôn cần người.
- **Hệ thống mình có:** mọi hành động ghi đều đi qua người (hai cổng: operator xác nhận, người khác duyệt), bất kể độ tin cậy. Độ tin cậy trong RCA chỉ dùng để chọn "thu thêm bằng chứng hay đề xuất" và để hạ cấp xuống `escalate_to_human` khi dưới 0.4; không dùng để bỏ qua người.
- **Vì sao không làm:** tự duyệt theo con số mô hình tự báo là chuyển quyền quyết định cho đúng thứ chưa được hiệu chỉnh (chưa đo calibration, tài liệu 10 mục 3). Nếu sau này muốn: đo calibration trước (độ tin cậy khai báo so với tỷ lệ đúng thực), chỉ áp cho hành động mức `observe` (đọc), và giữ cổng người cho mọi hành động ghi.

### 2.3 Semantic cache (#33)

- **Để làm gì:** câu hỏi giống nhau (FAQ) trả lời ngay từ cache thay vì gọi mô hình: rẻ và nhanh; thêm `stale-if-error` để khi mọi nhà cung cấp sập vẫn có câu trả lời cũ.
- **Bài đòi gì:** cache có kiểm soát: phạm vi (scope), ngưỡng tương đồng, thời hạn (TTL), cách vô hiệu hoá; stale-if-error cho FAQ nhưng "không được trả sai".
- **Hệ thống mình có:** chi phí đã bị chặn bằng trần token mỗi lượt, mỗi ngày mỗi người, trần số lần gọi mô hình/công cụ; breaker nay giúp thất bại nhanh khi nhà cung cấp sập.
- **Vì sao không làm:** câu hỏi của mình là DỮ LIỆU RIÊNG và ĐỔI LIÊN TỤC ("đơn của tôi đâu", "còn hàng không", "doanh thu hôm nay"); cache theo độ tương đồng sẽ trả đơn của người khác hoặc số liệu cũ, là lỗi nghiêm trọng hơn là tiết kiệm tiền. Chỉ nên cache phần tĩnh thật sự (vd chính sách đổi trả từ `get_shop_rules`), và phần đó đã rẻ.

### 2.4 Metric RAG và LLM làm giám khảo (#24, #25)

- **Để làm gì:** nếu hệ thống có RAG (tìm đoạn tài liệu rồi sinh câu trả lời), cần đo riêng bước tìm (context recall, context precision) và bước sinh (faithfulness, relevance, completeness). LLM làm giám khảo chấm câu trả lời tự do theo rubric 1-5, nhưng có thiên lệch (vị trí, dễ dãi, khắt khe, thích câu dài hoặc do chính nó viết), nên phải kiểm bằng cách chấm hai thứ tự và đo đồng thuận với nhãn của người (Cohen κ).
- **Bài đòi gì:** bộ golden phân tầng (dễ/vừa/khó/đối kháng), RAGAS trên 50 câu, 5 cặp A/B chấm hai thứ tự, 10 nhãn người, tính κ, cổng CI chặn khi điểm tụt hơn 0.05.
- **Hệ thống mình có:** không có kho vector và không có truy xuất văn bản; "truy xuất" là chọn công cụ và tham số, đã đo trực tiếp bằng so khớp (tool, tham số, số lần gọi, đề xuất, rò rỉ, từ chối) trên 422 ca viết tay; RCA chấm bằng 8 tiêu chí xác định. Không có LLM chấm nên không có thiên lệch giám khảo và không tốn tiền chấm.
- **Vì sao không làm:** metric RAG không áp dụng khi không có RAG. LLM giám khảo sẽ thêm một nguồn chi phí và một nguồn nhiễu cần kiểm. Cái giá của việc không có: chưa đo được "câu chữ có hay không" và "số liệu trong câu trả lời có đúng không" (nay có đo gián tiếp bằng `mm_ai_answer_numbers_total`). Khi nào nên làm: nếu thêm RAG (vd hỏi đáp trên tài liệu hướng dẫn) hoặc muốn chấm chất lượng văn phong; khi đó làm cả bộ: chấm hai thứ tự, 10-20 nhãn người, κ.

### 2.5 Multi-agent, MCP, A2A (#39)

- **Để làm gì:** chia việc cho nhiều agent chuyên biệt (Coordinator, Order, Payment, Delivery, Policy, Verifier), giao tiếp có handoff (A2A); MCP là giao thức chuẩn để agent gọi công cụ và lấy bằng chứng qua một server (Evidence Gateway) có audit.
- **Bài đòi gì:** đề Olist: 50 yêu cầu, mỗi ca đối chiếu nhiều nguồn rồi kết luận có `evidence_ref` do hệ thống cấp, agent không tự tạo hay dùng chéo ca, dùng tool discovery, ghi trace mỗi lần dùng kết quả, Verifier kiểm ID, số tiền, schema. "Không có điểm cho việc đặt tên nhiều agent mà nhét hết vào một prompt."
- **Hệ thống mình có:** một agent mỗi persona với công cụ chọn lọc; tool discovery do gateway cấp (`GET /ai/tools` từ `scopes.go`); quyền ép ở gateway; mã bằng chứng `e1..eN` do hệ thống cấp và Verifier xác định cho RCA (mục 7, 40); nhật ký từng bước.
- **Vì sao không làm:** quyết định ở ADR-49. MCP server thêm một hop và một bề mặt tấn công, mà gateway đã làm đúng việc discovery, phân quyền, audit. Multi-agent làm tăng chi phí và độ trễ, và chính bài để câu hỏi mở "có đáng chi phí thêm không". Nếu đề bài yêu cầu: bọc một MCP mỏng chỉ-đọc gọi lại REST của gateway (không sửa gì phía sau).

## 3. Nhìn lại: điểm mạnh, khoảng trống, thứ tự nên làm

**Mạnh hơn mức các lab đòi:** quyền thật ép ở gateway (lệnh tiêm vào cũng không vượt vai), che dữ liệu nhạy cảm ở nguồn, hai cổng người cho hành động ghi, token hệ thống chỉ đọc có kiểm bằng token thật, checkpoint bền, dọn dữ liệu cũ. Các lab chủ yếu phòng thủ ở tầng prompt và bộ lọc; ở đây phần lớn ở tầng kiến trúc, nên ít phụ thuộc vào model nghe lời.

**Khoảng trống thật (cần):**
1. Kiểm chứng kết luận với bằng chứng (#7, #40). Ảnh hưởng độ đúng của RCA nhiều nhất, và là điểm yếu nhất theo tài liệu 10 §2.6.
2. Cổng chất lượng trong CI (#26): gần như miễn phí, vì phần offline đã có sẵn.
3. Độ lặp lại và tập giữ kín (#23, #27): trước khi nêu bất kỳ con số "đạt x%" nào trong báo cáo.
4. Chịu lỗi nhà cung cấp LLM (#28-#31): phân loại lỗi và ghi lý do vào phiên; model dự phòng nếu cần chạy liên tục.

**Cố ý không làm (có lý do):** memory bền (riêng tư), định tuyến theo độ tin cậy tự báo (không đáng tin), semantic cache (dữ liệu đổi liên tục), RAG metric và LLM giám khảo (không có RAG; chấm xác định), multi-agent/MCP/A2A (ADR-49).

**Thứ tự đề xuất:** #26 (CI, không tốn tiền) → thêm nhóm `missing_info` và tách tập giữ kín (#4, #23, không tốn tiền) → Verifier và `evidence_ref` cho RCA (#7, #40, theo ADR-49) → chạy lặp k lần trên 19 ca (#27, tốn tiền, cần đồng ý) → phân loại lỗi LLM và model dự phòng (#28, #31).

## 4. Đã làm sau đánh giá (2026-11-17, không gọi LLM)

Bốn khoảng trống ở mục 3 đã được xử lý trong phạm vi không cần mô hình thật. Chi tiết kỹ thuật và số đo: `11_RCA_BUILD_LOG.md` §16.35.

| Khoảng trống | Đã làm | Còn lại |
|---|---|---|
| 1. Kiểm chứng kết luận với bằng chứng | `rca/grounding.py` + nối vào đồ thị RCA; tiêu chí eval `grounded`; `guard/grounding.py` đo con số trong câu trả lời chat | Chưa chứng minh bằng chứng ủng hộ (mới kiểm có nhắc tới); chat chưa chặn; chưa đo trên mô hình thật |
| 2. Cổng chất lượng trong CI | Job `ai-agent` trong `ci.yml`; `push-images` cần nó | Chưa chạy trên GitHub; chưa có ngưỡng điểm cho eval live |
| 3. Tập giữ kín và chạy lặp | `--split dev/heldout`, nhóm `clarify` mới (16 ca), `--repeat k` với khoảng tin cậy Wilson | Chưa chạy live lần nào; quy ước dev/heldout chưa được ép bằng mã |
| 4. Chịu lỗi nhà cung cấp LLM | `llm/resilience.py`: phân loại lỗi, breaker theo profile, dự phòng `llm.fallbacks`, `max_retries` ghi rõ; chat báo `model_unavailable`; RCA ghi lý do lỗi | Chưa cấu hình profile dự phòng thật; chưa có cảnh báo Prometheus cho `mm_ai_llm_fallback_total` |

**Việc cần chi phí mô hình, CHƯA làm (chờ đồng ý từng lần):** chạy `run_rca_evals.py --live --repeat 3` (22 ca x 3 lần, tối đa khoảng 792 lượt gọi); chạy 3 ca `inject-*` thật; chạy `run_evals.py --live --split heldout` một lần khi chốt prompt; red team live cho `leak.py`.

## 5. Ghi chú về độ tin cậy của bản so sánh

Các mục [đọc code] đã đối chiếu với mã ngày 2026-11-17; chưa chạy riêng từng mục. Mục "không có thử lại ở `GatewayClient`" dựa trên tìm kiếm trong `tools/gateway.py` (không thấy từ khoá retry/breaker). Mặc định thử lại của SDK OpenAI chưa được kiểm tra trong lần chạy thật. Các bài nghiên cứu là ghi chú học tập ở trạng thái `draft`: dùng làm danh mục kiểm tra, không phải bằng chứng rằng mỗi tiêu chí là bắt buộc.

## 6. Điểm chưa chắc, cần kiểm thêm (ghi 2026-11-17, sau khi làm bốn khoảng trống)

Mục này liệt kê những chỗ code đã chạy qua test nhưng CHƯA chứng minh là đúng với mô hình thật hoặc dữ liệu thật. Mỗi dòng ghi rõ cách kiểm.

### 6.1 Kiểm chứng RCA bằng regex (`rca/grounding.py`)

| Điểm yếu | Hệ quả | Cách kiểm hoặc cải thiện |
|---|---|---|
| Rút "mẩu cần đối chiếu" bằng regex: tên có `-`, `_`, `.` (`_IDENT`), danh sách viết tay (`_COMPONENTS`: postgres, kafka, redis, clickhouse, traefik, prometheus, grafana, outbox, loki, tempo, vector), số 2+ chữ số (`_NUM`) | Mô hình viết "gateway" hay "cổng API" thì không rút được gì: giả thuyết thành `grounded=None` (không bị phạt), tức lọt kiểm tra | Chạy live, đếm tỷ lệ giả thuyết `None` trên 22 ca; nếu cao thì thêm tên từ tập thật của hệ thống (enum service trong `ops_list_actions`, kết quả `ops_system_health`) thay cho danh sách cứng |
| So khớp chuỗi con trên văn bản đã hạ chữ thường | "50" khớp trong "500"; tên ngắn khớp nhầm | Thêm ranh giới từ (`\b`) cho số và tên; thêm test |
| Chuẩn hoá số khác nhau: RCA chỉ đổi `,` thành `.` ("5,000" thành "5.000", không khớp "5000"); chat bỏ mọi ký tự không phải chữ số (`guard/grounding.py`) | RCA có thể hạ nhầm giả thuyết đúng vì khác định dạng số | Dùng cùng một hàm chuẩn hoá số cho cả hai nơi; test các định dạng "5,000", "5.000", "5000", "5k" |
| Regex tên bắt cả thứ không phải tên service ("v1.2", "e.g.") | Có thể đòi bằng chứng chứa những chuỗi vô nghĩa, hạ nhầm | Đo số giả thuyết bị hạ trên dữ liệu live và đọc tay từng ca |
| Kiểm "bằng chứng có NHẮC TỚI", không phải "bằng chứng ỦNG HỘ" | "payment-service" xuất hiện trong bằng chứng chưa nói nó đang chết | Hướng chắc hơn: bắt mô hình trả giả thuyết có cấu trúc `{component, metric, value, evidence_id}` rồi đối chiếu từng trường; đổi schema bước giả thuyết nên cần chạy lại eval live |
| Hạ độ tin cậy xuống 0.3 (dưới ngưỡng đề xuất 0.4) và thành `escalate_to_human` | Hạ nhầm giả thuyết đúng làm RCA ít hữu ích hơn (an toàn nhưng tốn công người) | Theo dõi tỷ lệ `escalated` trước/sau khi bật; so với 22 ca; chỉnh ngưỡng nếu tăng bất thường |
| Tham số hành động phải có trong bằng chứng hoặc thông tin sự cố | Tham số hợp lệ nhưng không xuất hiện nguyên văn (alias tên service) sẽ bị chuyển cho người | Thử trên tất cả hành động trong sổ với dữ liệu live; nếu cần thì cho phép đối chiếu theo tập enum của sổ hành động |
| Chưa chạy với mô hình thật | Mọi con số trong tài liệu này là với Brain đáp án và dữ liệu giả | Chạy live `run_rca_evals.py --repeat 3` (cần đồng ý chi phí) và xem tiêu chí `grounded` |

### 6.2 Chat: đo con số (`guard/grounding.py`)

- Chỉ xét số từ 4 chữ số trở lên (`MIN_DIGITS = 4`): bỏ sót số nhỏ (số lượng 12, giảm 20%). Số suy ra (tổng, làm tròn) bị tính "ungrounded" dù hợp lệ. Chưa biết tỷ lệ nhiễu: cần quan sát `mm_ai_answer_numbers_total{result="ungrounded"}` trên traffic thật vài ngày rồi mới đặt ngưỡng cảnh báo, và chưa nên chặn.
- `PumpResult.tool_text` chỉ lấy `ToolMessage` ở các nút không phải nút mô hình, cắt ở 200 KB; chưa kiểm với agent thật (test dùng agent giả) rằng nút công cụ đúng tên và văn bản đã che (model_view) có chứa số cần đối chiếu.
- Test `test_numbers_in_the_answer_are_counted_against_tool_data` yếu: chỉ kiểm chỉ số tồn tại, chưa kiểm số đếm tăng đúng. Nên viết lại với câu trả lời có số lớn.

### 6.3 Nhóm `clarify` (hỏi lại khi thiếu thông tin)

- **Xung đột với prompt hiện tại (đã kiểm bằng grep).** Prompt của buyer, seller, admin, operator có quy tắc "khi câu hỏi đã đủ để chọn bộ lọc thì gọi công cụ ngay, KHÔNG hỏi lại"; KHÔNG có quy tắc "thiếu thông tin bắt buộc thì hỏi lại". Nên 16 ca `clarify` nhiều khả năng TRƯỢT với mô hình thật. Chưa chạy nên chưa biết tỷ lệ.
- **Chữa nếu trượt:** sửa prompt (thêm một quy tắc ngắn: thiếu mã đơn/tên service/sản phẩm thì hỏi lại đúng một câu, không đoán), KHÔNG sửa vòng lặp hay thêm công cụ. Phải đo cân bằng hai chiều: nhóm `lookup`/`filter` (không được hỏi thừa, vì dòng "KHÔNG hỏi lại" được thêm để chữa lỗi hỏi thừa) và nhóm `clarify`. Chạy `run_evals.py --live --split dev` cả hai nhóm trước và sau (cần đồng ý chi phí).
- **Chuẩn chấm nghiêm có thể chưa hợp lý:** ca yêu cầu KHÔNG gọi công cụ nào và có dấu "?". Agent gọi `list_my_orders` rồi hỏi "đơn nào?" bị tính trượt dù có thể là hành vi tốt. Quyết định cần người: giữ nghiêm hay cho phép (`tool: [none, list_my_orders]`).
- **16 ca do AI viết, chưa được người duyệt.** Một số câu mơ hồ có thể có đáp án hợp lệ khác (vd admin: "Sản phẩm kia bán thế nào?", "Đơn đó của ai?"); cần bạn đọc lại và sửa hoặc bỏ.
- Dấu "?" là điều kiện duy nhất của `need`: câu trả lời có "?" ở chỗ khác (không phải hỏi lại) vẫn đạt. Nên thêm kiểm ngữ nghĩa nhẹ (nhắc tới "đơn nào", "service nào", "mã") hoặc đọc tay mẫu.

### 6.4 Tập giữ kín (held-out)

- Chỉ là quy ước (`--split`), không có cơ chế ép; chạy `heldout` hằng ngày là làm mất tác dụng.
- Cùng người viết và cùng phong cách với tập dev, nên chỉ chống được quá khớp "học thuộc ca"; chống chắc hơn cần ca VIẾT MỚI sau khi chốt prompt.
- Băm 1/5 theo câu hỏi cho tỷ lệ khoảng 12-20% mỗi persona (test chấp nhận 8-35%): một số nhóm nhỏ (vd `propose` của buyer) có thể không có ca heldout nào. Kiểm bằng `python evals/run_evals.py --split heldout` xem từng nhóm.
- Chưa chạy `heldout` live lần nào, nên chưa có điểm gốc để so.

### 6.5 Chịu lỗi nhà cung cấp LLM (`llm/resilience.py`)

- Mới thử bằng lỗi giả và mô hình giả; chưa thử với nhà cung cấp thật (OpenCode) trả 429/5xx/timeout. Cần kiểm xem SDK ném lỗi có thuộc tính `status_code` đúng như `classify` giả định (openai và anthropic có; lỗi từ lớp `httpx` thuần thì không, sẽ rơi vào `unknown` hoặc theo tên lớp).
- `max_retries: 2` của SDK và ngưỡng breaker (≥50% lỗi, ≥6 lần trong 60 s, ngắt 30 s) là giá trị khởi đầu, chưa chỉnh theo traffic thật. Với lưu lượng thấp (vài lượt mỗi phút) thì `min_calls: 6` có thể mất cả phút mới ngắt, hoặc ngắt vì một cụm lỗi ngắn.
- Lượt chat đang stream mà lỗi giữa chừng vẫn báo lỗi (không ghép mô hình dự phòng).
- `llm.fallbacks` để trống: chưa có profile dự phòng, chưa có kiểm tương thích tool calling tự động (chọn nhầm profile không hỗ trợ tool sẽ lỗi lúc chạy).
- Đã kiểm đường `create_agent` thật với một mô hình lỗi giả; chưa kiểm với `ModelCallLimitMiddleware` lồng cùng trong agent chạy thật dài (gọi nhiều vòng, có công cụ).
- Chưa có cảnh báo Prometheus cho `mm_ai_llm_fallback_total`.

### 6.6 CI và triển khai

- Job `ai-agent` chưa chạy trên GitHub; `requirements.lock.txt` được tạo trên máy dev (có thể có gói chỉ có ở máy này hoặc khác nền tảng).
- 252 test (13 bỏ qua) khi không có Postgres; 264 khi có (trước khi thêm test `create_agent` thật). Con số có thể đổi.
- ai-agent chưa được triển khai lại với các thay đổi này (tránh ngắt phiên `running`); cấu hình `llm.resilience`, `llm.fallbacks`, `rca.retention_days` chưa chạy trên Swarm. Nên chạy `scripts/smoke-rca-tools.sh` (không LLM) sau khi triển khai.

### 6.7 Việc kiểm thêm xếp theo thứ tự rẻ đến đắt

1. (Miễn phí) Viết lại test chỉ số `mm_ai_answer_numbers_total`; thêm test ranh giới từ và chuẩn hoá số cho `grounding.py`; thống nhất hàm chuẩn hoá số.
2. (Miễn phí) Bạn đọc 16 ca `clarify` và quyết chuẩn chấm nghiêm hay lỏng; xem `--split heldout` có nhóm nào rỗng.
3. (Miễn phí) Đẩy lên GitHub xem job `ai-agent` có xanh không.
4. (Tốn tiền, cần đồng ý) Chạy `run_evals.py --live --split dev --group clarify` và `--group lookup` để đo xung đột prompt `clarify` ở 6.3, trước khi sửa prompt.
5. (Tốn tiền) `run_rca_evals.py --live --repeat 3` để xem tiêu chí `grounded`, tỷ lệ giả thuyết `None`, và số ca bị hạ nhầm.

