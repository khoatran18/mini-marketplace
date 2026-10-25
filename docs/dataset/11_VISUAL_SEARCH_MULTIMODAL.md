# Tìm kiếm bằng hình ảnh và đa phương thức (Visual Search / Multimodal)

> Tìm sản phẩm tương tự theo ảnh, embedding ảnh+text dùng chung, kiểm tra ảnh listing (trùng lặp, sai danh mục). **Trạng thái: chưa triển khai trong hệ thống; đây là tài liệu nghiên cứu dữ liệu.** Ngày tra cứu: 2026-10-25.
>
> Ghi chú phương pháp: WebFetch không mở được Kaggle, HuggingFace, arXiv, Stanford, CUHK (lỗi DNS); GitHub và S3 mở được. Thông tin lấy từ WebFetch GitHub/S3 ("đã xác minh") và WebSearch ("theo search"). Dữ liệu tiếng Anh/quốc tế dùng được để chọn phương pháp; không yêu cầu ảnh sản phẩm Việt Nam. Số liệu nào chỉ đến từ snippet tìm kiếm được ghi rõ; chỗ không kiểm chứng ghi "chưa xác minh".
>
> Quy tắc ngôn ngữ (chủ dự án): chỉ giữ bộ có văn bản tiếng Anh/Việt hoặc ảnh không có văn bản quan trọng; bộ ngôn ngữ khác đã bị loại; bộ đa ngữ chỉ dùng phần tiếng Anh (và Việt nếu có). Dung lượng: lấy từ file này trước, nếu thiếu thì từ search (Kaggle/HF/arXiv không mở được nên chỉ dựa vào snippet); không có thì ghi "không tìm thấy". Cột "Dung lượng / ngôn ngữ" của bảng 4 ghi dung lượng và ngôn ngữ kèm nguồn.

| Liên quan | File |
|---|---|
| Tìm kiếm ngữ nghĩa văn bản | [02_PRODUCT_SEARCH_SEMANTIC.md](02_PRODUCT_SEARCH_SEMANTIC.md) |
| Phân loại/thuộc tính sản phẩm | [10_PRODUCT_CATEGORIZATION_ATTRIBUTES.md](10_PRODUCT_CATEGORIZATION_ATTRIBUTES.md) |
| Kiểm duyệt ảnh/listing | [12_CONTENT_MODERATION_AGENT_SAFETY.md](12_CONTENT_MODERATION_AGENT_SAFETY.md) |
| Gợi ý sản phẩm | [01_RECOMMENDATION_CLICK.md](01_RECOMMENDATION_CLICK.md) |

## 1. Mục tiêu và vị trí trong hệ thống

- **Buyer**: chụp/tải ảnh để tìm sản phẩm giống (visual search), "sản phẩm tương tự" trên trang chi tiết, tìm bằng ảnh + chỉnh sửa bằng lời ("áo này nhưng màu đỏ").
- **Seller**: cảnh báo khi ảnh listing trùng/na ná listing khác (sao chép, ăn cắp ảnh), ảnh mờ/không có sản phẩm, ảnh không khớp danh mục.
- **Admin**: phát hiện listing trùng (cùng sản phẩm đăng nhiều lần), hàng nhái nghi vấn (xem file 12).
- **Input**: ảnh truy vấn (người dùng tải lên) hoặc ảnh listing mới; tuỳ chọn text. **Output**: top-k `product_id` + điểm tương đồng; cờ trùng lặp/chất lượng ảnh.
- **Dữ liệu của hệ thống nuôi nó**: ảnh và tên/mô tả/danh mục sản phẩm trong product-service (catalog); `events_raw` trong ClickHouse (click vào kết quả tìm bằng ảnh = nhãn ngầm); `fact_order_items` (sản phẩm bán được, dùng làm trọng số/đánh giá). Vector lưu ở chỉ mục riêng (pgvector hoặc vector store; chưa có trong hệ thống).

## 2. Hướng làm đề xuất (baseline → nâng cao)

1. **Baseline**: CLIP/SigLIP (pretrained, zero-shot) lấy embedding ảnh; kNN cosine. Có thể kèm perceptual hash (pHash) để bắt ảnh trùng gần như tuyệt đối.
2. **Embedding ảnh+text chung**: nhúng cả tiêu đề tiếng Việt (mô hình đa ngữ, xem [15](15_VIETNAMESE_NLP_QUERY_UNDERSTANDING.md)) và ảnh vào cùng không gian; truy vấn hybrid (text + ảnh).
3. **Fine-tune theo miền**: contrastive/metric learning (triplet, ArcFace, Generalised Contrastive Learning) trên cặp ảnh cùng sản phẩm; hard-negative mining.
4. **Nâng cao**: crop theo detector (tách sản phẩm khỏi nền), composed retrieval (ảnh + chỉnh sửa lời, kiểu Fashion IQ), rerank bằng thuộc tính (màu, loại), phát hiện trùng bằng ngưỡng + ghép nhóm (graph clustering) như Shopee Product Matching.

## 3. Metric đánh giá

- Truy hồi: **Recall@K** (K=1,5,10), **mAP@R**, **NDCG@10** (với nhãn độ liên quan phân bậc như Marqo-GS-10M), MRR.
- Ghép sản phẩm trùng: **F1 theo nhóm** (cách chấm của Shopee Product Matching, theo mô tả tìm được: đánh giá nhóm sản phẩm khớp; chi tiết công thức chưa xác minh).
- Vận hành: p95 độ trễ truy vấn, kích thước chỉ mục, chi phí embedding mỗi ảnh.
- Online (khi triển khai): CTR kết quả tìm bằng ảnh, tỉ lệ thêm giỏ sau tìm ảnh (từ `events_raw`).

## 4. Bộ dữ liệu

### Bảng tổng hợp

| # | Bộ dữ liệu | Nhóm | Quy mô (theo nguồn đã tra) | Dung lượng / ngôn ngữ (nguồn) | License | Phù hợp |
|---|---|---|---|---|---|---|
| 1 | [Shopee Product Matching (Kaggle)](https://www.kaggle.com/competitions/shopee-product-matching) | b | ~34K ảnh train, 11K nhóm (xác minh qua search) | Đa ngữ: chỉ dùng phần tiêu đề tiếng Anh (số phần Anh: không tìm thấy). Dung lượng ảnh: không tìm thấy | Luật cuộc thi Kaggle (chưa xác minh) | ★★★★★ |
| 2 | [Amazon Berkeley Objects (ABO)](https://amazon-berkeley-objects.s3.amazonaws.com/index.html) | b | 147,702 sản phẩm, 398,212 ảnh | Metadata 83 MB; ảnh nhỏ 3 GB; ảnh gốc 110 GB (trang S3). Metadata đa ngữ: chỉ dùng thẻ tiếng Anh | CC BY 4.0 | ★★★★ |
| 3 | [Marqo-GS-10M](https://github.com/marqo-ai/GCL) | b | ~10M cặp query-sản phẩm, có ảnh | Tiếng Anh. Dung lượng: không tìm thấy (search không trả trang HF) | Repo Apache-2.0 (đã xác minh, README GitHub); license riêng của dữ liệu/ảnh chưa xác minh | ★★★★ |
| 4 | [Stanford Online Products (SOP)](https://arxiv.org/pdf/1511.06452) | b/c | 120,053 ảnh, 22,634 sản phẩm | Tải 2,87 GiB / giải nén 2,89 GiB (TFDS qua search). Ảnh, không có văn bản | Chưa xác minh (search không thấy license; trang gốc ai.stanford.edu/~sijia/Stanford-Online-Products/) | ★★★★ |
| 5 | [Products-10K (JD.com)](https://products-10k.github.io/) | b | 10,000 SKU, gần 150,000 ảnh (theo search: paper + v7labs/neurohive; ảnh shop + ảnh khách, nhãn kiểm ≥3 người, nhiễu <0.5%) | Ảnh JD.com, nhãn là SKU (không dùng văn bản). Dung lượng: không tìm thấy | Chưa xác minh; host products-10k.github.io | ★★★★ |
| 6 | [DeepFashion (Consumer-to-Shop)](https://mmlab.ie.cuhk.edu.hk/projects/DeepFashion/Consumer2ShopRetrieval.html) | c | >800K ảnh tổng, 50 danh mục, 1,000 thuộc tính, >300K cặp (paper CVPR 2016, theo search); C2S: 33,881 item, 239,557 ảnh | Ảnh + thuộc tính tiếng Anh. Dung lượng: không tìm thấy | Chỉ học thuật, cấm thương mại; phải ký thỏa thuận để lấy mật khẩu (theo search, trang UCF mirror) | ★★★ |
| 7 | [Street2Shop](https://openaccess.thecvf.com/content_iccv_2015/papers/Kiapour_Where_to_Buy_ICCV_2015_paper.pdf) | c | 404,683 ảnh shop, 20,357 ảnh street | Ảnh. Dung lượng: không tìm thấy | Chưa xác minh | ★★★ |
| 8 | [Fashion IQ](https://github.com/XiaoxiaoGuo/fashion-iq) | c | Chưa xác minh quy mô | Chú thích tiếng Anh. Số mẫu và dung lượng: không tìm thấy | CDLA (repo) | ★★★ |
| 9 | [Fashion-Gen](https://arxiv.org/pdf/1806.08317) | c | 293,008 ảnh | Mô tả tiếng Anh. Dung lượng: không tìm thấy | Chưa xác minh | ★★ |
| 10 | [H&M Personalized Fashion (ảnh)](https://www.kaggle.com/competitions/h-and-m-personalized-fashion-recommendations) | b | 105,542 sản phẩm có ảnh | Tiếng Anh. Dung lượng ảnh: không tìm thấy | Luật cuộc thi Kaggle | ★★★ |
| 11 | [LRVS-Fashion](https://arxiv.org/pdf/2306.02928) | c | Chưa xác minh quy mô | Ngôn ngữ chưa xác minh (nguồn LAION). Dung lượng: không tìm thấy | CC BY-NC 4.0 | ★★ |
| 12 | [LAION-5B / Re-LAION](https://laion.ai/blog/laion-5b/) | d | ~5B cặp ảnh-text (metadata) | Chỉ dùng phần LAION-2B-en: 2,32 tỉ cặp (paper qua search). Cả bộ: metadata ~2,65 TB, ảnh ~220 TB (slide LAION qua search); dung lượng riêng phần Anh: không tìm thấy | Metadata CC BY 4.0; ảnh thuộc chủ sở hữu | ★★ |
| 13 | [MS COCO captions](https://arxiv.org/pdf/1405.0312) | c | Chưa xác minh trong phiên này | Chú thích tiếng Anh. Dung lượng: không tìm thấy | Chưa xác minh | ★ |
| 14 | [Visual Product Search Benchmark (2026)](https://arxiv.org/pdf/2603.17186) | c | Chưa xác minh | Ngôn ngữ chưa xác minh. Dung lượng: không tìm thấy | Chưa xác minh | ★★ |
| 15 | [SQID (ESCI + ảnh)](https://github.com/Crossing-Minds/shopping-queries-image-dataset) | b | >190,000 sản phẩm; 164,900 product ID test + 27,139 bổ sung; ~156,545 URL ảnh | Kế thừa ESCI (đa ngữ): chỉ dùng phần tiếng Anh. Dung lượng ảnh: không tìm thấy | MIT (đã xác minh, README GitHub) | ★★★★ |
| 16 | [Amazon Reviews 2023 (ảnh + metadata)](https://huggingface.co/datasets/McAuley-Lab/Amazon-Reviews-2023) | b | >570M review, 48M sản phẩm, 33 danh mục (theo search) | Tiếng Anh. Dung lượng GB: không tìm thấy (thẻ HF chỉ ghi nhóm 10B–100B bản ghi) | CC BY 4.0 (theo arXiv, xem file 10) | ★★★ |

Ghi chú: chủ dự án xác nhận không cần ảnh sản phẩm Việt Nam; bộ quốc tế bên dưới đủ để chọn phương pháp, sau đó bổ sung bằng bộ ảnh nội bộ (mục 5).

### (a) Dữ liệu Việt Nam (ghi chú)
Không yêu cầu bộ ảnh Việt Nam. Dùng các bộ quốc tế bên dưới để validate phương pháp, rồi đánh giá trên ảnh nội bộ.

### (b) TMĐT / miền liên quan

**Shopee Product Matching (Kaggle "Shopee - Price Match Guarantee", 2021)**
- Link: https://www.kaggle.com/competitions/shopee-product-matching (không mở được trực tiếp; thông tin qua search)
- Nguồn: Shopee. Quy mô: ~34,251 ảnh JPEG, 11,014 nhóm (label_group) ở train; test ẩn (con số 70K chưa xác minh; một nguồn khác ghi ~32,4K file train — nguồn lệch, search 'xác nhận 11,014 nhóm' nhưng không chốt số ảnh). Ngôn ngữ: tiêu đề đa ngữ khu vực Đông Nam Á (chưa xác minh chi tiết) — **chỉ dùng phần tiêu đề tiếng Anh**, số mẫu phần Anh không tìm thấy; ảnh dùng cho embedding thị giác không bị ràng buộc ngôn ngữ.
- Cột: posting_id, image, image_phash, title, label_group. Dùng: validate phát hiện listing trùng và embedding ảnh+title; gần nhất với bài toán sàn VN. Hạn chế: dữ liệu Kaggle chỉ cho mục đích cuộc thi/học tập, phải kiểm tra điều khoản trước khi dùng thương mại; nhãn nhóm có nhiễu. ★5.

**Amazon Berkeley Objects (ABO)**
- Link: https://amazon-berkeley-objects.s3.amazonaws.com/index.html (đã đọc trang này)
- 147,702 sản phẩm có metadata (đa ngữ — chỉ dùng thẻ tiếng Anh), 398,212 ảnh catalog, >8,200 sản phẩm có 360 độ (72 ảnh mỗi sản phẩm), >7,900 mô hình 3D. Ảnh gốc 110 GB, ảnh thu nhỏ 3 GB, metadata 83 MB. License CC BY 4.0 (ghi công Amazon).
- Dùng: ảnh + tiêu đề + thuộc tính đa ngữ, thử embedding đa phương thức, nhiều góc nhìn. Hạn chế: thiên về đồ gia dụng/nội thất, ít thời trang. ★4.

**Marqo-GS-10M**
- Link: https://github.com/marqo-ai/GCL (đã đọc). Gồm 3 phần marqo_gs_full_10m, marqo_gs_fashion_5m, marqo_gs_wfash_1m; tải `marqo-gs-dataset.tar` (không gồm ảnh), `images_archive.tar` (ảnh đầy đủ), `images_wfash.tar` (mẫu thời trang nữ); mỗi split (in-domain, novel query, novel document, zero-shot) có CSV bộ ba kèm điểm liên quan phân bậc. README không nêu số query/sản phẩm; repo Apache-2.0, license dữ liệu không nêu riêng.
- Nội dung (theo paper qua search): query ↔ sản phẩm với điểm xếp hạng phân bậc, dựng bằng GPT-4 và Google Shopping; paper báo +29.3% NDCG@10 in-domain so với CLIP finetune. Repo Apache-2.0, nhưng license riêng của dữ liệu/ảnh chưa xác minh; trang HuggingFace dataset không mở được.
- Dùng: huấn luyện/đánh giá embedding query-ảnh có điểm phân bậc. Hạn chế: query tiếng Anh, nhãn do LLM sinh. ★4.

**Products-10K**
- Link: https://products-10k.github.io/ ; paper https://arxiv.org/pdf/2008.10545
- 10,000 SKU cấp độ sản phẩm từ JD.com, nhãn người chấm, xếp thành đồ thị phân cấp; thời trang, 3C, thực phẩm, y tế, gia dụng. Số ảnh >150K chỉ có ở một bài tóm tắt thứ cấp (chưa xác minh với paper). License chưa xác minh.
- Dùng: nhận dạng sản phẩm cấp SKU. Hạn chế: sản phẩm Trung Quốc. ★4.

**H&M Personalized Fashion Recommendations (Kaggle)**
- Link: https://www.kaggle.com/competitions/h-and-m-personalized-fashion-recommendations
- 105,542 sản phẩm có ảnh, ~1.37M khách, ~32M giao dịch (2018-09-20 đến 2020-09-22; con số 31M không tìm thấy nguồn). Dùng: ảnh + metadata để huấn luyện embedding ảnh và kết hợp với hành vi (xem [01](01_RECOMMENDATION_CLICK.md)). Hạn chế: một thương hiệu, chỉ thời trang; luật cuộc thi. ★3.

### (c) Benchmark / paper học thuật

**Stanford Online Products (SOP)** (số liệu đã xác nhận qua search: paper 1511.06452, TIB, Robust Calibrate Proxy Loss): 120,053 ảnh của 22,634 sản phẩm eBay (~5.3 ảnh/sản phẩm), train 59,551 ảnh/11,318 lớp, test 60,502 ảnh/11,316 lớp; 12 nhóm lớn. Paper: https://arxiv.org/pdf/1511.06452 . Chuẩn đo metric learning (Recall@K). Hạn chế: ảnh studio sạch, ít giống ảnh người bán VN. License chưa xác minh. ★4.

**DeepFashion (Consumer-to-Shop)**: 33,881 item, 239,557 ảnh, 195,540 cặp (con số cặp khác nhau giữa nguồn). Trang: https://mmlab.ie.cuhk.edu.hk/projects/DeepFashion/Consumer2ShopRetrieval.html . Chỉ dùng học thuật, cấm thương mại. Cảnh báo nhãn trùng: cùng sản phẩm bị gán nhiều ID. ★3.

**Street2Shop (Kiapour 2015, "Where to Buy It")**: 404,683 ảnh shop từ 25 retailer, 20,357 ảnh street, 39,479 cặp khớp. Paper: https://openaccess.thecvf.com/content_iccv_2015/papers/Kiapour_Where_to_Buy_ICCV_2015_paper.pdf . Tình trạng tải/license chưa xác minh. Dùng: ảnh người dùng chụp → ảnh shop. ★3.

**Fashion IQ**: truy hồi ảnh có chỉnh sửa bằng lời. Repo https://github.com/XiaoxiaoGuo/fashion-iq (CDLA; ảnh ở repo metadata riêng; tình trạng tải chưa xác minh). Dùng: composed retrieval. ★3.

**Fashion-Gen**: 293,008 ảnh (260,480 train; 32,528 val; 32,528 test), 1360x1360, mô tả của chuyên gia, 48 danh mục chính/121 mịn. Paper: https://arxiv.org/pdf/1806.08317 . Tình trạng phân phối chưa xác minh. ★2.

**LRVS-Fashion**: mở rộng visual search với chỉ dẫn; CC BY-NC 4.0 (thuộc tính từ LAION giữ CC BY 4.0). Paper: https://arxiv.org/pdf/2306.02928 . Chỉ phi thương mại. ★2.

**Visual Product Search Benchmark (2026)**: https://arxiv.org/pdf/2603.17186 ; xuất hiện trong kết quả, chưa đọc nội dung (chưa xác minh quy mô/license). ★2.

### (d) Blog / cuộc thi / tham chiếu
**LAION-5B**: metadata CC BY 4.0 (đa ngữ — **chỉ dùng phần LAION-2B-en, 2,32 tỉ cặp**), ảnh thuộc bản quyền chủ sở hữu; bị gỡ ngày 2023-12-20 do phát hiện 3,226 mục nghi CSAM; bản làm sạch Re-LAION-5B phát hành 2024-08-30. Dùng: chỉ tham chiếu tiền huấn luyện CLIP, không dùng trực tiếp. https://laion.ai/blog/laion-5b/ ★2.

**MS COCO**: chỉ tham chiếu tiền huấn luyện; kích thước/license chưa xác minh. ★1.

**SQID (Shopping Queries Image Dataset)**: https://github.com/Crossing-Minds/shopping-queries-image-dataset (đã đọc README). Bổ sung ảnh cho ESCI: "hơn 190,000 sản phẩm", 164,900 product ID test cộng 27,139 sản phẩm bổ sung, ~156,545 URL ảnh (~95%; 442 URL là ảnh placeholder video). Các file: `product_image_urls.csv`, `product_features.parquet`, `query_features.parquet`, `supp_product_image_urls.csv`. Repo MIT (ảnh thuộc chủ sở hữu gốc). Dùng: xếp hạng tìm kiếm đa phương thức (text+ảnh chung không gian CLIP), tiếng Anh/Nhật/Tây Ban Nha (**chỉ dùng phần tiếng Anh**). ★4.

**Amazon Reviews 2023 (ảnh + metadata)**: HF `McAuley-Lab/Amazon-Reviews-2023` (trang gốc không mở được; số liệu theo search qua hyper.ai): >570M review, 48M sản phẩm, 33 danh mục, cập nhật đến 9/2023, metadata có mô tả/giá/ảnh. Dùng: nguồn ảnh–tiêu đề lớn để thử embedding và lấy mẫu theo danh mục. ★3. License: xem file 10 (chưa xác minh trang gốc).

**Products-10K bổ sung**: theo search, tổ chức SKU thành đồ thị phân cấp, từng dùng cho challenge SKU-level ICPR 2020; có cả ảnh khách chụp nên gần ảnh "người dùng chụp" hơn SOP.


## 5. Dữ liệu tự tạo / synthetic từ chính hệ thống

- **Cặp dương từ listing**: nhiều ảnh của cùng một `product_id` (ảnh chính/phụ) là cặp dương; sản phẩm khác danh mục là âm dễ; cùng danh mục là âm khó.
- **Truy vấn giả lập**: augment ảnh (crop, xoay, đổi nền, chụp màn hình giả) để mô phỏng ảnh người dùng chụp.
- **Nhãn hành vi**: `events_raw` (click kết quả tìm kiếm, thêm giỏ) + `fact_order_items` (mua) làm nhãn yếu; đừng dùng mà chưa gỡ PII/ID người dùng.
- **Trùng listing**: tạo test bằng sao chép listing + đổi nhẹ ảnh/tiêu đề (đã biết đáp án), đo Recall của bộ phát hiện trùng.
- **Gold set**: 300-500 truy vấn ảnh thật do nhân viên gắn nhãn sản phẩm đúng (3 mức liên quan).
- Dùng `SEED_DEMO_DATA=true` cho demo; không bao giờ cho môi trường production.

## 6. Lộ trình dùng dữ liệu đề xuất

1. **Pilot (tuần 1-2)**: CLIP/SigLIP zero-shot, đánh giá Recall@K trên SOP và Shopee Product Matching để chọn mô hình/ngưỡng.
2. **Miền thương mại (tuần 3-4)**: thử Marqo-GS-10M (phần sampling nhỏ) và ABO để kiểm embedding đa phương thức; kiểm tra tiêu đề tiếng Việt bằng dữ liệu nội bộ.
3. **Nội bộ**: dựng gold set + cặp tự tạo (mục 5), fine-tune nhẹ, so sánh với baseline.
4. **Nâng cao**: Fashion IQ cho composed retrieval nếu ngành hàng thời trang quan trọng.
5. Chỉ dùng DeepFashion/LRVS (phi thương mại) cho thử nghiệm nội bộ, không đưa vào mô hình phát hành.

## 7. Rủi ro, license, thiên lệch, lưu ý pháp lý/PII

- **License**: nhiều bộ chỉ cho nghiên cứu (DeepFashion, LRVS CC BY-NC); Kaggle theo luật cuộc thi. Mô hình huấn luyện từ dữ liệu phi thương mại có thể bị hạn chế khi triển khai thương mại: cần tư vấn pháp lý.
- **Bản quyền ảnh**: ảnh LAION/Street2Shop thuộc chủ sở hữu gốc; ảnh seller trong hệ thống thuộc seller — cần điều khoản cho phép dùng để huấn luyện.
- **Thiên lệch**: dữ liệu phương Tây/Trung Quốc, ảnh studio sạch; sản phẩm VN (nền lộn xộn, ảnh chụp điện thoại, chữ tiếng Việt trên bao bì) chênh lệch miền.
- **PII**: ảnh người mặc (khuôn mặt), biển số, địa chỉ trên ảnh bưu kiện; làm mờ hoặc loại trước khi lưu; tuân thủ pháp luật bảo vệ dữ liệu cá nhân của Việt Nam.
- **An toàn dữ liệu**: LAION từng chứa nội dung bất hợp pháp; chỉ dùng bản đã làm sạch.

## 8. Nguồn tham khảo

- ABO: https://amazon-berkeley-objects.s3.amazonaws.com/index.html
- Marqo GCL: https://github.com/marqo-ai/GCL
- Fashion IQ: https://github.com/XiaoxiaoGuo/fashion-iq
- Shopee Product Matching (qua search): https://www.kaggle.com/competitions/shopee-product-matching ; ví dụ giải pháp https://github.com/jingxuanyang/Shopee-Product-Matching
- Products-10K: https://arxiv.org/pdf/2008.10545
- SOP: https://arxiv.org/pdf/1511.06452
- DeepFashion C2S: https://mmlab.ie.cuhk.edu.hk/projects/DeepFashion/Consumer2ShopRetrieval.html
- Street2Shop: https://openaccess.thecvf.com/content_iccv_2015/papers/Kiapour_Where_to_Buy_ICCV_2015_paper.pdf
- Fashion-Gen: https://arxiv.org/pdf/1806.08317
- LRVS-Fashion: https://arxiv.org/pdf/2306.02928
- LAION-5B: https://laion.ai/blog/laion-5b/ ; https://arxiv.org/pdf/2210.08402
- H&M (qua search): https://www.shaped.ai/blog/h-m-dataset-powering-personalized-fashion-recommendations-at-scale
- Visual Product Search Benchmark: https://arxiv.org/pdf/2603.17186 (chưa đọc nội dung)
- SQID: https://github.com/Crossing-Minds/shopping-queries-image-dataset
- Products-10K: https://products-10k.github.io (theo search)
- DeepFashion: https://mmlab.ie.cuhk.edu.hk/projects/DeepFashion.html (theo search)
- Amazon Reviews 2023: https://huggingface.co/datasets/McAuley-Lab/Amazon-Reviews-2023 (theo search)
