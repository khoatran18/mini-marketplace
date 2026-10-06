# 04 · Hệ gợi ý (RecSys)

Mục tiêu tài liệu: (1) chốt **các yếu tố một hệ gợi ý cần**, (2) chốt **dữ liệu phải thu ngay**, (3) định nghĩa **serving contract** và **cách đánh giá** để bạn tự chọn thuật toán. Phần thuật toán là việc của bạn.

## 1. Bài toán trên Mini Marketplace
- Mục tiêu kinh doanh: tăng CVR/GMV, giúp khám phá hàng mới (long tail), giữ người dùng quay lại; ràng buộc: không gợi ý hết hàng/bị ẩn, công bằng giữa shop, đa dạng.
- Mục tiêu kỹ thuật: top-K sản phẩm cho (người dùng | phiên | sản phẩm | giỏ) × surface, độ trễ P95 < 100 ms (đọc từ cache/feature đã tính).

## 2. Các thành phần của một recsys (checklist "đã nghĩ tới chưa")
| Thành phần | Câu hỏi cần trả lời | Ở đây |
|---|---|---|
| Tín hiệu phản hồi | explicit vs implicit; mạnh/yếu; âm/dương | §3 |
| Đặc trưng item | text, danh mục, giá, ảnh, shop, tuổi, chất lượng | §4 |
| Đặc trưng user/phiên | lịch sử, sở thích, ngữ cảnh, phiên hiện tại | §4 |
| Sinh ứng viên (retrieval) | rẻ, recall cao, nhiều nguồn | §5 |
| Xếp hạng (ranking) | tối ưu click/mua, đặc trưng giàu | §5 |
| Tái xếp hạng (re-rank) | đa dạng, luật kinh doanh, công bằng, tồn kho | §5 |
| Cold start | user mới, item mới, shop mới | §6 |
| Thiên lệch | position, popularity, selection, exposure | §7 |
| Đánh giá offline | split theo thời gian, metric | §8 |
| Đánh giá online | A/B, guardrail | §8 |
| Vòng phản hồi | gợi ý ảnh hưởng dữ liệu tương lai | §7 |
| Serving & cache | latency, fallback, versioning | §9 |
| Giải thích & kiểm soát | "vì bạn đã xem…", ẩn/không quan tâm | §10 |
| Riêng tư | consent, ẩn danh | 02 §7 |

## 3. Tín hiệu phản hồi
| Tín hiệu | Loại | Mạnh | Trọng số gợi ý (khởi đầu, **cần học lại**) | Nguồn |
|---|---|---|---|---|
| `impression` không click | implicit âm yếu | rất nhiễu (chưa chắc nhìn thấy) | −0.05, chỉ dùng làm negative sampling | tracking |
| `product_click` | implicit dương | yếu | 1 | tracking |
| `dwell` ≥ 30 s / scroll sâu | implicit dương | trung bình | 2 | tracking |
| `wishlist_add` | explicit dương | khá | 3 | tracking |
| `add_to_cart` | intent mạnh | mạnh | 4 | tracking/cart |
| `checkout_start` | intent rất mạnh | | 5 | tracking |
| `purchase` (đơn hoàn tất) | conversion | rất mạnh | 8 | **order outbox** (nguồn sự thật) |
| `review` rating 4–5 / 1–2 | explicit ± | mạnh | +6 / −6 | reviews |
| `cancel`/`refund`/`return` | âm mạnh | | −4 | order |
| `remove_from_cart` | âm nhẹ | | −1 | tracking |
| `reco_feedback: not_interested` | explicit âm | rất mạnh | −8 (loại khỏi gợi ý) | UI |
| `search` + `search_click` | intent theo truy vấn | | 2 (gắn với `q`) | tracking |
Ghi chú: trọng số chỉ để khởi động mô hình implicit (ALS/BPR…); về sau thay bằng mô hình học trực tiếp mục tiêu (click, mua).

## 4. Đặc trưng (feature)
**Item**: `category_path`, `brand`, `price` (log + bucket, so với trung vị danh mục), `tags`, `attributes` (JSONB), embedding văn bản (từ `name+description`), embedding ảnh (tuỳ chọn), `age_days`, `inventory_level`, `seller_id`, chất lượng shop (tỉ lệ huỷ, rating), thống kê động (CTR 7d, CVR 7d, bán 7d/30d, xu hướng).
**User (dài hạn)**: số đơn, AOV, danh mục ưa thích (phân phối), dải giá ưa thích, tần suất, recency, thiết bị chính, thành phố; embedding người dùng (từ lịch sử).
**Phiên (ngắn hạn, quan trọng cho vãng lai)**: chuỗi item vừa xem/giỏ, truy vấn vừa gõ, thời gian trong phiên, nguồn truy cập.
**Ngữ cảnh**: giờ, thứ, ngày lễ/khuyến mãi, thiết bị, `surface`, vị trí hiển thị.
**Cặp (user,item)**: đã xem/mua chưa, lần cuối tương tác, khớp danh mục ưa thích, chênh lệch giá so với dải ưa thích.
Lưu trữ: offline = ClickHouse (bảng `features_*` dựng bằng job); online = Redis (`feat:user:{id}`, `feat:item:{id}`, TTL & version). **Cùng một định nghĩa feature cho offline/online** (tránh training-serving skew) – định nghĩa đặt trong một file khai báo (YAML) mà cả job và serving đọc.

## 5. Kiến trúc nhiều tầng

```
 request(user|anon, surface, context)
   │
   ├─ 1. CANDIDATE GENERATION (nhiều nguồn, mỗi nguồn ~100–500)
   │     • trending/popular (theo danh mục, theo vùng)        ← baseline, luôn có
   │     • item-item co-occurrence (xem/mua cùng)              ← "bought together", PDP
   │     • content-based (embedding văn bản/ảnh, ANN)          ← "tương tự", cold-start item
   │     • collaborative (ALS/BPR/two-tower user→item ANN)     ← home "dành cho bạn"
   │     • session-based (GRU4Rec/SASRec/co-visitation)        ← vãng lai, trong phiên
   │     • recent-interest (danh mục vừa xem), repurchase (hàng tiêu hao)
   │
   ├─ 2. FILTER  (hết hàng, bị ẩn, đã mua gần đây*, user đã "không quan tâm", shop bị khoá)
   │
   ├─ 3. RANKING (pointwise/LTR: LightGBM/GBDT → sau đó DNN/DCN, đa mục tiêu click+cart+mua)
   │
   ├─ 4. RE-RANK (đa dạng: MMR/danh mục tối đa/shop tối đa; công bằng shop mới; boost khuyến mãi; exploration ε)
   │
   └─ 5. LOG: request_id, model_version, experiment, danh sách item+điểm+vị trí → `reco_requests`
```
Chọn surface ↔ nguồn ứng viên:
| Surface | Ứng viên chính | Ghi chú |
|---|---|---|
| `home_for_you` | collaborative + session + trending (fallback) | vãng lai: trending + session |
| `pdp_similar` | content-based (embedding) + cùng danh mục/giá | không cần lịch sử người dùng |
| `pdp_bought_together` | item-item co-purchase | cần đủ đơn; fallback similar |
| `cart_addon` | co-purchase với toàn giỏ + phụ kiện | lọc đã có trong giỏ |
| `search_results` | rerank kết quả tìm kiếm theo cá nhân hoá nhẹ | giữ tính liên quan là ưu tiên |
| `post_purchase` / email | repurchase + bổ trợ | sau |

## 6. Cold start
- **User mới/vãng lai**: trending theo danh mục/vùng + session-based sau vài click; hỏi sở thích (tuỳ chọn onboarding: chọn 3 danh mục).
- **Item mới**: content-based từ embedding; dành **quota exploration** (vd. 5–10% vị trí) cho item mới để thu impression; bandit (Thompson/UCB) khi có đủ traffic.
- **Shop mới**: boost nhẹ có thời hạn để tránh vòng "giàu càng giàu".

## 7. Thiên lệch & vòng phản hồi
- **Position bias**: log `position`; huấn luyện với position làm feature (và bỏ lúc serving) hoặc IPS weighting.
- **Exposure bias**: chỉ có phản hồi trên cái *đã được hiển thị* → bắt buộc log **impression** (không chỉ click); duy trì ε-exploration để thu dữ liệu ngoài chính sách hiện tại và ghi `propensity` (xác suất được chọn) nếu muốn off-policy evaluation.
- **Popularity bias**: theo dõi coverage/Gini; re-rank đa dạng.
- **Feedback loop**: giữ một nhóm *holdout* (1–5% người dùng) nhìn gợi ý baseline để đo tác động thật.
- **Gian lận/spam**: loại bot (UA, tốc độ), loại tự-click của shop lên sản phẩm mình, giới hạn trọng số mỗi user–item.

## 8. Đánh giá
**Offline** (dữ liệu tách theo **thời gian**, không random; mỗi user chỉ dùng quá khứ để dự đoán tương lai):
- Retrieval: Recall@K, HitRate@K, MRR; Ranking: NDCG@K, MAP@K, AUC/logloss.
- Ngoài độ chính xác: **coverage**, **novelty**, **diversity** (intra-list), **serendipity**, tỉ lệ item mới được gợi ý, phân bố theo shop (công bằng).
- Slice: user mới vs cũ, từng danh mục, từng surface, thiết bị.
- Baseline bắt buộc để so: *popular*, *recently-viewed*, *random-in-category*. Mô hình phức tạp mà không thắng popular thì chưa nên triển khai.
**Online** (A/B qua flag + `experiment` gán theo hash(user_id|anonymous_id)):
- Chính: CTR, add-to-cart rate, CVR, doanh thu/phiên. Guardrail: latency, tỉ lệ lỗi, tỉ lệ huỷ/hoàn, tỉ lệ hết hàng sau khi gợi ý, khiếu nại.
- Hạ tầng cần: gán nhóm ổn định, log nhóm trên mọi event, trang `/admin/experiments`, công cụ tính ý nghĩa thống kê (bạn code/ dùng notebook).

## 9. Serving contract (xem [08](08-api-contracts.md#22-ai-service-grpc))
- `Recommend(user_ref, surface, context, k, exclude[])` → `{request_id, model_version, experiment, items[{product_id, score, reason_code}]}`
- `Similar(product_id, k)`; `BoughtTogether(product_id, k)`; `CartAddons(product_ids, k)`.
- Gateway **hậu xử lý** (bắt buộc, không tin AI): loại item không `active`/hết hàng, hydrate thông tin hiển thị từ product-service, trả `request_id`.
- Cache: `reco:{surface}:{user|anon}:{ctx_hash}` TTL 60–300 s; **fallback chain**: model → trending theo danh mục → trending toàn sàn → hàng mới. Timeout 150 ms rồi chuyển fallback.
- Batch/precompute: job đêm tính top-N cho người dùng hoạt động; online chỉ rerank nhẹ.

## 10. Giải thích & kiểm soát
- `reason_code`: `similar_to:<id>`, `bought_together`, `trending_in:<cat>`, `because_viewed:<id>`, `new_arrival`. UI hiển thị ("Vì bạn đã xem …").
- Nút "Không quan tâm"/"Ẩn" → `reco_feedback`, áp dụng ngay (filter) và lưu.
- Người dùng có thể tắt cá nhân hoá (consent) → chỉ trending/similar theo ngữ cảnh.

## 11. Lộ trình mô hình đề xuất (bạn làm)
| Mức | Mô hình | Điều kiện dữ liệu | Thu được |
|---|---|---|---|
| v0 | Trending theo danh mục + mới | có `fact_order_items`/`events` | baseline + fallback |
| v1 | Item-item co-visit/co-purchase (cosine/Jaccard, SQL trên ClickHouse) | vài nghìn đơn/phiên | "tương tự/hay mua cùng" |
| v2 | Content embedding + ANN (pgvector) | mô tả sản phẩm tốt | cold-start item |
| v3 | ALS/BPR (implicit) hoặc two-tower | đủ tương tác user–item | home cá nhân hoá |
| v4 | Ranker GBDT với feature giàu + LTR; session-based | impression log đầy đủ | tối ưu CTR/CVR |
| v5 | Đa mục tiêu, bandit, off-policy eval | traffic lớn | tối ưu liên tục |

Trước khi có traffic thật: dùng `scripts/datagen` để dev/test pipeline (đừng kết luận chất lượng mô hình từ dữ liệu giả).
