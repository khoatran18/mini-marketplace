# Trợ lý mua sắm / CSKH cho buyer (Shopper Assistant & Customer Support)

> Chatbot cho buyer: hỏi giá, tìm sản phẩm, xem/sửa giỏ hàng, tra cứu và huỷ đơn, hỏi đổi trả/hoàn tiền, hội thoại nhiều lượt có gọi công cụ (tool use) và tuân thủ chính sách. **Trạng thái: chưa triển khai trong hệ thống; đây là tài liệu nghiên cứu dữ liệu.** Ngày tra cứu: 2026-10-25.
>
> Ghi chú phương pháp: các repo GitHub (tau-bench, tau2-bench, tau2-bench-verified, WebShop, ABCD, MultiWOZ, Taskmaster, EcomGPT, Shopping MMLU, ShoppingBench) được đọc trực tiếp ở phiên trước; phiên bổ sung (2026-10-25) xác minh thêm bằng WebSearch/WebFetch (WebFetch không truy cập được `huggingface.co`, `arxiv.org`; số liệu HF lấy từ kết quả WebSearch, ghi rõ nguồn). Dự án **không yêu cầu dữ liệu tiếng Việt**: dữ liệu tiếng Anh/đa ngôn ngữ dùng trực tiếp được; tiếng Việt chỉ là hướng mở rộng (mục (a), mục 5). **Quy tắc ngôn ngữ (chủ dự án, 2026-10-26):** chỉ giữ bộ có nội dung tiếng Anh hoặc tiếng Việt; bộ đa ngôn ngữ chỉ dùng phần tiếng Anh (và tiếng Việt nếu có); bộ thuần ngôn ngữ khác (Trung, Nhật, Bồ Đào Nha, Pháp, Đức, Ả Rập, Indonesia, Hàn, Nga, Tây Ban Nha...) bị loại. **Quy mô / dung lượng:** cột 'Dung lượng tải/đĩa' lấy từ README GitHub, API GitHub (kích thước file hoặc repo, repo gồm cả lịch sử git nên chỉ là cận trên), API Hugging Face (tổng kích thước file của repo dataset) và API Kaggle (`totalBytes`); số nào không tìm thấy ghi 'không tìm thấy'. Không có bộ nào trong file này bị loại hoàn toàn; các bộ đa ngôn ngữ (τ²-bench, EcomGPT, Shopping MMLU, SEATauBench) chỉ dùng phần tiếng Anh/Việt.

| Liên quan | File |
|---|---|
| Chính sách đổi trả / pháp lý (RAG) | [06_POLICY_LEGAL_RAG.md](06_POLICY_LEGAL_RAG.md) |
| Tìm kiếm sản phẩm (tool của trợ lý) | [02_PRODUCT_SEARCH_SEMANTIC.md](02_PRODUCT_SEARCH_SEMANTIC.md) |
| Gợi ý sản phẩm | [01_RECOMMENDATION_CLICK.md](01_RECOMMENDATION_CLICK.md) |
| Hiểu truy vấn tiếng Việt | [15_VIETNAMESE_NLP_QUERY_UNDERSTANDING.md](15_VIETNAMESE_NLP_QUERY_UNDERSTANDING.md) |
| Agent tool-use & đánh giá | [16_AGENT_TOOL_USE_EVAL.md](16_AGENT_TOOL_USE_EVAL.md) |
| An toàn / kiểm duyệt agent | [12_CONTENT_MODERATION_AGENT_SAFETY.md](12_CONTENT_MODERATION_AGENT_SAFETY.md) |
| Gian lận / rủi ro | [08_FRAUD_ANOMALY_RISK.md](08_FRAUD_ANOMALY_RISK.md) |
| Trợ lý số liệu cho seller/admin | [04_ANALYTICS_QA_TEXT2SQL.md](04_ANALYTICS_QA_TEXT2SQL.md) |

## 1. Mục tiêu và vị trí trong hệ thống

- **Vai trò:** buyer (chính); nhân viên CSKH/admin (xem tóm tắt hội thoại, gợi ý trả lời); seller (tuỳ chọn: trợ lý trả lời tin nhắn khách).
- **Input:** tin nhắn tiếng Việt của buyer (có dấu/không dấu/teencode), danh tính **chỉ từ JWT** (không tin ID trong nội dung), lịch sử hội thoại.
- **Output:** câu trả lời + hành động qua tool (gọi gateway REST/gRPC với quyền của chính buyer): tìm sản phẩm, xem giá/tồn, thêm/bớt giỏ (`cart_items`), xem đơn (`orders`, `order_status_histories`), huỷ đơn (chỉ khi trạng thái cho phép), yêu cầu trả hàng (`REFUND_REQUESTED` trong `RETURN_WINDOW_DAYS`, một lần), xem địa chỉ (`addresses`), trạng thái thanh toán (`payments`).
- **Dữ liệu hệ thống nuôi nó:**
  - Postgres: `products` (price, inventory, status, description, attributes), `cart_items`, `orders` / `order_items` (snapshot), `order_status_histories`, `payments`/`refunds`, `addresses`, `categories`.
  - Máy trạng thái đơn (PENDING → AWAITING_PAYMENT/CONFIRMED → PAID → SHIPPED → DELIVERED → REFUND_REQUESTED → REFUNDED...) là **luật kinh doanh cứng** cho agent: câu hỏi "huỷ đơn được không?" phải trả lời theo `transitions`, không theo LLM đoán.
  - Tài liệu chính sách (điều khoản đổi trả, vận chuyển) cho RAG (xem file 06).
  - ClickHouse `events_raw`: log tương tác với widget chat (sau khi có consent) để đo CSAT, tỉ lệ giải quyết.
- **Ràng buộc từ CLAUDE.md:** không tin ID/giá/tổng/trạng thái từ body request; mọi hành động đi qua gateway với JWT của buyer; thanh toán chỉ mô phỏng, không lưu dữ liệu thẻ -> agent không bao giờ xin hay nhận số thẻ/CVV.

## 2. Hướng làm đề xuất (baseline → nâng cao)

1. **FAQ/RAG + luồng cố định (baseline):** nhận diện ý định (tra đơn, huỷ, đổi trả, giá) → gọi API tương ứng → mẫu câu trả lời; chính sách trả lời bằng RAG có trích dẫn.
2. **LLM tool-calling có chính sách (agent):** tool read-only trước (search, get_order, get_cart), tool ghi (add_to_cart, cancel_order, request_refund) cần **xác nhận rõ ràng của buyer** và idempotency key (hệ thống đã có `idempotency_key` cho `POST /orders`).
3. **State machine guard:** kiểm tra điều kiện ở server; agent chỉ đề xuất, server quyết định (tránh "jailbreak" bằng hội thoại).
4. **Quản lý hội thoại:** tóm tắt dài, làm rõ khi thiếu thông tin (slot filling: mã đơn, lý do trả), chuyển người thật khi cảm xúc tiêu cực/số tiền lớn.
5. **Fine-tune/đánh giá bằng mô phỏng người dùng:** user simulator (LLM) theo kịch bản như tau-bench; đo pass^k.
6. **Đa phương thức (tuỳ chọn):** ảnh sản phẩm hỏng khi đổi trả (xem file 11).

## 3. Metric đánh giá

- **Task success / pass^k** (tau-bench: tỉ lệ thành công *cả k lần chạy*; đo tính nhất quán), kiểm tra **trạng thái DB cuối** đúng như vàng.
- Tool-use: độ chính xác chọn tool và tham số; số lượt gọi thừa; tỉ lệ vi phạm chính sách (hành động bị cấm).
- Hội thoại: Action State Tracking / Cascading Dialogue Success (ABCD), Joint Goal Accuracy, Inform/Success (MultiWOZ).
- Chất lượng ngôn ngữ: LLM-judge + người, tính đúng thông tin (giá, tồn kho, thời gian giao), tỉ lệ ảo giác; CSAT; handoff rate; chi phí và độ trễ mỗi lượt.
- An toàn: tỉ lệ làm theo chỉ dẫn độc hại (prompt injection trong tên sản phẩm, đánh giá, ghi chú đơn), rò dữ liệu người khác.

## 4. Bộ dữ liệu

### Bảng tổng hợp

| # | Bộ dữ liệu | Nhóm | Quy mô (số mẫu) | Dung lượng tải/đĩa (nguồn) | Ngôn ngữ | License | ★ |
|---|---|---|---|---|---|---|---|
| 1 | [τ-bench / τ²-bench (retail, airline, telecom)](https://github.com/sierra-research/tau2-bench) | (c) | τ²: 50 task/domain (text-v1, phần tiếng Anh); voice-v1 EN: 50 airline, 114 retail, 114 telecom | dung lượng dữ liệu: không tìm thấy (repo git ~452 MB gồm lịch sử, API GitHub) | Anh (bản gốc); **chỉ dùng phần tiếng Anh**, bỏ các bản dịch ngôn ngữ khác | MIT | 5 |
| 2 | [τ²-bench-Verified (Amazon AGI)](https://github.com/amazon-agi/tau2-bench-verified) | (c) | bản sửa dữ liệu của τ²; số task retail chưa xác minh | không tìm thấy | Anh | MIT | 4 |
| 3 | [ABCD (ASAPP)](https://github.com/asappresearch/abcd) | (c) | >10K hội thoại người–người, 55 ý định | `abcd_v1.1.json.gz` 37,0 MB (API GitHub); `utterances.json` 5,2 MB | Anh | MIT | 5 |
| 4 | [WebShop](https://github.com/princeton-nlp/WebShop) | (c) | 1,18 triệu sản phẩm, 12.087 chỉ dẫn (`-d small`: 1.000 sản phẩm) | dung lượng: không tìm thấy (README không nêu) | Anh | MIT | 4 |
| 5 | [ShoppingBench (AAAI 2026)](https://github.com/yjwjy/ShoppingBench) | (c) | 3.310 chỉ dẫn; sandbox >2,5 triệu sản phẩm; 4 bộ test | sandbox ~1,4 GB nén / 4,8 GB giải nén (README GitHub); 4 file test 0,1–0,35 MB (API GitHub) | Anh (chưa xác minh rõ) | Apache-2.0 | 4 |
| 6 | [Shopping MMLU](https://github.com/KL4805/ShoppingMMLU) | (c) | 57 task, 20.799 câu hỏi (arXiv v1) | `data.zip` 8,2 MB (API GitHub) | chủ yếu Anh; một nhóm kỹ năng đa ngôn ngữ (danh sách ngôn ngữ chưa xác minh) -> **chỉ dùng phần tiếng Anh** | Apache-2.0 | 3 |
| 7 | [EcomGPT / EcomInstruct](https://github.com/Alibaba-NLP/EcomGPT) | (c) | 2,5 triệu instruction (Anh + Trung, **chỉ dùng phần tiếng Anh**; số mẫu riêng phần tiếng Anh: không tìm thấy); 12 tập đánh giá × 500 | dung lượng: không tìm thấy (repo mã ~5 MB, API GitHub) | Anh (bỏ phần tiếng Trung) | chưa nêu | 3 |
| 8 | [MultiWOZ](https://github.com/budzianowski/multiwoz) | (c) | ~10,4K hội thoại (3.406 đơn miền + 7.032 đa miền) | dung lượng dữ liệu: không tìm thấy (repo ~123 MB gồm lịch sử, API GitHub) | Anh | code MIT; dữ liệu chưa nêu | 2 |
| 9 | [Taskmaster (TM-1..TM-4)](https://github.com/google-research-datasets/Taskmaster) | (c)/(d) | >55.000 hội thoại | TM-1: `self-dialogs.json` 65,7 MB + `woz-dialogs.json` 37,2 MB (API GitHub); TM-2..4: không tìm thấy | Anh | chưa nêu | 2 |
| 10 | [CSConDa (trong ViRE)](https://github.com/longstnguyen/ViRE) | (a) | tối đa 1.000 cặp question–context (mẫu ViRE) | riêng CSConDa: không tìm thấy (toàn repo ViRE ~58 MB, API GitHub) | Việt | CC BY-NC 4.0 | 3 |
| 11 | [Bitext Customer Support LLM Chatbot Training Dataset](https://huggingface.co/datasets/bitext/Bitext-customer-support-llm-chatbot-training-dataset) | (b) | 26.872 cặp hỏi–đáp, 27 intent, 10 nhóm (~1.000/intent) | CSV 19,2 MB (HF API) | Anh | CDLA-Sharing-1.0 | 4 |
| 12 | [MultiDoGO (Amazon)](https://huggingface.co/datasets/jpcorb20/multidogo) | (c) | >81K hội thoại, 6 miền (>54K có nhãn intent/slot); trang Amazon ghi >65K, 7 miền | ~82 MB tổng file parquet (HF API) | Anh | CDLA-Permissive-1.0 (theo bản HF mirror) | 3 |
| 13 | Customer Support on Twitter (Kaggle) (chưa có link) | (b) | ~2,81 triệu tweet (1,54 triệu của khách, 1,27 triệu của kênh hỗ trợ), thu thập 2017 | 516,5 MB (API Kaggle) | Anh | CC BY-NC-SA 4.0 (API Kaggle) | 3 |
| 14 | [Amazon-C4 (McAuley Lab, BLaIR)](https://huggingface.co/datasets/McAuley-Lab/Amazon-C4) | (b) | test 21.223 query; item pool ~1,06 triệu sản phẩm | ~656 MB: `sampled_item_metadata_1M.jsonl` 643 MB + `test.csv` 12,5 MB (HF API) | Anh | chưa xác minh | 3 |
| 15 | [ECInstruct / eCeLLM (Ohio State)](https://github.com/ninglab/eCeLLM) | (c) | 116.528 mẫu, 10 tác vụ TMĐT, 4 nhóm | `dataset.json` 529 MB (HF API `ninglab/ECInstruct`) | Anh | CC BY 4.0 (thẻ HF API) | 3 |
| 16 | [SEATauBench (SEACrowd)](https://github.com/SEACrowd/SEATauBench) | (c) | bản địa hoá τ²-bench; số task chưa xác minh; **chỉ dùng phần tiếng Việt** (bỏ Indonesia, Thái, Filipino, Trung) | không tìm thấy (repo ~316 MB gồm lịch sử, API GitHub) | Việt (bỏ các ngôn ngữ khác) | chưa xác minh | 3 |
| 17 | [Amazon Reviews 2023 (cho FAQ/review-QA)](https://amazon-reviews-2023.github.io/) | (b) | >570 triệu review, 48 triệu sản phẩm, 33 danh mục | ~750 GB tổng file thô review + metadata trong repo HF `McAuley-Lab/Amazon-Reviews-2023` (HF API; tải theo từng danh mục, ví dụ Home_and_Kitchen ~31 GB) | Anh | xem file 07 | 3 |

### (a) Tiếng Việt / Việt Nam

**CSConDa — Customer Support Conversation Dataset (trong benchmark ViRE)** — https://github.com/longstnguyen/ViRE (đã đọc ở phiên này): miền "customer support — conversational service requests" cho retrieval tiếng Việt; ViRE lấy tối đa 1.000 cặp question–context; mô tả riêng của CSConDa (kích thước gốc, nguồn công ty, nơi tải): **chưa xác minh**; license CC BY-NC 4.0. Dùng để thử retrieval câu trả lời CSKH tiếng Việt, không phải hội thoại nhiều lượt có tool. Ánh xạ: kho FAQ/KB của RAG. ★3. Hạn chế: phi thương mại, nhỏ.

> Ghi chú: ngoài CSConDa, chưa có bộ hội thoại CSKH/TMĐT tiếng Việt khác được xác minh; do dự án chấp nhận dữ liệu tiếng Anh/quốc tế nên dùng các bộ ở (b)/(c) trực tiếp. Hai hướng có tiếng Việt nếu cần sau này: **SEATauBench** (xem (c): τ²-bench được địa hoá sang tiếng Việt và vài ngôn ngữ khác; chỉ dùng phần tiếng Việt) và tự dựng/dịch kịch bản (mục 5). τ²-bench text-v1 có bản dịch sang nhiều ngôn ngữ khác tiếng Anh (không có tiếng Việt); theo quy tắc ngôn ngữ chỉ dùng bản tiếng Anh.

### (b) TMĐT công khai

**WebShop (Princeton NLP)** — https://github.com/princeton-nlp/WebShop (đã đọc). 1,18 triệu sản phẩm thật (Amazon), 12.087 chỉ dẫn do crowdsource; agent phải tìm–tuỳ chọn–mua. MIT. Metric (reward) không được README định nghĩa rõ. Ánh xạ: kịch bản "tìm và đặt hàng theo mô tả" → tool search + cart + checkout. ★4. Dung lượng: không tìm thấy (README không nêu; `setup.sh -d small` chỉ 1.000 sản phẩm). Hạn chế: môi trường web tiếng Anh, không có hậu mãi (huỷ/đổi trả).

**ShoppingBench** — https://github.com/yjwjy/ShoppingBench (đã đọc). Sandbox mua sắm tương tác trên >2,5 triệu sản phẩm thật (~1,4 GB nén, 4,8 GB giải nén), 4 tập test theo ý định: Product Intent, Shop Intent, Voucher Intent, Web-search Intent (SimpleQA). Apache-2.0. Số chỉ dẫn: 3.310 (đã xác minh, bài arXiv 2508.04266, Alibaba International Digital Commercial Group, AAAI 2026; GPT-4.1 đạt dưới 50% thành công tuyệt đối). Voucher/Shop intent gần với khuyến mãi/cửa hàng của sàn đa seller. ★4.

**Shopping MMLU** — https://github.com/KL4805/ShoppingMMLU (đã đọc). Benchmark đa nhiệm cho LLM về mua sắm online (NeurIPS 2024 D&B; dùng cho KDD Cup 2024): 4 kỹ năng (shopping concept understanding, shopping knowledge reasoning, user behavior alignment, multilingual), 5 loại tác vụ (multiple choice, retrieval, ranking, NER, generation). Apache-2.0, `data.zip` (CSV/JSON). Quy mô: 57 task, 20.799 câu hỏi dựng từ dữ liệu Amazon; `data.zip` 8,2 MB (API GitHub) (số theo arXiv v1 2410.20745, đã xác minh qua WebSearch; chưa kiểm v2); KDD Cup 2024 có >500 đội. Danh sách ngôn ngữ cụ thể: **chưa xác minh**; chỉ dùng phần tiếng Anh. Dùng để chọn LLM nền cho miền TMĐT. ★3.

**EcomGPT / EcomInstruct** — https://github.com/Alibaba-NLP/EcomGPT (đã đọc). 2,5 triệu instruction (Chain-of-Task); 12 tập đánh giá mở (500 mẫu/tập): NER, span detection, extractive QA, review topic classification, attribute value recognition/detection, product selection/alignment, title attribute matching, product classification, title generation (Anh + Trung; **chỉ dùng phần tiếng Anh**, số mẫu riêng phần tiếng Anh: không tìm thấy; dung lượng: không tìm thấy). Trọng số EcomGPT (7b1) trên ModelScope. README **không nêu license** và không xác nhận toàn bộ tập huấn luyện được công bố. ★3. Lưu ý: thiên về hiểu sản phẩm (xem file 10), ít hội thoại.

**Bitext Customer Support LLM Chatbot Training Dataset** — HF `bitext/Bitext-customer-support-llm-chatbot-training-dataset`, mirror GitHub `bitext/customer-support-llm-chatbot-training-dataset` (đã xác minh qua kết quả WebSearch, thẻ dataset). 26.872 cặp hỏi–đáp, 27 intent chia 10 nhóm (~1.000 mẫu/intent), CSV; license **CDLA-Sharing-1.0** (thẻ HF); bản cũ chỉ 4.514 câu nên cần ghi rõ phiên bản. Dung lượng: file CSV 19,2 MB (API HF). Intent kiểu CSKH TMĐT (huỷ đơn, theo dõi đơn, hoàn tiền, đổi địa chỉ…). Dùng: fine-tune/phân loại intent, mẫu câu trả lời theo template. Ánh xạ: intent ↔ tool/luồng cố định (mục 2.1). ★4. Hạn chế: dữ liệu tổng hợp theo mẫu nên văn phong đơn điệu, một lượt (không phải hội thoại nhiều lượt có tool); license share-alike.

**Customer Support on Twitter (Kaggle, thoughtvector)** — Kaggle (đã xác minh qua một phân tích Univ. Waterloo trong kết quả WebSearch): ~2.811.774 tweet (1.537.843 của khách, 1.273.931 của kênh hỗ trợ của nhiều thương hiệu), thu thập năm 2017; dung lượng 516,5 MB, license CC BY-NC-SA 4.0 (API Kaggle). Dùng: học giọng điệu CSKH thật, tóm tắt/chuyển người thật theo cảm xúc. ★3. Hạn chế: không chỉ TMĐT, cần ẩn danh/làm sạch, điều khoản Twitter.

**Amazon-C4 (McAuley Lab)** — HF `McAuley-Lab/Amazon-C4` (đã xác minh qua kết quả WebSearch, README): bộ tìm kiếm sản phẩm với ngữ cảnh phức tạp do ChatGPT tạo, dựng trên Amazon Reviews 2023 (liên quan BLaIR); test 21.223 dòng (`qid, query, item_id, user_id, ori_rating, ori_review`); item pool ~1,06 triệu sản phẩm (`sampled_item_metadata_1M.jsonl`); dung lượng ~656 MB (metadata 643 MB + test 12,5 MB, API HF). License: **chưa xác minh**. Dùng: kiểm tra tool `search_products` với yêu cầu mua sắm dài. ★3. Hạn chế: bài MR.Rec nhận xét câu truy vấn tổng hợp chứa chi tiết (thương hiệu, model) mà người dùng thật ít gõ.

**Amazon Reviews 2023** — xem [07](07_REVIEW_SENTIMENT_SPAM.md) (>570 triệu review, 48 triệu sản phẩm, 33 danh mục); dùng review/metadata làm nguồn giả lập kho tri thức sản phẩm cho trợ lý. ★3.

### (c) Benchmark / paper học thuật

**τ-bench / τ²-bench (Sierra)** — https://github.com/sierra-research/tau-bench , https://github.com/sierra-research/tau2-bench (đã đọc). Miền retail & airline (τ²: thêm telecom, banking_knowledge, intake/τ-Entity); agent có API tool theo miền và **chính sách phải tuân thủ**, đối thoại với user giả lập; MIT. τ²: text-v1 gồm 50 task cố định mỗi miền (nhiều ngôn ngữ; chỉ dùng bản tiếng Anh); voice-v1 EN: 50 airline, 114 retail, 114 telecom; intake >200 task. Metric trên leaderboard là Pass^1..Pass^4 (README tau-bench). Một số task retail English của τ² là 50 task đã được xem xét thay thế so với v1.0.1 nên điểm không so sánh trực tiếp với 1.x. **Rất sát bài toán** (đổi/trả/huỷ đơn có chính sách). Ánh xạ: DB giả `orders/items` ↔ `orders/order_items`, chính sách ↔ máy trạng thái đơn. ★5. Hạn chế: gốc tiếng Anh (các bản dịch khác bị bỏ theo quy tắc ngôn ngữ; tiếng Việt qua SEATauBench), policy kiểu Mỹ. Dung lượng dữ liệu: không tìm thấy (repo git ~452 MB gồm lịch sử).

**SEATauBench (SEACrowd)** — arXiv 2606.28715, mã/dữ liệu `github.com/SEACrowd/SEATauBench` (đã xác minh qua WebSearch, chưa mở repo). Địa hoá τ²-bench sang 5 ngôn ngữ (Việt, Indonesia, Thái, Filipino, Trung); **chỉ dùng phần tiếng Việt**, bỏ 4 ngôn ngữ còn lại; 4 kịch bản từ giữ nguyên bối cảnh tiếng Anh đến địa hoá toàn bộ môi trường tác vụ; kết luận: chỉ đổi ngôn ngữ hội thoại thì giữ khá tốt, địa hoá thêm bối cảnh thì suy giảm mạnh. Số task/license: **chưa xác minh**; dung lượng: không tìm thấy (repo ~316 MB gồm lịch sử, API GitHub). Dùng: nếu sau này cần tiếng Việt. ★3.

**MultiDoGO (Amazon)** — Amazon Science / HF `jpcorb20/multidogo` (đã xác minh qua WebSearch): hội thoại hướng mục tiêu, Wizard-of-Oz (crowd đóng khách, nhân viên được huấn luyện đóng agent); >81K hội thoại, 6 miền theo bài EMNLP 2019 (>54K có nhãn intent/slot), trang Amazon ghi >65K/7 miền (khác phiên bản); license CDLA-Permissive-1.0 (theo thẻ HF mirror). Có nhãn intent riêng cho khách và agent. Có cả miền không TMĐT; có dữ liệu đơn hàng/hoàn tiền hay không: chưa xác minh. ★3.

**ECInstruct / eCeLLM** — arXiv 2402.08831, ICML 2024, GitHub `ninglab/eCeLLM` (đã xác minh qua WebSearch): 116.528 mẫu cho 10 tác vụ TMĐT thực tế, 4 nhóm; mỗi mẫu gồm instruction/input/output; license dataset CC BY 4.0 (thẻ HF `ninglab/ECInstruct` qua API); `dataset.json` 529 MB. Dùng thay/bổ sung EcomGPT (cùng hướng, được công bố rõ hơn). ★3.

**τ²-bench-Verified (Amazon AGI)** — https://github.com/amazon-agi/tau2-bench-verified (đã đọc). Bản sửa lỗi dữ liệu của τ²-bench (chính sách, DB, logic, mơ hồ), MIT; framework không đổi. Dùng thay cho bản gốc để giảm nhiễu nhãn. ★4.

**ABCD — Action-Based Conversations Dataset** — https://github.com/asappresearch/abcd (đã đọc). >10K hội thoại người–người (Expert Live Chat), 55 ý định, 30 hành động (click), tác vụ Action State Tracking và Cascading Dialogue Success; kịch bản mua sắm trực tuyến (ví dụ yêu cầu hoàn tiền kiểm tra theo chính sách công ty). MIT. Dung lượng: `abcd_v1.1.json.gz` 37,0 MB (API GitHub). Ánh xạ: ý định/hành động → tool; chính sách → kiểm tra server. ★5. Hạn chế: tiếng Anh, kịch bản công ty giả.

**MultiWOZ** — https://github.com/budzianowski/multiwoz (đã đọc). ~10,4K hội thoại (3.406 một miền, 7.032 đa miền), miền du lịch (hotel, restaurant, taxi, train, attraction...). Code MIT; license dữ liệu README không nêu riêng. Dùng để thử state tracking nói chung, không TMĐT. ★2.

**Taskmaster (Google Research)** — https://github.com/google-research-datasets/Taskmaster (đã đọc). >55.000 hội thoại nói/viết, hơn chục miền, các thư mục TM-1-2019, TM-2-2020, TM-3-2020, TM-4-2024; README không liệt kê miền hay license. ★2.

### (d) Blog / cuộc thi / công ty

- **KDD Cup 2024 (ShopBench/Shopping MMLU)** — đã nêu ở (c); >500 đội tham gia (theo kết quả WebSearch). Blog công ty (Shopee/Tiki/Lazada/Zalo/VinAI) về CSKH: chưa thu thập.

## 5. Dữ liệu tự tạo / synthetic từ chính hệ thống

- **Kịch bản chính sách làm "luật":** từ máy trạng thái đơn (docs) rút ra bảng quyết định (huỷ được khi `AWAITING_PAYMENT`/`CONFIRMED`/`PAID` chưa SHIPPED; trả hàng khi `DELIVERED` trong `RETURN_WINDOW_DAYS`, một lần; v.v.).
- **Môi trường mô phỏng kiểu τ-bench:** một DB demo cố định (`SEED_DEMO_DATA=true`: buyer, store, sản phẩm, đơn ở nhiều trạng thái) + user simulator (LLM) + tập tool = API gateway. Mỗi task: `scenario_vi`, `persona` (kiểu nói: lịch sự/nóng giận/viết tắt/không dấu), `user_id` (JWT), `expected_final_state` (so khớp DB), `forbidden_actions`, `expected_tool_calls`.
- **Nhóm task nên có:** hỏi giá/tồn; thêm giỏ nhiều sản phẩm; kiểm tra đơn theo mã/theo "đơn gần nhất"; huỷ đơn hợp lệ/không hợp lệ (đã SHIPPED); trả hàng hết hạn; hoàn tiền một phần; đổi địa chỉ (chỉ khi cho phép); hỏi giá có phí ship; yêu cầu xem đơn của người khác (phải từ chối); prompt injection ("bỏ qua chính sách…") nhúng trong tên sản phẩm/ghi chú.
- **Thu thập hội thoại thật:** sau khi đồng ý, ẩn danh PII (SĐT, địa chỉ, tên) trước khi lưu/gán nhãn; chia train/dev/test **theo thời gian và theo kịch bản** (không chia ngẫu nhiên từng lượt).
- **Dịch/việt hoá:** dịch τ-bench retail, ABCD sang tiếng Việt bằng LLM rồi để người Việt duyệt (chú ý đơn vị tiền VND, địa danh, COD, shipper); nhớ kiểm tra license khi phân phối lại.

## 6. Lộ trình dùng dữ liệu đề xuất

1. **Bước 1:** τ-bench/τ²-bench (+ Verified) và ABCD để chọn LLM, mẫu prompt và kiến trúc tool+policy; báo cáo pass^k cơ sở.
2. **Bước 2:** WebShop/ShoppingBench để thử luồng tìm–chọn–mua và voucher; Shopping MMLU/EcomGPT eval để so các LLM trên kiến thức TMĐT.
3. **Bước 3:** Bitext (intent CSKH) + MultiDoGO/Twitter CS cho NLU/giọng điệu; (tuỳ chọn) CSConDa/ViRE và SEATauBench nếu cần tiếng Việt; tự dựng kịch bản theo mục 5.
4. **Bước 4:** pilot nội bộ với buyer thử nghiệm; log đã ẩn danh -> mở rộng bộ vàng; kiểm tra red-team (injection, rò dữ liệu).
5. **Bước 5:** triển khai từng bước (chỉ read-only → ghi có xác nhận), theo dõi CSAT và handoff.

## 7. Rủi ro, license, thiên lệch, lưu ý pháp lý/PII

- **Hành động có hậu quả:** agent không được tự huỷ/hoàn tiền; luôn xác nhận và để server kiểm tra quyền sở hữu (ownership check ở gateway). Giữ idempotency.
- **Prompt injection gián tiếp:** nội dung do seller/buyer kiểm soát (tên/mô tả sản phẩm, đánh giá, ghi chú đơn) là *dữ liệu không tin cậy*, phải cách ly khỏi chỉ dẫn hệ thống.
- **PII và thanh toán:** không xử lý số thẻ/CVV (thanh toán mô phỏng, `card_last4` chỉ 4 số); không đưa SĐT/địa chỉ vào log gửi bên ngoài; tuân thủ Nghị định 13/2023/NĐ-CP (xác nhận với chuyên gia pháp lý); hệ thống chưa có `DELETE /users/me/data` (roadmap).
- **License:** τ-bench, τ²-bench, WebShop, ABCD, τ²-Verified: MIT; ShoppingBench, Shopping MMLU: Apache-2.0; Bitext: CDLA-Sharing-1.0 (share-alike); MultiDoGO: CDLA-Permissive-1.0 (theo mirror HF); EcomGPT, MultiWOZ (dữ liệu), Taskmaster, Amazon-C4: chưa xác minh; Twitter CS: CC BY-NC-SA 4.0 (API Kaggle); ECInstruct: CC BY 4.0 (thẻ HF); CSConDa: CC BY-NC 4.0 (phi thương mại).
- **Thiên lệch:** tiếng Anh/chính sách phương Tây; user simulator LLM không phản ánh hành vi người Việt thật (xưng hô "anh/chị/em", viết tắt).
- **Số liệu benchmark:** điểm cao trên τ-bench không đảm bảo an toàn thực tế; τ²-bench có chấm bằng LLM judge cho một số tiêu chí nên có nhiễu.

## 8. Nguồn tham khảo

Đã đọc trực tiếp (GitHub):
- https://github.com/sierra-research/tau-bench
- https://github.com/sierra-research/tau2-bench
- https://github.com/amazon-agi/tau2-bench-verified
- https://github.com/princeton-nlp/WebShop
- https://github.com/yjwjy/ShoppingBench
- https://github.com/KL4805/ShoppingMMLU
- https://github.com/Alibaba-NLP/EcomGPT
- https://github.com/asappresearch/abcd
- https://github.com/budzianowski/multiwoz
- https://github.com/google-research-datasets/Taskmaster
- https://github.com/longstnguyen/ViRE (CSConDa)

Đã xác minh qua WebSearch/WebFetch ở phiên bổ sung (2026-10-25):
- Bitext: https://github.com/bitext/customer-support-llm-chatbot-training-dataset ; https://huggingface.co/datasets/bitext/Bitext-customer-support-llm-chatbot-training-dataset
- ShoppingBench: https://arxiv.org/pdf/2508.04266 ; Shopping MMLU: https://arxiv.org/html/2410.20745v2
- Amazon-C4: https://huggingface.co/datasets/McAuley-Lab/Amazon-C4/blob/main/README.md
- τ²-bench ngôn ngữ (WebFetch GitHub): https://github.com/sierra-research/tau2-bench ; SEATauBench: https://arxiv.org/pdf/2606.28715
- MultiDoGO: https://amazon.science/code-and-datasets/multi-domain-goal-oriented-dialogues-multidogo-strategies-toward-curating-and-annotating-large-scale-dialogue-data ; https://huggingface.co/datasets/jpcorb20/multidogo
- eCeLLM: https://arxiv.org/pdf/2402.08831 ; https://github.com/ninglab/eCeLLM

Còn chưa xác minh: license Amazon-C4, MultiWOZ/Taskmaster/EcomGPT; dung lượng dữ liệu của τ²-bench, τ²-Verified, WebShop, EcomGPT, SEATauBench; số task SEATauBench; danh sách ngôn ngữ Shopping MMLU; số task retail của τ²-Verified; blog CSKH của các sàn Việt.
