# Gợi ý sản phẩm từ click/view/cart/mua (Recommendation)

> Gợi ý sản phẩm theo hành vi (session-based, co-visitation, collaborative filtering, ranking, dự đoán CTR) cho buyer trên sàn. **Trạng thái: chưa triển khai trong hệ thống; đây là tài liệu nghiên cứu dữ liệu.** Ngày tra cứu: 2026-10-25.
>
> Ghi chú phương pháp tra cứu: công cụ fetch không mở được Kaggle / HuggingFace / arXiv (lỗi DNS), nên thông tin lấy từ GitHub (đọc trực tiếp) và kết quả WebSearch. Nguồn nào chỉ là kết quả search được ghi rõ; số liệu/license không thấy trong nguồn ghi **"chưa xác minh"**. Phạm vi dữ liệu: dữ liệu tiếng Anh/quốc tế dùng được để chọn phương pháp và dựng pipeline; dữ liệu tiếng Việt: tự tạo từ hệ thống (mục 5) hoặc ViEcomRec. Theo quy tắc ngôn ngữ của dự án (chỉ giữ tiếng Anh/tiếng Việt hoặc dữ liệu không có ngôn ngữ tự nhiên), đã loại các bộ có văn bản tiếng Trung: Taobao User Behavior, Ali-CCP. Cột "Quy mô / dung lượng" nêu số mẫu và dung lượng tải; "không tìm thấy" nghĩa là README/kết quả search không nêu con số (không suy đoán); Kaggle/HuggingFace/arXiv không mở trực tiếp được nên các số mới đều lấy từ snippet search.

| Liên quan | File |
|---|---|
| Tìm kiếm ngữ nghĩa (query-product) | [02_PRODUCT_SEARCH_SEMANTIC.md](02_PRODUCT_SEARCH_SEMANTIC.md) |
| Phân loại/thuộc tính sản phẩm (feature cho ranking) | [10_PRODUCT_CATEGORIZATION_ATTRIBUTES.md](10_PRODUCT_CATEGORIZATION_ATTRIBUTES.md) |
| CLV, churn, định giá | [13_CUSTOMER_ANALYTICS_CLV_CHURN_PRICING.md](13_CUSTOMER_ANALYTICS_CLV_CHURN_PRICING.md) |
| NLP tiếng Việt / hiểu truy vấn | [15_VIETNAMESE_NLP_QUERY_UNDERSTANDING.md](15_VIETNAMESE_NLP_QUERY_UNDERSTANDING.md) |
| Đánh giá agent/tool use | [16_AGENT_TOOL_USE_EVAL.md](16_AGENT_TOOL_USE_EVAL.md) |

## 1. Mục tiêu và vị trí trong hệ thống

- **Vai trò hưởng lợi:** buyer (khối "Gợi ý cho bạn", "Người mua xem cũng xem", "Mua kèm" ở trang sản phẩm/giỏ hàng/trang chủ); seller (gián tiếp: tăng hiển thị); admin (theo dõi chất lượng gợi ý).
- **Input:** `anonymous_id` / `session_id` / `user_id`, chuỗi sự kiện gần nhất trong phiên, `product_id` đang xem, ngữ cảnh (`surface`, `device_type`).
- **Output:** danh sách top-K `product_id` đã lọc (status `active`, còn hàng `inventory>0`, không phải hàng của chính buyer-seller) kèm điểm số.
- **Dữ liệu của hệ thống nuôi nó** (xem `docs/design/02_DATA_MODEL.md` §3):
  - ClickHouse `events_raw`: `event_type` (click/view/add_to_cart/impression...), `anonymous_id`, `session_id`, `user_id`, `product_id`, `position`, `surface`, `ts`, `analytics_consent`, `props`.
  - `fact_order_items` (`order_id, product_id, qty, unit_price_minor, category_id, store_id, at`) + `fact_order_status` (chỉ tính đơn PAID/DELIVERED là "mua thật").
  - `dim_products` (`name, category_id, brand, price_minor, status`), `fact_inventory` (lọc hết hàng).
  - Postgres `cart_items` (giỏ hiện tại), `products` (catalog).
- Lưu ý: tracking chỉ ghi khi `analytics_consent` đồng ý; `position` trong `events_raw` cho phép học/khử position bias nếu có event `impression`.

## 2. Hướng làm đề xuất (baseline → nâng cao)

1. **Baseline phi mô hình:** top bán chạy / top xem theo danh mục, theo cửa sổ thời gian (từ `fact_order_items`, `events_raw`).
2. **Co-visitation / item-to-item:** đếm cặp sản phẩm cùng phiên (view→view, view→cart, cart→order) có trọng số theo thời gian — cách thắng cuộc thi OTTO; chạy được ngay bằng SQL ClickHouse.
3. **Session-based:** GRU4Rec / SASRec / item-kNN (SKNN, V-SKNN) cho phiên ẩn danh.
4. **Collaborative (user đã đăng nhập):** ALS/implicit-feedback, LightFM, two-tower retrieval.
5. **Ranking 2 tầng:** retrieval (co-visit + embedding + popularity) → re-rank bằng GBDT (LightGBM) hoặc mạng CTR (DeepFM/DCN) dùng đặc trưng sản phẩm, giá, danh mục, cửa sổ thời gian.
6. **Debias:** dùng `position` từ `impression` để huấn luyện propensity (IPS), hoặc thêm position làm feature chỉ ở lúc train.
7. **Cold-start:** nội dung (tên, danh mục, brand, embedding mô tả — xem file 02/10).

## 3. Metric đánh giá

- Offline (leave-last-out / chia theo thời gian): **Recall@K, HitRate@K, MRR@K, NDCG@K** (K=10/20), **MAP@12** (kiểu H&M), cộng **catalog coverage, diversity, novelty, popularity bias**.
- CTR/ranking: **AUC, LogLoss, GAUC (theo user/session)**, calibration (ECE).
- Counterfactual/OPE: **IPS, SNIPS, Doubly Robust** (cần propensity từ `position`).
- Online (sau này): CTR của khối gợi ý, add-to-cart rate, conversion, **doanh thu/phiên** (định nghĩa doanh thu của hệ thống: subtotal đơn PAID/DELIVERED trừ hoàn tiền), A/B test với guardrail (độ trễ, tỷ lệ hết hàng).

## 4. Bộ dữ liệu

### Bảng tổng hợp

| # | Bộ dữ liệu | Nhóm | Quy mô / dung lượng | Loại tín hiệu | License | ★ |
|---|---|---|---|---|---|---|
| 1 | [OTTO RecSys Dataset](https://github.com/otto-de/recsys-dataset) | (b) | 12.899.779 session train, 216.716.096 event, 1.855.603 item; test 1.671.803 session (README GitHub). Dung lượng: không tìm thấy (README và search không nêu GB) | click/cart/order | CC-BY 4.0 (xác minh từ GitHub) | 5 |
| 2 | [REES46 eCommerce behavior (Kaggle)](https://www.kaggle.com/datasets/mkechinov/ecommerce-behavior-data-from-multi-category-store) | (b) | 2 tháng (2019-10, 2019-11); dung lượng ~9 GB (hướng dẫn nạp dữ liệu YDB qua search, chưa rõ là riêng file 2019-Nov hay cả hai); số dòng chưa xác minh (nguồn thứ cấp ghi ~9 triệu dòng, không khớp với ~9 GB, nên coi là chưa đáng tin) | view/cart/purchase | chưa xác minh | 5 |
| 3 | [RetailRocket](https://www.kaggle.com/datasets/retailrocket/ecommerce-dataset) | (b) | 2.756.101 event, 1.407.580 visitor (nguồn thứ cấp); dung lượng: không tìm thấy (một thư viện chỉ nêu file zip `ecommerce-dataset.zip`) | view/addtocart/transaction | chưa xác minh | 4 |
| 4 | [YooChoose / RecSys Challenge 2015](https://www.cs.toronto.edu/~mvolkovs/recsys2015.pdf) | (c)/(d) | 9.249.729 session, 33M click, 1,15M buy (paper); dung lượng: không tìm thấy | click/buy | chưa xác minh | 4 |
| 5 | [Diginetica (CIKM Cup 2016)](https://competitions.codalab.org/competitions/11161) | (d) | 573.935 session, 1.127.764 click, 1.235.380 view, 18.025 purchase (paper); dung lượng: không tìm thấy | query/click/view/purchase | chưa xác minh | 4 |
| 6 | [Coveo Data Challenge (SIGIR eCom 2021)](https://github.com/coveooss/SIGIR-ecom-data-challenge) | (d) | 4.934.699 session, 36.079.307 event (bảng thống kê paper qua search; README nêu ">30M event"); 3 file .csv lớn, dung lượng: không tìm thấy | browse/search/cart/purchase | Điều khoản trong repo (xem mục) | 4 |
| 7 | [Amazon Reviews 2023 (McAuley Lab)](https://huggingface.co/datasets/McAuley-Lab/Amazon-Reviews-2023) | (b) | ~570 triệu review, ~48 triệu sản phẩm, 33 danh mục (thẻ HF qua search); thẻ HF chỉ ghi nhóm "10B<n<100B" (không phải GB); dung lượng tải: không tìm thấy | rating/review/metadata | chưa xác minh (xem thẻ HF) | 3 |
| 8 | H&M Personalized Fashion (Kaggle) (chưa có link) | (b) | 31.788.324 giao dịch, ~1,37M khách, 105.542 article (nguồn thứ cấp); dung lượng GB: không tìm thấy (có kèm ảnh sản phẩm) | mua + ảnh/metadata | điều khoản cuộc thi (chưa xác minh) | 3 |
| 9 | [Instacart Market Basket](https://www.kaggle.com/c/instacart-market-basket-analysis/data) | (b) | 3.346.083 đơn (~3,4M dòng `orders.csv`), 206.209 user, 49.685 sản phẩm (BigML/Databricks qua search); dung lượng: không tìm thấy | basket (mua) | "research use" (nguồn thứ cấp) | 3 |
| 10 | [Dunnhumby Complete Journey](https://dunnhumby.com/sourcefiles) | (b) | 2.500 household, 2 năm, 8 bảng; dung lượng: không tìm thấy | giao dịch + coupon | chưa xác minh | 2 |
| 11 | [Open Bandit Dataset (ZOZOTOWN)](https://research.zozo.com/data.html) | (c) | ~26M dòng impression (trang ZOZO qua search); paper: kịch bản "All" có 1.374.327 dòng (policy Random) và 12.168.084 dòng (Bernoulli TS); dung lượng GB: không tìm thấy. Không có văn bản ngôn ngữ tự nhiên (feature số/ID) nên giữ | impression + position + propensity | CC BY 4.0 (nguồn thứ cấp; README repo chỉ nêu Apache-2.0 cho code) | 4 (position bias) |
| 12 | [ViEcomRec (face cleanser, Việt Nam)](https://github.com/linh222/face_cleanser_recommendation_dataset) | (a) | 369.099 interaction (paper/search); dung lượng: không tìm thấy | user–item | chưa xác minh | 3 |
| 13 | [KuaiRec](https://github.com/chongminggao/KuaiRec) | (c) | small: 1.411 user × 3.327 video, 4.676.570 tương tác; big: 7.176 × 10.728, 12.530.806; dung lượng ~437 MB (bản trên Zenodo qua search, gồm một file ~432 MB; có thể khác bản kuairec.com). Dùng phần ma trận tương tác (ID), không dùng caption tiếng Trung | xem/tương tác | CC-BY-SA-4.0 (đã xác minh, GitHub) | 2 |
| 14 | [Tenrec (Tencent)](https://github.com/yuangh-x/2022-NIPS-Tenrec) | (c) | ~5M user, ~140M interaction (paper); riêng QK-video: 5,0M user, 142M tương tác (bảng phụ lục NeurIPS qua search); dung lượng: không tìm thấy. Dùng phần tương tác (ID), không có văn bản | click/like/share/follow + negative thật | CC BY-NC 4.0 (đã xác minh, GitHub) | 2 |
| 15 | [Yambda (Yandex)](https://arxiv.org/pdf/2505.22238) | (d) | ~4,79–5 tỷ event (bản 500M, 50M), 10 tháng (tin tức qua search); dung lượng ~135,41 GB cho gói `yambda.zip` (danh mục hyper.ai qua search, nguồn thứ ba, ghi tác giả không khớp, chưa xác minh); dung lượng từng bản 500M/50M: không tìm thấy | listen/like/dislike + `is_organic` | chưa xác minh | 2 |
| 16 | [Amazon-M2 (KDD Cup 2023)](https://arxiv.org/pdf/2307.09688) | (c)/(d) | Đa locale (UK, JP, DE, ES, FR, IT); chỉ dùng phần tiếng Anh (locale UK). Toàn bộ 6 locale: 3.606.249 session train, 361.659 session test, 1.410.675 sản phẩm (bảng so sánh paper NeurIPS qua search); riêng phần UK và dung lượng: không tìm thấy | session-based + sinh tiêu đề | chưa xác minh | 4 |
| 17 | [Dressipi RecSys Challenge 2022](https://www.research-collection.ethz.ch/items/558fe8bb-63e0-47c9-a97f-ed5d9a493058) | (d) | ~1,1M session thời trang, ~24K item, 18 tháng; tháng cuối làm test (~100K session) (search); dung lượng: không tìm thấy | view → 1 purchase + nhãn thuộc tính | chưa xác minh | 3 |
| 18 | [Criteo 1TB Click Logs](https://ailab.criteo.com/download-criteo-1tb-click-logs-dataset/) | (d) | ~4 tỷ dòng, 24 file (mỗi ngày một file `day_N.gz`); dung lượng: ~1,6–1,7 GB mỗi file nén trên HF (search), trang Criteo ghi toàn bộ 342 GB (có thể lỗi thời); tổng HF 39,2 GB hiển thị mâu thuẫn, chưa xác minh | click quảng cáo (feature ẩn danh) | Terms of use (chưa xác minh) | 2 |

### (a) Tiếng Việt / Việt Nam

**ViEcomRec — Vietnamese e-commerce recommendation dataset**
- Link: GitHub `linh222/face_cleanser_recommendation_dataset`; bài CSoNet 2023 (Springer, chương 7 trong https://link.springer.com/chapter/10.1007/978-981-97-0669-3_7) và bản DCU https://doras.dcu.ie/29693/1/viecomrec.pdf. Nguồn: GitHub + paper.
- Quy mô: 369.099 tương tác user–item, chỉ ngành hàng sữa rửa mặt (theo tóm tắt tìm kiếm). Ngôn ngữ: tiếng Việt. License: **chưa xác minh**. Nền tảng gốc (Tiki/Shopee/khác): **chưa xác minh**.
- Cột/nhãn chính: chưa xác minh (kiểm tra README repo). Baseline trong bài: content-based, thêm popularity cho +27,21% NDCG@10.
- Ánh xạ: user–item → `events_raw` (view/click) hoặc `fact_order_items`; dùng để thử pipeline gợi ý nội dung/popularity trên văn bản tiếng Việt.
- Phù hợp ★3. Hạn chế: một ngành hàng, không có session/timestamp chắc chắn (chưa xác minh), quy mô nhỏ.

> Ghi chú: dữ liệu hành vi tiếng Việt: tự tạo từ hệ thống (mục 5) hoặc dùng ViEcomRec; các bộ tiếng Anh/quốc tế bên dưới dùng được để chọn phương pháp và dựng pipeline (ID đã băm nên không phụ thuộc ngôn ngữ).

### (b) TMĐT công khai

**OTTO RecSys Dataset** — https://github.com/otto-de/recsys-dataset (GitHub + cuộc thi Kaggle OTTO).
- Quy mô (xác minh từ README GitHub): train 12.899.779 session, 216.716.096 event, 1.855.603 item; test 1.671.803 session, 13.851.293 event. Định dạng `.jsonl`. License dữ liệu **CC-BY 4.0**, code MIT. Ngôn ngữ: không có văn bản (ID).
- Cột: `session`, `events[]` gồm `aid`, `ts`, `type` ∈ {clicks, carts, orders}.
- Ánh xạ: `session`→`session_id`, `aid`→`product_id`, `ts`→`ts`, `type`→`event_type` (click/add_to_cart/order từ `fact_order_*`). Dùng validate co-visitation + LightGBM rerank và tối ưu thuật toán trước khi có dữ liệu thật.
- ★5. Hạn chế: không có giá/danh mục/văn bản, không có impression/position, chỉ 4 tuần; lớn (cần chia nhỏ khi thử).

**REES46 — eCommerce behavior data from multi category store** — Kaggle `mkechinov/ecommerce-behavior-data-from-multi-category-store` (tác giả Michael Kechinov, từ nền tảng REES46).
- Cột (theo nguồn thứ cấp): event time, event type (view/cart/purchase, gồm cả remove_from_cart theo mô tả khác: chưa xác minh), product_id, category_id/code, brand, price, user_id, user_session. File 2019-Nov ~9 GB. Có dòng trùng hệt nhau (cần dedupe). Quy mô theo search: ~9 triệu dòng cho 2 tháng (2019-10, 2019-11) từ cửa hàng mỹ phẩm/đa danh mục (nguồn thứ cấp; một nguồn khác nói file 900K dòng — khác file). License: **chưa xác minh** (một thảo luận cộng đồng coi là dùng phi thương mại — không phải văn bản license).
- Ánh xạ gần như 1-1 với `events_raw` + `dim_products` (brand, price, category) → rất sát schema của ta. ★5. Hạn chế: không impression/position, giá USD, license chưa rõ.

**RetailRocket** — Kaggle `retailrocket/ecommerce-dataset` (không mở được trang; thông tin qua GitHub/tài liệu thư viện).
- 2.756.101 event (2.664.312 view, 69.332 addtocart, 22.457 transaction), 1.407.580 visitor, ~4,5 tháng; item properties 417.053 item (nguồn thứ cấp). File: `events.csv`, item properties, category tree. Giá trị đã băm. License: **chưa xác minh**.
- Ánh xạ: events→`events_raw`, category tree→`categories`. ★4. Hạn chế: thưa, đã băm nên không có văn bản.

**Amazon Reviews 2023** — https://huggingface.co/datasets/McAuley-Lab/Amazon-Reviews-2023 ; site dự án amazon-reviews-2023.github.io; paper arXiv 2403.03952. Đã xác minh (nguồn: WebSearch): ~570 triệu review, ~48 triệu sản phẩm, 33 danh mục (+ Unknown), dữ liệu tương tác đến 09/2023, tăng 245,2% so với bản trước; số chính xác 571,54M/48,19M không xác nhận được từ search.
- Gồm review (rating, text, helpful votes), item metadata (mô tả, giá, ảnh), link graph (bought-together); có split benchmark 5-core theo timestamp trong repo HF. License: **chưa xác minh**.
- Ánh xạ: rating/review→tín hiệu ngầm; metadata→`products`. ★3. Hạn chế: rating/review chứ không phải click; tiếng Anh; không session.

**H&M Personalized Fashion Recommendations** — Kaggle (cuộc thi 2022).
- 31.788.324 giao dịch, ~1,37M khách, 105.542 article, ~2 năm (2018-09 → 2020-09), kèm ảnh (nguồn thứ cấp). Metric cuộc thi MAP@12. Đã xác minh (WebSearch): ~2 năm giao dịch, ~1,3M khách, >100K item, ~32M giao dịch, cuộc thi H&M trên Kaggle 2022. License = điều khoản cuộc thi, **chưa xác minh** (không thấy văn bản license).
- Ánh xạ: transactions→`fact_order_items`; articles→`dim_products`. ★3. Hạn chế: chỉ mua, không click, thời trang châu Âu.

**Instacart Market Basket Analysis** — Kaggle 2017.
- Đã xác minh (nguồn: WebSearch, trang BigML): 3.346.083 đơn, 206.209 user, 49.685 sản phẩm; ẩn danh. License: một thư viện ghi "competition-specific (research use)" (không chính thức).
- Dùng cho "mua kèm"/reorder, cart-completion. ★3. Hạn chế: tạp hoá, không click.

**Dunnhumby — The Complete Journey** — https://dunnhumby.com/sourcefiles
- 2.500 household, 2 năm giao dịch, kèm nhân khẩu học, chiến dịch, coupon (8 bảng). Nhiều bản mirror khác con số (1 năm, 2.469 hộ...) nên cần ghi phiên bản. License: **chưa xác minh**; trang nói dành cho nghiên cứu học thuật.
- Dùng cho basket, khuyến mãi (liên quan file 13). ★2.

### (c) Benchmark / paper học thuật

**Open Bandit Dataset (ZOZOTOWN)** — https://research.zozo.com/data.html ; paper arXiv 2008.07146.
- ~26M dòng (dung lượng GB: không tìm thấy; dữ liệu là feature số/ID, không có văn bản nên không bị loại theo quy tắc ngôn ngữ); mỗi dòng là một impression có feature, item được chọn, **position (1/2/3 = trái/giữa/phải)**, xác suất hành động logging và nhãn click; chiến dịch All/Men/Women. License **CC BY 4.0** (theo kết quả tìm kiếm; kiểm tra lại trang gốc).
- Ánh xạ: `impression` + `position` → `events_raw.position`; dùng để validate IPS/DR và mô hình position bias. ★4 cho mảng debias. Hạn chế: chỉ 3 vị trí, 80 item.

**KuaiRec** — https://kuairec.com ; https://github.com/chongminggao/KuaiRec ; arXiv 2202.10842. 1.411 user × 3.327 video, ma trận "small" quan sát gần đầy đủ để đánh giá không thiên lệch; có "big matrix" riêng. License: **CC-BY-SA-4.0** (đã xác minh, GitHub). Tải từ Zenodo (https://zenodo.org/records/18164998). Small matrix 4.676.570 tương tác (mật độ 99,6%), big matrix 12.530.806 tương tác (mật độ 16,3%). Dùng để thử đánh giá chống selection bias. ★2 (video, không phải TMĐT).

**Tenrec** — https://github.com/yuangh-x/2022-NIPS-Tenrec ; NeurIPS 2022 D&B. ~5M user, ~140M tương tác, 4 kịch bản, có negative thật, nhiều loại phản hồi (theo paper). Đã xác minh (GitHub): license **CC BY-NC 4.0** (phi thương mại), phải chấp nhận thỏa thuận ở trang chính thức khi tải; file QK-video/QB-video/QK-article/QB-article. ★2 (nội dung/video).

**Yambda (Yandex)** — arXiv 2505.22238 ; Hugging Face (theo bài báo). ~5B event (cũng có bản 500M, 50M), ~1M user, ~9,4M track, flag `is_organic`, Parquet, chia theo thời gian toàn cục (GTS). Đã xác minh (nguồn: WebSearch, tin tức): ~4,79 tỷ tương tác ẩn danh trong 10 tháng, phát hành 05/2025, có trên Hugging Face. License: **chưa xác minh**. Hữu ích để học cách chia theo thời gian & phân biệt organic/recommended. ★2.

**Criteo 1TB Click Logs** — https://ailab.criteo.com/download-criteo-1tb-click-logs-dataset/ . 24 ngày, 13 feature số + 26 feature phân loại băm, ~4 tỷ dòng. Đã xác minh (WebSearch, trang Criteo AI Lab): >4 tỷ dòng, 24 file (mỗi ngày một file), host trên HuggingFace; Terms of use: **chưa xác minh** (trang không hiện văn bản; CC BY-NC-SA 4.0 chỉ thấy cho dataset Attribution Modeling khác của Criteo, không áp dụng suy diễn). Dùng để thử mạng CTR (DeepFM, DCN). ★2.

**Amazon-M2 (NeurIPS 2023 D&B, KDD Cup 2023)** — paper arXiv 2307.09688; NeurIPS: https://papers.nips.cc/paper_files/paper/2023/hash/193df57a2366d032fb18dcac0698d09a-Abstract-Datasets_and_Benchmarks.html . Đã xác minh (nguồn: WebSearch): session mua sắm đa ngôn ngữ/đa locale (UK, JP, DE, ES, FR, IT), 3 tác vụ: next-product recommendation, next-product dưới domain shift, sinh tiêu đề sản phẩm tiếp theo; dùng cho KDD Cup 2023 (AIcrowd). **Chỉ dùng phần tiếng Anh (locale UK)**; các locale JP/DE/ES/FR/IT bỏ qua theo quy tắc ngôn ngữ của dự án. Quy mô toàn bộ 6 locale (bảng so sánh trong paper NeurIPS, qua search): 3.606.249 session train, 361.659 session test, 1.410.675 sản phẩm; quy mô riêng phần UK và dung lượng tải: **không tìm thấy** (search không trả về bảng thống kê theo locale). License: **chưa xác minh** (link GitHub `Amazon-M2/Amazon-M2` đoán thử trả 404). Có văn bản sản phẩm nên dùng cho session-based có content. ★4.

**Dressipi — RecSys Challenge 2022 (1M Fashion Sessions)** — https://www.research-collection.ethz.ch/items/558fe8bb-63e0-47c9-a97f-ed5d9a493058 ; https://recsys.acm.org/recsys22/challenge/ . Đã xác minh (nguồn: WebSearch): session duyệt ẩn danh, mỗi session kết thúc bằng 1 lần mua, kèm nhãn thuộc tính sản phẩm (màu, cổ áo...); ~1,1 triệu session, ~24K item, 01/2020–06/2021; tháng cuối làm test, có item mới (cold-start). License: **chưa xác minh**. Dùng cho dự đoán item mua cuối phiên + cold-start bằng thuộc tính. ★3.

### (d) Cuộc thi / công ty

**YooChoose — RecSys Challenge 2015** — (nguồn qua các paper). Train 9.249.729 session, 33.003.944 click, 1.150.753 buy (509.696 session có buy); test 2.312.432 session; ~6 tháng 2014, nhà bán lẻ châu Âu. Đã xác minh (nguồn: WebSearch): trang chính thức RecSys Challenge 2015 mô tả "six months of activities of a big e-commerce business in Europe"; thư viện beta-recsys trỏ tới `yoochoose-data.7z` gồm file click và file buy (link S3 chưa kiểm tra còn sống). License: **chưa xác minh** (trang không nêu). Dùng cho dự đoán "session có mua không" và session-based. ★4.

**Diginetica (CIKM Cup 2016 Track 2)** — https://competitions.codalab.org/competitions/11161 ; arXiv 1708.04479. 53.427 query có văn bản, 869.700 query không văn bản, 573.935 session, 1.127.764 click, 1.235.380 view, 18.025 purchase; metadata sản phẩm (danh mục, mô tả, giá). Nhiều user ẩn danh. Điều khoản tải: **chưa xác minh**. Vừa dùng cho recsys vừa cho search cá nhân hoá (file 02). ★4.

**Coveo Data Challenge (SIGIR eCom 2021)** — https://github.com/coveooss/SIGIR-ecom-data-challenge ; arXiv 2104.09423. >30M browsing event, vài triệu session; dữ liệu ẩn danh, metadata băm. Điều khoản trong repo: chỉ dùng cho mục đích nêu trong Terms, không được de-anonymise; "freely available outside the Challenge" theo README. (Link `coveooss/shopping-session-dataset` bị 404 — đừng dùng.) ★4.

## 5. Dữ liệu tự tạo / synthetic từ chính hệ thống

- **Nguồn thật quan trọng nhất:** `events_raw` (thu thập dưới `analytics_consent`), `fact_order_items`. Lọc `analytics_consent=1`, dedupe theo `event_id`, bỏ bot (UA/ip_hash tần suất cao).
- **Chia train/valid/test theo thời gian (không random):** ví dụ train 8 tuần, valid 1 tuần, test 1 tuần kế tiếp (theo `ts`, múi giờ Asia/Ho_Chi_Minh); với session-based: leave-last-item-out trong session thuộc tuần test.
- **Nhãn:** positive = `add_to_cart` / `order` (PAID/DELIVERED), weak = `click`/`view`; negative = `impression` không click (dùng `position` để IPS).
- **Cold-start dữ liệu:** `SEED_DEMO_DATA=true` + `scripts/dev/gen_traffic.py` tạo traffic giả (chỉ để kiểm pipeline, không đánh giá chất lượng). Có thể dùng **OTTO/REES46 để "replay"**: ánh xạ `aid`/`product_id` sang `dim_products` giả và nạp vào `events_raw` để kiểm tải ClickHouse và truy vấn co-visitation.
- **Synthetic sạch hơn:** sinh phiên bằng chuỗi Markov theo danh mục + nhiễu popularity, để có "chân lý" (ground truth) đã biết cho unit test của metric.
- **Dữ liệu vàng:** giữ lại nhóm holdout (tuần cuối), không bao giờ dùng để huấn luyện; log phiên bản mô hình + propensity (xác suất hiển thị) mỗi lần gợi ý để OPE sau này.

## 6. Lộ trình dùng dữ liệu đề xuất

1. **Bước 1 (dựng pipeline & metric):** OTTO (co-visitation + LightGBM, Recall@20) và REES46 (schema giống `events_raw`, có brand/price/category) — validate SQL ClickHouse, chia thời gian, metric.
2. **Bước 2 (độ tổng quát):** RetailRocket + YooChoose/Diginetica cho session-based (GRU4Rec/SASRec, SKNN).
3. **Bước 3 (position bias / CTR):** Open Bandit Dataset (IPS/DR) và Criteo (mạng CTR).
4. **Bước 4 (tiếng Việt, nội dung):** ViEcomRec; sau đó dữ liệu thật của hệ thống khi có đủ traffic.
5. **Bước 5:** A/B online với guardrail; cập nhật mô hình theo `events_raw` mới.

## 7. Rủi ro, license, thiên lệch, lưu ý pháp lý/PII

- Nhiều bộ chỉ cho **nghiên cứu/phi thương mại** (Coveo, YooChoose, Criteo, có thể REES46): kiểm tra điều khoản trước khi dùng cho mô hình triển khai thương mại; chỉ OTTO có CC-BY 4.0 được xác minh rõ trong phiên này.
- Thiên lệch: popularity bias, selection/exposure bias (người dùng chỉ click cái được hiển thị), phân phối khác thị trường Việt Nam (mùa vụ Tết, 11.11, COD, mô hình mua nhiều shop/đơn).
- PII: `events_raw` có `user_id`, `ip_hash`; chỉ dùng khi `analytics_consent`; dữ liệu vào mô hình/bên thứ ba phải ẩn danh hoá; chưa có `DELETE /users/me/data` (roadmap) nên cần chốt chính sách xoá trước khi huấn luyện trên dữ liệu thật. Tuân thủ Nghị định 13/2023/NĐ-CP về bảo vệ dữ liệu cá nhân (cần xác nhận pháp lý với chuyên gia).
- Dữ liệu công khai là ID đã băm: không thể dùng trực tiếp cho mô hình nội dung tiếng Việt.

## 8. Nguồn tham khảo

Đã thấy trong phiên (trang hoặc kết quả tìm kiếm):
- https://github.com/otto-de/recsys-dataset (đã đọc trực tiếp)
- https://github.com/coveooss/SIGIR-ecom-data-challenge ; https://arxiv.org/pdf/2104.09423
- https://competitions.codalab.org/competitions/11161 ; https://arxiv.org/pdf/1708.04479
- https://arxiv.org/pdf/2408.14851 ; https://www.cs.toronto.edu/~mvolkovs/recsys2015.pdf (YooChoose thống kê)
- https://paperswithcode.com/dataset/retailrocket ; https://beta-recsys.readthedocs.io/en/latest/_modules/beta_rec/datasets/retailrocket.html
- https://huggingface.co/datasets/McAuley-Lab/Amazon-Reviews-2023 (xuất hiện trong kết quả) ; https://arxiv.org/abs/2403.03952 (chưa xác minh trực tiếp)
- https://arxiv.org/pdf/2202.10842 ; https://github.com/chongminggao/KuaiRec
- https://arxiv.org/pdf/2210.10629v1 ; https://github.com/yuangh-x/2022-NIPS-Tenrec
- https://arxiv.org/pdf/2008.07146 ; https://research.zozo.com/data.html
- https://arxiv.org/pdf/2505.22238 ; https://yandex.com/company/news/28-05-2025
- https://dunnhumby.com/sourcefiles
- https://ailab.criteo.com/download-criteo-1tb-click-logs-dataset/
- https://doras.dcu.ie/29693/1/viecomrec.pdf ; https://link.springer.com/chapter/10.1007/978-981-97-0669-3_7

Chưa xác minh trực tiếp (không mở được trang): trang Kaggle của REES46, RetailRocket, H&M, Instacart; license của hầu hết bộ nói trên.
