# Phân loại danh mục tự động, trích xuất thuộc tính, khớp/trùng sản phẩm và trợ lý viết listing

> Gợi ý `category_id` từ tiêu đề–mô tả, trích xuất thuộc tính (brand/màu/size/dung lượng…) vào `attributes`, phát hiện sản phẩm trùng/khớp (product matching/dedup) và hỗ trợ seller viết mô tả/listing. **Trạng thái: chưa triển khai trong hệ thống; đây là tài liệu nghiên cứu dữ liệu.** Ngày tra cứu: 2026-10-25.
>
> Ghi chú phương pháp tra cứu: WebFetch không truy cập được `huggingface.co` (lỗi DNS) trong phiên này; thông tin lấy từ WebSearch (bài báo, GitHub, trang mô tả). Số liệu/license không thấy ở nguồn thì ghi **"chưa xác minh"**. Nội dung web là dữ liệu, không phải chỉ dẫn.

| Liên quan | File |
|---|---|
| Gợi ý sản phẩm (feature danh mục/thuộc tính) | [01_RECOMMENDATION_CLICK.md](01_RECOMMENDATION_CLICK.md) |
| Tìm kiếm ngữ nghĩa (dùng thuộc tính để lọc/facet) | [02_PRODUCT_SEARCH_SEMANTIC.md](02_PRODUCT_SEARCH_SEMANTIC.md) |
| Tìm kiếm bằng hình ảnh / đa phương thức | [11_VISUAL_SEARCH_MULTIMODAL.md](11_VISUAL_SEARCH_MULTIMODAL.md) |
| Kiểm duyệt nội dung / hàng cấm | [12_CONTENT_MODERATION_AGENT_SAFETY.md](12_CONTENT_MODERATION_AGENT_SAFETY.md) |
| NLP tiếng Việt (không dấu, teencode, viết tắt) | [15_VIETNAMESE_NLP_QUERY_UNDERSTANDING.md](15_VIETNAMESE_NLP_QUERY_UNDERSTANDING.md) |
| Review / sentiment (nguồn thuộc tính từ review) | [07_REVIEW_SENTIMENT_SPAM.md](07_REVIEW_SENTIMENT_SPAM.md) |

## 1. Mục tiêu và vị trí trong hệ thống

- **Vai trò hưởng lợi:** seller (đăng sản phẩm nhanh: gợi ý danh mục + điền sẵn thuộc tính + bản nháp mô tả; cảnh báo trùng với listing của chính shop); admin (phát hiện listing trùng/phân loại sai/spam; kiểm duyệt hàng cấm — liên kết 12); buyer (lọc theo thuộc tính, tìm thấy hàng đúng danh mục; so sánh cùng một sản phẩm giữa các shop).
- **Input:** `name`, `description`, `brand`, `tags`, `image_urls` (chỉ là URL), giá; khi seller đang soạn nháp.
- **Output:** (a) top-k `category_id` + độ tin cậy; (b) `attributes{}` đề xuất (brand, màu, kích cỡ, dung lượng, chất liệu…) kèm span trong văn bản gốc; (c) danh sách sản phẩm có khả năng trùng (`product_id` + điểm); (d) mô tả/listing nháp. Seller luôn xác nhận; không tự ghi đè dữ liệu.
- **Dữ liệu của hệ thống nuôi nó** (xem `docs/design/02_DATA_MODEL.md`):
  - Postgres `products` (`name, description, category_id, brand, attributes JSONB, tags JSONB, image_urls JSONB, sku, price, status, weight_g`), `categories` (`id, parent_id, name, slug`; cây ≤ 3 cấp; seed mặc định 7 danh mục gốc + con) — danh mục là **nhãn** của bài toán phân loại; số lớp nhỏ nên bắt đầu đơn giản.
  - `domain_events` `product.changed` -> ClickHouse `dim_products` (`product_id, store_id, name, sku, category_id, brand, price_minor, status`) và `fact_order_items.category_id`.
  - `events_raw` (`search`, `view`, `click` kèm `product_id`, `props` chứa truy vấn nếu có) để học từ khoá–danh mục.
  - Ràng buộc: seller chỉ sửa sản phẩm của mình; `banned` chỉ admin; gợi ý phải tôn trọng quyền (identity từ JWT), và API phải có test trong `router_test.go` khi thêm route.

## 2. Hướng làm đề xuất (baseline → nâng cao)

1. **Danh mục:** TF-IDF (n-gram ký tự để chịu không dấu/viết tắt) + linear classifier; sau đó PhoBERT/XLM-R hoặc embedding đa ngữ + kNN; phân cấp (cha -> con) và ngưỡng "không chắc -> hỏi seller".
2. **Thuộc tính:** rule/regex + từ điển (dung lượng, màu, size VN: S/M/L, 39/40, ml/g) -> sequence tagging (BiLSTM-CRF/PhoBERT-CRF, kiểu OpenTag/AdaTag) -> LLM structured output (JSON schema theo từng danh mục) có kiểm tra giá trị trong nguồn (span grounding).
3. **Khớp sản phẩm/dedup:** blocking (brand + category + n-gram tiêu đề/embedding gần nhất) -> cặp ứng viên -> bộ chấm (cross-encoder/Ditto-style, hoặc LLM) kết hợp tiêu đề, thuộc tính, giá, ảnh (hash/embedding ảnh); xếp cụm bằng ngưỡng + connected components. Với ta: ưu tiên dedup nội bộ shop và chống sao chép giữa shop.
4. **Trợ lý listing:** sinh mô tả từ `name + attributes + ảnh`, ràng buộc fidelity (không bịa thông số), kiểm tra sau sinh (mọi giá trị số/thuộc tính trong mô tả phải có trong input); kiểm duyệt từ cấm (liên kết 12); seller phê duyệt.
5. **Vòng phản hồi:** việc seller sửa gợi ý -> nhãn mới (active learning).

## 3. Metric đánh giá

| Tác vụ | Metric |
|---|---|
| Phân loại danh mục | Accuracy@1/@3, macro-F1, F1 có trọng số (Rakuten dùng weighted P/R/F1), **hierarchical F1** (đúng cha nhưng sai con được tính một phần), tỉ lệ từ chối đúng |
| Trích xuất thuộc tính | Micro/Macro P/R/F1 ở mức (thuộc tính, giá trị), exact-match và partial-match span; F1 zero-shot cho thuộc tính chưa thấy (MAVE) |
| Khớp/dedup | Precision/Recall/F1 trên cặp, cluster-F1 (B-cubed) hoặc F1 theo `label_group` (Shopee), recall@k của blocking, tỉ lệ false merge |
| Sinh mô tả | Fidelity (tỉ lệ thuộc tính được nêu đúng/bịa), ROUGE/BERTScore (thứ yếu), đánh giá người (tự nhiên, hấp dẫn, đúng thông tin), tỉ lệ seller chấp nhận không sửa |
| Vận hành | Tỉ lệ seller bỏ qua gợi ý, thời gian đăng sản phẩm, độ trễ p95 |

## 4. Bộ dữ liệu

### Bảng tổng hợp

| # | Tên | Nhóm | Tác vụ | Quy mô (theo nguồn đã thấy) | License / truy cập | ★ |
|---|---|---|---|---|---|---|
| 1 | Rakuten Data Challenge (SIGIR eCom 2018) | b | Phân loại taxonomy theo tiêu đề | ~1 triệu tiêu đề (0,8 tr train/0,2 tr test), ~3.400 nhãn | cần đăng ký/điều khoản; chưa xác minh | 4 |
| 2 | Shopee Product Matching (Kaggle 2021) | b | Khớp sản phẩm (ảnh+tiêu đề), `label_group` | ~32,4 nghìn ảnh train; test ~70 nghìn (nguồn thứ cấp) | điều khoản cuộc thi; chưa xác minh | 5 |
| 3 | WDC Products / WDC LSPM (Web Data Commons) | b | Khớp sản phẩm | v2.0: 26 triệu offer/79 nghìn shop; gold 4.400 cặp; train 9K–214K cặp | Mannheim (kiểm tra điều khoản) | 5 |
| 4 | Abt-Buy, Amazon-Google | c | Entity matching sản phẩm | 9.575 và 11.460 cặp | chưa xác minh | 3 |
| 5 | MAVE (Google) | c | Trích xuất thuộc tính | 2,2 triệu sản phẩm Amazon, 3 triệu cặp attr-value, 1.257 danh mục | chưa xác minh (GitHub google-research-datasets) | 5 |
| 6 | OpenTag / AdaTag / AE-110k | c | Trích xuất thuộc tính | OpenTag: 10K SKU/13K instance (Amazon); AE-110k: ~50K SKU/110K instance (AliExpress, Sports & Entertainment) | chưa xác minh | 3 |
| 7 | Shopping Queries (ESCI) – KDD Cup 2022 | b | Query-product + metadata | ~130K query, 2,6 triệu nhãn; en/ja/es | repo Apache-2.0 (chỉ thấy ở fork; chưa xác minh gốc) | 3 |
| 8 | Amazon Reviews 2023 – metadata | b | Danh mục, tiêu đề, mô tả, bullet | 48 triệu item, 33 danh mục | CC BY 4.0 (arXiv) | 4 |
| 9 | Amazon Berkeley Objects (ABO) | b | Metadata đa ngữ + ảnh | 147.702 listing, 398.212 ảnh, ≤18 thuộc tính | CC BY-NC 4.0 (listing AWS) | 3 |
| 10 | Mercari Price Suggestion | b | Danh mục phân cấp + tiêu đề + mô tả | ~1,48 triệu dòng | điều khoản cuộc thi; chưa xác minh | 2 |
| 11 | Flipkart (reviews/ sản phẩm scrape) | b | Danh mục/review | ~194K–205K dòng review (bản Zenodo/Kaggle) | chưa xác minh; dữ liệu scrape | 1 |
| 12 | TaoDescribe, JDPDG, FPDG (Stick to Facts) | c | Sinh mô tả sản phẩm | TaoDescribe ~2,13 triệu mẫu; JDPDG >300.000 sản phẩm | chưa xác minh | 3 |
| 13 | Shopee Code League 2020 (Kaggle) | a/d | Cuộc thi nhiều bài | chưa xác minh bài product detection/title | điều khoản Shopee | 2 |
| 14 | VMLU (benchmark LLM tiếng Việt) | a/c | Đánh giá chung LLM tiếng Việt | Vi-MQA 10.880 câu | chưa xác minh | 1 |

### (a) Tiếng Việt / Việt Nam

Trong phiên này **không tìm thấy bộ dữ liệu phân loại danh mục / trích xuất thuộc tính / khớp sản phẩm / sinh mô tả tiếng Việt công khai** (kết quả chỉ có scraper thương mại trên Apify cho Tiki, bài thống kê thị trường Statista/LMC). Các dataset UIT về review (ViSFD, ViOCD, ViSpamReviews) có văn bản sản phẩm tiếng Việt nhưng nhãn không phải danh mục/thuộc tính — xem [07_REVIEW_SENTIMENT_SPAM.md](07_REVIEW_SENTIMENT_SPAM.md). Gợi ý: (i) crawl danh mục/thuộc tính các sàn là việc **pháp lý nhạy cảm** (điều khoản trang, cạnh tranh, bản quyền ảnh/mô tả) — chưa xác minh được trong phiên; (ii) tự gán nhãn bộ nhỏ từ catalog của chính ta (mục 5) và dịch/điều chỉnh các bộ nước ngoài.

**13. Shopee Code League 2020.** [Repo giải 12 phần sentiment](https://github.com/ilos-vigil/scl-2020-sentiment-analysis) cho biết dữ liệu thuộc điều khoản Shopee trên Kaggle. Tìm kiếm không xác nhận có dataset "product detection / title translation" công khai nào. ★2. (Tránh nhầm với Shopee Product Matching 2021, mục 2.)

### (b) TMĐT công khai

**1. Rakuten Data Challenge (SIGIR eCom 2018)**
- Link: [overview paper (CEUR Vol-2319)](https://ceur-ws.org/Vol-2319/ecom18DC_paper_13.pdf), [bản NUS](https://www.comp.nus.edu.sg/~skok/papers/ecomdc18.pdf), [Rakuten RIT](https://rit.rakuten.com/?p=333). Nguồn: Rakuten.com.
- ~1 triệu tiêu đề sản phẩm + đường dẫn danh mục ẩn danh (0,8 triệu train, 0,2 triệu test), ~3.400 nhãn, lớp mất cân bằng; 26 đội/28 hệ thống, tốt nhất weighted F1 = 0,8513; metric weighted P/R/F1. Ngôn ngữ en (chủ yếu). License/truy cập: **chưa xác minh**; theo mô tả cuộc thi, cần đăng ký.
- Cột: `title`, `category path` (ẩn danh, phân cấp ngăn bởi dấu >).
- Ánh xạ: `title` ↔ `products.name`; `category path` ↔ chuỗi `categories` (cha–con ≤ 3 cấp); lớp chỉ tiêu đề (không mô tả).
- ★4. Hạn chế: chỉ tiêu đề; nhãn ẩn danh; taxonomy khác; tiếng Anh.

**2. Shopee Product Matching (Shopee – Price Match Guarantee)**
- Link: tổng hợp [W&B EDA](https://wandb.ai/andrada/shopee-kaggle/reports/I-Shopee-Competition-EDA-and-Preprocessing--Vmlldzo1NDEzNzU), [Albumentations](https://albumentations.ai/adoption/competitions/kaggle/shopee-product-matching/), [giải pháp](https://github.com/jingxuanyang/Shopee-Product-Matching); trang Kaggle chính thức chưa mở.
- Cuộc thi 3–5/2021, giải 30.000 USD, 3.032 người thi/2.426 đội. Train ~32,4 nghìn ảnh sản phẩm (tổng chính xác 32.412 **chưa xác minh**); test ~70 nghìn (một nguồn). Mỗi dòng: `posting_id`, `image`, `image_phash`, `title`, `label_group` (mọi posting cùng sản phẩm chung nhãn). Đa dạng ngôn ngữ do Shopee Đông Nam Á (có tiêu đề Indonesia/Malaysia/Anh; tiếng Việt: chưa xác minh). Mục tiêu: nhóm cặp sản phẩm giống nhau từ ảnh + tiêu đề.
- Ánh xạ: `posting_id` ↔ `product_id`; `title` ↔ `name`; `image` ↔ `image_urls[0]`; `label_group` ↔ cụm trùng; `image_phash` ↔ băm ảnh.
- ★5 cho dedup đa phương thức. Hạn chế: test có nhóm chưa từng thấy; điều khoản dữ liệu chưa xác minh; ảnh nặng (không tải trong phiên).

**3. WDC Products / WDC LSPM**
- Link: [WDC Training and Test Sets v2.0 (Mannheim)](https://madata.bib.uni-mannheim.de/351/), [tin v2.0](https://www.uni-mannheim.de/dws/news/wdc-product-data-corpus-and-gold-standard-for-large-scale-product-matching-version-20-released/), [WDC Products paper (arXiv 2301.09521)](https://web3.arxiv.org/pdf/2301.09521v1), [Benchmark matching tasks](https://data.dws.informatik.uni-mannheim.de/benchmarkmatchingtasks/).
- Xây từ schema.org markup do Web Data Commons trích từ Common Crawl. LSPM: cặp match/non-match cho 4 nhóm (computers, cameras, watches, shoes), train 4 cỡ (2.000–70.000 cặp), test 1.100 cặp/nhóm đã duyệt tay, nhãn train từ mã định danh chung. Corpus v2.0: 26 triệu offer (16 triệu tiếng Anh) từ 79 nghìn e-shop; gold 4.400 cặp; train/val 9.000–214.000 cặp. WDC Products: benchmark với 3 trục (corner case, unseen entity, cỡ dev), có cả công thức pairwise và multi-class. License: chưa xác minh (xem trang madata).
- Ánh xạ: `title/description/brand/price` ↔ cột tương ứng của `products`; nhãn match ↔ quan hệ trùng.
- ★5. Hạn chế: chủ yếu tiếng Anh; nhóm sản phẩm hẹp; nhãn train suy ra từ GTIN/MPN nên có nhiễu.

**4. Abt-Buy, Amazon-Google** (entity matching chuẩn Magellan/DeepMatcher). Abt-Buy: 9.575 cặp, ~10,7% match; Amazon-Google: 11.460 cặp, ~10,2% match (chủ yếu phần mềm). F1 tham chiếu: Magellan 43,6/49,1; DeepMatcher 62,8/69,3; Ditto 75,58 trên Amazon-Google (số lệch theo thiết lập, có cảnh báo rò rỉ chọn mô hình). Tải: [Mannheim](https://data.dws.informatik.uni-mannheim.de/benchmarkmatchingtasks/), [Zenodo 8164151](https://zenodo.org/records/8164151). ★3. Hạn chế: nhỏ, cũ, tiếng Anh.

**7. Shopping Queries Dataset (ESCI)** — chi tiết ở [02_PRODUCT_SEARCH_SEMANTIC.md](02_PRODUCT_SEARCH_SEMANTIC.md): ~130K query, 2,6 triệu nhãn E/S/C/I, en/ja/es, kèm metadata sản phẩm (tiêu đề, brand, màu, bullet) — [arXiv 2206.06588](https://arxiv.org/abs/2206.06588v1), [SQID có ảnh arXiv 2405.15190](https://arxiv.org/pdf/2405.15190). Ở scope này: dùng trường `product_title, brand, color, bullet_point` làm input/đích cho trích xuất thuộc tính và cho sinh mô tả (một nghiên cứu dùng ESCI làm benchmark mô tả tiếng Anh). ★3. License: Apache-2.0 chỉ thấy ở fork; kiểm tra repo gốc `amazon-science/esci-data`.

**8. Amazon Reviews 2023 – metadata.** [arXiv 2403.03952](https://arxiv.org/html/2403.03952v1): 48 triệu item/33 danh mục với mô tả, giá, ảnh, danh mục phân cấp (`categories`), `details` (dạng dict thuộc tính), `bought_together`. Dùng: nguồn lớn nhất để huấn luyện/kiểm tra phân loại danh mục phân cấp và trích xuất thuộc tính yếu (từ `details`); ★4. Hạn chế: tiếng Anh, thiên về sản phẩm Mỹ; thuộc tính do seller nhập nên thiếu/sai.

**9. Amazon Berkeley Objects (ABO).** [arXiv 2110.06199](https://ar5iv.arxiv.org/html/2110.06199), [Amazon Science](https://www.amazon.science/blog/amazon-berkeley-release-dataset-of-product-images-and-metadata), [AWS Marketplace](https://aws.amazon.com/marketplace/pp/prodview-ygocdgcstr7v2). 147.702 listing, 398.212 ảnh catalog, ≤ 18 thuộc tính (đa ngữ: tiêu đề, brand, model, năm, loại sản phẩm, kích thước, chất liệu); có mô hình 3D cho 7.953 sản phẩm. License CC BY-NC 4.0 (theo listing AWS; phi thương mại). Dùng: ghép ảnh–thuộc tính (liên kết 11), thử trợ lý mô tả từ ảnh+metadata. ★3.

**10. Mercari Price Suggestion.** ~1,48 triệu dòng, cột: tên, tình trạng, category phân cấp (dạng `A/B/C`), brand, mô tả, giá (giá trong text bị thay bằng `[rm]`) ([torch_frame](https://pytorch-frame.readthedocs.io/en/stable/_modules/torch_frame/datasets/mercari.html)). Dùng: thử phân loại phân cấp từ tên+mô tả; ★2. Hạn chế: hàng cũ C2C, tiếng Anh.

**11. Flipkart.** Kết quả chỉ thấy tập review (khoảng 194.000–205.000 dòng, 104 loại sản phẩm; [Zenodo](https://zenodo.org/records/17612521)) và scraper; không xác nhận dataset sản phẩm kèm cây danh mục. ★1; dữ liệu scrape có rủi ro điều khoản.

### (c) Benchmark / bài báo học thuật

**5. MAVE**
- Link: [arXiv 2112.08663](https://arxiv.org/abs/2112.08663v1), [Google Research](https://research.google/pubs/pub50791), GitHub `google-research-datasets/MAVE`. WSDM 2022.
- 2,2 triệu sản phẩm Amazon (trang sản phẩm, không phải review), 3 triệu chú thích attribute-value, 1.257 danh mục; biểu diễn đa nguồn (tiêu đề, mô tả, bullet, thông số…); có tập test zero-shot cho thuộc tính chưa thấy; tác giả nói lớn gấp 8 lần bộ trước. License: chưa xác minh.
- Ánh xạ: sản phẩm ↔ `products`; attribute-value ↔ khoá/giá trị trong `attributes JSONB`; nguồn văn bản ↔ `name`, `description`.
- ★5. Hạn chế: tiếng Anh; nhãn nhiễu (tự động từ markup + xác minh); khác phân bố tiếng Việt.

**6. OpenTag / AdaTag / AE-110k**
- OpenTag ([arXiv 1806.01264](https://ar5iv.labs.arxiv.org/html/1806.01264), KDD 2018): BiLSTM-CRF + attention + active learning; dữ liệu 10K SKU/13K instance, 3 danh mục Amazon (tiêu đề, mô tả, highlights), chỉ tiếng Anh. AdaTag ([ACL 2021](https://paperswithcode.com/paper/adatag-multi-attribute-value-extraction-from)): giải mã thích ứng theo thuộc tính (hypernetwork + MoE), thử trên AliExpress. AE-110k: theo một bài 2025 ([KnowledgeNLP](https://preview.aclanthology.org/fix-sig-urls/2025.knowledgenlp-1.6.pdf)) chỉ danh mục Sports & Entertainment, ~50K SKU/110K instance, thuộc tính lấy từ tiêu đề, không gán tay — số liệu thứ cấp, chưa xác minh bài gốc (Xu et al. 2019). ★3. Hạn chế: nhỏ, tiếng Anh.

**12. Sinh mô tả sản phẩm.** [TaoDescribe/ KOBE (arXiv 1903.12457)](https://arxiv.org/pdf/1903.12457): 2.129.187 mẫu (thuộc tính, mô tả, khía cạnh, loại người dùng) từ Taobao, tiếng Trung, kèm tri thức CN-DBpedia; JDPDG (JD.com, >300.000 sản phẩm, mô tả do biên tập viên viết tham chiếu tiêu đề và thuộc tính, trung bình 9,5 khoá thuộc tính); FPDG [EMNLP 2019 D19-1501](https://aclanthology.org/D19-1501/) và [Stick to Facts 2503.08454](https://arxiv.org/pdf/2503.08454) nhấn mạnh fidelity với thuộc tính; không có benchmark tiếng Anh chuẩn, một nghiên cứu phải dựng từ ESCI. License/truy cập tải: chưa xác minh. ★3 (cho cách đo fidelity). Không có dataset mô tả sản phẩm tiếng Việt công khai trong phiên.

**14. VMLU.** [ACL 2025 VMLU Benchmarks](https://aclanthology.org/2025.acl-long.563): benchmark LLM tiếng Việt (Vi-MQA 10.880 câu 58 chủ đề, Vi-SQuAD, Vi-DROP, Vi-Dialog) — dùng để chọn LLM nền cho trợ lý viết mô tả, không liên quan trực tiếp tới dữ liệu sản phẩm. ★1.

### (d) Blog / cuộc thi công ty

- Rakuten (mục 1), Shopee (mục 2, 13), Mercari (mục 10), KDD Cup/Amazon (mục 7) là các cuộc thi công ty; Rakuten Institute of Technology có bài viết liên quan ([ví dụ seq2seq phân loại](https://rit.rakuten.com/publications/2018/unconstrained-product-categorization-with-sequence-to-sequence-models/)) và người thắng 2018 dùng chiến lược "balanced pooling views" (nguồn: thông cáo báo chí, độ tin cậy thấp).
- Không có blog Việt Nam (Tiki/Shopee VN/Lazada VN) về dữ liệu sản phẩm được xác nhận trong phiên.

## 5. Dữ liệu tự tạo / synthetic từ chính hệ thống

1. **Nhãn danh mục:** `products.category_id` + `categories` hiện có là nhãn "vàng yếu" (seller tự chọn, có thể sai); lọc bằng đồng thuận giữa mô hình và seller, rồi admin duyệt mẫu để có tập vàng ≥ 500 sản phẩm/danh mục lá lớn. Seed mặc định 7 danh mục gốc nên ban đầu phân loại ở cấp gốc/cấp 2.
2. **Thuộc tính:** bắt đầu từ `attributes JSONB` seller đã nhập (khoá/giá trị chuẩn hoá) làm nhãn từ xa (distant supervision) -> gióng hàng giá trị với `name/description` để tạo span (kiểu MAVE/OpenTag); gán tay 300–500 mẫu làm bộ vàng; tách riêng tập thuộc tính chưa thấy để đo zero-shot.
3. **Cặp trùng:** tạo cặp dương bằng (i) cùng `sku`/brand + tiêu đề gần giống; (ii) tăng cường tiêu đề (đảo từ, bỏ dấu, viết tắt, thêm từ khoá quảng cáo "HOT", "FREESHIP"); (iii) cùng ảnh pHash. Âm: cùng brand khác model (hard negative), cùng danh mục. Giữ nhãn do người xác nhận cho tập test.
4. **Nhiễu tiếng Việt thực tế:** sinh biến thể không dấu, teencode, lẫn tiếng Anh, lỗi chính tả, emoji (liên kết 15) để kiểm tra độ bền; bộ test riêng cho "tiêu đề spam từ khoá".
5. **Mô tả mẫu:** dùng LLM sinh nháp từ `name + attributes` rồi người sửa cho 200–300 sản phẩm làm tham chiếu; đo fidelity bằng kiểm tra tự động (mọi con số/thuộc tính trong output phải có trong input).
6. **Chia tập:** chia theo **thời gian tạo sản phẩm** (`created_at`) và **theo shop** (không để cùng shop ở train và test) để tránh rò rỉ phong cách; với matching, chia theo cụm (không để hai sản phẩm cùng cụm ở hai tập); giữ nhóm sản phẩm mới/danh mục hiếm riêng cho test.
7. **Quyền riêng tư/IP:** ảnh/mô tả của seller thuộc về seller — chỉ dùng để huấn luyện khi điều khoản cho phép (ghi trong điều khoản sàn).

## 6. Lộ trình dùng dữ liệu đề xuất

1. **Giai đoạn 0:** baseline phân loại phân cấp trên Amazon Reviews 2023 metadata/Rakuten (chọn kiến trúc, đo hierarchical F1); thử WDC/Abt-Buy cho matching; MAVE cho trích xuất thuộc tính.
2. **Giai đoạn 1:** lấy catalog của ta (demo rồi thật) làm bộ vàng nhỏ; fine-tune/few-shot cho tiếng Việt; tích hợp dạng "gợi ý" trong form đăng sản phẩm.
3. **Giai đoạn 2:** dedup nội bộ (blocking + cross-encoder); thêm ảnh khi có pipeline ảnh (Shopee Product Matching, liên kết 11).
4. **Giai đoạn 3:** trợ lý mô tả có fidelity check; vòng phản hồi từ chỉnh sửa của seller.
5. **Liên tục:** theo dõi tỉ lệ chấp nhận gợi ý; làm mới bộ vàng khi thêm danh mục.

## 7. Rủi ro, license, thiên lệch, lưu ý pháp lý/PII

- **License:** ABO (CC BY-NC), Olist-type NC; Rakuten cần đăng ký; Shopee/ Mercari là dữ liệu cuộc thi (điều khoản riêng); MAVE/WDC/ESCI: chưa xác minh trong phiên — kiểm tra trước khi dùng thương mại và trước khi phát hành mô hình huấn luyện từ chúng.
- **Bản quyền nội dung:** mô tả/ảnh sản phẩm của bên thứ ba (Tiki/Shopee…) là tài sản của seller/sàn; crawl có rủi ro pháp lý (điều khoản, cạnh tranh không lành mạnh).
- **Thiên lệch:** dữ liệu tiếng Anh/Mỹ-Trung; taxonomy khác Việt Nam; hàng "local brand", thời trang, thực phẩm Việt ít xuất hiện; trích xuất size/màu kiểu Việt ("M", "xanh rêu", "freesize") cần lexicon riêng.
- **Rủi ro LLM:** hallucination thông số (công suất, hạn dùng, công dụng y tế) -> **rủi ro quảng cáo sai sự thật**; bắt buộc fidelity check và cấm sinh claims y tế/hiệu quả (liên kết 12, 06).
- **Khớp sản phẩm sai:** false merge có thể gộp nhầm hai sản phẩm khác nhau, ảnh hưởng đơn hàng/tồn kho — chỉ gợi ý, không tự động gộp; luôn giữ `product_id` riêng.
- **Hàng cấm/giả:** phân loại tự động không thay kiểm duyệt; các sản phẩm `banned` chỉ admin đặt.
- **PII:** ít, nhưng mô tả seller có thể chứa SĐT/Zalo; che khi đưa vào tập huấn luyện.

## 8. Nguồn tham khảo (URL)

- Rakuten: https://ceur-ws.org/Vol-2319/ecom18DC_paper_13.pdf ; https://www.comp.nus.edu.sg/~skok/papers/ecomdc18.pdf ; https://rit.rakuten.com/?p=333 ; https://rit.rakuten.com/publications/2018/unconstrained-product-categorization-with-sequence-to-sequence-models/
- Shopee Product Matching: https://wandb.ai/andrada/shopee-kaggle/reports/I-Shopee-Competition-EDA-and-Preprocessing--Vmlldzo1NDEzNzU ; https://albumentations.ai/adoption/competitions/kaggle/shopee-product-matching/ ; https://github.com/jingxuanyang/Shopee-Product-Matching
- Shopee Code League: https://github.com/ilos-vigil/scl-2020-sentiment-analysis
- WDC: https://madata.bib.uni-mannheim.de/351/ ; https://www.uni-mannheim.de/dws/news/wdc-product-data-corpus-and-gold-standard-for-large-scale-product-matching-version-20-released/ ; https://web3.arxiv.org/pdf/2301.09521v1 ; https://data.dws.informatik.uni-mannheim.de/benchmarkmatchingtasks/ ; https://ml.paperswithcode.com/dataset/wdc-products
- Abt-Buy/Amazon-Google: https://arxiv.org/pdf/2004.00584 ; https://zenodo.org/records/8164151
- MAVE: https://arxiv.org/abs/2112.08663v1 ; https://research.google/pubs/pub50791
- OpenTag/AdaTag/AE-110k: https://ar5iv.labs.arxiv.org/html/1806.01264 ; https://paperswithcode.com/paper/adatag-multi-attribute-value-extraction-from ; https://preview.aclanthology.org/fix-sig-urls/2025.knowledgenlp-1.6.pdf
- ESCI/SQID: https://arxiv.org/abs/2206.06588v1 ; https://arxiv.org/pdf/2405.15190
- Amazon Reviews 2023: https://arxiv.org/html/2403.03952v1
- ABO: https://ar5iv.arxiv.org/html/2110.06199 ; https://www.amazon.science/blog/amazon-berkeley-release-dataset-of-product-images-and-metadata ; https://aws.amazon.com/marketplace/pp/prodview-ygocdgcstr7v2
- Mercari: https://pytorch-frame.readthedocs.io/en/stable/_modules/torch_frame/datasets/mercari.html
- Flipkart: https://zenodo.org/records/17612521
- Sinh mô tả: https://arxiv.org/pdf/1903.12457 ; https://aclanthology.org/D19-1501/ ; https://arxiv.org/pdf/2503.08454 ; https://arxiv.org/pdf/2408.12779
- VMLU: https://aclanthology.org/2025.acl-long.563
- Không tìm thấy/không xác minh trong phiên: dataset phân loại/thuộc tính/matching/mô tả sản phẩm tiếng Việt công khai (Tiki/Shopee/Lazada), Kaggle "Flipkart products" có cây danh mục, trang Kaggle gốc của Shopee Product Matching, tải trực tiếp của Rakuten và MAVE.
