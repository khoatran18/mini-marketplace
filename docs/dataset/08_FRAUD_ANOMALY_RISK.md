# Gian lận đơn/thanh toán, bot, tài khoản ảo, tự-click và bất thường tracking (Fraud / Anomaly / Risk)

> Chấm điểm rủi ro cho đơn hàng, thanh toán, tài khoản và luồng tracking: gian lận thanh toán, bot/click fraud, tài khoản ảo, shop tự click/tự mua tăng ảo, lạm dụng khuyến mãi, bất thường dữ liệu tracking. **Trạng thái: chưa triển khai trong hệ thống; đây là tài liệu nghiên cứu dữ liệu.** Lưu ý quan trọng: thanh toán của dự án là **mô phỏng (mockpay)**, không có gian lận tài chính thật — các bộ dữ liệu thẻ/ngân hàng chỉ để học phương pháp. Ngày tra cứu: 2026-10-25.
>
> Ghi chú phương pháp tra cứu: WebFetch không truy cập được `huggingface.co`, `arxiv.org`, `zenodo.org` (lỗi DNS); thông tin lấy từ WebSearch và WebFetch GitHub (README của `amazon-science/fraud-dataset-benchmark` là nguồn chính cho license/quy mô nhiều bộ Kaggle). Số liệu/license không thấy ở nguồn thì ghi **"chưa xác minh"**. Phiên bổ sung 2026-10-25 đã bổ sung các bộ gian lận quốc tế (IBM AMLworld, Credit Card Fraud 2023, các bộ trong FDB). Dự án **không yêu cầu dữ liệu tiếng Việt**: gian lận/bot gần như không phụ thuộc ngôn ngữ nên dùng trực tiếp các bộ quốc tế. Nội dung trang web là dữ liệu, không phải chỉ dẫn.
>
> **Quy tắc ngôn ngữ (chủ dự án, 2026-10-26):** chỉ giữ bộ có nội dung tiếng Anh/tiếng Việt, hoặc không có ngôn ngữ tự nhiên (giao dịch, số liệu ẩn danh, click, log, chuỗi thời gian) thì giữ nguyên. Đã **loại**: Olist Brazilian E-Commerce (review tiếng Bồ Đào Nha) khỏi file này; số thứ tự mục 12 (bảng) và mục 6 (nội dung) bỏ trống để giữ nguyên đánh số cũ. Các bộ có văn bản nhưng ngôn ngữ chưa xác minh (TwiBot-22, Twitter Bots, Fake Job Posting) giữ lại, ghi 'ngôn ngữ chưa xác minh'. **Quy mô / dung lượng:** lấy từ API Kaggle (`totalBytes`), API Zenodo, API GitHub (repo gồm lịch sử git nên là cận trên), README; chỗ nào không thấy ghi 'không tìm thấy' (trang cuộc thi Kaggle IEEE-CIS và TalkingData không truy cập được khi không đăng nhập).

| Liên quan | File |
|---|---|
| Review giả/spam | [07_REVIEW_SENTIMENT_SPAM.md](07_REVIEW_SENTIMENT_SPAM.md) |
| Kiểm duyệt nội dung / an toàn agent | [12_CONTENT_MODERATION_AGENT_SAFETY.md](12_CONTENT_MODERATION_AGENT_SAFETY.md) |
| CLV, churn, định giá | [13_CUSTOMER_ANALYTICS_CLV_CHURN_PRICING.md](13_CUSTOMER_ANALYTICS_CLV_CHURN_PRICING.md) |
| AIOps, phát hiện bất thường vận hành | [14_AIOPS_ANOMALY_ALERT_EXPLAIN.md](14_AIOPS_ANOMALY_ALERT_EXPLAIN.md) |
| Gợi ý/click (nhiễu do bot) | [01_RECOMMENDATION_CLICK.md](01_RECOMMENDATION_CLICK.md) |

## 1. Mục tiêu và vị trí trong hệ thống

- **Vai trò hưởng lợi:** admin (hàng đợi rủi ro: đơn/tài khoản/shop nghi vấn, lý do giải thích được); seller (cảnh báo "đơn bất thường", chống đơn ảo từ đối thủ); buyer (gián tiếp: ít spam, ít khoá nhầm); vận hành (đánh dấu traffic bot để không làm bẩn analytics và recommendation).
- **Input:** sự kiện tracking (`events_raw`), đơn hàng, thanh toán, hoàn tiền, đăng ký tài khoản, hành vi trong phiên.
- **Output:** `risk_score` (0–1) + lý do (feature đóng góp) cho từng thực thể (đơn, thanh toán, tài khoản, shop, phiên); hành động đề xuất (cho qua / yêu cầu xác minh / giữ đơn / chặn tạm) — luôn có người duyệt cho hành động bất lợi.
- **Dữ liệu của hệ thống nuôi nó** (xem `docs/design/02_DATA_MODEL.md`):
  - ClickHouse `events_raw` (`event_type, anonymous_id, session_id, user_id, role, store_id, product_id, position, surface, ts, device_type, os, country, ua_family, ip_hash, analytics_consent, props`): phát hiện bot (tần suất, độ đều đặn, thiếu sự kiện trung gian), tự-click (user thuộc `store_id` xem/click sản phẩm của chính shop), bất thường thể tích.
  - `fact_order_status` (`order_id, status, buyer_id, store_id, payment_method, payment_status, total_minor, at`), `fact_order_items`, `fact_payment_events` (`method, amount_minor, failure_code, terminal`), `fact_refunds` (`amount_minor, reason`), `fact_inventory`.
  - Postgres: `accounts` (`role, store_id, created`), `payments` (`failed_count, three_ds_passed, card_last4`, `attempts.scenario`), `orders` (`cancel_reason`, `shipping_address JSONB`), `addresses`; `order_status_histories` (`actor_type`) cho vòng đời đơn.
  - Lưu ý PII: `ip_hash` và `card_last4` đã được băm/rút gọn; không thêm trường PII mới cho mục đích fraud mà không có cơ sở pháp lý.
  - Các kịch bản mockpay (`attempts.scenario`) cho phép **sinh nhãn gian lận mô phỏng có kiểm soát** (mục 5), không phải dữ liệu thật.

## 2. Hướng làm đề xuất (baseline → nâng cao)

1. **Rule-based trước:** velocity (số đơn/thanh toán thất bại trong N phút theo tài khoản, `ip_hash`, thiết bị), tự-click/tự mua (buyer cùng `store_id` hoặc cùng cụm thiết bị với seller), đơn giá trị lớn tài khoản mới, nhiều địa chỉ/1 thẻ.
2. **Bất thường không giám sát:** Isolation Forest / LOF / autoencoder trên feature phiên và tài khoản; thay đổi mức thể tích bằng STL/z-score trên `events_raw` (liên kết 14).
3. **Giám sát có nhãn:** gradient boosting (LightGBM/XGBoost) trên feature tabular (như IEEE-CIS, BAF), xử lý mất cân bằng bằng trọng số/focal, chia **theo thời gian**; calibration xác suất.
4. **Đồ thị:** người dùng–thiết bị–IP–shop–sản phẩm; GNN hoặc đặc trưng đồ thị (cụm tài khoản, PageRank) cho tài khoản ảo/farm (TwiBot-22, Elliptic, PromoGuardian).
5. **Bot/click fraud ở cấp phiên:** mô hình trên chuỗi sự kiện + khoảng thời gian (web robot detection, TalkingData).
6. **Giải thích & vòng phản hồi:** SHAP/reason code, hàng đợi duyệt, nhãn quyết định của admin quay lại làm dữ liệu huấn luyện (chú ý selection bias).

## 3. Metric đánh giá

| Loại | Metric | Ghi chú |
|---|---|---|
| Phân loại mất cân bằng | PR-AUC (ưu tiên), ROC-AUC, recall@precision cố định, F1/F2 | tỉ lệ dương thường rất thấp (0,17%–3,5% trên các bộ công khai) |
| Chi phí | chi phí kỳ vọng = FN×giá trị gian lận + FP×chi phí review/khách bực | đặt ngưỡng theo chi phí |
| Vận hành | tỉ lệ cảnh báo/ngày, precision@k hàng đợi, thời gian xử lý, tỉ lệ kháng nghị thành công | |
| Công bằng | tỉ lệ FPR theo nhóm (thiết bị, vùng, tuổi tài khoản); các bộ như BAF đo công bằng | |
| Bot | độ chính xác ở cấp phiên/request, tỉ lệ bot bị lọc đúng; tác động lên CTR/analytics | |
| Đồ thị | AUC/F1 mức nút, macro-F1; kiểm tra độ bền trước tấn công thích nghi | |
| Ổn định theo thời gian | PSI/KS của feature và điểm số theo tuần; hiệu năng theo cửa sổ trượt | |

## 4. Bộ dữ liệu

### Bảng tổng hợp

| # | Tên | Nhóm | Loại | Quy mô (số mẫu) | Dung lượng tải/đĩa (nguồn) | License / truy cập | ★ |
|---|---|---|---|---|---|---|---|
| 1 | [IEEE-CIS Fraud Detection (Vesta)](https://ieee-dataport.org/documents/ieee-cis-fraud-detection) | b | giao dịch TMĐT thẻ, có bảng identity | 590.540 giao dịch, 20.663 gian lận (3,5%); bản FDB: 561.013 train / 28.527 test | không tìm thấy (trang Kaggle cuộc thi không truy cập được) | điều khoản cuộc thi Kaggle (FDB: phải tham gia cuộc thi để tải) | 4 |
| 2 | [Kaggle Credit Card Fraud (ULB)](https://baselight.app/u/kaggle/dataset/mlg_ulb_creditcardfraud) | b | thẻ, PCA | 284.807 giao dịch, 492 gian lận (0,172%) | 150,8 MB (API Kaggle) | DbCL 1.0 (theo README FDB; Kaggle: Open Database) | 3 |
| 3 | [PaySim (Kaggle)](https://www.kaggle.com/datasets/ealaxi/paysim1) | b | tiền di động mô phỏng | ~6,3 triệu giao dịch (6.362.620), 11 cột, 744 bước giờ (30 ngày); thu nhỏ 1/4 log gốc | 493,5 MB (API Kaggle) | CC BY-SA 4.0 (API Kaggle) | 3 |
| 4 | [Sparkov credit card fraud (Kaggle mirror)](https://baselight.app/u/kaggle/dataset/kartik2112_fraud_detection) | b | giao dịch thẻ sinh mô phỏng | 1.852.394 giao dịch/999 khách (một bản); bản FDB: 1.296.675 train, fraud 5,7% | 501,6 MB (API Kaggle, bản kartik2112) | CC0 1.0 (README FDB; API Kaggle: CC0) | 3 |
| 5 | [Bank Account Fraud (BAF, Feedzai, NeurIPS 2022)](https://github.com/feedzai/bank-account-fraud) | c | mở tài khoản, có thời gian, bias | ~1 triệu dòng bản base, 32 cột; 6 biến thể (Base, I–V); tỉ lệ gian lận chưa xác minh | 1,36 GB cho cả 6 biến thể (API Kaggle); parquet nhỏ hơn CSV ~3 lần (README GitHub) | API Kaggle ghi CC BY-NC-SA 4.0; một bài báo ghi CC BY-NC-ND 4.0 (mâu thuẫn, đọc LICENSE của repo) | 4 |
| 6 | [Fraud Dataset Benchmark (FDB, arXiv 2208.14417)](https://github.com/amazon-science/fraud-dataset-benchmark) | c | tập hợp 9 dataset | 9 bộ (IEEE-CIS, ccfraud, fraudecom, Sparkov, Twitter bots, malicious URLs, fake job, vehicle loan, IP blocklist) | repo mã ~22 MB (API GitHub, gồm lịch sử git); dữ liệu tải riêng từng bộ (xem dòng 18) | repo MIT-0; license từng bộ xem mục 6 | 4 |
| 7 | [TalkingData AdTracking](https://github.com/stys/kaggle-talkingdata-adtracking-fraud-detection) | b | click fraud quảng cáo di động | ~184,9 triệu dòng train; ~18,8 triệu test | không tìm thấy (trang Kaggle cuộc thi không truy cập được; quy mô hàng GB) | điều khoản cuộc thi | 4 |
| 8 | [Web robot detection (AUTh, Zenodo 3477932)](https://zenodo.org/records/3477932) | c | log máy chủ + phiên có nhãn | ~4,09 triệu request, ~27.000 IP, 1 tháng | ~3,24 GB: `public_v2.json` 3,22 GB + `simple_features.csv` 15,9 MB + `semantic_features.csv` 4,3 MB (API Zenodo) | CC BY 4.0 (API Zenodo) | 4 |
| 9 | [TwiBot-22](https://twibot22.github.io) | c | bot mạng xã hội, đồ thị (nội dung tweet: ngôn ngữ chưa xác minh) | số liệu chi tiết chưa xác minh (4 loại thực thể, 14 quan hệ) | không tìm thấy | CC BY-NC-ND 4.0; chỉ cấp khi xin qua email (nghiên cứu) | 3 |
| 10 | [Elliptic (Bitcoin) / Elliptic2](https://arxiv.org/abs/1908.02591v1) | c | đồ thị giao dịch, AML | 204K nút, 234K cạnh; Elliptic2: 122K đồ thị con | không tìm thấy | MIT chỉ theo bản sao HF; chưa xác minh | 2 |
| 11 | [PromoGuardian / PPA (Meituan)](https://arxiv.org/pdf/2510.12652) | c | lạm dụng khuyến mãi | 2 tuần giao dịch Meituan (theo bài báo) | không công khai (chưa xác minh) | chưa xác minh có tải công khai | 3 |
| 13 | "Fraud ecommerce" (Kaggle, Fraud_Data.csv + IP-to-country) (chưa có link) | b | giao dịch e-commerce có `device_id`, `ip_address`, `signup_time`/`purchase_time`, `source`, `browser` | 151.112 giao dịch, 14.151 gian lận (~9,3%); bản FDB: 120.889 train, 10,6% | ~21,0 MB (API Kaggle, bản `vbinh002/fraud-ecommerce`; chưa chắc là cùng bản gốc) | Kaggle: Unknown; chưa xác minh | 3 |
| 14 | Santander Customer Transaction (chưa có link) | b | **không phải gian lận** | 200 cột số ẩn danh | không tìm thấy | – | 1 |
| 16 | [IBM AMLworld (Kaggle, HI/LI × Small/Medium/Large)](https://arxiv.org/pdf/2306.16424) | b | giao dịch tổng hợp rửa tiền, đồ thị | 6 bộ; HI-Small 515.088 nút/5.078.345 cạnh; LI-Large 2.070.980 nút/176.066.557 cạnh (~1/1.750 rửa tiền); lớn nhất 180 triệu giao dịch | ~41,6 GB cho cả 6 bộ (API Kaggle) | CDLA-Sharing-1.0 (API Kaggle) | 3 |
| 17 | Credit Card Fraud Detection Dataset 2023 (Kaggle) (chưa có link) | b | thẻ, V1–V28 ẩn danh | 568.630 giao dịch, 30 cột | 324,8 MB (API Kaggle) | Kaggle: "Other (specified in description)"; chưa xác minh | 2 |
| 18 | [Các bộ Kaggle trong FDB: Twitter Bots, Malicious URLs, Fake Job Posting, Vehicle Loan Default, IP Blocklist](https://github.com/amazon-science/fraud-dataset-benchmark) | b | bot/URL độc/tin tuyển dụng giả/vỡ nợ/IP xấu (Twitter Bots, Fake Job có văn bản: ngôn ngữ chưa xác minh, khả năng chủ yếu tiếng Anh) | bot: 29.950 train (33,1%); URL: 586.072 (34,2%); fake job: 14.304 (4,7%); loan: 186.523 (21,6%); IP: 172.000 (7%) | URL 45,7 MB; fake job 50,1 MB; loan 61,7 MB (API Kaggle); bot, IP: không tìm thấy | CC0 1.0 (bot, URL, fake job); loan, IP: không rõ | 2 |
| 15 | [Báo cáo invalid traffic (Pixalate)](https://www.pixalate.com/mobile-web-click-fraud-benchmarks-report), [EvilHunter](https://nsec.sjtu.edu.cn/publications/2021/ACMCCS_2021_ssb.pdf) | d | tham chiếu ngành | – | – (báo cáo, không phải dataset) | báo cáo của vendor | 1 |

### (a) Tiếng Việt / Việt Nam

Ghi chú: chưa thấy bộ gian lận/bot/tài khoản ảo công khai của Việt Nam; do dự án chấp nhận dữ liệu quốc tế nên dùng các bộ ở (b)/(c) (đặc trưng giao dịch/phiên/thiết bị không phụ thuộc ngôn ngữ). Bộ review spam tiếng Việt (ViSpamReviews) nằm ở [07_REVIEW_SENTIMENT_SPAM.md](07_REVIEW_SENTIMENT_SPAM.md). Dữ liệu nội bộ của sàn Việt Nam thì phải tự tạo (mục 5).

### (b) TMĐT / thanh toán công khai

**1. IEEE-CIS Fraud Detection**
- Link: [IEEE DataPort](https://ieee-dataport.org/documents/ieee-cis-fraud-detection); bài giải top 5: [Towards Data Science](https://towardsdatascience.com/ieee-cis-fraud-detection-top-5-solution-5488fc66e95f/). Nguồn: IEEE Computational Intelligence Society, dữ liệu do Vesta cung cấp (Kaggle 2019).
- 590.540 giao dịch, 20.663 gian lận (3,5%); hai bảng liên kết bằng `TransactionID` (transaction, identity; nhiều giao dịch không có identity). Dung lượng: không tìm thấy (bảng transaction + identity). Nhãn `isFraud`. Cột: thời gian là **độ lệch (timedelta) chứ không phải timestamp**, `TransactionAmt`, `ProductCD`, thuộc tính thẻ, địa chỉ, khoảng cách, miền email người mua/người nhận; identity: loại thiết bị, thông tin thiết bị, `id_12`–`id_38`. Tên cột bị che; số cột 394–434 tuỳ cách gộp. Ngôn ngữ: không áp dụng. License: điều khoản cuộc thi — chưa xác minh.
- Ánh xạ cột->hệ thống: `TransactionAmt` ↔ `payments.amount_minor`/`fact_payment_events.amount_minor`; `ProductCD` ↔ `payment_method`/danh mục; email domain ↔ không có (ta không lưu email); device ↔ `events_raw.device_type/os/ua_family`; `card*` ↔ `payments.card_last4` (hạn chế); card-not-present ↔ toàn bộ đơn online.
- ★4. Hạn chế: đặc trưng ẩn danh nên chỉ học quy trình (xây feature theo thực thể "card1+addr1", time-based CV), không mang mô hình sang dữ liệu ta; domain thẻ thật, ta mô phỏng.

**2. Credit Card Fraud (ULB MLG)**
- Link: bản sao tổng hợp [baselight](https://baselight.app/u/kaggle/dataset/mlg_ulb_creditcardfraud) (trang Kaggle gốc `mlg-ulb/creditcardfraud` chưa mở). 284.807 giao dịch/492 gian lận trong 2 ngày 9/2013 của chủ thẻ châu Âu; 29 đặc trưng (V1–V28 từ PCA + `Time`, `Amount`) + `Class`. Dung lượng 150,8 MB (API Kaggle). License: DbCL 1.0 theo README của FDB (GitHub `amazon-science/fraud-dataset-benchmark`); mirror OpenML ghi "Public"; xác nhận 492 gian lận/284.807 giao dịch (0,172%) trong 2 ngày 9/2013. Trong FDB: 227.845 train / 56.962 test. ★3. Hạn chế: PCA nên không có ngữ nghĩa; 2 ngày duy nhất nên không có kiểm thử theo thời gian dài.

**3. PaySim**
- Link: Kaggle [`ealaxi/paysim1`](https://www.kaggle.com/datasets/ealaxi/paysim1) (trang chưa mở), [repo mô tả](https://github.com/BBQtime/Synthetic-Financial-Datasets-For-Fraud-Detection). Tổng hợp từ simulator PaySim (Lopez-Rojas, Elmir, Axelsson, EMSS 2016) dựa trên mẫu log một tháng của dịch vụ tiền di động ở một nước châu Phi, thu nhỏ còn 1/4; ~6,3 triệu giao dịch, 11 feature, mỗi bước 1 giờ, 744 bước (30 ngày) (kết quả WebSearch). Lưu ý: giao dịch gian lận bị huỷ nên các cột số dư (`oldbalanceOrg`, `newbalanceOrig`, `oldbalanceDest`, `newbalanceDest`) gây rò rỉ nhãn, không dùng. Số hàng chính xác (6.362.620) và số gian lận (8.213) không thấy trong kết quả; license: chưa xác minh. Dung lượng 493,5 MB, license CC BY-SA 4.0 (API Kaggle). Dùng: ví dụ về **sinh gian lận bằng agent-based simulation** — hướng tự tạo của ta (mục 5). ★3. Hạn chế: không có đặc trưng thiết bị/phiên; đã có phê bình về tính đơn giản của mẫu gian lận (chưa xác minh trong phiên).

**4. Sparkov**
- Link: [kartik2112 fraud-detection (baselight)](https://baselight.app/u/kaggle/dataset/kartik2112_fraud_detection). Giao dịch thẻ sinh bởi Sparkov; một bản 1.852.394 giao dịch/999 khách trong 2 năm (1/1/2019–31/12/2020, ~800 merchant); bản khác ~1 triệu/18 tháng; bản trong FDB: 1.296.675 train (fraud 5,7%), license CC0 1.0 theo README FDB — con số khác nhau, chọn bản cụ thể. Dung lượng bản kartik2112: 501,6 MB (API Kaggle). Có `merchant`, `category`, `lat/long`, thời gian thật. ★3. Dùng: thử feature theo thời gian, khoảng cách địa lý, category. Hạn chế: mô phỏng, mẫu gian lận do generator; license CC0 1.0 (theo FDB).

**5. TalkingData AdTracking Fraud Detection**
- Link: [repo giải pháp](https://github.com/stys/kaggle-talkingdata-adtracking-fraud-detection), [thảo luận fast.ai](https://forums.fast.ai/t/kaggle-talkingdata-adtracking-fraud-detection-challenge/15946). ~184,9 triệu dòng train, ~18,8 triệu test; nhãn `is_attributed` (click có tải app hay không; lớp dương ~0,25%). Cột (theo cuộc thi, chưa xác minh trong phiên): `ip, app, device, os, channel, click_time`. TalkingData tuyên bố 90% trong 3 tỉ click/ngày có thể gian lận. Dung lượng: không tìm thấy. Giải thích: AUC ~0,98 (LightGBM, hạng 55). ★4. Dùng: feature đếm theo `ip/app/device/os` trong cửa sổ thời gian -> ánh xạ sang `ip_hash/anonymous_id/ua_family` trong `events_raw`. Hạn chế: ăn theo khái niệm "click -> không chuyển đổi" thay vì bot thật; license là điều khoản cuộc thi; dữ liệu cuộc thi cũ (2018).

**7. "Amazon Fraud" / "Fraud ecommerce" (Kaggle).** Kaggle "Fraud ecommerce" (Fraud_Data.csv, kèm bảng ánh xạ IP-sang-quốc gia): 151.112 bản ghi, 14.151 gian lận (~9,3%, theo một bài IJCSNS trong kết quả WebSearch; nguồn khác ghi ~9%), cột user (ID, tuổi, giới), `signup_time`, `purchase_time`, thiết bị, trình duyệt, nguồn traffic, IP, nhãn `class`. Phiên bản trong [FDB](https://github.com/amazon-science/fraud-dataset-benchmark): train 120.889, fraud 10,6%, license không ghi. Đặc trưng hay dùng: khoảng cách signup->purchase, dùng chung `device_id`/IP, quốc gia theo IP. Mỗi user một giao dịch nên khó phân tích đa tài khoản. Có bộ khác "Fraudulent E-Commerce Transactions" ~1,4 triệu dòng, ~5% gian lận (chỉ một nguồn, chưa xác minh) — đừng nhầm. Dung lượng ~21,0 MB (API Kaggle, bản `vbinh002/fraud-ecommerce`). Nguồn gốc/độ thật: chưa xác minh. ★3.

### (c) Benchmark / bài báo học thuật

**8. BAF (Bank Account Fraud).** [Turning the Tables, arXiv 2211.13358](https://arxiv.org/pdf/2211.13358), [GitHub feedzai/bank-account-fraud](https://github.com/feedzai/bank-account-fraud). Dữ liệu tổng hợp từ tập mở tài khoản ngân hàng thật bằng kỹ thuật sinh dữ liệu bảng, có động lực thời gian, mất cân bằng lớp và các biến thể chứa thiên lệch có chủ đích; mỗi dòng là một đơn mở tài khoản, nhãn `is_fraud`; tải qua Kaggle (`sgpjesus/bank-account-fraud-dataset-neurips-2022`, 1,36 GB cho cả 6 biến thể, license ghi CC BY-NC-SA 4.0 trên Kaggle; repo có datasheet và file LICENSE, chưa đọc nội dung). Theo một bài báo dùng dữ liệu: CC BY-NC-ND 4.0, chỉ cho nghiên cứu/phi thương mại; bản base ~1 triệu dòng và có 6 biến thể (Base, Variant I–V), mỗi biến thể chứa một loại thiên lệch. Dùng: ví dụ thực hành chia theo `month`, đo công bằng, drift; ánh xạ sang "đăng ký tài khoản ảo" của ta. ★4. Hạn chế: dữ liệu ngân hàng; số cột/tỉ lệ gian lận/số tháng chưa xác minh (README không nêu).

**9. Fraud Dataset Benchmark (FDB).** [arXiv 2208.14417](https://arxiv.org/pdf/2208.14417): chuẩn hoá nhiều dataset gian lận công khai (bao gồm IEEE-CIS và credit card ULB) với giao thức đánh giá thống nhất. Đã xác minh qua README GitHub (`amazon-science/fraud-dataset-benchmark`, repo MIT-0): 9 bộ — IEEE-CIS (561.013 train/28.527 test, 3,5%), ccfraud (227.845/56.962, 0,18%, DbCL 1.0), fraudecom (120.889/30.223, 10,6%), Sparkov (1.296.675/20.000, 5,7%, CC0 1.0), Twitter Bots (29.950/7.488, 33,1%, CC0), Malicious URLs (586.072/65.119, 34,2%, CC0), Fake Job Posting (14.304/3.576, 4,7%, CC0), Vehicle Loan Default (186.523/46.631, 21,6%, license không rõ), IP Blocklist (172.000/43.000, 7%, license không rõ). Dùng: tham chiếu giao thức chia/metric và tải một nơi nhiều bộ. ★4.

**10. Web robot detection (AUTh).** [Zenodo 3477932](https://zenodo.org/records/3477932), mô tả: [Intelligence Group AUTh](https://intelligence.csd.auth.gr/topics/web-robot-detection/), bài [Applied Intelligence 2020](https://intelligence.csd.auth.gr/wp-content/uploads/sites/319/2024/10/lagopoulos-apin-2020.pdf). Log máy chủ của công cụ tìm kiếm thư viện, 1 tháng 3/2018, ~4,09 triệu request/~27.000 IP (IP che một phần, URL cuối băm); có phiên gán nhãn kèm đặc trưng; tổng ~3,24 GB (API Zenodo); license CC BY 4.0; nhóm tác giả gọi là bộ đầu tiên công khai loại này. Dùng: chuyển các feature phiên (số request, tỉ lệ lỗi, thời gian giữa request, độ sâu) sang `events_raw.session_id`; bot crawler thường >1/3 traffic web. ★4. Hạn chế: log HTTP, không phải sự kiện click trình duyệt của ta; license CC BY 4.0 (API Zenodo). BotGraph (arXiv 1903.08074) dùng log thật nhưng chưa xác minh là công khai.

**11. TwiBot-22.** [NeurIPS 2022 D&B](https://proceedings.neurips.cc/paper_files/paper/2022/hash/e4fd610b1d77699a02df07ae97de992a-Abstract-Datasets_and_Benchmarks.html), [arXiv 2206.04564](https://arxiv.org/pdf/2206.04564v3), `twibot22.github.io`. Đồ thị dị thể (4 loại thực thể, 14 loại quan hệ; số nút/cạnh: không tìm thấy), lớn gấp ~5 lần bộ trước, nhãn bằng weak supervision; 35 baseline trên 9 dataset. Dùng: thử GNN cho **tài khoản ảo/farm** (đồ thị tài khoản–thiết bị–đơn). ★3. Hạn chế: miền Twitter; điều khoản dữ liệu (liên quan Twitter/X) chưa xác minh; ngôn ngữ tweet chưa xác minh; dung lượng: không tìm thấy.

**12. Elliptic / Elliptic2.** [Elliptic (arXiv 1908.02591)](https://arxiv.org/abs/1908.02591v1), [Elliptic2 (arXiv 2404.19109)](https://arxiv.org/pdf/2404.19109), GitHub `mitibmxgraph/elliptic2`. Elliptic: 204K nút giao dịch, 166 feature, 234K cạnh; ~21% hợp pháp, 2% bất hợp pháp, 77% chưa nhãn; Elliptic2: 122K đồ thị con trong nền 49 triệu cụm/196 triệu cạnh. Dung lượng: không tìm thấy. Dùng: học thuật cho nhãn đồ thị nửa giám sát (ít phù hợp). ★2.

**16. IBM AMLworld (Realistic Synthetic Financial Transactions for AML).** [arXiv 2306.16424](https://arxiv.org/pdf/2306.16424), dữ liệu trên Kaggle. Đã xác minh qua WebSearch: generator tạo 6 bộ công khai chia theo tỉ lệ bất hợp pháp (HI cao, LI thấp) và cỡ (Small/Medium/Large, lớn nhất 180 triệu giao dịch), nhãn đầy đủ, lan truyền nhãn cả với tiền trộn lẫn hợp pháp/bất hợp pháp; HI-Small 515.088 nút/5.078.345 cạnh; LI-Large 2.070.980 nút/176.066.557 cạnh, ~1/1.750 giao dịch là rửa tiền. Dung lượng ~41,6 GB cho cả 6 bộ, license CDLA-Sharing-1.0 (API Kaggle). Dùng: thử GNN/đồ thị giao dịch (cụm tài khoản ví, mô hình smurfing) cho gian lận hoàn tiền/ví nội bộ nếu có. ★3. Hạn chế: miền ngân hàng/AML, không có thiết bị/phiên.

**17. Credit Card Fraud Detection Dataset 2023 (Kaggle).** Theo một bài báo trong kết quả WebSearch: 568.630 giao dịch, 30 cột (V1–V28 ẩn danh, `Amount`, `Class`), mô tả là chủ thẻ châu Âu 2023. Dung lượng 324,8 MB (API Kaggle). License và độ thật (nhiều khả năng đã xử lý/cân bằng): **chưa xác minh**. ★2; ưu tiên bộ ULB gốc. License: MIT chỉ thấy trên bản sao HF của bên thứ ba; chưa xác minh.

**13. PromoGuardian.** [arXiv 2510.12652](https://arxiv.org/pdf/2510.12652): phát hiện lạm dụng khuyến mãi bằng GNN nhiều quan hệ trên dữ liệu Meituan; bài mô tả bộ dữ liệu ẩn danh "PPA" gồm 2 tuần giao dịch. Có tải công khai hay không: **chưa xác minh**. Một bài khác (JNTETI, UGM) mô tả phát hiện lạm dụng khuyến mãi bằng độ tương tự và chấm điểm rủi ro (coupon dùng nhiều lần bằng tài khoản giả). Không tìm thấy dataset coupon fraud công khai trên Kaggle trong phiên. ★3 (ý tưởng/feature).

**14. Synthetic tabular generators.** [arXiv 2604.13125](https://arxiv.org/pdf/2604.13125): benchmark cho thấy generator dạng bảng thường không giữ được mẫu hành vi (thời gian, velocity, đa tài khoản) — cảnh báo cho cách tạo synthetic ở mục 5. ★3 như tham khảo phương pháp.

**15. Santander Customer Transaction Prediction.** Không phải bộ gian lận (dự đoán khách có giao dịch tương lai; 200 cột số ẩn danh). Chỉ liệt kê để tránh dùng nhầm. ★1. Home Credit, Avito, Criteo, Open Bandit: **không tìm thấy trang trong phiên** nên không liệt kê.

### (d) Blog / báo cáo ngành

- **EvilHunter (ACM CCS 2021, "Understanding and Detecting Mobile Ad Fraud through the Lens of Invalid Traffic"):** phát hiện thiết bị gian lận từ log bid request, 97% precision/95% recall trên dữ liệu có nhãn thực — không rõ có công khai ([PDF](https://nsec.sjtu.edu.cn/publications/2021/ACMCCS_2021_ssb.pdf)). Dùng: ý tưởng feature (cụm thiết bị theo ứng dụng dùng). ★1.
- **Pixalate** (báo cáo vendor, [click fraud](https://www.pixalate.com/mobile-web-click-fraud-benchmarks-report)): 21% click Desktop Web toàn cầu không hợp lệ (Q2 2023), 7% mobile web (Q3 2024); số liệu của nhà cung cấp, chưa kiểm chứng độc lập. Dùng làm tham chiếu tỉ lệ nền. ★1.
- Lưu ý từ vendor: tín hiệu kỹ thuật (fingerprint, TLS, IP) thường trông hợp lệ, bot tinh vi nên cần tín hiệu hành vi và footprint chung (nhiều tài khoản/1 thiết bị) — là khẳng định thương mại, chưa kiểm chứng.

## 5. Dữ liệu tự tạo / synthetic từ chính hệ thống

Vì thanh toán là **mô phỏng**, nhãn gian lận phải do ta **tạo ra có kiểm soát**; mục tiêu là kiểm thử pipeline và logic cảnh báo, không phải "học gian lận thật".

1. **Nền sạch:** `SEED_DEMO_DATA=true` + chương trình sinh traffic (scripts/dev/gen_traffic.py nếu dùng được) cho buyer/seller bình thường: phiên có chuỗi view->click->add_to_cart->checkout hợp lý, mua theo giờ VN, phân bố giá và số lượng giống dữ liệu TMĐT công khai (nên tự ước lượng từ dữ liệu demo).
2. **Chèn kịch bản gian lận có nhãn** (ghi `fraud_scenario` ở bảng nhãn riêng, không đưa vào bảng nghiệp vụ):
   - Bot/scraper: request đều nhịp, không có `add_to_cart`, UA lạ, nhiều `anonymous_id` cùng `ip_hash`.
   - Click farm/tự-click: tài khoản thuộc cùng `store_id` hoặc thiết bị với seller click/view/đặt đơn sản phẩm của shop; tăng đột biến `click` nhưng không có chuyển đổi.
   - Tài khoản ảo: đăng ký hàng loạt, cùng địa chỉ giao (`addresses`), chỉ mua sản phẩm của 1–2 shop, huỷ đơn sau khi nhận voucher.
   - Lạm dụng khuyến mãi: nhiều tài khoản nhận cùng ưu đãi (ưu đãi miễn phí vận chuyển theo ngưỡng đơn) cùng thiết bị/địa chỉ.
   - Thanh toán bất thường (mockpay): nhiều lần thử `attempts` thất bại liên tiếp (`failed_count`, `failure_code`), `three_ds_passed=false`, đổi thẻ liên tục (`card_last4`).
   - Hoàn tiền lạm dụng: tỉ lệ `REFUND_REQUESTED` cao ở nhóm tài khoản, đơn trả trong `RETURN_WINDOW_DAYS`.
   - Bất thường tracking: sự kiện trùng `event_id`, `ts` lệch nhiều so với `ts_server`, volume tăng đột biến (liên kết 14).
3. **Tỉ lệ & độ khó:** giữ tỉ lệ dương thực tế (0,2–3%); có tầng "dễ" (rule bắt được) và "khó" (bot bắt chước người, sleep ngẫu nhiên) để đo recall theo độ khó; sinh nhiều lớp đối kháng.
4. **Chia tập theo thời gian:** train = các tuần đầu, valid = tuần kế, test = tuần cuối; kịch bản mới (ví dụ kiểu tấn công mới) chỉ xuất hiện ở test để đo khái quát hoá; tách theo thực thể (tài khoản/thiết bị) để tránh rò rỉ.
5. **Cảnh báo về synthetic:** mô hình dễ học "dấu vân tay generator" thay vì hành vi; luôn đối chiếu với IEEE-CIS/BAF/AUTh để đảm bảo phương pháp (feature, CV) có thật sự thắng trên dữ liệu ngoài. Bài benchmark 2604.13125 cho thấy generator bảng thường không giữ được mẫu hành vi.
6. **Nhãn từ vận hành thật (sau này):** quyết định của admin trên hàng đợi rủi ro, đơn bị khiếu nại, chargeback (nếu thanh toán thật) — nhớ selection bias (chỉ đơn bị gắn cờ mới có nhãn).

## 6. Lộ trình dùng dữ liệu đề xuất

1. **Giai đoạn 0:** rule velocity/tự-click trên dữ liệu demo; đo bằng kịch bản chèn (mục 5).
2. **Giai đoạn 1:** thử phương pháp trên IEEE-CIS (CV theo thời gian, feature theo thực thể) và BAF (chia theo tháng, công bằng); chọn LightGBM + calibration.
3. **Giai đoạn 2:** bot ở cấp phiên: học feature từ AUTh/TalkingData, áp lên `events_raw`; so sánh với rule; dùng kết quả làm cờ `is_bot` loại khỏi recommendation/analytics (liên kết 01).
4. **Giai đoạn 3:** đồ thị (TwiBot-22/PromoGuardian) cho tài khoản ảo/khuyến mãi khi đã có dữ liệu đa tài khoản.
5. **Giai đoạn 4:** hàng đợi admin + giải thích + vòng nhãn; giám sát drift (liên kết 14).

## 7. Rủi ro, license, thiên lệch, lưu ý pháp lý/PII

- **Domain gap:** dữ liệu công khai là thanh toán thật (thẻ/ngân hàng/tiền di động); ta là mô phỏng — chỉ mượn phương pháp, không mượn ngưỡng hay xác suất tiên nghiệm. Không đưa mô hình huấn luyện từ dữ liệu thẻ sang chấm điểm khách hàng thật khi chưa kiểm định.
- **License:** IEEE-CIS và TalkingData là dữ liệu cuộc thi (điều khoản riêng); ULB: DbCL 1.0; Sparkov, Twitter Bots, Malicious URLs, Fake Job: CC0 1.0 (theo FDB); TwiBot-22: CC BY-NC-ND 4.0; BAF: CC BY-NC-SA 4.0 theo API Kaggle (một bài báo ghi CC BY-NC-ND 4.0; phi thương mại, đọc LICENSE của repo); AUTh: CC BY 4.0 (API Zenodo); PaySim: CC BY-SA 4.0 và IBM AMLworld: CDLA-Sharing-1.0 (API Kaggle); Elliptic: chưa xác minh; Amazon Last Mile ở scope 09. Đọc điều khoản trước khi dùng thương mại.
- **Thiên lệch & công bằng:** feature địa lý/thiết bị/tuổi tài khoản có thể phân biệt nhóm người dùng (vùng, thiết bị rẻ); đo FPR theo nhóm; không dùng thuộc tính nhạy cảm.
- **Hành động tự động:** chặn/khoá tài khoản cần kháng nghị, ghi log lý do; mặc định "giữ để duyệt" thay vì "chặn" khi điểm không chắc chắn.
- **PII/pháp lý:** `ip_hash`, `anonymous_id`, `user_id` là dữ liệu cá nhân theo quy định bảo vệ dữ liệu cá nhân; chỉ xử lý khi có cơ sở/consent (`analytics_consent`), thời hạn lưu theo `EVENTS_RETENTION_MONTHS`; tránh lưu fingerprint chi tiết. Không lưu dữ liệu thẻ (dự án chỉ có `card_last4` mô phỏng).
- **Đối kháng:** kẻ gian thích nghi; detector sẽ hỏng dần — cần huấn luyện lại, đừng công khai ngưỡng.
- **Nhãn nhiễu/hồi tiếp:** nhãn từ hàng đợi bị thiên lệch; nên giữ một nhóm lấy mẫu ngẫu nhiên không bị lọc để đo thật.

## 8. Nguồn tham khảo (URL)

- IEEE-CIS: https://ieee-dataport.org/documents/ieee-cis-fraud-detection ; https://towardsdatascience.com/ieee-cis-fraud-detection-top-5-solution-5488fc66e95f/ ; https://github.com/fariha123/IEEE-CIS-Fraud-Detection
- ULB: https://baselight.app/u/kaggle/dataset/mlg_ulb_creditcardfraud ; https://arxiv.org/pdf/2309.14880
- PaySim: https://www.kaggle.com/datasets/ealaxi/paysim1 (chưa mở) ; https://github.com/BBQtime/Synthetic-Financial-Datasets-For-Fraud-Detection ; https://baselight.app/u/kaggle/dataset/ealaxi_paysim1
- Sparkov: https://baselight.app/u/kaggle/dataset/kartik2112_fraud_detection ; https://docs.nvidia.com/sdgm/fraud-detection.md
- BAF: https://arxiv.org/pdf/2211.13358 ; https://github.com/feedzai/bank-account-fraud ; https://papers.neurips.cc/paper_files/paper/2022/hash/d9696563856bd350e4e7ac5e5812f23c-Abstract-Datasets_and_Benchmarks.html
- FDB: https://arxiv.org/pdf/2208.14417
- TalkingData: https://github.com/stys/kaggle-talkingdata-adtracking-fraud-detection ; https://forums.fast.ai/t/kaggle-talkingdata-adtracking-fraud-detection-challenge/15946 ; https://pages.cs.wisc.edu/~sling/stat-project/report-1.html
- Web robots: https://zenodo.org/records/3477932 ; https://intelligence.csd.auth.gr/topics/web-robot-detection/ ; https://arxiv.org/pdf/1903.08074
- TwiBot-22: https://arxiv.org/pdf/2206.04564v3 ; https://proceedings.neurips.cc/paper_files/paper/2022/hash/e4fd610b1d77699a02df07ae97de992a-Abstract-Datasets_and_Benchmarks.html
- Elliptic: https://arxiv.org/abs/1908.02591v1 ; https://arxiv.org/pdf/2404.19109
- PromoGuardian: https://arxiv.org/pdf/2510.12652 ; JNTETI: https://journal.ugm.ac.id/v3/JNTETI/article/view/3743
- Synthetic fraud generators: https://arxiv.org/pdf/2604.13125
- EvilHunter: https://nsec.sjtu.edu.cn/publications/2021/ACMCCS_2021_ssb.pdf ; Pixalate: https://www.pixalate.com/mobile-web-click-fraud-benchmarks-report
- Không tìm thấy trong phiên: dataset gian lận công khai của Việt Nam; Home Credit; Avito/Criteo/Open Bandit; dataset coupon fraud công khai; license chính thức của nhiều bộ Kaggle.
