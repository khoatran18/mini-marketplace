# Tìm kiếm sản phẩm theo ngữ nghĩa / hybrid (BM25 + vector)

> Tìm sản phẩm bằng mô tả tự nhiên, từ khoá không dấu, đồng nghĩa; kết hợp BM25 và vector, rồi rerank theo mức độ liên quan query–sản phẩm. **Trạng thái: chưa triển khai trong hệ thống; đây là tài liệu nghiên cứu dữ liệu.** Ngày tra cứu: 2026-10-25.
>
> Ghi chú phương pháp: không mở được trực tiếp Kaggle/HuggingFace/arXiv (lỗi DNS của công cụ fetch); các trang GitHub (ESCI, WANDS, mMARCO, ViRE, JDsearch, SQID, Baidu-ULTR) được đọc trực tiếp, phần còn lại qua kết quả WebSearch. Điều gì không thấy trong nguồn được ghi **"chưa xác minh"**. Phạm vi: dữ liệu tiếng Anh/đa ngôn ngữ dùng được để chọn phương pháp (hybrid, rerank, LTR); dữ liệu tiếng Việt: tự tạo (mục 5) hoặc các benchmark retrieval Việt nêu ở mục (a).

| Liên quan | File |
|---|---|
| Gợi ý theo click | [01_RECOMMENDATION_CLICK.md](01_RECOMMENDATION_CLICK.md) |
| Phân loại / thuộc tính sản phẩm | [10_PRODUCT_CATEGORIZATION_ATTRIBUTES.md](10_PRODUCT_CATEGORIZATION_ATTRIBUTES.md) |
| Tìm bằng ảnh / đa phương thức | [11_VISUAL_SEARCH_MULTIMODAL.md](11_VISUAL_SEARCH_MULTIMODAL.md) |
| NLP tiếng Việt, hiểu truy vấn | [15_VIETNAMESE_NLP_QUERY_UNDERSTANDING.md](15_VIETNAMESE_NLP_QUERY_UNDERSTANDING.md) |
| RAG chính sách/pháp lý (retrieval tiếng Việt chung) | [06_POLICY_LEGAL_RAG.md](06_POLICY_LEGAL_RAG.md) |
| Trợ lý mua sắm (dùng search như một tool) | [05_SHOPPER_ASSISTANT_CUSTOMER_SUPPORT.md](05_SHOPPER_ASSISTANT_CUSTOMER_SUPPORT.md) |

## 1. Mục tiêu và vị trí trong hệ thống

- **Vai trò:** buyer (ô tìm kiếm, gợi ý truy vấn, "tìm theo mô tả"); seller (ít trực tiếp; được lợi từ chất lượng xếp hạng); admin (tuning).
- **Input:** chuỗi truy vấn tiếng Việt (có/không dấu, viết tắt, lỗi gõ, ví dụ "áo thun nam cotton dưới 200k"), bộ lọc (danh mục, giá, brand).
- **Output:** danh sách `product_id` xếp theo liên quan, có highlight/giải thích; cho phép "semantic" khi không khớp từ khoá.
- **Hiện trạng:** `products` đã có GIN index full-text `to_tsvector('simple', mm_unaccent(name‖brand‖sku‖description))` trong Postgres (product-service) — đây là baseline lexical tự nhiên. Bảng liên quan: `products(name, description, brand, tags JSONB, attributes JSONB, category_id, price, status, inventory)`, `categories`, và `dim_products` trong ClickHouse.
- **Dữ liệu hành vi nuôi mô hình:** `events_raw` với `event_type` search/click/impression, `props` (JSON chuỗi, có thể chứa query), `position`, `product_id` -> dùng làm nhãn ngầm (click, add_to_cart, order) cho learning-to-rank. Chưa thấy event "search" chuẩn hoá trong tài liệu data-model: cần xác nhận/ bổ sung khi triển khai.

## 2. Hướng làm đề xuất (baseline → nâng cao)

1. **Lexical tốt hơn:** Postgres FTS hiện có + `mm_unaccent`, đồng nghĩa, sửa chính tả; hoặc BM25 (OpenSearch/Tantivy) chạy cạnh.
2. **Dense retrieval:** embedding đa ngữ/tiếng Việt (BGE-M3, multilingual-e5, sentence-transformers fine-tune tiếng Việt) lưu trong pgvector hoặc vector store; chỉ lập chỉ mục `status=active`.
3. **Hybrid:** hợp nhất BM25 + vector bằng Reciprocal Rank Fusion hoặc cộng điểm có trọng số; lọc theo facet (giá, danh mục) trước/sau.
4. **Rerank:** cross-encoder tiếng Việt (ví dụ reranker fine-tune) trên top 50–100; thêm tín hiệu hành vi (CTR, bán chạy, tồn kho).
5. **Learning-to-rank** (LightGBM LambdaMART) với feature lexical + vector + CTR + giá; khử position bias bằng `position`.
6. **LLM hỗ trợ:** viết lại truy vấn, trích bộ lọc từ câu tự nhiên, sinh dữ liệu synthetic để fine-tune (chi tiết ở file 15).

## 3. Metric đánh giá

- Retrieval: **Recall@K (K=10/50/100), MRR@10, nDCG@10**, MAP; với nhãn nhiều mức (Exact/Substitute/...) dùng nDCG có gain khác nhau (ví dụ E=1.0, S=0.1, C=0.01, I=0 như ESCI).
- Quan hệ liên quan: F1/accuracy trên phân loại E/S/C/I; RMSE cho nhãn điểm (Home Depot).
- Hệ thống: latency p95, tỉ lệ "không có kết quả" (zero-result rate), CTR kết quả đầu, search-to-cart, search-to-order (online).
- Unbiased LTR: so sánh click-model vs. nhãn chuyên gia (kinh nghiệm Baidu-ULTR: cải thiện click prediction không chắc cải thiện ranking).

## 4. Bộ dữ liệu

### Bảng tổng hợp

| # | Bộ dữ liệu | Nhóm | Quy mô (theo nguồn) | Ngôn ngữ | License | ★ |
|---|---|---|---|---|---|---|
| 1 | [VN-MTEB (GreenNode + ĐH Quốc tế)](https://arxiv.org/pdf/2507.21500) | (a)/(c) | 41 dataset, 6 loại task | Việt | chưa xác minh | 4 |
| 2 | [ViRE (multi-domain Vietnamese IR)](https://github.com/longstnguyen/ViRE) | (a)/(c) | 10 dataset + 2 bonus, tối đa 1.000 cặp/dataset | Việt | code MIT; dữ liệu theo từng bộ (EduCoQA, CSConDa CC BY-NC 4.0) | 4 |
| 3 | [Vietnamese IR benchmark (PACLIC 2024)](https://aclanthology.org/2024.paclic-1.5) | (a)/(c) | chưa xác minh | Việt | chưa xác minh | 3 |
| 4 | [mMARCO (có tiếng Việt: chưa xác minh)](https://github.com/unicamp-dl/mMARCO) | (a)/(c) | MS MARCO 8,8M passage, dịch máy | 14 ngôn ngữ (Việt chưa xác minh) | Apache-2.0 (repo); gốc MS MARCO phi thương mại | 3 |
| 5 | [TVPL / Zalo Legal retrieval (Việt)](https://huggingface.co/datasets/GreenNode/TVPL-Retrieval) | (a)/(d) | TVPL: 165.334 query train, 10.000 test, 224.006 passage (theo paper) | Việt | chưa xác minh | 2 |
| 6 | [Mô hình/dữ liệu Việt: product-e5-small-vietnamese, hllj/synthetic-text-embedding](https://huggingface.co/trantuan1701/product-e5-small-vietnamese) | (a) | chưa xác minh | Việt | chưa xác minh | 3 |
| 7 | [Amazon ESCI Shopping Queries](https://github.com/amazon-science/esci-data) | (b) | 130.652 query, ~2,6M judgement (bản đầy đủ); 48.300 query/1.118.011 judgement (bản rút gọn) | Anh, Tây Ban Nha, Nhật | Apache-2.0 | 5 |
| 8 | [WANDS (Wayfair)](https://github.com/wayfair/WANDS) | (b) | 480 query, 42.994 sản phẩm, 233.448 judgement | Anh | MIT | 5 |
| 9 | [Home Depot Product Search Relevance (Kaggle)](https://nycdatascience.com/blog/student-works/improving-home-depot-search-relevance) | (b) | train 74.067 cặp, test ~166.693, 124.428 sản phẩm (đã xác minh, search) | Anh | điều khoản cuộc thi (chưa xác minh) | 3 |
| 10 | [Amazon-M2 (KDD Cup 2023)](https://arxiv.org/pdf/2307.09688) | (b)/(d) | hàng triệu session, 6 locale | Anh, Đức, Nhật, Pháp, Ý, Tây Ban Nha | Apache-2.0 (theo paper) | 2 |
| 11 | [Amazon Reviews 2023 (metadata)](https://huggingface.co/datasets/McAuley-Lab/Amazon-Reviews-2023) | (b) | chưa xác minh | Anh | chưa xác minh | 2 |
| 12 | [JDsearch (JD.com)](https://github.com/rucliujn/jdsearch) | (b) | ~173.831 user, 12,87M sản phẩm, ~26,7M tương tác, 171.728 query test | Trung (văn bản bị ẩn danh) | CC BY-NC-SA 4.0 (đã xác minh, GitHub) | 3 |
| 13 | [Baidu-ULTR](https://arxiv.org/pdf/2207.03051v2) | (b)/(c) | 1,2 tỷ session; 7.008 query có nhãn chuyên gia (theo paper) | Trung | chưa xác minh; README: hiện không tải công khai được do hạn chế pháp lý với Baidu, phải xin link riêng | 3 |
| 14 | [Shopee Price Match (Kaggle 2021)](https://github.com/jingxuanyang/Shopee-Product-Matching) | (d) | ~34K ảnh/tiêu đề, 11.014 nhóm | Chủ yếu Indonesia/Anh (chưa xác minh) | điều khoản cuộc thi (chưa xác minh) | 3 |
| 15 | [Marqo e-commerce evals (HF)](https://huggingface.co/datasets/Marqo/amazon-products-eval) | (b)/(d) | amazon-products-eval 1M–10M; google-shopping-general-eval 100K–1M | Anh | chưa xác minh | 2 |
| 16 | [MS MARCO / BEIR](https://microsoft.github.io/msmarco/) | (c) | 8,8M passage; 532.761 cặp train; BEIR 17–18 dataset | Anh | phi thương mại, nghiên cứu | 2 |
| 18 | [Shopping Queries Image Dataset (SQID)](https://github.com/Crossing-Minds/shopping-queries-image-dataset) | (b) | >190.000 sản phẩm (164.900 từ ESCI test US + 27.139 bổ sung); URL ảnh 156.545 sản phẩm; CLIP embedding | Anh | MIT (đã xác minh, GitHub); dữ liệu gốc ESCI Apache-2.0 | 4 |
| 19 | [TREC Product Search Track (2023–2025)](https://trec.nist.gov/data/product.html) | (c) | topics + qrels + corpus (số liệu chưa xác minh) | Anh | chưa xác minh | 3 |
| 17 | [KuaiSearch (arXiv 2026)](https://arxiv.org/pdf/2602.11518v1) | (c) | chưa xác minh | Trung (chưa xác minh) | chưa xác minh | 2 |

### (a) Tiếng Việt / Việt Nam

> Ghi chú: dữ liệu query–sản phẩm tiếng Việt: tự tạo (mục 5) và dùng các benchmark retrieval Việt chung dưới đây để kiểm tra encoder; tiếng Anh/đa ngôn ngữ (ESCI, WANDS, SQID...) dùng được để chọn phương pháp. (Scraper Apify cho Tiki.vn là công cụ bên thứ ba, không phải dataset; cần kiểm tra điều khoản của Tiki.)

**VN-MTEB** — arXiv 2507.21500 (EACL 2026 Findings); dữ liệu trên Hugging Face (theo bài báo; tên repo cụ thể **chưa xác minh**). 41 dataset, 6 loại task (gồm retrieval, rerank...), tạo bằng cách dịch tự động từ MTEB bằng LLM + embedding để lọc, giữ nguyên thực thể và code. Dùng để chọn mô hình embedding tiếng Việt (trước khi fine-tune). Ánh xạ: không có cột sản phẩm; chọn encoder cho `products.name/description`. ★4. Hạn chế: dịch máy (nhiễu), không phải domain TMĐT.

**ViRE** — https://github.com/longstnguyen/ViRE (đã đọc). Gồm EduCoQA, ViRHE4QA, CSConDa (CSKH), ALQAC, ZaloLegalQA, ViNewsQA, ViMedAQA, VlogQA, ViRe4MRC (review khách hàng), UIT-ViQuAD 2.0 (+ ViHERMES, VixSTORY bonus). Mỗi dataset tối đa 1.000 cặp question–context. Code MIT; EduCoQA, CSConDa CC BY-NC 4.0; các bộ khác giữ license gốc. Dùng để đo mô hình trên văn bản Việt "đời thường" (CSKH, review) — gần thực tế hơn dịch máy. ★4.

**Advancing Vietnamese Information Retrieval with Learning Objective and Benchmark** — arXiv 2503.07470 / ACL Anthology 2024.paclic-1.5. Benchmark retrieval tiếng Việt + objective huấn luyện; chi tiết dữ liệu và license **chưa xác minh**. ★3.

**mMARCO** — https://github.com/unicamp-dl/mMARCO (đã đọc); HF `unicamp-dl/mmarco`. MS MARCO dịch máy sang 14 ngôn ngữ (bao gồm tiếng Anh gốc) bằng Helsinki-NLP opus-mt (một số phiên bản dùng Google Translate theo model card thứ cấp). Đã xác minh (GitHub): 14 ngôn ngữ gồm tiếng Anh gốc, bản hiện hành là v2 (v1 ở README_old.md); README không liệt kê ngôn ngữ nên **chưa xác minh có tiếng Việt**; một model card cross-encoder liệt kê `vi`. License repo Apache-2.0. Dùng cho pretrain/fine-tune cross-encoder tiếng Việt. ★3. Hạn chế: passage Wikipedia/web, không phải sản phẩm; dịch máy.

**TVPL / Zalo Legal Text Retrieval** — bài arXiv 2412.00657 mô tả TVPL (165.334 query train, 10.000 test, 224.006 passage) và Zalo Legal 2021; HF `GreenNode/TVPL-Retrieval` được nhắc đến. Chỉ dùng để thử encoder tiếng Việt (thuộc file 06). ★2.

**Mô hình tham chiếu cho sản phẩm Việt:** `trantuan1701/product-e5-small-vietnamese` (HF) — model asymmetric query ngắn → mô tả dài; `hllj/synthetic-text-embedding` (dataset cặp short–long tổng hợp); `AITeamVN/Vietnamese_Reranker`. Chỉ ở mức gợi ý, cần tự đánh giá trên dữ liệu của mình. Dataset huấn luyện của model "product" **chưa xác minh**.

### (b) TMĐT công khai

**Amazon Shopping Queries (ESCI)** — https://github.com/amazon-science/esci-data (đã đọc); paper "Shopping Queries Dataset" (KDD Cup 2022; AICrowd). Bản rút gọn (Task 1): 48.300 query, 1.118.011 judgement; bản lớn (Task 2/3): 130.652 query, ~2,6M judgement (README ghi 2.621.738 trong văn bản và 2.621.288 trong bảng). Ngôn ngữ en/es/ja. Nhãn E/S/C/I. License Apache-2.0. Cột: `example_id, query, query_id, product_id, product_locale, esci_label, small_version, large_version, split`; bảng sản phẩm: `product_title, product_description, product_bullet_point, product_brand, product_color`. Ánh xạ: `product_title/description/brand`→`products.name/description/brand`; nhãn→ground truth rerank. ★5. Hạn chế: không có tiếng Việt; thị trường Amazon. Biến thể có ảnh: SQID (arXiv 2405.15190).

**WANDS** — https://github.com/wayfair/WANDS (đã đọc). `product.csv` 42.994 sản phẩm (product_name, product_class, category_hierarchy, product_description, product_features dạng `attr:value|...`, rating_count, average_rating, review_count), `query.csv` 480 query (query_class), `label.csv` 233.448 judgement với nhãn Exact/Partial/Irrelevant. License MIT. Ánh xạ: product_features→`products.attributes`, category_hierarchy→`categories`. ★5 vì nhãn chuyên gia sạch, nhỏ, chạy nhanh trên laptop. Hạn chế: đồ nội thất Mỹ, tiếng Anh, chỉ 480 query.

**Home Depot Product Search Relevance (Kaggle)** — cuộc thi 2016 (không mở được trang Kaggle; qua blog/paper). Nhãn relevance trung bình ≥3 người chấm, thang 1–3, hồi quy RMSE; đã xác minh (nguồn: WebSearch — towardsdatascience, NYC Data Science, báo cáo Stanford CS224D): train.csv 74.067 dòng, test ~166.693 (một nguồn ghi 166.694), 124.428 sản phẩm trong file mô tả. Điều khoản dữ liệu = luật cuộc thi (tải cần tài khoản Kaggle, chưa xác minh). ★3.

**Amazon-M2** — arXiv 2307.09688 (NeurIPS 2023 D&B); tải từ AIcrowd. Session mua sắm đa locale (6 locale; en, de, ja, fr, it, es), phục vụ gợi ý và sinh tiêu đề; Apache-2.0 theo phụ lục bài báo. Dùng cho query-less session; ít liên quan search → ★2.

**Amazon Reviews 2023** — https://huggingface.co/datasets/McAuley-Lab/Amazon-Reviews-2023 : metadata (mô tả, giá, ảnh, bought-together) giúp tạo cặp tổng hợp (tiêu đề ↔ mô tả). ★2; số liệu/license **chưa xác minh**.

**JDsearch** — https://github.com/rucliujn/jdsearch ; arXiv 2305.14810 (SIGIR 2023). Truy vấn thật + toàn bộ tương tác (click, cart, follow, purchase); README: 173.831 user, 12.872.636 item, 26.667.260 tương tác, 171.728 query test. Văn bản bị ẩn danh nên hạn chế cho mô hình ngữ nghĩa. Tải qua JD Cloud Disk (link có mật khẩu). License dữ liệu **CC BY-NC-SA 4.0**, repo Apache-2.0 (đã xác minh, GitHub). ★3 (cho personalized search/LTR).

**Baidu-ULTR** — arXiv 2207.03051 (NeurIPS 2022 D&B). 1,2 tỷ session tìm kiếm; train ~383M query; 7.008 query/367.262 doc có nhãn chuyên gia; có `position`, chiều cao hiển thị. Dùng để thử khử position bias; ánh xạ `events_raw.position`. Đã xác minh (GitHub README): hiện không tải công khai được do hạn chế pháp lý với Baidu, phải email tác giả xin link riêng; README không nêu license/kích thước. ★3. Hạn chế: tiếng Trung, search web (không TMĐT); nghiên cứu lặp lại (arXiv 2404.02543) cho thấy các kỹ thuật ULTR phổ biến không cải thiện ranking rõ rệt.

**Marqo e-commerce evals (HF)** — `Marqo/amazon-products-eval` (1M–10M dòng, ảnh+text), `Marqo/amazon-products-eval-100k`, `Marqo/google-shopping-general-eval`, `Marqo/marqo-general-ecommerce-eval`. Nguồn: Marqo (vendor), blog giới thiệu mô hình Marqo-Ecommerce-B/L; các số liệu là vendor-reported. Đã xác minh (nguồn: WebSearch): các repo `Marqo/marqo-general-ecommerce-eval` (nhóm 1M–10M, ảnh+text, có subset google_shopping) và `Marqo/google-shopping-general-eval` (100K–1M) tồn tại trên HF; không thấy license trong kết quả nên **license chưa xác minh**. ★2 (chủ yếu text-to-image).

### (c) Benchmark / paper học thuật

**MS MARCO / BEIR** — https://microsoft.github.io/msmarco/ ; arXiv 2104.08663 (BEIR). MS MARCO: 8,8M passage, 532.761 cặp train, dev 6.980 query; điều khoản chính thức: **phi thương mại, chỉ nghiên cứu** (một nguồn thứ cấp ghi MIT — mâu thuẫn, ưu tiên điều khoản chính thức). BEIR: 17–18 dataset zero-shot (tuỳ phiên bản). Dùng làm baseline nghiên cứu, không dùng để huấn luyện mô hình thương mại. ★2.

**KuaiSearch** — arXiv 2602.11518 (2026): bộ dữ liệu search TMĐT quy mô lớn cho recall/rank/relevance. Chỉ thấy tiêu đề và nhận xét rằng JDsearch ẩn danh văn bản; kích thước, ngôn ngữ, license, nơi tải **chưa xác minh**. ★2.

### (d) Cuộc thi / công ty

**Shopee — Price Match Guarantee (Kaggle 2021)** — ~34.251 ảnh JPEG, 11.014 lớp (nguồn thứ cấp); train.csv: `posting_id, image, image_phash, title, label_group`. Tiêu đề lẫn ngôn ngữ (Indonesia/Anh/Malay, chưa xác minh). Dùng cho entity matching/dedupe sản phẩm trùng. Điều khoản cuộc thi (chưa xác minh). ★3.

**Shopping Queries Image Dataset (SQID)** — https://github.com/Crossing-Minds/shopping-queries-image-dataset (đã đọc). Bổ sung cho ESCI: URL ảnh (156.545 sản phẩm, ~95% của 164.900 sản phẩm lõi thuộc ESCI test US small; 442 mục trỏ tới ảnh nền chung), CLIP embedding (ViT-L/14) cho ảnh, query và tiêu đề, cộng 27.139 sản phẩm bổ sung. Tổng >190.000 sản phẩm. License MIT. Dùng thử multimodal/hybrid text+ảnh trên nền nhãn ESCI. ★4. Hạn chế: chỉ locale US, ảnh là URL có thể hỏng.

**TREC Product Search Track (2023, 2024, 2025)** — https://trec.nist.gov/data/product2023.html ; https://trec.nist.gov/data/product.html . Đã xác minh (nguồn: WebSearch): NIST công bố topics, qrels và corpus (corpus trên GitHub của track) cho product search/discovery, mục tiêu benchmark công khai tái sử dụng. Số query/sản phẩm, nguồn gốc (có dựa trên ESCI hay không) và license: **chưa xác minh**. ★3.

## 5. Dữ liệu tự tạo / synthetic từ chính hệ thống

- **Tập query vàng:** trích top query từ `events_raw` (cần event search chuẩn hoá, hoặc `props.query`), chọn ~300–500 query phủ đầu/thân/đuôi; mỗi query lấy top-30 từ Postgres FTS + vector; **chấm nhãn bằng người (Exact/Partial/Irrelevant như WANDS)**; có thể dùng LLM làm judge sơ bộ rồi người duyệt mẫu (đo đồng thuận Cohen's kappa).
- **Nhãn ngầm từ hành vi:** positive = click/add_to_cart/order sau search (dùng `fact_order_items` cho đơn PAID/DELIVERED); negative = impression không click (có `position` để IPS). Chia **theo thời gian** (train các tuần cũ, valid/test tuần mới); không rò rỉ sản phẩm giữa train/test cho đánh giá cold-start.
- **Synthetic:** LLM sinh query từ `products.name/description/attributes` (câu mô tả, viết không dấu, viết tắt, lỗi chính tả, "dưới 200k") -> cặp (query, product) để fine-tune embedding/cross-encoder; hard negative từ cùng `category_id`/`brand`. Kiểm tra độ rò rỉ (query chứa nguyên tên sản phẩm).
- **Bộ test cấu trúc:** truy vấn có bộ lọc (giá, brand, danh mục) -> đáp án tính bằng SQL trên `products`, đúng 100%.
- Demo: `SEED_DEMO_DATA=true` chỉ để kiểm pipeline.

## 6. Lộ trình dùng dữ liệu đề xuất

1. **Bước 1:** WANDS + ESCI (small) để dựng harness đánh giá (nDCG@10), thử BM25 vs. dense vs. hybrid vs. rerank, chọn kiến trúc.
2. **Bước 2:** VN-MTEB + ViRE (+ PACLIC benchmark) để chọn/so sánh encoder và reranker tiếng Việt; mMARCO (nếu có bản `vi`) để fine-tune rerank.
3. **Bước 3:** tự dựng bộ vàng từ catalog thật (mục 5) – đây là thước đo chính cho go/no-go.
4. **Bước 4:** LTR với tín hiệu hành vi; thử khử position bias (Baidu-ULTR để kiểm quy trình, nhưng đừng kỳ vọng nhiều).
5. **Bước 5:** A/B online (CTR, search-to-cart, zero-result rate).

## 7. Rủi ro, license, thiên lệch, lưu ý pháp lý/PII

- Phi thương mại: MS MARCO, một số bộ ViRE (CC BY-NC), điều khoản cuộc thi Kaggle (Home Depot, Shopee). Thương mại được rõ: ESCI (Apache-2.0), WANDS (MIT) — vẫn cần giữ ghi công và kiểm lại LICENSE trước khi dùng.
- Không có dữ liệu TMĐT tiếng Việt công khai đủ lớn → rủi ro domain shift (tiếng Việt không dấu, tên thương hiệu, từ lóng); dịch máy làm sai sắc thái.
- Thiên lệch: click bias/position bias/popularity; nhãn chuyên gia theo văn hoá Mỹ (Wayfair).
- Query người dùng có thể chứa PII (số điện thoại, địa chỉ): lọc trước khi lưu/gửi cho LLM bên ngoài; chỉ dùng log khi `analytics_consent`.
- Scrape Tiki/Shopee: kiểm tra điều khoản sử dụng và pháp luật về dữ liệu; không dùng để huấn luyện/thương mại nếu bị cấm.

## 8. Nguồn tham khảo

Đã đọc trực tiếp:
- https://github.com/amazon-science/esci-data
- https://github.com/wayfair/WANDS
- https://github.com/unicamp-dl/mMARCO
- https://github.com/longstnguyen/ViRE

Qua kết quả tìm kiếm (chưa mở trực tiếp trang gốc):
- https://www.amazon.science/code-and-datasets/shopping-queries-dataset-a-large-scale-esci-benchmark-for-improving-product-search ; https://arxiv.org/pdf/2405.15190 (SQID)
- https://aboutwayfair.com/careers/tech-blog/wayfair-releases-wands-the-largest-and-richest-publicly-available-dataset-for-e-commerce-product-search-relevance
- https://nycdatascience.com/blog/student-works/improving-home-depot-search-relevance ; https://arxiv.org/pdf/2001.04980
- https://arxiv.org/pdf/2307.09688 (Amazon-M2)
- https://arxiv.org/pdf/2507.21500 (VN-MTEB)
- https://arxiv.org/pdf/2503.07470 ; https://aclanthology.org/2024.paclic-1.5
- https://arxiv.org/pdf/2412.00657 (TVPL, Zalo Legal)
- https://huggingface.co/trantuan1701/product-e5-small-vietnamese ; https://huggingface.co/AITeamVN/Vietnamese_Reranker
- https://github.com/rucliujn/jdsearch ; https://arxiv.org/abs/2305.14810
- https://arxiv.org/pdf/2207.03051v2 ; https://arxiv.org/pdf/2404.02543
- https://microsoft.github.io/msmarco/ ; https://arxiv.org/pdf/2104.08663
- https://arxiv.org/pdf/2602.11518v1 (KuaiSearch)
- https://github.com/jingxuanyang/Shopee-Product-Matching ; https://wandb.ai/andrada/shopee-kaggle/reports/I-Shopee-Competition-EDA-and-Preprocessing--Vmlldzo1NDEzNzU
- https://huggingface.co/datasets/Marqo/amazon-products-eval ; https://marqo.ai/blog/introducing-marqos-ecommerce-embedding-models

Chưa xác minh: license của VN-MTEB, TVPL, JDsearch, Baidu-ULTR, KuaiSearch, Marqo; có hay không cấu hình tiếng Việt trong mMARCO; số dòng của Home Depot.
