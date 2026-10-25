# Coroot, và agent điều tra sự cố cần kịch bản / dữ liệu gì

> **Trạng thái: nghiên cứu. Chưa cài Coroot, chưa có agent điều tra sự cố.** Phần về Coroot dựa trên trang chủ [coroot.com](https://coroot.com/) và kết quả tìm kiếm (docs cài đặt, bài InfoQ 2024); **chưa chạy thử**. Phần về benchmark RCA dựa trên [docs/dataset/14](../dataset/14_AIOPS_ANOMALY_ALERT_EXPLAIN.md) và một lượt tìm kiếm 2026-10-26 (chưa mở bài gốc).

## 0. Từ viết tắt dùng trong tài liệu này

| Viết tắt | Đầy đủ | Nghĩa |
|---|---|---|
| **RCA** | *Root Cause Analysis*, phân tích nguyên nhân gốc | từ triệu chứng nhìn thấy (ví dụ "đặt hàng chậm", "analytics trả 503") truy ngược ra **thứ thật sự gây lỗi** (ví dụ "ClickHouse hết đĩa"). Khác với *phát hiện bất thường* (chỉ nói "có gì đó lạ"): RCA trả lời "**vì sao** và **ở đâu**". Trong hệ nhiều service, lỗi ở một nơi kéo theo lỗi ở nơi gọi nó, nên service báo lỗi nhiều nhất thường **không** phải nguyên nhân gốc |
| **eBPF** | *extended Berkeley Packet Filter* | cơ chế của nhân Linux cho phép chạy chương trình nhỏ, an toàn trong kernel để quan sát (kết nối mạng, syscall, CPU...) mà không phải sửa code ứng dụng |
| **SLO** | *Service Level Objective* | mục tiêu chất lượng của một service, ví dụ "99% request dưới 500 ms" hoặc "tỉ lệ lỗi dưới 1%"; vi phạm SLO là tín hiệu có sự cố |
| **AIOps** | *AI for IT Operations* | dùng ML / LLM cho vận hành: phát hiện bất thường, RCA, giải thích cảnh báo |
| **MTTD / MTTR** | *Mean Time To Detect / Repair* | thời gian trung bình để phát hiện / khắc phục sự cố |
| **Top-k / AC@k** | độ chính xác Top-k | RCA trả về danh sách xếp hạng nguyên nhân nghi vấn; đúng nếu nguyên nhân thật nằm trong k vị trí đầu |
| **VUS-PR** | *Volume Under the Surface, Precision-Recall* | metric đánh giá phát hiện bất thường chuỗi thời gian, ít bị thổi phồng hơn point-adjust F1 |

## 1. Coroot là gì

Coroot là nền tảng **observability mã nguồn mở** (Community Edition trên GitHub, có bản Enterprise trả phí) gom metric, log, trace và profile vào một giao diện, nhấn mạnh hai thứ:

| Thứ | Ý nghĩa |
|---|---|
| **Thu thập bằng eBPF, "không sửa code"** | một *node agent* chạy trên mỗi máy (systemd hoặc container) quan sát từ kernel: theo dõi các sự kiện TCP (`connect()`, `listen()`) để tự dựng **bản đồ phụ thuộc** giữa service, trích mẫu log, đo latency / lỗi giữa các service. Service không cần gắn SDK tracing |
| **Phân tích nguyên nhân gốc (RCA) tự động** | kết hợp bản đồ phụ thuộc với các "inspection" dựng sẵn (CPU, bộ nhớ, đĩa, mạng, lỗi, latency, SLO...) để chỉ ra service / thành phần nghi vấn và gợi ý hướng sửa. Tác giả nói inspection dựng sẵn xác định được nguyên nhân cho hơn 80% sự cố không cần cấu hình (số của tác giả, qua bài InfoQ 2024, chưa kiểm chứng độc lập) |

Yêu cầu và cách chạy theo tài liệu cài đặt: kernel Linux từ 4.16 (profiling CO-RE cần bản mới hơn, như Ubuntu 20.10+ / Debian 11+); trong tài liệu, **Docker Swarm nằm trong danh sách orchestrator được hỗ trợ**, và bản 1.0 nói có thể triển khai cả stack bằng Docker Compose hoặc Swarm. Tôi **không** tìm được lệnh cài cụ thể cho Swarm trong lượt tìm kiếm; phải đọc trang cài đặt gốc.

## 2. Nó giúp gì ở đây: khoanh vùng khi có sự cố

Stack hiện tại đã có Prometheus + Grafana (11 dashboard) + Loki / Vector + cAdvisor, nhưng:
- **không có tracing** (ADR-27 để OpenTelemetry sau) nên không có bản đồ "service nào gọi service nào, chậm ở cạnh nào" lúc chạy; sơ đồ phụ thuộc chỉ nằm trong tài liệu runbook;
- **không có cảnh báo** (ADR-12), người vận hành tự nhìn dashboard;
- khi sự cố xảy ra phải tự ghép: dashboard nào đỏ ⇒ service nào ⇒ log nào.

Coroot nhắm đúng chỗ đó:

| Việc | Coroot làm thế nào | So với hiện tại |
|---|---|---|
| Biết service nào đang hỏng và **ai bị ảnh hưởng** | bản đồ phụ thuộc tự dựng + SLO theo service (latency, tỉ lệ lỗi) | hiện phải đọc dashboard *Services (RED)* rồi tự suy ra phụ thuộc |
| Khoanh vùng: lỗi do service đó hay do thứ nó gọi (Postgres, Kafka, Redis, ClickHouse) | RCA đi ngược bản đồ phụ thuộc, chỉ ra thành phần đầu nguồn | hiện dùng cây quyết định trong runbook bằng tay |
| Thấy vấn đề hạ tầng đi kèm (CPU throttling, bộ nhớ, đĩa, mạng, DNS) | node agent đo từ kernel | có cAdvisor / node-exporter nhưng không liên kết tự động với service bị ảnh hưởng |

Các loại sự cố trong repo mà nó có khả năng khoanh vùng (suy luận, **chưa thử**): Postgres / Redis / Kafka / ClickHouse down (kịch bản `resilience.sh` B, E, F, I), một service down kéo theo service gọi nó (C, D, G). Với lỗi **nghiệp vụ** của repo (dòng outbox `DEAD`, Kafka lag của một consumer group, DLQ, đơn kẹt `PENDING`) nó **không** có khái niệm sẵn; các thứ đó vẫn cần metric `mm_*` và runbook.

Cân nhắc trước khi cài:
- trùng chức năng với Prometheus / Grafana / Loki đã có; Coroot có kho lưu trữ riêng (Prometheus và ClickHouse của nó) nên tốn thêm RAM / đĩa; dự án đã có chế độ `LITE` cho máy yếu;
- node agent chạy đặc quyền và cần kernel mới; chưa kiểm tra máy này;
- phiên bản Enterprise có tính năng AI riêng (trang chủ có nhắc "AI-based root cause analysis" và "agentic-ready observability" nhưng không nêu chi tiết); cần đọc bảng so sánh Community / Enterprise trước khi dựa vào.

### 2.1 Kết luận: hiện tại **chưa cần** Coroot

Quy mô hệ thống hiện tại: 22 service Swarm trên **một node** (7 service Go + gateway 2 replica, frontend, Traefik, Postgres, Redis, Kafka, ClickHouse, và stack giám sát), đồ thị phụ thuộc **nhỏ và đã được viết ra** (sơ đồ trong runbook §1: Postgres → 5 service, Kafka critical với cả 7, ClickHouse → analytics, payment / analytics là non-critical với gateway). Với quy mô này:

| Câu hỏi khi có sự cố | Đã trả lời được bằng |
|---|---|
| Service nào đang hỏng | `GET /admin/system/health`, `/ready` từng service, dashboard *Overview* / *Services (RED)* |
| Hỏng vì thứ nó phụ thuộc hay vì chính nó | sơ đồ phụ thuộc trong runbook + `/ready` có `checks` JSON |
| Một request cụ thể chết ở đâu | `request_id` xuyên service (ADR-27) + Loki: lọc `request_id` ra log của gateway → service gọi (nhãn Loki chỉ `service`, `stack`, `level`, `env`) |
| Lỗi nghiệp vụ (outbox `DEAD`, DLQ, Kafka lag, đơn kẹt) | metric `mm_*` + runbook; **Coroot không hiểu các loại này** |

Nói cách khác: truy log theo `request_id` kết hợp metric `mm_*` và runbook **là đủ** cho tình huống hiện tại, và đã dựng sẵn. Coroot sẽ thêm một hệ thống nữa phải vận hành (kho lưu trữ riêng, agent đặc quyền, kernel ≥ 4.16) mà phần lớn việc nó làm đã có sẵn. Đây là đánh giá của tôi, chưa cài thử để đo.

Khi nào đáng cân nhắc lại:
- khi chạy **nhiều node** hoặc nhiều hơn khoảng một chục service, đồ thị phụ thuộc không còn nhớ được bằng tay;
- khi cần **tracing** mà không muốn sửa code từng service (ADR-27 đang để OpenTelemetry sau; eBPF là lối đi không cần SDK);
- khi dựng agent điều tra sự cố (scope 14) và muốn một nguồn **bản đồ phụ thuộc + kết luận RCA có sẵn** để so với agent LLM (mục 3). Lúc đó nên thử trên một máy riêng trước.

## 3. Ghép với AI agent: Coroot là nguồn tín hiệu, không phải đối thủ

Ý đồ phù hợp với [docs/ai/06_AGENT_FEATURES.md](06_AGENT_FEATURES.md) (agent admin chỉ-đọc): agent **không tự suy luận từ log thô**, mà gọi công cụ trả về dữ liệu đã tóm tắt. Coroot có thể là một trong các nguồn:
- tool `service_dependencies` / `root_cause_hint`: lấy bản đồ phụ thuộc và kết luận RCA từ Coroot (cần kiểm tra Coroot có API đọc ổn định; **chưa xác minh**);
- LLM chỉ **diễn giải** và trích dẫn bằng chứng (đúng với kết luận của docs/dataset/14: LLM làm RCA đầy đủ còn yếu, xem mục 4).

Cách đo có ý nghĩa: **so ba hệ trên cùng một lỗi có nhãn**: (a) baseline thống kê (z-score / đồ thị phụ thuộc), (b) Coroot, (c) agent LLM có tool. Lỗi có nhãn lấy từ kịch bản `resilience.sh` (mục 5).

## 4. Đã có kịch bản / dataset / test case cho agent điều tra sự cố chưa

**Benchmark công khai** (từ docs/dataset/14 và lượt tìm kiếm; số liệu chưa kiểm chứng ở trang gốc):

| Bộ | Nội dung | Ghi chú |
|---|---|---|
| **RCAEval** | 735 ca lỗi trên 3 hệ vi dịch vụ (Online Boutique, Sock Shop, Train Ticket), 11 loại lỗi; RE1 chỉ metric, RE2 metric + log + trace, RE3 lỗi mức mã; có 15 baseline | MIT; ~38 GB giải nén; dữ liệu là bản dump quá khứ (có bài phê bình việc này); schema cột RE1 không đồng nhất giữa các hệ |
| **OpenRCA** | 335 lỗi, >68 GB, đo LLM agent làm RCA | agent tốt nhất chỉ ~11% (Claude 3.5) theo file 14; nghiên cứu 2026 báo 3.9–12.5% trên 5 model |
| **AIOpsLab**, **ITBench** | *môi trường* sinh sự cố có chèn lỗi (ChaosMesh...) và chấm agent, không phải bộ nhãn sẵn | AIOpsLab bị phê bình là có nhiễu không tất định (Cloud-OpsBench) |
| **Cloud-OpsBench** (2026, qua search) | 452 ca lỗi, 40 loại nguyên nhân, "digital twin" tất định trên Kubernetes | cho Kubernetes, không phải Swarm |
| **ORCA-bench** (2026, qua search) | 1.079 tác vụ RCA on-call trên hệ OpenTelemetry thật | tác giả báo model tốt nhất đúng ~25% |
| TSB-AD, SMD, NAB | phát hiện bất thường chuỗi thời gian | cho phần "phát hiện", không phải RCA |

**Trong repo này:**

| Thứ | Có gì | Dùng làm gì |
|---|---|---|
| `scripts/e2e/resilience.sh` A..I | 9 kịch bản **chèn lỗi thật** bằng `docker service scale` (ClickHouse, Kafka, payment, product, Redis, Postgres, order-service, rolling restart, Postgres restart), mỗi cái có assertion hành vi hệ thống | biết **service gốc** và thời điểm dừng / bật ⇒ có thể gán nhãn nguyên nhân gốc |
| `docs/instruction/03_RUNBOOK.md` §3–9 | cây quyết định: đơn kẹt, payment không hiện, outbox backlog / `DEAD`, Kafka lag và DLQ, thanh toán lỗi, ClickHouse down / spool, tồn kho sai | đáp án chuẩn (đường chẩn đoán + lệnh) cho câu hỏi điều tra |
| Prometheus (`mm_*`, kafka-exporter, cAdvisor...), Loki + Vector (log JSON có `request_id`), `/admin/system/health` | nguồn dữ liệu thật cho tool của agent | đầu vào cho tool |
| `PAYMENT_CHAOS_RATE` (ENV=dev) | sinh `provider_error` ngẫu nhiên | một loại lỗi nghiệp vụ |

**Còn thiếu (chưa có gì trong repo):**
1. **Bộ eval**: chưa có file câu hỏi + đáp án + bằng chứng đúng. docs/dataset/14 đề xuất 100–200 câu hỏi tiếng Việt, nhưng chưa dựng.
2. **`resilience.sh` chỉ kiểm hệ thống phục hồi, không ghi nhãn**: không xuất `{start, end, root_service, fault_type}` và không lưu snapshot metric / log trong cửa sổ lỗi, nên chưa có dữ liệu dạng RCAEval để dùng lại baseline.
3. **Chưa có tool cho agent**: `system_metrics`, `outbox_status`, `kafka_lag`, `log_search` (REST admin còn thiếu); `ai_audit` chưa có.
4. **Chưa có cảnh báo** (ADR-12), nên "giải thích cảnh báo" chưa có đầu vào thật.
5. **Không có tracing**, nên chỉ tái hiện được dạng RE1 (metric) và một phần RE2 (metric + log).
6. **Thiếu nhiều loại lỗi**: nhiều lỗi đồng thời, lỗi chậm dần (latency, rò bộ nhớ), poison message / dòng `DEAD` (runbook mô tả nhưng `resilience.sh` không chèn), ClickHouse đầy đĩa, Postgres chậm.
7. **Chưa đo được MTTD / top-k** vì không ghi mốc thời gian lỗi.
8. Mâu thuẫn tài liệu cần sửa: docs/ai/06 mục A1 ghi "`log_search` chưa có kho log" trong khi Loki + Vector đã chạy (ưu tiên runbook / `observability.yml`).

**Đề xuất tối thiểu để có bộ eval đầu tiên** (chưa làm): (1) cho `resilience.sh` ghi `ground_truth.json` mỗi kịch bản (`start`, `end`, `root_service`, `fault_type`) và chụp truy vấn Prometheus + log Loki trong cửa sổ lỗi; (2) thêm các kịch bản thiếu ở mục 6 (poison row, ClickHouse đầy đĩa, latency Postgres); (3) viết 50–100 câu hỏi tiếng Việt gắn với từng nhãn, đáp án lấy từ runbook; (4) chạy baseline thống kê trước, rồi Coroot (nếu cài), rồi agent; chấm Top-k, thời gian phát hiện và độ đúng của trích dẫn bằng chứng.

Nguồn: [coroot.com](https://coroot.com/) · [tài liệu cài đặt Coroot Community](https://coroot.com/docs/coroot-community-edition/getting-started/installation) · [InfoQ 2024](https://www.infoq.com/news/2024/05/coroot-apm-observability/) · [RCAEval](https://arxiv.org/abs/2412.17015) · [Cloud-OpsBench](https://arxiv.org/abs/2603.00468) · [ORCA-bench](https://www.traversal.com/resources/solution-briefs/orca-bench-how-ready-are-language-model-agents-for-oncall).

## 5. Dùng Coroot thì dự án mất giá trị hay thêm giá trị

Câu trả lời phụ thuộc vào **cách dùng**; đây là đánh giá của tôi, không phải kết luận đã kiểm chứng.

**Dùng làm nguồn tín hiệu và đối chứng: thêm giá trị.**
- Nếu dự án có phần "agent điều tra sự cố" (scope 14), đo agent đó **so với một công cụ thật** (Coroot) và so với baseline thống kê thì kết luận mạnh hơn nhiều so với chỉ so với chính nó. Cách trình bày trung thực: "agent làm được X so với Coroot Y trên cùng bộ lỗi có nhãn từ `resilience.sh`".
- Những thứ Coroot **không làm** và dự án vẫn là của riêng nó: lỗi **nghiệp vụ** (dòng outbox `DEAD`, DLQ, Kafka lag của một consumer group, đơn kẹt `PENDING`), điều tra **dựa trên runbook** của chính hệ thống, hỏi đáp bằng **tiếng Việt**, đề xuất hành động có người xác nhận (`action_proposal`), và **bộ eval** cho tất cả những thứ đó. Đó mới là phần có giá trị riêng của dự án.
- Các đề tài nghiên cứu độc lập (§5.1 của [01_AI_SCOPES_OVERVIEW.md](01_AI_SCOPES_OVERVIEW.md): phát hiện bất thường trên TSB-AD / SMD, RCA trên RCAEval) **không bị ảnh hưởng**: chúng làm trên dữ liệu công khai, chỉ khi tích hợp mới liên quan đến Coroot.

**Dùng thay cho phần tự làm: mất một phần giá trị.**
- Nếu mục tiêu là cho thấy "tôi tự xây được một bộ khoanh vùng sự cố hạ tầng", thì với **lỗi hạ tầng** (Postgres / Kafka / một service down) Coroot đã làm sẵn và tốt hơn nhiều so với bản tự viết trong vài tuần. Khi đó phần đó chỉ còn là "tích hợp một công cụ có sẵn", và phải nói rõ như vậy.
- Rủi ro khác: phụ thuộc một hệ thống bên ngoài như hộp đen (một số tính năng AI chỉ có ở bản Enterprise trả phí), thêm vận hành và tài nguyên.

**Gợi ý định vị:** coi Coroot là **đối thủ tham chiếu và nguồn dữ liệu**, không phải đích. Phần của dự án là: tool cho agent + runbook làm nguồn tri thức + lỗi nghiệp vụ + bộ eval tiếng Việt có nhãn. Như vậy có Coroot thì dự án **chặt hơn** (có baseline mạnh để so), chứ không rỗng đi, miễn là không tuyên bố mình "tự làm RCA hạ tầng" khi phần đó do Coroot làm.

## 6. Đo RCA / agent điều tra sự cố bằng metric nào

Đo ở **ba tầng** khác nhau; mỗi tầng trả lời một câu hỏi khác. Nguồn: docs/dataset/14 và 16, và phần ghi chú về RCAEval; các định nghĩa dưới đây là cách dùng phổ biến, **hãy đối chiếu lại với paper / README của từng benchmark trước khi báo cáo số**.

| Tầng | Câu hỏi | Metric | Cách hiểu |
|---|---|---|---|
| **1. Phát hiện** | "Có sự cố không, bắt đầu khi nào?" | **VUS-PR** (khuyến nghị của TSB-AD), PR-AUC, **detection delay** (độ trễ phát hiện), điểm NAB (thưởng phát hiện sớm, phạt báo nhầm) | F1 có *point-adjust* bị chê là thổi phồng (một điểm đúng trong đoạn lỗi thì cả đoạn tính đúng) nên không nên chỉ báo số đó |
| **2. Định vị nguyên nhân (RCA)** | "Nguyên nhân gốc là service / metric nào?" | **Top-k accuracy (AC@k)**: nguyên nhân thật nằm trong k vị trí đầu của danh sách xếp hạng; **Avg@k**: trung bình AC@1..AC@k (RCAEval hay dùng k = 5); với agent LLM kiểu OpenRCA: đúng **đồng thời** thời điểm, thành phần và lý do | AC@1 khắt khe nhất; Avg@5 cho điểm một phần khi đáp án đứng hạng 2–5 |
| **3. Giải thích và hành động** | "Giải thích có đúng, có bằng chứng, có đề xuất an toàn không?" | độ đúng của **trích dẫn bằng chứng** (mỗi nhận định trỏ đúng metric / dòng log), đánh giá của người + LLM-judge đã hiệu chuẩn, tỉ lệ **bịa** (nhận định không có trong dữ liệu tool trả về); với agent có tool: tool-selection / argument accuracy, **pass^k** (đúng cả k lần chạy), tỉ lệ gọi tool vượt quyền (mục tiêu 0), chi phí và độ trễ | chấm "quyết định" (tool call, trạng thái cuối) thì tin cậy hơn chấm văn bản bằng LLM-judge |
| **Vận hành** (khi chạy thật) | "Có giúp người vận hành không?" | **MTTD / MTTR**, số cảnh báo giả mỗi ngày, tỉ lệ đề xuất được người chấp nhận | chỉ đo được sau khi có sử dụng thật |

Điểm cần biết:
- **Top-k chỉ chấm danh sách xếp hạng**, nên hợp với RCA thuật toán (baseline thống kê, đồ thị phụ thuộc). Agent LLM trả lời bằng ngôn ngữ nên thường chấm bằng "đúng nguyên nhân hay không" cộng bằng chứng; vì vậy cần bộ đáp án có cấu trúc (mục 7).
- Điểm của agent LLM trên benchmark RCA công khai **còn thấp**: file 14 ghi OpenRCA tác tử tốt nhất ~11%, và kết quả tìm kiếm cho ORCA-bench báo model tốt nhất ~25% (qua search, chưa mở bài gốc). Nên đặt kỳ vọng và luôn có baseline đơn giản để so.
- Bộ phát hiện đơn giản (z-score, PCA) thường **ngang** mô hình sâu trên nhiều benchmark (file 14, TSB-AD): cần so với baseline trước khi kết luận mô hình phức tạp tốt hơn.

## 7. Một bộ dữ liệu RCA chứa những gì (ví dụ cụ thể)

Một **ca lỗi** (case) trong benchmark RCA là một "vụ tai nạn" được ghi lại cùng đáp án. Mỗi ca thường gồm các phần sau:

| Phần | Nội dung | Có trong benchmark nào |
|---|---|---|
| **Metric** | chuỗi thời gian theo từng service / container: CPU, bộ nhớ, độ trễ, số request, tỉ lệ lỗi, đọc / ghi đĩa..., lấy mẫu mỗi vài giây | RCAEval RE1, RE2 |
| **Log** | các dòng log của từng service trong khoảng lỗi | RCAEval RE2, OpenRCA, Loghub |
| **Trace** | đường đi của từng request qua các service (span cha–con, thời gian từng đoạn) | RCAEval RE2, OpenRCA |
| **Mã nguồn / bản vá** | với lỗi ở mức code: service nào, hàm nào | RCAEval RE3 |
| **Mốc thời gian chèn lỗi** | thời điểm lỗi bắt đầu (`inject_time`), có khi cả thời điểm kết thúc | RCAEval, AIOpsLab |
| **Đáp án (nhãn)** | **service gốc**, **loại lỗi** (ví dụ CPU quá tải, rò bộ nhớ, mất gói tin, trễ mạng, service down), có khi cả **metric** biểu hiện lỗi | tất cả |
| **Câu hỏi (với benchmark cho LLM)** | một câu mô tả triệu chứng, ví dụ "lúc 14:05 tỉ lệ lỗi tăng, nguyên nhân là gì?" kèm đáp án chuẩn (thời điểm, thành phần, lý do) | OpenRCA, ORCA-bench |

Cách các benchmark kiểu RCAEval sinh dữ liệu: dựng một hệ vi dịch vụ mẫu (Online Boutique 12 service, Sock Shop 15, Train Ticket 64), sinh tải giả, **chèn lỗi có chủ đích** vào một service ở một thời điểm đã biết (bằng công cụ chaos), ghi lại toàn bộ telemetry, và lấy "service bị chèn lỗi" làm đáp án. Vì vậy nhãn là **miễn phí và chính xác**, đổi lại dữ liệu là hệ mô phỏng, không có mùa vụ và sự cố tự nhiên như hệ thật.

**Ví dụ minh họa trên chính repo này** (đây là ví dụ **tôi dựng ra để dễ hình dung**, không phải dữ liệu có thật; đây cũng là định dạng đề xuất cho bộ eval ở mục 4):

```text
ca_clickhouse_down_01/
  ground_truth.json    {"root_service": "clickhouse", "fault_type": "service_down",
                        "start": "2026-10-26T14:03:10+07:00", "end": "2026-10-26T14:08:40+07:00"}
  metrics.csv          time, analytics_http_503_rate, mm_ch_insert_lag_seconds, mm_ch_rows_dropped_total, up_clickhouse
                       14:03:00, 0, 0.4, 0, 1
                       14:03:30, 0.8, 31, 0, 0        <- ClickHouse tắt
                       14:05:00, 1.0, 120, 0, 0
  logs.jsonl           {"ts":"14:03:31","service":"analytics-service","level":"error","msg":"clickhouse insert failed: dial tcp clickhouse:8123: connection refused"}
                       {"ts":"14:03:32","service":"api-gateway","level":"warn","msg":"analytics unavailable","request_id":"..."}
  question.txt         "Lúc 14:05 dashboard analytics báo lỗi, vì sao và tôi nên làm gì?"
  expected_answer.json {"cause": "ClickHouse không chạy từ 14:03", "evidence": ["up_clickhouse=0", "log connection refused"],
                        "not_affected": ["đặt hàng", "POST /events vẫn 202"], "action": "bật lại ClickHouse; xem runbook §8"}
```
Một agent được chấm: (1) có tìm đúng `clickhouse` là nguyên nhân không (tầng 2), (2) bằng chứng nó trích có thật trong `metrics.csv` / `logs.jsonl` không và nó có bịa gì không (tầng 3), (3) có nói đúng rằng đặt hàng không bị ảnh hưởng (ADR-25) không. Mỗi kịch bản trong `resilience.sh` (A..I) có thể sinh ra một ca như vậy; hiện **chưa có** bước xuất `ground_truth.json` và chụp metric / log (xem mục 4, "còn thiếu").

Lưu ý khi dùng RCAEval thật: các nhà phân tích đã chỉ ra schema cột RE1 không đồng nhất giữa 3 hệ thống, và tra cứu ngây thơ trả rỗng ở 250 / 375 ca (theo file 14), nên phải đọc hướng dẫn của repo trước khi viết trình nạp dữ liệu.
