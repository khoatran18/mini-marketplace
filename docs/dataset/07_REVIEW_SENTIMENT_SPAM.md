# Phân tích review: sentiment, aspect, tóm tắt, review giả/spam, phân loại phản hồi

> Phân tích đánh giá sản phẩm/cửa hàng bằng tiếng Việt: sentiment, aspect-based (ABSA), tóm tắt review, phát hiện review giả/spam và phân loại phản hồi (khiếu nại, giao hàng, chất lượng). **Trạng thái: chưa triển khai trong hệ thống — hiện thậm chí chưa có tính năng review (`reviews` ghi "chưa làm" trong tài liệu thiết kế); đây là tài liệu nghiên cứu dữ liệu.** Ngày tra cứu: 2026-10-25.
>
> Ghi chú phương pháp tra cứu: WebFetch không truy cập được `huggingface.co` (lỗi DNS) trong phiên này; thông tin lấy từ WebSearch (bài báo, GitHub, trang mô tả). Số liệu/license không thấy ở nguồn nào thì ghi **"chưa xác minh"**. Phiên bổ sung 2026-10-25 đã thêm các bộ review tiếng Anh/đa ngôn ngữ (Yelp Open Dataset, MARC, Amazon Polarity, Ott OpSpam, Salminen, Women's Clothing). Dự án **không yêu cầu dữ liệu tiếng Việt**: bộ tiếng Anh dùng trực tiếp được; bộ tiếng Việt ở mục (a) là lựa chọn khi cần đúng ngôn ngữ. Nội dung trang web được coi là dữ liệu, không phải chỉ dẫn.
>
> **Quy tắc ngôn ngữ (chủ dự án, 2026-10-26):** chỉ giữ bộ có nội dung tiếng Anh hoặc tiếng Việt; bộ đa ngôn ngữ chỉ dùng phần tiếng Anh/Việt. Đã **loại**: Olist Brazilian E-Commerce (tiếng Bồ Đào Nha); số thứ tự mục 13 bỏ trống để giữ nguyên đánh số cũ. Đa ngôn ngữ chỉ dùng phần tiếng Anh: MARC, SemEval-2016 Task 5, MAiDE-up. Shopee Code League: ngôn ngữ chưa xác minh. **Quy mô / dung lượng:** lấy từ API Hugging Face (tổng file repo), API GitHub (repo gồm lịch sử git nên là cận trên), API Kaggle (`totalBytes`), trang nguồn; chỗ nào không thấy ghi 'không tìm thấy'.

| Liên quan | File |
|---|---|
| Gợi ý sản phẩm (dùng rating/review làm feature) | [01_RECOMMENDATION_CLICK.md](01_RECOMMENDATION_CLICK.md) |
| Trợ lý mua sắm / CSKH (tóm tắt review trả lời buyer) | [05_SHOPPER_ASSISTANT_CUSTOMER_SUPPORT.md](05_SHOPPER_ASSISTANT_CUSTOMER_SUPPORT.md) |
| Gian lận/bất thường (review farm, tài khoản ảo) | [08_FRAUD_ANOMALY_RISK.md](08_FRAUD_ANOMALY_RISK.md) |
| Kiểm duyệt nội dung | [12_CONTENT_MODERATION_AGENT_SAFETY.md](12_CONTENT_MODERATION_AGENT_SAFETY.md) |
| NLP tiếng Việt (tách từ, không dấu, teencode) | [15_VIETNAMESE_NLP_QUERY_UNDERSTANDING.md](15_VIETNAMESE_NLP_QUERY_UNDERSTANDING.md) |

## 1. Mục tiêu và vị trí trong hệ thống

- **Vai trò hưởng lợi:** seller (dashboard "khách khen/chê gì": aspect giao hàng, đóng gói, chất lượng; hộp thư khiếu nại cần xử lý); buyer (tóm tắt review trên trang sản phẩm, lọc "review đáng tin"); admin (hàng đợi review nghi giả/spam, theo dõi shop bị khiếu nại nhiều).
- **Input:** văn bản review (+ rating 1–5, `product_id`, `order_id`, `buyer_id`, thời điểm, có phải người đã mua hay không); tiếng Việt thực tế (không dấu, teencode, viết tắt, emoji, lẫn tiếng Anh).
- **Output:** (a) nhãn sentiment; (b) danh sách `(aspect, polarity, span)`; (c) tóm tắt theo sản phẩm/shop (ưu/nhược, kèm trích dẫn review); (d) `spam_score` + loại spam; (e) loại phản hồi (khiếu nại / hỏi đáp / khen) để định tuyến.
- **Dữ liệu của hệ thống nuôi nó** (xem `docs/design/02_DATA_MODEL.md`):
  - **Chưa có bảng review/rating** (`reviews`, `wishlists` ghi "chưa làm"; `/reviews` chưa có endpoint). Cần thiết kế mới (đề xuất: `reviews(id, order_item_id, product_id, store_id, buyer_id, rating, text, created_at, status, verified_purchase)` ở product- hoặc order-service + `domain_events` `review.created` qua outbox -> analytics). **Chưa tồn tại.**
  - Ràng buộc mua thật: `orders.status = DELIVERED`, `delivered_at`, `order_items` (chỉ cho review khi đơn DELIVERED — chặn một lớp review giả).
  - ClickHouse: `fact_order_items` (`product_id, category_id, store_id`), `fact_order_status` (thời điểm giao), `fact_refunds` (nhãn yếu: review tiêu cực + hoàn tiền), `events_raw` (`anonymous_id, session_id, ip_hash, device_type, ua_family` — tín hiệu hành vi cho phát hiện spam), `dim_products`.
  - Gateway: JWT là nguồn danh tính duy nhất; chặn review từ người không mua dựa trên đơn, không dựa trên body.

## 2. Hướng làm đề xuất (baseline → nâng cao)

1. **Sentiment baseline:** TF-IDF + logistic regression trên bộ UIT-VSFC/VLSP 2016; tính baseline từ rating (rating 4–5 = tích cực) làm nhãn yếu.
2. **Fine-tune PhoBERT / XLM-R / ViSoBERT** cho sentiment, phân loại khiếu nại (ViOCD), spam (ViSpamReviews); chuẩn hoá teencode/không dấu trước khi đưa vào (liên kết 15).
3. **ABSA:** span detection (BiLSTM-CRF / PhoBERT-CRF trên UIT-ViSD4SA) rồi phân loại polarity; gom theo aspect thành thống kê cho seller. Hướng LLM few-shot/structured output làm đối chứng.
4. **Tóm tắt review:** extractive (chọn câu) -> abstractive nhiều tài liệu (kiểu SPACE/FewSum/Opinosis); LLM có ràng buộc trích dẫn review nguồn, chấm bằng faithfulness.
5. **Spam/review giả:** đặc trưng văn bản (ViSpamReviews) + hành vi (thời điểm, trùng lặp, một tài khoản–nhiều shop, cụm IP/thiết bị từ `events_raw`) + đồ thị người dùng–sản phẩm (kiểu YelpChi); thêm lớp phát hiện review do LLM sinh (MAiDE-up, v.v.).
6. **Định tuyến phản hồi:** phân loại khiếu nại/giao hàng/sai mô tả -> hàng đợi seller; kết hợp rule với `fact_refunds`.

## 3. Metric đánh giá

| Tác vụ | Metric |
|---|---|
| Sentiment / topic | Accuracy, macro-F1, F1 từng lớp (lớp neutral rất ít, nên macro-F1) |
| ABSA | F1 aspect detection, F1 aspect+sentiment, F1 span (macro), exact/partial-match span |
| Tóm tắt | ROUGE-1/2/L, BERTScore, **faithfulness** (tỉ lệ mệnh đề có căn cứ trong review), coverage theo aspect, đánh giá người |
| Spam / review giả | Macro-F1, PR-AUC, recall@precision cố định (ví dụ 90%), tỉ lệ review bị ẩn nhầm (false positive cost cao) |
| Phân loại phản hồi | F1 lớp "khiếu nại" (recall ưu tiên), thời gian xử lý trung bình |
| Vận hành | Calibration (ECE), độ trôi phân bố theo tháng, latency |

## 4. Bộ dữ liệu

### Bảng tổng hợp

| # | Tên | Nhóm | Tác vụ | Quy mô (số mẫu) | Dung lượng tải/đĩa (nguồn) | License / truy cập | ★ |
|---|---|---|---|---|---|---|---|
| 1 | [UIT-ViSFD (smartphone feedback)](https://huggingface.co/datasets/visolex/ViSFD) | a | ABSA 10 aspect × 3 polarity | 11.122 comment | CSV 3,35 MB (HF API) | tự do cho nghiên cứu (license rõ: chưa xác minh) | 5 |
| 2 | [UIT-ViSD4SA](https://github.com/kimkim00/UIT-ViSD4SA) | a | Span detection ABSA | 11.122 comment, 35.396 span | repo ~1,5 MB (API GitHub, gồm lịch sử git) | tự do cho nghiên cứu (GitHub) | 5 |
| 3 | [ViSpamReviews (+ v2)](https://huggingface.co/datasets/visolex/ViSpamReviews) | a | Spam nhị phân + loại spam | 19.868 review (bản v1 theo snippet; chưa xác minh) | CSV 3,76 MB (HF API) | có bản CSV trên HF `visolex/ViSpamReviews`; license chưa xác minh | 5 |
| 4 | [UIT-ViOCD](https://huggingface.co/datasets/tarudesu/ViOCD) | a | Phát hiện khiếu nại | 5.485 review, 4 danh mục | 3 CSV tổng ~1,96 MB (HF API) | nghiên cứu; có trên HF `tarudesu/ViOCD` | 5 |
| 5 | [UIT-VSFC](https://github.com/kietnv/uit-vsfc) | a | Sentiment + topic (phản hồi sinh viên) | >16.000 câu | không tìm thấy (repo GitHub chỉ có hướng dẫn, dữ liệu nhận qua thỏa thuận) | phải ký thỏa thuận với nhóm UIT NLP | 3 |
| 6 | [VLSP 2016 (sentiment) / VLSP 2018 (ABSA khách sạn, nhà hàng)](https://vjst.vast.vn/jcc/article/view/13160) | a | Sentiment, ABSA | chưa xác minh | không tìm thấy | qua trang tài nguyên VLSP (đăng ký) | 3 |
| 7 | [UIT-VSMEC](https://ar5iv.labs.arxiv.org/html/1911.09339) | a | Cảm xúc 6 lớp (MXH) | 6.927 câu | không tìm thấy | trang corpus UIT (thỏa thuận) | 2 |
| 8 | [ViHSD / ViHOS / UIT-ViCTSD](https://github.com/sonlam1102/vihsd) | a | Hate/offensive, toxic (bình luận MXH) | ViHSD ~33K; ViHOS 11K comment/26K span; ViCTSD 10.000 | ViHSD: repo GitHub ~2 MB (API GitHub); ViHOS, ViCTSD: không tìm thấy | ViHSD: HF + yêu cầu email tác giả | 2 |
| 9 | [Review Tiki/Shopee/Sendo/Hasaki (bài 2024)](https://doaj.org/article/625f8f28cd8e4f1888076fc69bc97418) | a | ABSA | 10.021 review | không công bố | chưa công bố trong kết quả | 1 |
| 10 | [Shopee Code League 2020 – Sentiment Analysis](https://github.com/ilos-vigil/scl-2020-sentiment-analysis) | a | Sentiment (review Shopee, đa nước; **ngôn ngữ chưa xác minh**) | tăng ~10,2x khi thêm dữ liệu ngoài (số mẫu gốc: không tìm thấy) | không tìm thấy | thuộc điều khoản Shopee trên Kaggle | 2 |
| 11 | [Amazon Reviews 2023 (McAuley Lab)](https://amazon-reviews-2023.github.io/) | b | Rating, review, metadata | >570 triệu review, 48 triệu item, 33 danh mục | ~750 GB tổng file thô review + metadata trong repo HF `McAuley-Lab/Amazon-Reviews-2023` (HF API; theo từng danh mục, ví dụ Home_and_Kitchen ~31 GB) | CC BY 4.0 (theo bản arXiv); xem điều khoản | 4 |
| 12 | [Yelp (YelpChi, YelpNYC, YelpZip; Rayana & Akoglu)](https://odds.cs.stonybrook.edu/?p=562) | b | Review spam có nhãn filter | YelpChi 67.395 review (13,23% giả); NYC ~359K; Zip ~608K | không tìm thấy | gửi email tác giả (ODDS) | 4 |
| 14 | [Dữ liệu review LLM-sinh (MAiDE-up, Ghostwriters, Salminen GPT-2, FraudSquad)](https://arxiv.org/pdf/2404.12938v1) | c | Phát hiện review máy sinh | MAiDE-up: 10K thật + 10K giả trong 10 ngôn ngữ -> **chỉ dùng phần tiếng Anh, ước tính ~1.000 thật + ~1.000 giả** (chia đều, chưa xác minh); Salminen ~40.433 (20.238 CG / 20.216 OR) | Salminen: repo `joolsa/FakeReviews` ~2,3 MB (API GitHub); MAiDE-up: không tìm thấy | từng bộ; chưa xác minh | 4 |
| 15 | [SemEval-2016 Task 5 (ABSA)](https://alt.qcri.org/semeval2016/task5/) | c | ABSA (**chỉ dùng phần tiếng Anh**: nhà hàng, laptop, khách sạn) | số câu/review phần tiếng Anh: không tìm thấy | không tìm thấy | trang task; license chưa xác minh | 3 |
| 16 | [Opinosis / SPACE / FewSum (Yelp, Amazon)](https://kavita-ganesan.com/?p=59) | c | Tóm tắt review | Opinosis 51 chủ đề; SPACE 50 khách sạn có tóm tắt | không tìm thấy (nhỏ) | từng bộ; chưa xác minh | 3 |
| 17 | [Yelp Open Dataset](https://www.yelp.com/dataset) | b | Review, business, rating | 6.990.280 review, 150.346 business, 11 vùng đô thị, 200.100 ảnh | tar 4,35 GB (8,65 GB giải nén) (trang Yelp qua search) | mục đích giáo dục/nghiên cứu; điều khoản hiện hành chưa xác minh | 4 |
| 18 | [Multilingual Amazon Reviews Corpus (MARC)](https://docs.opendata.aws/amazon-reviews-ml/readme.html) | b | Rating 1–5 (**chỉ dùng phần tiếng Anh**) | tiếng Anh: 200K train + 5K dev + 5K test = 210.000 review, 2015–2019, cân bằng 20% mỗi sao | không tìm thấy (S3 từ chối truy cập, không liệt kê được) | chỉ học thuật theo điều khoản Amazon (AWS Open Data) | 4 |
| 19 | [Amazon Polarity (Zhang et al., bản HF)](https://www.tensorflow.org/datasets/community_catalog/huggingface/amazon_polarity) | b | Sentiment nhị phân | 3,6 triệu train + 0,4 triệu test; review đến 3/2013; bỏ sao 3 | ~1,15 GB parquet (HF API) | Apache-2.0 (nhãn của gói HF; nguồn gốc có thể có điều khoản khác) | 3 |
| 20 | [Ott et al. – Deceptive Opinion Spam (OpSpam)](https://ar5iv.arxiv.org/html/1107.4557) | c | Review giả viết thuê | 400 thật + 400 giả (hotel Chicago) bản gốc 2011; mở rộng đến 1.280 giả (hotel + nhà hàng) | không tìm thấy (rất nhỏ) | chưa xác minh | 3 |
| 21 | [Salminen et al. – Fake reviews (GPT-2)](https://github.com/joolsa/FakeReviews) | c | Review máy sinh vs thật | ~40.433 review, 10 danh mục Amazon | repo ~2,3 MB (API GitHub, gồm lịch sử git) | chưa xác minh | 4 |
| 22 | [Women's E-Commerce Clothing Reviews (Kaggle)](https://baselight.app/u/kaggle/dataset/nicapotato_womens_ecommerce_clothing_reviews) | b | Rating, recommend, text | 23.486 dòng, 10 cột; tên retailer đã thay | 8,5 MB (API Kaggle) | CC0 (API Kaggle) | 3 |
| 23 | [Amazon Fine Food Reviews (Kaggle)](https://snap.stanford.edu/data/web-FineFoods.html) | b | Rating + text | 568.454 review (đến 10/2012) | 673,7 MB (API Kaggle) | CC0 theo API Kaggle (các mirror khác ghi non-commercial) | 2 |

### (a) Tiếng Việt / Việt Nam

**1. UIT-ViSFD**
- Link: [arXiv 2105.15079](https://arxiv.org/abs/2105.15079v1); bản gộp trên HF [`visolex/ViSFD`](https://huggingface.co/datasets/visolex/ViSFD/blob/main/README.md); bản SEACrowd [`uit_visfd`](https://huggingface.co/datasets/SEACrowd/uit_visfd/blob/main/uit_visfd.py). Nguồn: nhóm UIT-NLP (ĐH CNTT, ĐHQG TP.HCM).
- 11.122 comment người dùng về điện thoại trên một sàn TMĐT Việt Nam (CSV 3,35 MB, HF API); 10 aspect × 3 polarity (pos/neu/neg). Baseline BiLSTM+fastText: F1 84,48% (aspect), 63,06% (sentiment). Ngôn ngữ vi. License: tự do cho nghiên cứu; SEACrowd ghi "unknown" -> **chưa xác minh**.
- Cột: `comment`, nhãn aspect#polarity (cấu trúc chính xác: chưa xác minh); bản HF có cột `type` chỉ split.
- Ánh xạ: dữ liệu huấn luyện/kiểm chứng ABSA cho review của ta (catalog điện thoại ~ `category_id` điện tử); thang aspect (giao hàng, đóng gói, pin, màn hình…) có thể thành `aspect` trong báo cáo seller.
- ★5. Hạn chế: chỉ ngành điện thoại; thiên về thông số kỹ thuật, ít hàng thời trang/tiêu dùng; nguồn gốc nền tảng gốc ẩn.

**2. UIT-ViSD4SA**
- Link: [arXiv 2110.07833](https://arxiv.org/abs/2110.07833v1), GitHub `kimkim00/UIT-ViSD4SA`. 11.122 comment, 35.396 span (repo GitHub ~1,5 MB, API GitHub); BiLSTM-CRF F1 macro 62,76%. Dùng: huấn luyện trích span khía cạnh (bước 3). ★5. Hạn chế: cùng miền điện thoại như ViSFD; license "tự do cho nghiên cứu".

**3. ViSpamReviews (và v2)**
- Link: [arXiv 2207.14636](https://arxiv.org/abs/2207.14636) (ACIIDS 2022), [v2 arXiv 2405.13292](https://arxiv.org/abs/2405.13292v2). Nhóm: Co Van Dinh, Son T. Luu, Anh Gia-Tuan Nguyen.
- Review thu từ các sàn TMĐT VN, 19.868 review (số theo snippet bản v1); hai tác vụ: spam hay không (nhị phân) và loại spam (đa lớp); gán nhãn theo khung MATTER. PhoBERT: 86,89% / 72,17% macro-F1. Bản v2 thêm metadata/mô tả sản phẩm: PhoBERT + mô tả sản phẩm đạt 87,22% macro-F1; kết luận chỉ comment là chưa đủ.
- Nơi tải/license: abstract trỏ tới link; có bản CSV 3,76 MB trên HF [`visolex/ViSpamReviews`](https://huggingface.co/datasets/visolex/ViSpamReviews/blob/main/ViSpamReviews.csv) (kết quả WebSearch; phân loại Text Classification, 10K–100K dòng). Số 19.868 và license của dataset: **chưa xác minh** (thẻ Apache-2.0 trong kết quả là của mô hình `visolex/bilstm-spam-binary`, không phải dữ liệu).
- Ánh xạ: nhãn spam cho mô hình trên `reviews.text` + ngữ cảnh sản phẩm (`products.description`, `name`). Loại spam có thể thành lý do trong hàng đợi admin.
- ★5. Hạn chế: nguồn sàn không công bố; thời điểm thu thập cũ; spam do người viết, chưa có LLM-sinh.

**4. UIT-ViOCD**
- Link: [arXiv 2104.11969](https://arxiv.org/pdf/2104.11969), HF [`tarudesu/ViOCD`](https://hf.co/datasets/tarudesu/ViOCD), SEACrowd `uit_viocd`. 5.485 review gán nhãn (3 file CSV tổng ~1,96 MB theo HF API; đồng thuận Fleiss' Kappa 87%), 4 danh mục; PhoBERT F1 92,16%. Tách: ~4.387 train / 548 dev / 549 test; 2.854 complaint vs 2.630 không. Chỉ dùng nghiên cứu. Ánh xạ: bộ phân loại "khiếu nại" định tuyến sang hộp thư seller và so với `REFUND_REQUESTED`/`fact_refunds`. ★5. Hạn chế: nhãn nhị phân; tổng số có sai lệch nhỏ giữa nguồn.

**5. UIT-VSFC**
- Link: [bài arXiv 1911.07223](https://arxiv.org/pdf/1911.07223), [GitHub kietnv/uit-vsfc](https://github.com/kietnv/uit-vsfc), [trang datasets của UIT NLP](https://nlp.uit.edu.vn/datasets). >16.000 phản hồi sinh viên 2013–2016; sentiment (tích cực 49,8%, tiêu cực 45,8%, trung tính 4,3%) và topic (giảng viên, chương trình, cơ sở vật chất, khác); đồng thuận >91% (sentiment). **Phải ký thỏa thuận sử dụng** trước khi nhận dữ liệu; dung lượng: không tìm thấy. ★3. Hạn chế: miền giáo dục, không phải TMĐT; NEU-ESC (arXiv 2506.23524) là bộ giáo dục mới hơn nhưng không liên quan đủ để liệt kê chi tiết.

**6. VLSP 2016 / 2018**
- Link: [bài tổng quan VLSP sentiment](https://vjst.vast.vn/jcc/article/view/13160), [PDF](https://vjs.ac.vn/index.php/jcc/article/download/13160/382797/389009). 2016: phân cực (pos/neg/neu) trên review sản phẩm điện tử; 2018: ABSA nhà hàng và khách sạn, nhãn `entity#attribute` + polarity, không có opinion target. Dữ liệu qua `vlsp.org.vn/resources` (đăng ký). Kích thước và dung lượng: **chưa xác minh / không tìm thấy**. ★3. Dùng thêm dữ liệu để kiểm tra khái quát hoá sang miền khác.

**7. UIT-VSMEC.** [arXiv 1911.09339](https://ar5iv.labs.arxiv.org/html/1911.09339): 6.927 câu 6 cảm xúc (sadness, enjoyment, anger, disgust, fear, surprise); CNN F1 59,74%; lớp lệch; dung lượng: không tìm thấy. Dùng: cảm xúc để tinh chỉnh tông giọng trả lời CSKH. ★2.

**8. ViHSD / ViHOS / UIT-ViCTSD.** [ViHOS arXiv 2301.10186](https://arxiv.org/pdf/2301.10186), [ViHSD GitHub](https://Github.Com/sonlam1102/vihsd), HF `sonlam1102/vihsd`. ViHSD ~33K comment (CLEAN 27.624 / HATE 3.514 / OFFENSIVE 2.262 theo bài ViHOS); ViHOS 11K comment/26K span; UIT-ViCTSD 10.000 comment (constructive/toxic). Dùng: lọc review độc hại (liên kết 12). Lấy ViHSD cần email tác giả (GitHub); dung lượng ViHSD: repo ~2 MB (API GitHub), ViHOS/ViCTSD: không tìm thấy. ★2 cho scope này.

**9. Dữ liệu review Tiki/Shopee/Sendo/Hasaki.** Một bài 2024 (IJIMDI) dùng 10.021 review 2015–2023, BERT + Bi-GRU >70% accuracy ([DOAJ](https://doaj.org/article/625f8f28cd8e4f1888076fc69bc97418)). Kết quả không cho thấy dữ liệu được công bố (dung lượng: không có). ★1; không thấy dataset Tiki/Shopee/Lazada công khai nào trên Kaggle/HF trong phiên (các dataset scrape tồn tại nhưng chưa xác minh được). Một bộ Lipstick ABSA (16.227 review) cũng được nhắc nhưng chưa xác minh nơi tải.

**10. Shopee Code League 2020 – Sentiment.** [Repo giải 12](https://github.com/ilos-vigil/scl-2020-sentiment-analysis): dữ liệu và dữ liệu sinh ra thuộc Điều khoản của Shopee (trên Kaggle); tác giả bổ sung dữ liệu ngoài tăng ~10,2 lần. Review của Shopee đa nước (có tiếng Anh/Indo…); độ phù hợp tiếng Việt và **ngôn ngữ: chưa xác minh**; số mẫu và dung lượng: không tìm thấy. ★2.

### (b) TMĐT công khai nước ngoài

**11. Amazon Reviews 2023.** Hou et al., [arXiv 2403.03952](https://arxiv.org/html/2403.03952v1) (CC BY 4.0 theo trang arXiv); trang dữ liệu amazon-reviews-2023.github.io, mã `hyp1231/AmazonReviews2023`; nhiều bản sao HF lỗi loading script — dùng nguồn chính thức. (~750 GB tổng file thô trong repo HF theo API) >570 triệu review, 48 triệu item, 33 danh mục, có rating/văn bản/helpful vote/ảnh/metadata/đồ thị. Dùng: pretrain/đối chứng sentiment, tóm tắt, helpful-vote -> nhãn chất lượng review; metadata cho scope 10. ★4. Hạn chế: tiếng Anh; mốc 9/2023; có review khuyến mãi.

**12. Yelp spam sets (YelpChi, YelpNYC, YelpZip).** Rayana & Akoglu, KDD 2015 — mô tả tại [ODDS Stony Brook](https://odds.cs.stonybrook.edu/?p=562). Nhãn lấy từ bộ lọc của Yelp (filtered = giả, recommended = thật) nên là "gần sự thật"; YelpChi 67.395 review (13,23% giả); YelpNYC ~359.052 (10,27%); YelpZip ~608.598 (13,22%). Truy cập: email tác giả; dung lượng: không tìm thấy. Yelp Open Dataset gốc: license chưa xác minh. Dùng: thử mô hình đồ thị + hành vi người dùng–sản phẩm. ★4. Hạn chế: nhãn nhiễu; nhà hàng; tiếng Anh. Ott et al. (OpSpam): xem mục 20.

**17. Yelp Open Dataset.** [trang Yelp](https://www.yelp.com/dataset). Đã xác minh qua WebSearch: 6.990.280 review, 150.346 business, 11 vùng đô thị, 200.100 ảnh; tar 4,35 GB, giải nén 8,65 GB gồm 1 PDF + 5 file JSON; mô tả là bộ con dành cho mục đích giáo dục/học thuật (bài cũ nói thêm có cuộc thi dataset challenge chỉ cho sinh viên). Một bài arXiv ghi 8.021.122 review/209.393 business — đó là phiên bản cũ. License hiện hành: **chưa xác minh** (đọc file license trong tar). Dùng: sentiment, ABSA, nền cho phát hiện spam (khác với YelpChi/NYC/Zip đã gán nhãn filter ở mục 12). ★4. Hạn chế: nhà hàng/dịch vụ, tiếng Anh, không có nhãn spam.

**18. Multilingual Amazon Reviews Corpus (MARC).** [arXiv 2010.02573](https://arxiv.org/pdf/2010.02573), [AWS Open Data](https://docs.opendata.aws/amazon-reviews-ml/readme.html) (Keung, Lu, Szarvas, Smith, EMNLP 2020). Đã xác minh qua WebSearch: 6 ngôn ngữ, mỗi ngôn ngữ 200.000 train + 5.000 dev + 5.000 test; **theo quy tắc ngôn ngữ chỉ dùng phần tiếng Anh = 210.000 review** (bỏ 5 ngôn ngữ còn lại); dung lượng: không tìm thấy; thu thập 2015–2019, cân bằng 20% mỗi mức sao, tối đa 20 review/người và 20 review/sản phẩm. Điều khoản: dùng cho mục đích học thuật/phi thương mại theo Conditions of Use của Amazon (đọc README để xác nhận trước khi dùng rộng hơn). Dùng: huấn luyện/kiểm thử sentiment-rating đa ngôn ngữ, thử chuyển giao zero-shot sang ngôn ngữ khác. ★4. Hạn chế: không có tiếng Việt (chỉ phần tiếng Anh dùng được); phi thương mại.

**19. Amazon Polarity.** [Bản HF (qua kết quả TFDS community catalog)](https://www.tensorflow.org/datasets/community_catalog/huggingface/amazon_polarity). Đã xác minh qua WebSearch: 3,6 triệu train + 0,4 triệu test (~1,15 GB parquet, HF API); nhãn nhị phân (sao 1–2 âm, 4–5 dương, bỏ sao 3); ~35 triệu review đến 3/2013 (Xiang Zhang); gói HF ghi Apache-2.0, nhưng văn bản gốc đến từ McAuley & Leskovec nên điều khoản có thể khác. Dùng: baseline sentiment nhanh, pretrain. ★3. Hạn chế: cũ (2013), nhãn nhị phân.

**20. Ott et al. (OpSpam).** [Ott, Choi, Cardie, Hancock, ACL 2011](https://ar5iv.arxiv.org/html/1107.4557). Đã xác minh qua WebSearch: 400 review giả (crowd viết cho 20 hotel Chicago) đối chiếu 400 review thật (tổng 800); người đánh giá không hơn ngẫu nhiên, bộ phân loại ~90% accuracy; luận án của Ott mở rộng lên 1.280 review giả (hotel + nhà hàng). Trang tải/license: **chưa xác minh**. Dùng: baseline review giả "viết thuê" (n-gram, LIWC). ★3. Hạn chế: nhỏ, miền hotel, nhãn giả do crowd nên khác review spam thực.

**22–23. Kaggle review thuần (Women's Clothing, Fine Food).** Women's E-Commerce Clothing Reviews: 23.486 dòng, 10 cột, 8,5 MB, CC0 (API Kaggle), tên retailer được thay bằng "retailer". Amazon Fine Food Reviews: 568.454 review đến 10/2012; 673,7 MB, license CC0 theo API Kaggle (mirror hyper.ai ghi non-commercial, mâu thuẫn; nguồn gốc SNAP/Stanford cần đọc). Dùng: thử nhanh sentiment/recommend, ABSA mức thô. ★3 và ★2.

### (c) Benchmark / bài báo học thuật

**14. Review do máy sinh.** [MAiDE-up (arXiv 2404.12938)](https://arxiv.org/pdf/2404.12938v1): 10.000 review thật + 10.000 giả của khách sạn, 10 ngôn ngữ cân bằng (**chỉ dùng phần tiếng Anh, ước tính ~1.000 thật + ~1.000 giả**, suy từ chia đều, chưa xác minh; mã trên GitHub); [Ghostwriters of the Marketplace (ICWSM 2026)](https://ojs.aaai.org/index.php/ICWSM/article/view/42791): review Google do máy sinh, RoBERTa F1 99,98% trên tập đã thấy nhưng tụt trên RAID; [FraudSquad arXiv 2510.01801](https://arxiv.org/html/2510.01801v1): ba tập spam do LLM sinh; Salminen et al. 2021 (~40.433 review Amazon CG/OR, repo `joolsa/FakeReviews`, chưa xác minh bản gốc). Dùng: test chống review do LLM sinh (các bộ hiện có là tiếng Anh/đa ngôn ngữ, chỉ dùng phần tiếng Anh; ghi chú: chưa thấy bộ tiếng Việt). Salminen: đã xác minh qua WebSearch ~40.433 review (20.238 CG, 20.216 OR) trên 10 danh mục Amazon, bài *Journal of Retailing and Consumer Services* 64 (2022), doi 10.1016/j.jretconser.2021.102771; một repo GitHub mô tả bản 40.000 review cân bằng; license/trang Kaggle gốc chưa xác minh. ★4 (cho miền tiếng Anh). Hạn chế: miền khác; nguy cơ detector overfit generator.

**15. SemEval-2016 Task 5.** [trang task](https://alt.qcri.org/semeval2016/task5/): trang task liệt kê nhiều ngôn ngữ (nhà hàng, laptop, khách sạn); **chỉ dùng phần tiếng Anh** (nhà hàng, laptop, khách sạn), bỏ các ngôn ngữ khác; số câu/review phần tiếng Anh và dung lượng: không tìm thấy; nhãn entity#attribute + polarity, F1 aspect + accuracy polarity. Dùng: định dạng chuẩn để quy đổi nhãn ABSA của ta; kiểm chứng cross-lingual. ★3.

**16. Tóm tắt review: Opinosis, SPACE, FewSum.** Opinosis (51 chủ đề, ~100 câu/chủ đề, có tóm tắt tham chiếu; [tác giả](https://kavita-ganesan.com/?p=59)); SPACE (TripAdvisor, 50 khách sạn, tóm tắt tổng quát + 6 aspect; ~100 review/entity; giới thiệu trong [arXiv 2012.04443](https://arxiv.org/pdf/2012.04443)); FewSum (Amazon+Yelp, 3 tóm tắt/entity Amazon; Chu & Liu 2019 cho Yelp 200 entity). Các bài kết hợp chúng trong đánh giá, số liệu khác nhau theo bài. Dùng: đo ROUGE/faithfulness ngoài miền trước khi có tóm tắt tiếng Việt. ★3. Ghi chú: các bộ trên đều tiếng Anh; chưa xác nhận bộ tóm tắt review tiếng Việt nào.

### (d) Blog / cuộc thi công ty

- **Shopee Code League 2020** (mục 10). **AIVIVN 2019** (cuộc thi sentiment review tiếng Việt, được một bài 2020 dùng; link/tải: chưa xác minh) — ★2.
- Pixalate/vendor report về click fraud nằm ở scope 08, không dùng ở đây.

## 5. Dữ liệu tự tạo / synthetic từ chính hệ thống

1. **Khởi động lạnh:** `SEED_DEMO_DATA=true` -> sinh review demo bằng LLM với ràng buộc: gắn với `order_items` của đơn DELIVERED, rating tương quan với chủ đề (giao chậm -> aspect "giao hàng" tiêu cực), phân bố độ dài/teencode/không dấu/emoji thực tế. **Gắn cờ `synthetic=true`, không đưa vào production hay số liệu doanh nghiệp.**
2. **Bộ vàng sentiment/ABSA:** lấy 1–2K review (khi đã có thật, có consent) và gán tay 2 người, đo kappa; dùng ViSFD/ViSD4SA làm khởi tạo và so khớp thang aspect.
3. **Nhãn yếu từ hệ thống:** rating thấp + `REFUND_REQUESTED`/`fact_refunds.reason` -> nhãn "khiếu nại"; rating cao + giao đúng hạn -> tích cực; dùng để huấn luyện bán giám sát và để phát hiện lệch nhãn (rating 5 nhưng văn bản tiêu cực).
4. **Bộ vàng spam:** tự chèn có kiểm soát các mẫu spam đã biết (copy-paste, review cụm theo `ip_hash`/thiết bị, review ngay sau đặt đơn, 1 tài khoản nhiều shop, tăng đột biến rating 5 sao) vào dữ liệu demo để kiểm thử detector và pipeline cảnh báo (liên kết 08, 14 là scope khác). Ghi tỉ lệ chèn để tính recall.
5. **Chia train/valid/test theo thời gian:** train = tháng t0–t1, valid = t1–t2, test = t2–t3 (không xáo trộn) để phát hiện trôi (từ lóng, sản phẩm mới); với spam, **tách theo tài khoản/shop** (không để cùng tài khoản ở hai tập) để tránh rò rỉ danh tính; test spam có thêm nhóm "chiến thuật mới" (LLM sinh) không có trong train.
6. **Tóm tắt:** dựng 100–200 tóm tắt tham chiếu người viết cho sản phẩm có ≥ 20 review; chấm faithfulness bằng checklist mệnh đề (mỗi ý trong tóm tắt trỏ tới ≥ 2 review nguồn).

## 6. Lộ trình dùng dữ liệu đề xuất

1. **Giai đoạn 0 (trước khi có review):** huấn luyện/đo trên ViSFD, ViSD4SA, ViOCD, ViSpamReviews; chọn mô hình (PhoBERT vs. XLM-R vs. LLM few-shot); chuẩn hoá tiền xử lý tiếng Việt.
2. **Giai đoạn 1:** xây tính năng review (tối thiểu: rating+text+verified purchase) + outbox `review.created`; dùng mô hình đã huấn luyện chạy ở chế độ shadow, không hiển thị.
3. **Giai đoạn 2:** thu 1–2K review thật để gán tay; hiệu chuẩn ngưỡng; bật tóm tắt/aspect cho seller và hàng đợi spam cho admin.
4. **Giai đoạn 3:** thêm đặc trưng hành vi từ `events_raw`/`fact_order_*`; đối chiếu Yelp/ODDS để thử mô hình đồ thị; thêm kiểm thử review LLM-sinh (MAiDE-up kiểu).
5. **Liên tục:** giám sát trôi phân bố hằng tháng; cập nhật bộ vàng theo trường hợp sai.

## 7. Rủi ro, license, thiên lệch, lưu ý pháp lý/PII

- **License hạn chế:** UIT-VSFC phải ký thỏa thuận; ViHSD cần email tác giả; Shopee Code League dưới điều khoản Shopee; Amazon Reviews 2023 CC BY 4.0 nhưng điều khoản Amazon cần đọc; ABSA/ViSpam: "tự do cho nghiên cứu" chưa chắc cho phép dùng thương mại — **làm rõ trước khi đưa mô hình huấn luyện từ các bộ này vào sản phẩm**.
- **Thiên lệch:** ViSFD/ViSD4SA chỉ điện thoại; UIT-VSFC miền giáo dục; Yelp/Amazon tiếng Anh; nhãn Yelp là của bộ lọc Yelp (nhiễu); lớp neutral hiếm -> mô hình thiên tích cực.
- **Rủi ro false positive spam:** ẩn nhầm review thật gây tổn hại seller/buyer; cần quy trình kháng nghị, không tự động xoá.
- **PII:** review có thể chứa tên, SĐT, địa chỉ, link; che trước khi lưu vào bộ huấn luyện; chỉ dùng tracking khi `analytics_consent`; `ip_hash` không được dùng ngoài mục đích chống gian lận đã công bố.
- **Review do LLM sinh:** có thể chính seller dùng LLM; detector dễ lỗi thời nhanh.
- **Tóm tắt LLM:** có thể hallucinate hoặc thiên lệch về review dài; luôn kèm trích dẫn.
- **Pháp lý:** quảng cáo/đánh giá sai sự thật có thể vi phạm quy định TMĐT (liên kết 06); minh bạch về việc review đã được kiểm duyệt tự động.

## 8. Nguồn tham khảo (URL)

- UIT-ViSFD: https://arxiv.org/abs/2105.15079v1 ; https://huggingface.co/datasets/visolex/ViSFD/blob/main/README.md ; https://huggingface.co/datasets/SEACrowd/uit_visfd/blob/main/uit_visfd.py
- UIT-ViSD4SA: https://arxiv.org/abs/2110.07833v1 ; GitHub kimkim00/UIT-ViSD4SA (tên repo theo bài báo)
- ViSpamReviews: https://arxiv.org/abs/2207.14636 ; https://arxiv.org/abs/2405.13292v2
- UIT-ViOCD: https://arxiv.org/pdf/2104.11969 ; https://hf.co/datasets/tarudesu/ViOCD ; SMTCE https://arxiv.org/pdf/2209.10482
- UIT-VSFC: https://arxiv.org/pdf/1911.07223 ; https://github.com/kietnv/uit-vsfc ; https://nlp.uit.edu.vn/datasets
- VLSP: https://vjst.vast.vn/jcc/article/view/13160 ; https://vjs.ac.vn/index.php/jcc/article/download/13160/382797/389009
- UIT-VSMEC: https://ar5iv.labs.arxiv.org/html/1911.09339
- ViHSD/ViHOS: https://arxiv.org/pdf/2301.10186 ; https://Github.Com/sonlam1102/vihsd ; https://arxiv.org/abs/2103.10069v2
- Review Tiki/Shopee/Sendo/Hasaki: https://doaj.org/article/625f8f28cd8e4f1888076fc69bc97418
- Shopee Code League sentiment: https://github.com/ilos-vigil/scl-2020-sentiment-analysis
- Amazon Reviews 2023: https://arxiv.org/html/2403.03952v1 ; https://huggingface.co/datasets/McAuley-Lab/Amazon-C4/blob/main/README.md
- Yelp spam: https://odds.cs.stonybrook.edu/?p=562 ; https://odds.cs.stonybrook.edu/?p=565
- Review LLM-sinh: https://arxiv.org/pdf/2404.12938v1 ; https://ojs.aaai.org/index.php/ICWSM/article/view/42791 ; https://arxiv.org/html/2510.01801v1
- SemEval-2016 T5: https://alt.qcri.org/semeval2016/task5/
- Tóm tắt: https://kavita-ganesan.com/?p=59 ; https://arxiv.org/pdf/2012.04443 ; https://arxiv.org/pdf/2211.15914
- Không tìm thấy/không xác minh trong phiên: dataset review Tiki/Shopee/Lazada công khai, bộ tóm tắt review tiếng Việt, trang Kaggle của AIVIVN 2019.
