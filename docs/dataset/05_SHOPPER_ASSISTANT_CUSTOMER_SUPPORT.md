# Trợ lý mua sắm / CSKH cho buyer (Shopper Assistant & Customer Support)

> Chatbot cho buyer: hỏi giá, tìm sản phẩm, xem/sửa giỏ hàng, tra cứu và huỷ đơn, hỏi đổi trả/hoàn tiền, hội thoại nhiều lượt có gọi công cụ (tool use) và tuân thủ chính sách. **Trạng thái: chưa triển khai trong hệ thống; đây là tài liệu nghiên cứu dữ liệu.** Ngày tra cứu: 2026-10-25.
>
> Ghi chú phương pháp: hết hạn mức WebSearch giữa phiên, Kaggle/HuggingFace không mở được; các repo GitHub (tau-bench, tau2-bench, tau2-bench-verified, WebShop, ABCD, MultiWOZ, Taskmaster, EcomGPT, Shopping MMLU, ShoppingBench) được đọc trực tiếp. Bitext, Amazon-C4 **không** xác minh được trang nên không đưa vào bảng. Không tìm thấy bộ hội thoại CSKH tiếng Việt công khai đã xác minh ngoài CSConDa trong ViRE (xem file 02).

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

| # | Bộ dữ liệu | Nhóm | Quy mô (theo README đã đọc) | Ngôn ngữ | License | ★ |
|---|---|---|---|---|---|---|
| 1 | τ-bench / τ²-bench (retail, airline, telecom) | (c) | τ²: 50 task/domain × 6 ngôn ngữ (text-v1); voice-v1 EN: 114 retail | Anh + 5 ngôn ngữ khác (danh sách chưa xác minh) | MIT | 5 |
| 2 | τ²-bench-Verified (Amazon AGI) | (c) | bản sửa dữ liệu của τ²; số task retail chưa xác minh | Anh | MIT | 4 |
| 3 | ABCD (ASAPP) | (c) | >10K hội thoại người–người, 55 ý định | Anh | MIT | 5 |
| 4 | WebShop | (c) | 1,18 triệu sản phẩm, 12.087 chỉ dẫn | Anh | MIT | 4 |
| 5 | ShoppingBench | (c) | sandbox >2,5 triệu sản phẩm; 4 bộ test | Anh (chưa xác minh) | Apache-2.0 | 4 |
| 6 | Shopping MMLU | (c) | số lượng chưa xác minh | đa ngôn ngữ (danh sách chưa xác minh) | Apache-2.0 | 3 |
| 7 | EcomGPT / EcomInstruct | (c) | 2,5 triệu instruction; 12 tập đánh giá × 500 | Anh, Trung | chưa nêu | 3 |
| 8 | MultiWOZ | (c) | ~10,4K hội thoại (3.406 đơn miền + 7.032 đa miền) | Anh | code MIT; dữ liệu chưa nêu | 2 |
| 9 | Taskmaster (TM-1..TM-4) | (c)/(d) | >55.000 hội thoại | Anh | chưa nêu | 2 |
| 10 | CSConDa (trong ViRE) | (a) | tối đa 1.000 cặp question–context (mẫu ViRE) | Việt | CC BY-NC 4.0 | 3 |

### (a) Tiếng Việt / Việt Nam

**CSConDa — Customer Support Conversation Dataset (trong benchmark ViRE)** — https://github.com/longstnguyen/ViRE (đã đọc ở phiên này): miền "customer support — conversational service requests" cho retrieval tiếng Việt; ViRE lấy tối đa 1.000 cặp question–context; mô tả riêng của CSConDa (kích thước gốc, nguồn công ty, nơi tải): **chưa xác minh**; license CC BY-NC 4.0. Dùng để thử retrieval câu trả lời CSKH tiếng Việt, không phải hội thoại nhiều lượt có tool. Ánh xạ: kho FAQ/KB của RAG. ★3. Hạn chế: phi thương mại, nhỏ.

> Không tìm thấy bộ hội thoại CSKH/TMĐT tiếng Việt công khai nào khác đã xác minh (tìm GitHub "vietnamese customer service dialogue", "Vietnamese task-oriented dialogue e-commerce" cho 0 kết quả). Tiếng Việt vì vậy cần: dịch/việt hoá kịch bản τ-bench, hoặc tự dựng (mục 5). τ²-bench hỗ trợ "six languages" nhưng README đã đọc không liệt kê ngôn ngữ — **chưa xác minh có tiếng Việt**.

### (b) TMĐT công khai

**WebShop (Princeton NLP)** — https://github.com/princeton-nlp/WebShop (đã đọc). 1,18 triệu sản phẩm thật (Amazon), 12.087 chỉ dẫn do crowdsource; agent phải tìm–tuỳ chọn–mua. MIT. Metric (reward) không được README định nghĩa rõ. Ánh xạ: kịch bản "tìm và đặt hàng theo mô tả" → tool search + cart + checkout. ★4. Hạn chế: môi trường web tiếng Anh, không có hậu mãi (huỷ/đổi trả).

**ShoppingBench** — https://github.com/yjwjy/ShoppingBench (đã đọc). Sandbox mua sắm tương tác trên >2,5 triệu sản phẩm thật (~1,4 GB nén, 4,8 GB giải nén), 4 tập test theo ý định: Product Intent, Shop Intent, Voucher Intent, Web-search Intent (SimpleQA). Apache-2.0. Số task: chưa xác minh; metric cụ thể chưa nêu. Voucher/Shop intent gần với khuyến mãi/cửa hàng của sàn đa seller. ★4.

**Shopping MMLU** — https://github.com/KL4805/ShoppingMMLU (đã đọc). Benchmark đa nhiệm cho LLM về mua sắm online (NeurIPS 2024 D&B; dùng cho KDD Cup 2024): 4 kỹ năng (shopping concept understanding, shopping knowledge reasoning, user behavior alignment, multilingual), 5 loại tác vụ (multiple choice, retrieval, ranking, NER, generation). Apache-2.0, `data.zip` (CSV/JSON). Số mẫu và ngôn ngữ cụ thể: **chưa xác minh**. Dùng để chọn LLM nền cho miền TMĐT. ★3.

**EcomGPT / EcomInstruct** — https://github.com/Alibaba-NLP/EcomGPT (đã đọc). 2,5 triệu instruction (Chain-of-Task); 12 tập đánh giá mở (500 mẫu/tập): NER, span detection, extractive QA, review topic classification, attribute value recognition/detection, product selection/alignment, title attribute matching, product classification, title generation (Anh + Trung). Trọng số EcomGPT (7b1) trên ModelScope. README **không nêu license** và không xác nhận toàn bộ tập huấn luyện được công bố. ★3. Lưu ý: thiên về hiểu sản phẩm (xem file 10), ít hội thoại.

### (c) Benchmark / paper học thuật

**τ-bench / τ²-bench (Sierra)** — https://github.com/sierra-research/tau-bench , https://github.com/sierra-research/tau2-bench (đã đọc). Miền retail & airline (τ²: thêm telecom, banking_knowledge, intake/τ-Entity); agent có API tool theo miền và **chính sách phải tuân thủ**, đối thoại với user giả lập; MIT. τ²: text-v1 gồm 50 task cố định mỗi miền × 6 ngôn ngữ; voice-v1 EN: 50 airline, 114 retail, 114 telecom; intake >200 task. Metric trên leaderboard là Pass^1..Pass^4 (README tau-bench). Một số task retail English của τ² là 50 task đã được xem xét thay thế so với v1.0.1 nên điểm không so sánh trực tiếp với 1.x. **Rất sát bài toán** (đổi/trả/huỷ đơn có chính sách). Ánh xạ: DB giả `orders/items` ↔ `orders/order_items`, chính sách ↔ máy trạng thái đơn. ★5. Hạn chế: không phải tiếng Việt (chưa xác minh), policy kiểu Mỹ.

**τ²-bench-Verified (Amazon AGI)** — https://github.com/amazon-agi/tau2-bench-verified (đã đọc). Bản sửa lỗi dữ liệu của τ²-bench (chính sách, DB, logic, mơ hồ), MIT; framework không đổi. Dùng thay cho bản gốc để giảm nhiễu nhãn. ★4.

**ABCD — Action-Based Conversations Dataset** — https://github.com/asappresearch/abcd (đã đọc). >10K hội thoại người–người (Expert Live Chat), 55 ý định, 30 hành động (click), tác vụ Action State Tracking và Cascading Dialogue Success; kịch bản mua sắm trực tuyến (ví dụ yêu cầu hoàn tiền kiểm tra theo chính sách công ty). MIT. Ánh xạ: ý định/hành động → tool; chính sách → kiểm tra server. ★5. Hạn chế: tiếng Anh, kịch bản công ty giả.

**MultiWOZ** — https://github.com/budzianowski/multiwoz (đã đọc). ~10,4K hội thoại (3.406 một miền, 7.032 đa miền), miền du lịch (hotel, restaurant, taxi, train, attraction...). Code MIT; license dữ liệu README không nêu riêng. Dùng để thử state tracking nói chung, không TMĐT. ★2.

**Taskmaster (Google Research)** — https://github.com/google-research-datasets/Taskmaster (đã đọc). >55.000 hội thoại nói/viết, hơn chục miền, các thư mục TM-1-2019, TM-2-2020, TM-3-2020, TM-4-2024; README không liệt kê miền hay license. ★2.

### (d) Blog / cuộc thi / công ty

- **KDD Cup 2024 (ShopBench/Shopping MMLU)** — đã nêu ở (b); trang cuộc thi nêu bên README. Không thu thập được blog Shopee/Tiki/Lazada/Zalo/VinAI về CSKH trong phiên này.

## 5. Dữ liệu tự tạo / synthetic từ chính hệ thống

- **Kịch bản chính sách làm "luật":** từ máy trạng thái đơn (docs) rút ra bảng quyết định (huỷ được khi `AWAITING_PAYMENT`/`CONFIRMED`/`PAID` chưa SHIPPED; trả hàng khi `DELIVERED` trong `RETURN_WINDOW_DAYS`, một lần; v.v.).
- **Môi trường mô phỏng kiểu τ-bench:** một DB demo cố định (`SEED_DEMO_DATA=true`: buyer, store, sản phẩm, đơn ở nhiều trạng thái) + user simulator (LLM) + tập tool = API gateway. Mỗi task: `scenario_vi`, `persona` (kiểu nói: lịch sự/nóng giận/viết tắt/không dấu), `user_id` (JWT), `expected_final_state` (so khớp DB), `forbidden_actions`, `expected_tool_calls`.
- **Nhóm task nên có:** hỏi giá/tồn; thêm giỏ nhiều sản phẩm; kiểm tra đơn theo mã/theo "đơn gần nhất"; huỷ đơn hợp lệ/không hợp lệ (đã SHIPPED); trả hàng hết hạn; hoàn tiền một phần; đổi địa chỉ (chỉ khi cho phép); hỏi giá có phí ship; yêu cầu xem đơn của người khác (phải từ chối); prompt injection ("bỏ qua chính sách…") nhúng trong tên sản phẩm/ghi chú.
- **Thu thập hội thoại thật:** sau khi đồng ý, ẩn danh PII (SĐT, địa chỉ, tên) trước khi lưu/gán nhãn; chia train/dev/test **theo thời gian và theo kịch bản** (không chia ngẫu nhiên từng lượt).
- **Dịch/việt hoá:** dịch τ-bench retail, ABCD sang tiếng Việt bằng LLM rồi để người Việt duyệt (chú ý đơn vị tiền VND, địa danh, COD, shipper); nhớ kiểm tra license khi phân phối lại.

## 6. Lộ trình dùng dữ liệu đề xuất

1. **Bước 1:** τ-bench/τ²-bench (+ Verified) và ABCD để chọn LLM, mẫu prompt và kiến trúc tool+policy; báo cáo pass^k cơ sở.
2. **Bước 2:** WebShop/ShoppingBench để thử luồng tìm–chọn–mua và voucher; Shopping MMLU/EcomGPT eval để so các LLM trên kiến thức TMĐT.
3. **Bước 3:** CSConDa/ViRE để thử retrieval KB tiếng Việt; tự dựng kịch bản tiếng Việt theo mục 5.
4. **Bước 4:** pilot nội bộ với buyer thử nghiệm; log đã ẩn danh -> mở rộng bộ vàng; kiểm tra red-team (injection, rò dữ liệu).
5. **Bước 5:** triển khai từng bước (chỉ read-only → ghi có xác nhận), theo dõi CSAT và handoff.

## 7. Rủi ro, license, thiên lệch, lưu ý pháp lý/PII

- **Hành động có hậu quả:** agent không được tự huỷ/hoàn tiền; luôn xác nhận và để server kiểm tra quyền sở hữu (ownership check ở gateway). Giữ idempotency.
- **Prompt injection gián tiếp:** nội dung do seller/buyer kiểm soát (tên/mô tả sản phẩm, đánh giá, ghi chú đơn) là *dữ liệu không tin cậy*, phải cách ly khỏi chỉ dẫn hệ thống.
- **PII và thanh toán:** không xử lý số thẻ/CVV (thanh toán mô phỏng, `card_last4` chỉ 4 số); không đưa SĐT/địa chỉ vào log gửi bên ngoài; tuân thủ Nghị định 13/2023/NĐ-CP (xác nhận với chuyên gia pháp lý); hệ thống chưa có `DELETE /users/me/data` (roadmap).
- **License:** τ-bench, WebShop, ABCD, τ²-Verified: MIT; ShoppingBench, Shopping MMLU: Apache-2.0; EcomGPT, MultiWOZ (dữ liệu), Taskmaster: README không nêu; CSConDa: CC BY-NC 4.0 (phi thương mại).
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

Chưa xác minh (không mở được trang hoặc tìm không thấy trong phiên): Bitext customer support dataset (Hugging Face), Amazon-C4, eCeLLM/ECInstruct (tìm GitHub 0 kết quả), bộ hội thoại CSKH Việt Nam khác, danh sách 6 ngôn ngữ của τ²-bench, số mẫu của Shopping MMLU/ShoppingBench, license dữ liệu MultiWOZ/Taskmaster/EcomGPT.
