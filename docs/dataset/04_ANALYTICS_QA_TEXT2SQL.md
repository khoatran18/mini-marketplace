# Trợ lý hỏi đáp số liệu (Analytics QA / Text-to-SQL)

> Seller/admin hỏi bằng tiếng Việt ("doanh thu tuần này của shop?", "sản phẩm nào sắp hết hàng?", "traffic từ đâu?") và nhận số liệu có kiểm chứng qua text-to-SQL / semantic layer / tool calling trên ClickHouse. **Trạng thái: chưa triển khai trong hệ thống; đây là tài liệu nghiên cứu dữ liệu.** Ngày tra cứu: 2026-10-25.
>
> Ghi chú phương pháp: các repo GitHub (Spider, Spider 2.0, BIRD, BIRD-Interact, LiveSQLBench, ViText2SQL, WikiSQL, Dr.Spider) được đọc trực tiếp; phần bổ sung (UCI Online Retail, số liệu Spider/SParC/CoSQL/WikiSQL, Gretel, KaggleDBQA) lấy từ kết quả WebSearch (Kaggle/HF/arXiv không mở được qua fetch). Phạm vi: dữ liệu tiếng Anh/quốc tế dùng được để chọn kiến trúc text-to-SQL; dữ liệu tiếng Việt: ViText2SQL hoặc tự tạo (mục 5). Theo quy tắc ngôn ngữ của dự án (chỉ giữ tiếng Anh/tiếng Việt), đã loại Olist Brazilian E-Commerce (văn bản tiếng Bồ Đào Nha). Cột "Quy mô / dung lượng" nêu số mẫu và dung lượng tải; "không tìm thấy" nghĩa là README/kết quả search không nêu con số (không suy đoán); Kaggle/HuggingFace/arXiv không mở trực tiếp được nên các số mới lấy từ snippet search.

| Liên quan | File |
|---|---|
| Tool-use / đánh giá agent | [16_AGENT_TOOL_USE_EVAL.md](16_AGENT_TOOL_USE_EVAL.md) |
| AIOps: giám sát hệ thống, cảnh báo | [14_AIOPS_ANOMALY_ALERT_EXPLAIN.md](14_AIOPS_ANOMALY_ALERT_EXPLAIN.md) |
| Dự báo nhu cầu / logistics | [09_DEMAND_FORECAST_LOGISTICS.md](09_DEMAND_FORECAST_LOGISTICS.md) |
| Phân tích khách hàng (CLV, churn) | [13_CUSTOMER_ANALYTICS_CLV_CHURN_PRICING.md](13_CUSTOMER_ANALYTICS_CLV_CHURN_PRICING.md) |
| Hiểu truy vấn tiếng Việt | [15_VIETNAMESE_NLP_QUERY_UNDERSTANDING.md](15_VIETNAMESE_NLP_QUERY_UNDERSTANDING.md) |
| Trợ lý mua sắm / CSKH (buyer) | [05_SHOPPER_ASSISTANT_CUSTOMER_SUPPORT.md](05_SHOPPER_ASSISTANT_CUSTOMER_SUPPORT.md) |
| Gian lận / bất thường | [08_FRAUD_ANOMALY_RISK.md](08_FRAUD_ANOMALY_RISK.md) |

## 1. Mục tiêu và vị trí trong hệ thống

- **Vai trò:** seller (chỉ dữ liệu **của store mình**), admin (toàn sàn + sức khoẻ hệ thống). Không dành cho buyer.
- **Input:** câu hỏi tiếng Việt + ngữ cảnh đăng nhập (vai trò, `store_id` lấy **từ JWT**, không từ nội dung câu hỏi — đúng quy ước "identity comes only from the JWT").
- **Output:** số liệu/bảng/biểu đồ nhỏ + giải thích ngắn + câu SQL/tool call đã chạy (để kiểm tra), kèm khoảng thời gian theo múi giờ `Asia/Ho_Chi_Minh`.
- **Nguồn dữ liệu (ClickHouse DB `marketplace`, xem `docs/design/02_DATA_MODEL.md` §3):**
  - `events_raw` (event_type, anonymous_id, session_id, user_id, role, store_id, surface, path, referrer, product_id, position, props, device_type, os, country, ua_family, analytics_consent, ts);
  - `fact_order_status` (một dòng cho mỗi (order_id, status): store_id, payment_method, payment_status, subtotal_minor, shipping_fee_minor, total_minor, at);
  - `fact_order_items` (order_id, item_id, store_id, product_id, category_id, qty, unit_price_minor, at);
  - `fact_payment_events`, `fact_refunds`, `dim_products`, `fact_inventory`.
  - Engine `ReplacingMergeTree` → mọi truy vấn phải dùng `FINAL`; tiền là `Int64` `*_minor` (1/100 VND) → đổi về VND khi trả ra.
- **Định nghĩa nghiệp vụ không được để LLM tự đoán:** doanh thu = `subtotal_minor` của đơn có dòng `PAID` (online) hoặc `DELIVERED` (COD) trong `fact_order_status`, trừ hoàn tiền (`fact_refunds`) theo thời điểm hoàn; GMV loại đơn `EXPIRED/FAILED` và `CANCELED` chưa từng `PAID`. Logic này nằm ở `ordersCTE` (`analytics-service/internal/store/queries.go`) — nên **bọc thành tool/semantic layer** thay vì để model viết lại.
- Có sẵn API báo cáo "scoped" của analytics-service (cache Redis `analytics:v1:*`), nên phương án an toàn nhất là agent **gọi các report API/tool đã có** trước, chỉ cho text-to-SQL trong phạm vi view đã duyệt.
- Giám sát hệ thống (admin): `data-health`, Prometheus/Grafana (`mm_outbox_pending`, v.v.) — xem file 14.

## 2. Hướng làm đề xuất (baseline → nâng cao)

1. **Tool calling trên báo cáo có sẵn (baseline, an toàn):** mỗi báo cáo = 1 function (doanh thu theo ngày, top sản phẩm, đơn theo trạng thái, tồn kho thấp, traffic theo nguồn...) với tham số (khoảng ngày, nhóm theo). LLM chỉ chọn tool + điền tham số; `store_id` do server gắn.
2. **Semantic layer:** định nghĩa metric (revenue, GMV, AOV, conversion, refund_rate), dimension (ngày theo giờ VN, store, category, brand, payment_method), join cố định; LLM sinh *truy vấn trên semantic layer* (JSON) thay vì SQL thô.
3. **Text-to-SQL có ràng buộc:** schema linking + few-shot + glossary tiếng Việt ("doanh thu" → `revenue`), chỉ đọc (user ClickHouse `readonly`), thêm bộ lọc `store_id` bằng cách **viết lại AST** hoặc row policy, giới hạn `LIMIT`, `max_execution_time`, whitelist bảng/view.
4. **Self-correction:** chạy → lỗi/rỗng → sửa; kiểm tra đối chiếu (tổng chi tiết = tổng).
5. **Fine-tune/đánh giá riêng** trên bộ vàng tự dựng (mục 5); dùng Spider/BIRD để chọn kiến trúc, không để quyết định cuối.

## 3. Metric đánh giá

- **Execution Accuracy (EX):** kết quả chạy SQL sinh ra = kết quả SQL vàng (chuẩn hoá thứ tự, làm tròn); **Test-suite Accuracy** (Spider); **Soft EX** (LiveSQLBench); **VES** (hiệu quả thực thi, BIRD).
- **Answer accuracy** ở mức đáp án tiếng Việt cuối (số đúng + đơn vị + khoảng thời gian đúng).
- **Tool-selection accuracy / argument F1** nếu dùng tool calling.
- An toàn: tỉ lệ rò dữ liệu chéo store (phải = 0), tỉ lệ truy vấn bị chặn (DDL/DML), p95 latency, chi phí/lượt hỏi, tỉ lệ từ chối đúng với câu hỏi mơ hồ/ngoài phạm vi.

## 4. Bộ dữ liệu

### Bảng tổng hợp

| # | Bộ dữ liệu | Nhóm | Quy mô / dung lượng | Ngôn ngữ | License | ★ |
|---|---|---|---|---|---|---|
| 1 | [ViText2SQL (VinAI)](https://github.com/VinAIResearch/ViText2SQL) | (a) | ~10K cặp câu hỏi–SQL (README GitHub); bản mirror HF `TeeA/ViText2SQL-2`: 6.831 train / 954 val / 1.908 test (search, mirror bên thứ ba); dung lượng: không tìm thấy (nhỏ, dạng JSON/SQL) | Việt | chỉ nghiên cứu/giáo dục, cấm phân phối lại | 5 |
| 2 | [BIRD](https://github.com/AlibabaResearch/DAMO-ConvAI/tree/main/bird) | (c) | 12.751 cặp (9.428 train / 1.534 dev), 95 DB, 37 domain; dung lượng 33,4 GB (README GitHub, là tổng dung lượng DB) | Anh | CC BY-SA 4.0 | 4 |
| 3 | [Spider 1.0](https://github.com/taoyds/spider) | (c) | 10.181 câu hỏi, 5.693 SQL khác nhau, 200 DB, 138 domain; chia 8.659 train / 1.034 dev / 2.147 test (đã xác minh, search paper); dung lượng tải: không tìm thấy | Anh | repo Apache-2.0 | 4 |
| 4 | [Spider 2.0 (Lite/Snow/DBT)](https://github.com/xlang-ai/Spider2) | (c) | 547 + 547 + 68 task (README GitHub); dung lượng: không tìm thấy | Anh | MIT | 3 |
| 5 | [LiveSQLBench](https://github.com/bird-bench/livesqlbench) | (c) | Base-Lite 270 task/18 DB; Base-Full v1 600/22; Large-v1 480/18 (README GitHub); dung lượng: không tìm thấy | Anh | MIT (sidebar) / CC BY-SA 4.0 (badge) — kiểm tra LICENSE | 4 |
| 6 | [BIRD-Interact](https://github.com/bird-bench/BIRD-Interact) | (c) | Full 600 task/22 DB (PostgreSQL), 244 bảng, 2.011 cột, ~272 MB; Lite 270 task (README GitHub) | Anh | code MIT, dữ liệu CC BY-SA 4.0 | 3 |
| 7 | [Dr.Spider](https://github.com/awslabs/diagnostic-robustness-text-to-sql) | (c) | 17 bộ nhiễu từ Spider dev (README GitHub); số mẫu và dung lượng: không tìm thấy | Anh | Apache-2.0 + CC-BY-4.0 | 3 |
| 8 | [WikiSQL](https://github.com/salesforce/WikiSQL) | (c) | 80.654 cặp, 24.241 bảng Wikipedia (đã xác minh, search); dung lượng `data.tar.bz2`: không tìm thấy | Anh | BSD-3-Clause | 2 |
| 9 | [SParC / CoSQL](https://yale-lily.github.io/sparc) | (c) | SParC 4.298 chuỗi câu hỏi; CoSQL ~3K hội thoại, 30K+ lượt, 10K+ SQL (search); dung lượng: không tìm thấy | Anh | chưa xác minh | 3 |
| 10 | [KaggleDBQA](https://arxiv.org/pdf/2106.11455) | (c) | 272 câu hỏi trả lời được (trong 400 thu thập) trên DB thật từ Kaggle (search); dung lượng: không tìm thấy | Anh | chưa xác minh | 3 |
| 11 | [Gretel synthetic_text_to_sql](https://huggingface.co/datasets/gretelai/synthetic_text_to_sql) | (c)/(d) | 105.851 bản ghi (100.000 train + 5.851 test), ~100 vertical, ~23M token (thông cáo HF/Gretel qua search); dung lượng GB: không tìm thấy | Anh | Apache-2.0 (đã xác minh, search) | 3 |
| 12 | [UCI Online Retail / Online Retail II](https://archive.ics.uci.edu/dataset/502/online+retail+ii) | (b) | 541.909 dòng (2010-12→2011-12) / 1.067.371 dòng (2009–2011) (trang UCI qua search); dung lượng file: không tìm thấy | Anh | CC BY 4.0 (đã xác minh, search UCI) | 3 |

### (a) Tiếng Việt / Việt Nam

**ViText2SQL (VinAI, EMNLP 2020 Findings)** — https://github.com/VinAIResearch/ViText2SQL (đã đọc). ~10K cặp câu hỏi–SQL tiếng Việt (bản dịch/điều chỉnh từ Spider, theo mô tả bài báo — chi tiết tokenization syllable/word: **chưa xác minh**). Điều khoản: chỉ nghiên cứu/giáo dục, cấm phân phối lại bản gốc/đã sửa, phải trích dẫn, "AS IS". Số lượng ~10K cặp (README); chia train/dev/test và định dạng file: chưa xác minh (README không liệt kê nội dung thư mục `data`; embedding Word2Vec syllable/word tải riêng). Ánh xạ: dùng để kiểm tra khả năng hiểu câu hỏi tiếng Việt → SQL (schema linking trên từ ghép tiếng Việt). ★5 (duy nhất tiếng Việt). Hạn chế: schema Spider (không phải TMĐT), license hạn chế, cần tự xác minh chất lượng dịch.

### (b) TMĐT công khai

**UCI Online Retail / Online Retail II** — https://archive.ics.uci.edu/dataset/502/online+retail+ii . Đã xác minh (nguồn: WebSearch, trang UCI): Online Retail II 1.067.371 dòng, giao dịch nhà bán lẻ online UK 2009–2011; Online Retail gốc 541.909 dòng, 6 feature, 01/12/2010–09/12/2011; cả hai **CC BY 4.0**. Một bảng giao dịch phẳng, dùng để tự dựng bộ câu hỏi doanh thu/top sản phẩm. ★3.

**Gretel synthetic_text_to_sql** — HF `gretelai/synthetic_text_to_sql`. Đã xác minh (nguồn: WebSearch/bài báo của Gretel và HPCwire/Analytics Vidhya): >105.851 bản ghi, ~100 vertical, mỗi bản ghi có giải thích bằng tiếng Anh, **Apache-2.0**; có metadata độ phức tạp SQL. Dùng để fine-tune/few-shot nền SQL, không phải TMĐT/ClickHouse. ★3.

### (c) Benchmark / paper học thuật

**BIRD** — https://github.com/AlibabaResearch/DAMO-ConvAI/tree/main/bird (đã đọc). 12.751 cặp text-to-SQL, 95 DB, 37 domain chuyên môn, 33,4 GB; **CC BY-SA 4.0**. Metric EX và VES (timeout 3 s/truy vấn, lặp 5 lần). Có "external knowledge" (gợi ý nghiệp vụ) -> tương ứng glossary/semantic layer của ta. ★4. Hạn chế: SQLite, tiếng Anh, DB nhỏ hơn thực tế ClickHouse.

**Spider 1.0** — https://github.com/taoyds/spider (đã đọc). Annotated by 11 college students; metric chính thức Test Suite Accuracy (từ 2020-11-15), còn Component/Exact Matching; repo Apache-2.0 (license dữ liệu không nêu riêng). Số liệu (nguồn: WebSearch, paper EMNLP 2018): 10.181 câu hỏi, 5.693 SQL phức tạp khác nhau, 200 DB nhiều bảng, 138 domain; chia chuẩn 8.659 train (146 DB) / 1.034 dev (20 DB) / 2.147 test (34 DB), không trùng DB giữa các tập. Dataset license riêng: **chưa xác minh** (một bài viết của Gretel ghi Spider là CC BY-SA 4.0 — nguồn thứ cấp). Nền tảng đối sánh. ★4. Hạn chế: schema đơn giản, ít phép tính thời gian/tiền.

**Spider 2.0** — https://github.com/xlang-ai/Spider2 (đã đọc). Spider 2.0-Snow 547 task (Snowflake), Lite 547 (BigQuery 214, Snowflake 198, SQLite 135), DBT 68 (DuckDB), MIT. Cần tài khoản BigQuery/Snowflake; README ghi (thông báo 2026-08-12) tài khoản Snowflake đánh giá đang bị đình chỉ; gold SQL chỉ phát hành một phần và không khuyến nghị fine-tune bằng nó. Gần quy trình doanh nghiệp (schema lớn, nhiều dialect). ★3. Hạn chế: khó chạy, chi phí cloud.

**LiveSQLBench** — https://github.com/bird-bench/livesqlbench (đã đọc). Base-Lite (2025-05-30): 18 DB, 270 task (180 SELECT, 90 CRUD); Base-Full v1 (2025-09-04): 22 DB, 600 task; Large-v1 (2026-03-02): 18 DB, 480 task (~1K cột, 54 bảng/DB); LiveSQLBench-CLI/Agent (2026-04-04). Mỗi bản phát hành thêm DB mới, tập ẩn bản này thành tập mở bản sau (chống nhiễm dữ liệu). Metric Soft EX, success rate, chi phí/tác vụ. License hai nơi mâu thuẫn (MIT vs CC BY-SA 4.0) → đọc LICENSE. ★4 (có nghiệp vụ thực + CRUD).

**BIRD-Interact** — https://github.com/bird-bench/BIRD-Interact (đã đọc). Full: 22 DB, 244 bảng, 2.011 cột (~272 MB), 600 task PostgreSQL gồm BI, CRUD; Lite: 18 DB, 270 task; hai chế độ c-Interact (hội thoại) và a-Interact (agentic). Code MIT; dữ liệu CC BY-SA 4.0. Phù hợp đánh giá hỏi lại làm rõ khi câu hỏi mơ hồ. ★3.

**Dr.Spider** — https://github.com/awslabs/diagnostic-robustness-text-to-sql (đã đọc). 17 bộ nhiễu từ Spider dev: DB_ (3), NLQ_ (9), SQL_ (5); Apache-2.0 + CC-BY-4.0. Dùng đo độ bền với diễn đạt khác/đổi tên cột. ★3.

**WikiSQL** — https://github.com/salesforce/WikiSQL (đã đọc). Câu hỏi đơn bảng (sel/agg/conds), tệp `data.tar.bz2`, SQLite; BSD-3-Clause; 80.654 cặp hỏi–SQL trên 24.241 bảng Wikipedia (nguồn: WebSearch, paper Seq2SQL arXiv 1709.00103). Quá đơn giản cho bài toán ta. ★2.

**CoSQL / SParC** (hội thoại text-to-SQL): trang GitHub `yale-nlp/cosql` trả 404 khi thử. Đã xác minh (nguồn: WebSearch): SParC 4.298 chuỗi câu hỏi mạch lạc; CoSQL ~3K hội thoại, 30K+ lượt, 10K+ SQL gán nhãn (thu theo kiểu Wizard-of-Oz); cả hai dựng trên cùng 200 DB của Spider, không trùng DB giữa các tập. License: **chưa xác minh**. Hữu ích cho hỏi đáp nhiều lượt ("còn tuần trước thì sao?"). ★3.

**KaggleDBQA** — arXiv 2106.11455 (ACL-IJCNLP 2021), tải ở aka.ms/KaggleDBQA (theo footnote paper). Đã xác minh (nguồn: WebSearch): 400 câu hỏi thu thập, ~1/3 ngoài phạm vi parser SOTA → 272 câu trả lời được; DB thật từ Kaggle, tên cột viết tắt khó hiểu, có kèm tài liệu DB (thêm >13,2% accuracy). Số DB và license: **chưa xác minh**. Gần thực tế hơn Spider (cột khó hiểu, tương tự `*_minor`). ★3.

### (d) Blog / cuộc thi / công ty

- Ghi chú: Gretel (synthetic_text_to_sql) ở mục (c) là bộ do công ty công bố. Chưa xác minh blog/báo cáo công ty (Shopee, Tiki, Lazada, Zalo) về analytics QA; dữ liệu tiếng Việt: ViText2SQL hoặc tự tạo.

## 5. Dữ liệu tự tạo / synthetic từ chính hệ thống

**Cách dựng bộ câu hỏi–đáp án vàng trên schema ClickHouse của dự án:**

1. **Sinh dữ liệu cố định (seed)**: dùng `SEED_DEMO_DATA=true` và/hoặc `scripts/dev/gen_traffic.py` + `scripts/dev/chdb_server.py` (ClickHouse không cần Docker) với **hạt giống ngẫu nhiên cố định** để mọi lần chạy ra cùng dữ liệu; đóng băng một snapshot làm "database vàng" (nhiều store, nhiều trạng thái đơn gồm PAID/DELIVERED COD/CANCELED/EXPIRED/REFUNDED, có hoàn tiền một phần, đơn qua nửa đêm giờ VN, sản phẩm tồn thấp).
2. **Viết 150–300 câu hỏi theo họ**, mỗi câu có: `question_vi` (nhiều cách diễn đạt), `role`, `store_id`, `now` cố định (để "tuần này", "hôm qua" xác định), `gold_sql`, `gold_result`, `notes`.
   - Doanh thu: theo ngày/tuần/tháng, theo store, theo `category_id`, theo `payment_method`; "doanh thu ròng sau hoàn tiền".
   - Đơn hàng: số đơn theo trạng thái, tỉ lệ huỷ/hết hạn, thời gian từ PAID → SHIPPED → DELIVERED (`fact_order_status`).
   - Sản phẩm: top bán chạy theo `qty`, sản phẩm tồn thấp (`fact_inventory.level`), tồn đọng.
   - Traffic: view/click/add_to_cart theo `surface`, `referrer`, `device_type`; tỉ lệ chuyển đổi (cần `events_raw` có consent).
   - Hệ thống (admin): outbox backlog, lỗi thanh toán (`fact_payment_events.failure_code`), độ trễ ingest (`ingested_at` vs `ts_server`).
   - **Bẫy:** `FINAL` bắt buộc; đổi `*_minor` → VND; múi giờ `toTimeZone(…,'Asia/Ho_Chi_Minh')`; không tính phí ship vào doanh thu; không tính đơn COD khi chưa DELIVERED; cô lập store (seller A hỏi dữ liệu seller B → phải từ chối/không rò).
3. **Câu vàng sinh bằng tay hoặc bán tự động:** viết SQL vàng bởi người, LLM chỉ đề xuất cách diễn đạt khác; hai người duyệt chéo. Kiểm tra đáp án vàng bằng **truy vấn độc lập** (ví dụ so với API báo cáo của analytics-service) để bắt lỗi định nghĩa.
4. **Câu hỏi âm (negative):** không có dữ liệu, ngoài phạm vi (hỏi dữ liệu cá nhân buyer), mơ hồ ("doanh thu" không kỳ) → kỳ vọng hành vi "hỏi lại/từ chối".
5. **Chia tập:** không chia ngẫu nhiên theo câu; chia theo **họ câu hỏi và mẫu SQL** (template) để thử khái quát; dành một nhóm "hold-out" chỉ chạy trước mỗi phát hành. Nếu dùng dữ liệu theo thời gian thật: dev/test là các tuần sau.
6. **Biến thể tiếng Việt:** có dấu/không dấu, viết tắt ("dt", "sp"), 1k = nghìn, "tuần trước" theo thứ Hai–Chủ nhật; mở rộng bằng perturbation kiểu Dr.Spider.
7. **Hồi quy sự cố:** mỗi lỗi sản xuất thành 1 ca vàng mới.

## 6. Lộ trình dùng dữ liệu đề xuất

1. **Bước 1:** dựng harness EX và bộ 30 câu vàng đầu tiên trên snapshot demo; thử baseline tool-calling trên API báo cáo có sẵn.
2. **Bước 2:** BIRD (dev) + Spider (test-suite) để chọn kiến trúc text-to-SQL (schema linking, few-shot, self-correction) và prompt "external knowledge"; ViText2SQL để kiểm hiểu tiếng Việt (chỉ nội bộ nghiên cứu, theo điều khoản).
3. **Bước 3:** mở rộng bộ vàng tự dựng lên 150–300; thêm biến thể Dr.Spider-style; đo rò dữ liệu chéo store.
4. **Bước 4:** LiveSQLBench / BIRD-Interact để thử hội thoại làm rõ + CRUD (chỉ dùng đọc ở bản ta).
5. **Bước 5:** pilot với vài seller, log câu hỏi thật (ẩn danh) → mở rộng bộ vàng.

## 7. Rủi ro, license, thiên lệch, lưu ý pháp lý/PII

- **An toàn truy vấn:** user ClickHouse chỉ-đọc, whitelist bảng/view, chặn `system.*`, giới hạn thời gian/bộ nhớ/hàng; luôn gắn `store_id` từ JWT ở tầng server (không tin LLM). Prompt injection qua dữ liệu (tên sản phẩm, `props`, ghi chú đơn là *dữ liệu không tin cậy*).
- **Rò rỉ dữ liệu:** seller không được thấy `user_id`/`buyer_id`, `ip_hash` hay dữ liệu cá nhân của buyer; lọc cột ở semantic layer. `events_raw` chỉ có dữ liệu khi `analytics_consent` đồng ý; số liệu traffic là mẫu, phải nói rõ.
- **Sai số nghiệp vụ:** LLM dễ sai định nghĩa doanh thu/GMV; dùng metric đã định nghĩa sẵn.
- **License:** ViText2SQL cấm phân phối lại → không đóng gói trong repo; BIRD CC BY-SA 4.0 (ShareAlike); LiveSQLBench có thông tin license mâu thuẫn → đọc file LICENSE; Spider 2.0 khuyến nghị không fine-tune bằng gold SQL.
- **Thiên lệch / nhiễm dữ liệu:** Spider/BIRD đã nằm trong dữ liệu huấn luyện LLM → điểm cao không chắc chuyển sang schema ta; LiveSQLBench cố tình xoay vòng để giảm nhiễm.
- **Chi phí:** truy vấn lớn trên `events_raw` (TTL 13 tháng) có thể đắt; đặt quota theo vai trò.

## 8. Nguồn tham khảo

Đã đọc trực tiếp (GitHub):
- https://github.com/taoyds/spider
- https://github.com/xlang-ai/Spider2
- https://github.com/AlibabaResearch/DAMO-ConvAI/tree/main/bird
- https://github.com/VinAIResearch/ViText2SQL
- https://github.com/salesforce/WikiSQL
- https://github.com/bird-bench/livesqlbench
- https://github.com/bird-bench/BIRD-Interact
- https://github.com/awslabs/diagnostic-robustness-text-to-sql

Tài liệu nội bộ: `docs/design/02_DATA_MODEL.md` (§3 ClickHouse), `analytics-service/internal/store/queries.go` (`ordersCTE`).

Đã xác minh qua WebSearch (không mở trang gốc): UCI Online Retail (trang archive.ics.uci.edu/dataset/502), số liệu Spider/SParC/CoSQL/WikiSQL/KaggleDBQA/Gretel. Chưa xác minh: license SParC/CoSQL, KaggleDBQA; dung lượng tải của hầu hết bộ (search không trả về GB); split và tokenization của ViText2SQL.
