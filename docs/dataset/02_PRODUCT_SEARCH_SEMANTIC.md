# Tìm kiếm sản phẩm theo ngữ nghĩa / hybrid (BM25 + vector)

> Tìm sản phẩm bằng mô tả tự nhiên, từ khoá không dấu, đồng nghĩa; kết hợp BM25 và vector, rồi rerank theo mức độ liên quan query–sản phẩm. **Trạng thái: chưa triển khai trong hệ thống; đây là tài liệu nghiên cứu dữ liệu.** Ngày tra cứu: 2026-10-25.
>
> Ghi chú phương pháp: không mở được trực tiếp Kaggle/HuggingFace/arXiv (lỗi DNS của công cụ fetch); các trang GitHub (ESCI, WANDS, mMARCO, ViRE, SQID) được đọc trực tiếp, phần còn lại qua kết quả WebSearch. Điều gì không thấy trong nguồn được ghi **"chưa xác minh"**. Phạm vi: dữ liệu tiếng Anh/đa ngôn ngữ dùng được để chọn phương pháp (hybrid, rerank, LTR); dữ liệu tiếng Việt: tự tạo (mục 5) hoặc các benchmark retrieval Việt nêu ở mục (a). Theo quy tắc ngôn ngữ của dự án (chỉ giữ tiếng Anh/tiếng Việt), đã loại JDsearch và Baidu-ULTR (tiếng Trung), KuaiSearch (nền tảng Kuaishou, văn bản tiếng Trung suy ra từ nguồn), Shopee Price Match (tiêu đề chủ yếu tiếng Indonesia); bộ đa ngôn ngữ (ESCI, Amazon-M2, mMARCO) chỉ dùng phần tiếng Anh (và Việt nếu có). Cột "Quy mô / dung lượng" nêu số mẫu và dung lượng tải; "không tìm thấy" nghĩa là README/kết quả search không nêu con số (không suy đoán); Kaggle/HuggingFace/arXiv không mở trực tiếp được nên các số mới lấy từ snippet search.

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
- Unbiased LTR: so sánh click-model vs. nhãn chuyên gia (cải thiện click prediction không chắc cải thiện ranking).

## 4. Bộ dữ liệu

### Bảng tổng hợp

| # | Bộ dữ liệu | Nhóm | Quy mô / dung lượng | Ngôn ngữ | License | ★ |
|---|---|---|---|---|---|---|
| 1 | [VN-MTEB (GreenNode + ĐH Quốc tế)](https://arxiv.org/pdf/2507.21500) | (a)/(c) | 41 dataset, 6 loại task (paper); số mẫu tổng và dung lượng: không tìm thấy | Việt | chưa xác minh | 4 |
| 2 | [ViRE (multi-domain Vietnamese IR)](https://github.com/longstnguyen/ViRE) | (a)/(c) | 10 dataset + 2 bonus, tối đa 1.000 cặp/dataset (README GitHub), tức tối đa ~12.000 cặp (suy ra, ước tính); dung lượng: không tìm thấy (nhỏ) | Việt | code MIT; dữ liệu theo từng bộ (EduCoQA, CSConDa CC BY-NC 4.0) | 4 |
| 3 | [Vietnamese IR benchmark (PACLIC 2024)](https://aclanthology.org/2024.paclic-1.5) | (a)/(c) | số mẫu và dung lượng: không tìm thấy | Việt | chưa xác minh | 3 |
| 4 | [mMARCO (có tiếng Việt: chưa xác minh)](https://github.com/unicamp-dl/mMARCO) | (a)/(c) | MS MARCO gốc 8.841.823 passage (paper MS MARCO qua search), bản dịch giữ cùng quy mô mỗi ngôn ngữ; chỉ dùng phần Anh gốc và phần Việt (nếu có). Bản dịch Việt khác trên HF (5CD-AI/Vietnamese-msMARCO-ggtranslated): nhóm 10M–100M dòng (thẻ HF qua search). Dung lượng GB: không tìm thấy | Anh (gốc) + Việt (chưa xác minh trong mMARCO); bỏ các ngôn ngữ khác | Apache-2.0 (repo); gốc MS MARCO phi thương mại | 3 |
| 5 | [TVPL / Zalo Legal retrieval (Việt)](https://huggingface.co/datasets/GreenNode/TVPL-Retrieval) | (a)/(d) | TVPL: 165.334 query train, 10.000 test, 224.006 passage (paper); dung lượng: không tìm thấy | Việt | chưa xác minh | 2 |
| 6 | [Mô hình/dữ liệu Việt: product-e5-small-vietnamese, hllj/synthetic-text-embedding](https://huggingface.co/trantuan1701/product-e5-small-vietnamese) | (a) | số mẫu và dung lượng: không tìm thấy | Việt | chưa xác minh | 3 |
| 7 | [Amazon ESCI Shopping Queries](https://github.com/amazon-science/esci-data) | (b) | Toàn bộ: 130.652 query, ~2,6M judgement (bản rút gọn 48.300 query/1.118.011 judgement) (README GitHub). **Chỉ dùng phần tiếng Anh**: ~97K query (74,9K train + 13,5K val + 8,9K test) và ~1,2M sản phẩm (bản mirror HF qua search, chưa kiểm trực tiếp). Dung lượng: không tìm thấy | Anh (dùng); bỏ Tây Ban Nha, Nhật | Apache-2.0 | 5 |
| 8 | [WANDS (Wayfair)](https://github.com/wayfair/WANDS) | (b) | 480 query, 42.994 sản phẩm, 233.448 judgement (README GitHub); dung lượng MB: không tìm thấy (3 file CSV, clone trực tiếp từ git) | Anh | MIT | 5 |
| 9 | [Home Depot Product Search Relevance (Kaggle)](https://nycdatascience.com/blog/student-works/improving-home-depot-search-relevance) | (b) | train 74.067 cặp, test ~166.693, 124.428 sản phẩm (đã xác minh, search); dung lượng: không tìm thấy | Anh | điều khoản cuộc thi (chưa xác minh) | 3 |
| 10 | [Amazon-M2 (KDD Cup 2023)](https://arxiv.org/pdf/2307.09688) | (b)/(d) | 6 locale; **chỉ dùng phần tiếng Anh (UK)**. Toàn bộ: 3.606.249 session train, 361.659 test, 1.410.675 sản phẩm (paper NeurIPS qua search); riêng phần UK và dung lượng: không tìm thấy | Anh (UK, dùng); bỏ Đức, Nhật, Pháp, Ý, Tây Ban Nha | Apache-2.0 (theo paper) | 2 |
| 11 | [Amazon Reviews 2023 (metadata)](https://huggingface.co/datasets/McAuley-Lab/Amazon-Reviews-2023) | (b) | ~570M review, ~48M sản phẩm, 33 danh mục (thẻ HF qua search); dung lượng GB: không tìm thấy | Anh | chưa xác minh | 2 |
| 12 | [Marqo e-commerce evals (HF)](https://huggingface.co/datasets/Marqo/amazon-products-eval) | (b)/(d) | amazon-products-eval nhóm 1M–10M dòng; google-shopping-general-eval nhóm 100K–1M (thẻ HF qua search; số chính xác và dung lượng: không tìm thấy) | Anh | chưa xác minh | 2 |
| 13 | [MS MARCO / BEIR](https://microsoft.github.io/msmarco/) | (c) | 8.841.823 passage từ 3.563.535 tài liệu web (search); 532.761 cặp train; BEIR 17–18 dataset; dung lượng: không tìm thấy | Anh | phi thương mại, nghiên cứu | 2 |
| 14 | [Shopping Queries Image Dataset (SQID)](https://github.com/Crossing-Minds/shopping-queries-image-dataset) | (b) | >190.000 sản phẩm (164.900 từ ESCI test US + 27.139 bổ sung); URL ảnh 156.545 sản phẩm; CLIP embedding (README GitHub); dung lượng: không tìm thấy | Anh | MIT (đã xác minh, GitHub); dữ liệu gốc ESCI Apache-2.0 | 4 |
| 15 | [TREC Product Search Track (2023–2025)](https://trec.nist.gov/data/product.html) | (c) | topics + qrels + corpus; số liệu và dung lượng: không tìm thấy (chưa xác minh) | Anh | chưa xác minh | 3 |

### (a) Tiếng Việt / Việt Nam

> Ghi chú: dữ liệu query–sản phẩm tiếng Việt: tự tạo (mục 5) và dùng các benchmark retrieval Việt chung dưới đây để kiểm tra encoder; tiếng Anh/đa ngôn ngữ (ESCI, WANDS, SQID...) dùng được để chọn phương pháp. (Scraper Apify cho Tiki.vn là công cụ bên thứ ba, không phải dataset; cần kiểm tra điều khoản của Tiki.)

**VN-MTEB** — arXiv 2507.21500 (EACL 2026 Findings); dữ liệu trên Hugging Face (theo bài báo; tên repo cụ thể **chưa xác minh**). 41 dataset, 6 loại task (gồm retrieval, rerank...), tạo bằng cách dịch tự động từ MTEB bằng LLM + embedding để lọc, giữ nguyên thực thể và code. Dùng để chọn mô hình embedding tiếng Việt (trước khi fine-tune). Ánh xạ: không có cột sản phẩm; chọn encoder cho `products.name/description`. ★4. Hạn chế: dịch máy (nhiễu), không phải domain TMĐT.

**ViRE** — https://github.com/longstnguyen/ViRE (đã đọc). Gồm EduCoQA, ViRHE4QA, CSConDa (CSKH), ALQAC, ZaloLegalQA, ViNewsQA, ViMedAQA, VlogQA, ViRe4MRC (review khách hàng), UIT-ViQuAD 2.0 (+ ViHERMES, VixSTORY bonus). Mỗi dataset tối đa 1.000 cặp question–context. Code MIT; EduCoQA, CSConDa CC BY-NC 4.0; các bộ khác giữ license gốc. Dùng để đo mô hình trên văn bản Việt "đời thường" (CSKH, review) — gần thực tế hơn dịch máy. ★4.

**Advancing Vietnamese Information Retrieval with Learning Objective and Benchmark** — arXiv 2503.07470 / ACL Anthology 2024.paclic-1.5. Benchmark retrieval tiếng Việt + objective huấn luyện; chi tiết dữ liệu và license **chưa xác minh**. ★3.

**mMARCO** — https://github.com/unicamp-dl/mMARCO (đã đọc); HF `unicamp-dl/mmarco`. MS MARCO dịch máy sang 14 ngôn ngữ (bao gồm tiếng Anh gốc) bằng Helsinki-NLP opus-mt (một số phiên bản dùng Google Translate theo model card thứ cấp). Đã xác minh (GitHub): 14 ngôn ngữ gồm tiếng Anh gốc, bản hiện hành là v2 (v1 ở README_old.md); README không liệt kê ngôn ngữ nên **chưa xác minh có tiếng Việt**; một model card cross-encoder liệt kê `vi`. License repo Apache-2.0. Quy mô: MS MARCO gốc 8.841.823 passage; bản dịch giữ cùng quy mô mỗi ngôn ngữ; **chỉ dùng phần Anh gốc và Việt**, bỏ các ngôn ngữ khác; dung lượng GB: không tìm thấy. Dùng cho pretrain/fine-tune cross-encoder tiếng Việt. ★3. Hạn chế: passage Wikipedia/web, không phải sản phẩm; dịch máy.

**TVPL / Zalo Legal Text Retrieval** — bài arXiv 2412.00657 mô tả TVPL (165.334 query train, 10.000 test, 224.006 passage) và Zalo Legal 2021; HF `GreenNode/TVPL-Retrieval` được nhắc đến. Chỉ dùng để thử encoder tiếng Việt (thuộc file 06). ★2.

**Mô hình tham chiếu cho sản phẩm Việt:** `trantuan1701/product-e5-small-vietnamese` (HF) — model asymmetric query ngắn → mô tả dài; `hllj/synthetic-text-embedding` (dataset cặp short–long tổng hợp); `AITeamVN/Vietnamese_Reranker`. Chỉ ở mức gợi ý, cần tự đánh giá trên dữ liệu của mình. Dataset huấn luyện của model "product" **chưa xác minh**.

### (b) TMĐT công khai

**Amazon Shopping Queries (ESCI)** — https://github.com/amazon-science/esci-data (đã đọc); paper "Shopping Queries Dataset" (KDD Cup 2022; AICrowd). Bản rút gọn (Task 1): 48.300 query, 1.118.011 judgement; bản lớn (Task 2/3): 130.652 query, ~2,6M judgement (README ghi 2.621.738 trong văn bản và 2.621.288 trong bảng). Ngôn ngữ en/es/ja: **chỉ dùng phần tiếng Anh** (~97K query, ~1,2M sản phẩm theo bản mirror HF qua search; bỏ es, ja); dung lượng tải: không tìm thấy. Nhãn E/S/C/I. License Apache-2.0. Cột: `example_id, query, query_id, product_id, product_locale, esci_label, small_version, large_version, split`; bảng sản phẩm: `product_title, product_description, product_bullet_point, product_brand, product_color`. Ánh xạ: `product_title/description/brand`→`products.name/description/brand`; nhãn→ground truth rerank. ★5. Hạn chế: không có tiếng Việt; thị trường Amazon. Biến thể có ảnh: SQID (arXiv 2405.15190).

**WANDS** — https://github.com/wayfair/WANDS (đã đọc). `product.csv` 42.994 sản phẩm (product_name, product_class, category_hierarchy, product_description, product_features dạng `attr:value|...`, rating_count, average_rating, review_count), `query.csv` 480 query (query_class), `label.csv` 233.448 judgement với nhãn Exact/Partial/Irrelevant. License MIT. Ánh xạ: product_features→`products.attributes`, category_hierarchy→`categories`. ★5 vì nhãn chuyên gia sạch, nhỏ, chạy nhanh trên laptop. Hạn chế: đồ nội thất Mỹ, tiếng Anh, chỉ 480 query.

**Home Depot Product Search Relevance (Kaggle)** — cuộc thi 2016 (không mở được trang Kaggle; qua blog/paper). Nhãn relevance trung bình ≥3 người chấm, thang 1–3, hồi quy RMSE; đã xác minh (nguồn: WebSearch — towardsdatascience, NYC Data Science, báo cáo Stanford CS224D): train.csv 74.067 dòng, test ~166.693 (một nguồn ghi 166.694), 124.428 sản phẩm trong file mô tả. Điều khoản dữ liệu = luật cuộc thi (tải cần tài khoản Kaggle, chưa xác minh). ★3.

**Amazon-M2** — arXiv 2307.09688 (NeurIPS 2023 D&B); tải từ AIcrowd. Session mua sắm đa locale (6 locale; en, de, ja, fr, it, es), **chỉ dùng phần tiếng Anh (UK)**; toàn bộ 3.606.249 session train, 361.659 test, 1.410.675 sản phẩm (qua search), riêng phần UK và dung lượng: không tìm thấy; phục vụ gợi ý và sinh tiêu đề; Apache-2.0 theo phụ lục bài báo. Dùng cho query-less session; ít liên quan search → ★2.

**Amazon Reviews 2023** — https://huggingface.co/datasets/McAuley-Lab/Amazon-Reviews-2023 : metadata (mô tả, giá, ảnh, bought-together) giúp tạo cặp tổng hợp (tiêu đề ↔ mô tả). ★2; số liệu/license **chưa xác minh**.

**Marqo e-commerce evals (HF)** — `Marqo/amazon-products-eval` (1M–10M dòng, ảnh+text), `Marqo/amazon-products-eval-100k`, `Marqo/google-shopping-general-eval`, `Marqo/marqo-general-ecommerce-eval`. Nguồn: Marqo (vendor), blog giới thiệu mô hình Marqo-Ecommerce-B/L; các số liệu là vendor-reported. Đã xác minh (nguồn: WebSearch): các repo `Marqo/marqo-general-ecommerce-eval` (nhóm 1M–10M, ảnh+text, có subset google_shopping) và `Marqo/google-shopping-general-eval` (100K–1M) tồn tại trên HF; không thấy license trong kết quả nên **license chưa xác minh**. ★2 (chủ yếu text-to-image).

### (c) Benchmark / paper học thuật

**MS MARCO / BEIR** — https://microsoft.github.io/msmarco/ ; arXiv 2104.08663 (BEIR). MS MARCO: 8,8M passage, 532.761 cặp train, dev 6.980 query; điều khoản chính thức: **phi thương mại, chỉ nghiên cứu** (một nguồn thứ cấp ghi MIT — mâu thuẫn, ưu tiên điều khoản chính thức). BEIR: 17–18 dataset zero-shot (tuỳ phiên bản). Dùng làm baseline nghiên cứu, không dùng để huấn luyện mô hình thương mại. ★2.

### (d) Cuộc thi / công ty

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
4. **Bước 4:** LTR với tín hiệu hành vi; thử khử position bias bằng `position` trong `events_raw` (dữ liệu công khai phù hợp: Open Bandit, xem file 01).
5. **Bước 5:** A/B online (CTR, search-to-cart, zero-result rate).

## 7. Rủi ro, license, thiên lệch, lưu ý pháp lý/PII

- Phi thương mại: MS MARCO, một số bộ ViRE (CC BY-NC), điều khoản cuộc thi Kaggle (Home Depot). Thương mại được rõ: ESCI (Apache-2.0), WANDS (MIT) — vẫn cần giữ ghi công và kiểm lại LICENSE trước khi dùng.
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
- https://microsoft.github.io/msmarco/ ; https://arxiv.org/pdf/2104.08663
- https://huggingface.co/datasets/Marqo/amazon-products-eval ; https://marqo.ai/blog/introducing-marqos-ecommerce-embedding-models

Chưa xác minh: license của VN-MTEB, TVPL, Marqo; dung lượng tải của hầu hết bộ (search không trả về GB); có hay không cấu hình tiếng Việt trong mMARCO; số dòng của Home Depot.
