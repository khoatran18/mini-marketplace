# Hiểu truy vấn tiếng Việt cho tìm kiếm

> Tìm KHÔNG DẤU (khôi phục dấu), sửa lỗi chính tả, tách từ, NER (tên sản phẩm, thương hiệu, size), intent, autocomplete/suggest, embedding tiếng Việt. **Trạng thái: chưa triển khai trong hệ thống; đây là tài liệu nghiên cứu dữ liệu.** Ngày tra cứu: 2026-10-25.
>
> Ghi chú phương pháp: WebFetch không mở được HuggingFace/arXiv (DNS); thông tin lấy từ WebSearch (nguồn ghi là "kết quả search") và vài trang GitHub. Chủ dự án xác nhận không bắt buộc dùng dữ liệu tiếng Việt: mục 4(d) thêm các bộ tiếng Anh/đa ngôn ngữ (ESCI, AmazonQAC, MASSIVE, MTEB/MMTEB, Webis-QSpell-17, ORCAS, WANDS...) để chọn phương pháp. Các bộ tiếng Việt công khai hiện biết tập trung ở tách từ, chính tả, intent miền khác; phần query log, NER sản phẩm và autocomplete của sàn sẽ dựa trên dữ liệu hệ thống (mục 5). Chỗ không kiểm chứng ghi "chưa xác minh".

| Liên quan | File |
|---|---|
| Tìm kiếm ngữ nghĩa sản phẩm | [02_PRODUCT_SEARCH_SEMANTIC.md](02_PRODUCT_SEARCH_SEMANTIC.md) |
| Phân loại/thuộc tính sản phẩm | [10_PRODUCT_CATEGORIZATION_ATTRIBUTES.md](10_PRODUCT_CATEGORIZATION_ATTRIBUTES.md) |
| Review/cảm xúc | [07_REVIEW_SENTIMENT_SPAM.md](07_REVIEW_SENTIMENT_SPAM.md) |
| Kiểm duyệt (chuẩn hoá teencode) | [12_CONTENT_MODERATION_AGENT_SAFETY.md](12_CONTENT_MODERATION_AGENT_SAFETY.md) |
| Hỗ trợ khách / shopper agent | [05_SHOPPER_ASSISTANT_CUSTOMER_SUPPORT.md](05_SHOPPER_ASSISTANT_CUSTOMER_SUPPORT.md) |
| Eval agent | [16_AGENT_TOOL_USE_EVAL.md](16_AGENT_TOOL_USE_EVAL.md) |

## 1. Mục tiêu và vị trí trong hệ thống

- **Buyer** (và khách vãng lai): gõ "ao thun nam cotton size L", "iphone 15 pro max 256gb", "giay the thao nu dưới 500k". Hệ thống phải: khôi phục dấu, sửa lỗi gõ, tách từ/ghép, nhận diện thương hiệu/size/màu/khoảng giá, đoán ý định (tìm hàng, hỏi tồn kho, hỏi chính sách, theo dõi đơn), gợi ý khi gõ.
- **Seller**: chuẩn hoá tiêu đề listing (tránh nhồi từ khoá), tách thuộc tính tự động; **Agent shopper**: bước hiểu câu hỏi trước khi gọi `search_products` (tham số `query`, `category_id`, `price_min/max`, `in_stock`).
- **Input**: chuỗi truy vấn thô; **output**: truy vấn chuẩn hoá + thực thể + intent + bộ lọc có cấu trúc.
- **Dữ liệu của hệ thống**: `events_raw` (sự kiện tìm kiếm, click kết quả, thêm giỏ — nguồn query log để học đồng nghĩa, sửa chính tả, autocomplete), catalog (tên sản phẩm, thương hiệu, danh mục từ product-service), `fact_order_items` (độ phổ biến thực), review/Q&A (từ vựng người dùng).

## 2. Hướng làm đề xuất (baseline → nâng cao)

1. **Baseline (không cần ML nặng)**: chuẩn hoá Unicode (NFC), hạ chữ, bỏ ký tự thừa; từ điển thương hiệu/danh mục/size từ chính catalog; n-gram không dấu: tạo trường index "không dấu" song song (tìm "ao thun" khớp "áo thun") thay vì đoán lại dấu.
2. **Khôi phục dấu**: mô hình seq2seq/token classification hoặc n-gram LM trên corpus tin tức/wiki; ràng buộc ứng viên bằng từ điển catalog để giảm nhập nhằng ("ma" → "má/mã/mà").
3. **Sửa chính tả**: sinh lỗi tổng hợp (Telex/VNI gõ nhầm, nhầm phụ âm vùng miền, dính phím) + mô hình sửa (xem VSEC, Viwiki-Spelling); kết hợp tần suất truy vấn thật.
4. **Tách từ + NER**: VnCoreNLP/PhoBERT cho tách từ; NER sản phẩm bằng mô hình nhỏ huấn luyện từ gán nhãn bán tự động từ catalog (BIO: BRAND, PRODUCT, SIZE, COLOR, PRICE...). Intent: classifier nhỏ hoặc LLM few-shot đã đóng khung đầu ra JSON.
5. **Embedding**: mô hình đa ngữ (BGE-M3, mô hình tham chiếu) hoặc PhoBERT-based cho query↔sản phẩm (xem [02](02_PRODUCT_SEARCH_SEMANTIC.md)); đánh giá bằng VN-MTEB, mMARCO.
6. **Autocomplete**: prefix trie + tần suất từ `events_raw` + catalog; xếp hạng bằng CTR/đơn hàng; lọc từ nhạy cảm.

## 3. Metric đánh giá

- Khôi phục dấu: độ chính xác mức ký tự/âm tiết và mức câu (WER/CER); trên truy vấn ngắn nên đo thêm "kết quả tìm kiếm đúng sau khôi phục".
- Sửa chính tả: precision/recall sửa, tỉ lệ sửa sai làm hỏng truy vấn đúng (quan trọng).
- Tách từ: F1 (VLSP 2013: VnCoreNLP báo ~97.9 F1 qua trang tổng hợp); NER: F1 thực thể (entity-level); intent/slot: accuracy intent, F1 slot, sentence accuracy.
- Embedding: NDCG@10, Recall@100 (MIRACL/mMARCO-vi, VN-MTEB), độ trễ.
- Online: tỉ lệ truy vấn không kết quả, CTR kết quả đầu, tỉ lệ nhập lại truy vấn (reformulation).

## 4. Bộ dữ liệu

### Bảng tổng hợp

| # | Bộ dữ liệu | Nhóm | Quy mô (theo nguồn đã tra) | License / truy cập | Phù hợp |
|---|---|---|---|---|---|
| 1 | PhoATIS (JointIDSF) | a | 5,871 câu; 28 intent, 82 slot | GitHub VinAI; license chưa xác minh | ★★★★ (cách làm) |
| 2 | VSEC (spelling) | a | Test 9,341 câu thật/11,202 lỗi; train tổng hợp 5M câu | Chưa xác minh | ★★★★ |
| 3 | Viwiki-Spelling | a | ~14,000 câu, ~1,500 lỗi sửa tay | CC BY 4.0 (theo search) | ★★★ |
| 4 | Binhvq News Corpus | a | ~15M câu tin tức (làm gốc sinh lỗi) | Chưa xác minh | ★★★ |
| 5 | DiacBench (HF olaverse/diacbench) | a/c | ~1,000 cặp mỗi ngôn ngữ (có tiếng Việt) | Chưa xác minh | ★★★ |
| 6 | Corpus khôi phục dấu đa ngữ (LINDAT) | c | 12 ngôn ngữ gồm tiếng Việt, từ Wikipedia | CC BY-NC-SA 4.0 | ★★★ |
| 7 | VLSP 2013 (tách từ, POS) / VLSP 2016 (NER) | a | WordSeg train 75,389 câu, test 2,120; POS ~27K câu | Chưa xác minh (qua VnCoreNLP) | ★★★★ |
| 8 | PhoNER_COVID19 | a | Số liệu chưa xác minh | GitHub VinAI; license chưa xác minh | ★★ |
| 9 | UIT-ViQuAD 1.0/2.0 | a | 23K+ cặp QA / 174 bài wiki; 2.0 có câu không trả lời | Ký thoả thuận corpus | ★★ |
| 10 | UIT-ViSFD | a | 11,122 comment điện thoại | Xem GitHub; thương mại cần liên hệ | ★★ |
| 11 | Vietnamese similar-question | a | 7,911 cặp câu hỏi | Chưa có link tải | ★ |
| 12 | VN-MTEB | a/c | 41 bộ, 6 loại tác vụ, dịch từ MTEB bằng LLM (đã xác minh, nguồn: kết quả search về arXiv 2507.21500) | HF (GreenNode, mteb); license chưa xác minh | ★★★★★ |
| 13 | mMARCO | c | 8 ngôn ngữ (bản đầu) / 13 (paper v5) / README ghi 14 gồm tiếng Anh; có tiếng Việt hay không: chưa xác minh | Chưa xác minh | ★★★ |
| 14 | OSCAR / CC100 (tiếng Việt) | b | CC100-vi 138 GB (qua ViT5); OSCAR ~98.9 GB | Web crawl | ★★ |
| 15 | ModelScope viet-ecommerce-title/query | b | Mô hình NER TMĐT VN (F1 77.8; brand F1 78.91) | Chưa xác minh; chỉ mô hình, không phải dữ liệu | ★★ |
| 16 | Mô hình tham chiếu: PhoBERT, BGE-M3, ViSoBERT | c | — | PhoBERT (VinAI); BGE-M3 MIT (theo 2 aggregator: llmgateway.io, thegtmdirectory.com; chưa đối chiếu thẻ HF) | tham chiếu |
| 17 | Amazon Shopping Queries (ESCI) | b (EN/JA/ES) | ~130K truy vấn, 2.6M nhãn (query, product); mỗi truy vấn tối đa 40 ứng viên; nhãn E/S/C/I | Apache-2.0 theo một mirror GitHub; chưa đối chiếu repo amazon-science/esci-data | ★★★★★ |
| 18 | AmazonQAC | b (EN) | 395M mẫu train + 20K test; chuỗi prefix gõ, session, timestamp | HF `amazon/AmazonQAC`; license chưa xác minh | ★★★★★ (autocomplete) |
| 19 | MASSIVE (Amazon) | c (đa ngữ, có vi-VN) | >1M câu, 51 locale (1.1: 52, thêm Catalan), 18 domain, 60 intent, 55 slot | Chưa xác minh (repo có LICENSE.txt) | ★★★★ |
| 20 | MTEB / MMTEB | c | MMTEB: >500 tác vụ, 10 nhóm tác vụ, hàng trăm ngôn ngữ (250+ hoặc 1000+ tuỳ nguồn) | Theo từng tác vụ | ★★★★★ |
| 21 | Webis-QSpell-17 | b (EN) | 54,772 truy vấn web, 9,171 có biến thể sửa chính tả | Chưa xác minh (Zenodo) | ★★★★ |
| 22 | ORCAS | b (EN) | 18,823,602 bản ghi click, 10M truy vấn, 1.4M tài liệu TREC DL | Chỉ nghiên cứu phi thương mại | ★★★ |
| 23 | Wayfair WANDS | b (EN) | 42,994 sản phẩm, 480 truy vấn, 233,448 nhãn (Exact/Partial/Irrelevant) | Chưa xác minh | ★★★★ |
| 24 | AOL query log | b (EN) | ~20M truy vấn, ~650K user (2006) | Đã bị thu hồi vì riêng tư; README gốc: chỉ nghiên cứu phi thương mại | ★ (cân nhắc đạo đức) |
| 25 | BANKING77 | c (EN) | 13,083 câu, 77 intent | CC BY 4.0 (theo TFDS qua search) | ★★ (mẫu intent) |

### (a) Tiếng Việt / Việt Nam

**PhoATIS** — https://arxiv.org/pdf/2104.02021 ; code/data VinAIResearch JointIDSF (GitHub); bản SEACrowd trên HF. 5,871 câu dịch tay từ ATIS (4,478 train / 500 val / 893 test), 28 intent, 82 slot; giữ đặc điểm nói (lặp từ, ngắt quãng). Miền đặt vé máy bay, không phải mua sắm; dùng như **khuôn** để dựng bộ intent+slot của sàn. Mô hình JointIDSF. ★4.

**Spelling: VSEC** — https://arxiv.org/pdf/2111.00640 : test thật 618 tài liệu Tailieu (9,341 câu, 11,202 lỗi, 3 người sửa), train tổng hợp từ 5M câu tin tức. **Viwiki-Spelling** — https://arxiv.org/pdf/2105.13578 : ~14K câu Wikipedia nháp, ~1,500 lỗi sửa tay, JSONL, CC BY 4.0. **BERT+Transformer (2405.02573)** — dùng Binhvq News Corpus (~15M câu) sinh lỗi vùng miền, fat-finger, Telex. Mô hình HF: nrl-ai/vn-spell-correction-base (corpus tổng hợp 459K cặp). Hạn chế: lỗi văn bản dài, không phải truy vấn ngắn; cần sinh lỗi riêng cho query. ★3-4.

**Khôi phục dấu** — không có một bộ chuẩn Việt-only; các lựa chọn: DiacBench https://huggingface.co/datasets/olaverse/diacbench (~1,000 cặp stripped/original mỗi ngôn ngữ gồm tiếng Việt), corpus đa ngữ LINDAT/UFAL (12 ngôn ngữ, Wikipedia + web, CC BY-NC-SA 4.0; repo huấn luyện https://github.com/arahusky/diacritics_restoration), bài MT-based https://arxiv.org/pdf/1709.07104 (phrase-based 97.32% vs neural 96.15% trên bộ lớn), công cụ https://github.com/kanjirz50/restore-tonemark ; vina2vi (PyPI). Có thể tự tạo gần như vô hạn bằng cách bỏ dấu corpus có dấu (mục 5). ★3.

**VLSP 2013/2016 + VnCoreNLP** — https://arxiv.org/html/1801.01331 : tách từ & POS từ VLSP 2013 (WordSeg train 75,389 câu, test 2,120; POS ~27K câu), NER từ VLSP 2016. Tải thường qua đăng ký VLSP; license chưa xác minh. Dùng: tách từ, baseline NER nhưng nhãn (PER/LOC/ORG...) không phải thương hiệu/size. ★4.

**PhoNER_COVID19** — https://arxiv.org/pdf/2104.03879 ; VinAIResearch/PhoNER_COVID19 : NER miền COVID (quan trọng: mô hình mức từ > mức âm tiết, PhoBERT > XLM-R). Chỉ làm tham khảo kỹ thuật. ★2.

**UIT-ViQuAD** — https://nlp.uit.edu.vn/datasets ; COLING 2020 : 23K+ cặp QA, 5,109 đoạn, 174 bài; 2.0 (https://arxiv.org/html/2203.11400v3) có câu không trả lời, 28,457 cặp QA train. Truy cập: email tác giả ký thoả thuận corpus. Dùng cho FAQ/policy RAG hơn là truy vấn. ★2.

**UIT-ViSFD** — https://arxiv.org/pdf/2105.15079 : 11,122 comment về điện thoại từ sàn TMĐT VN, 10 khía cạnh, 3 mức cảm xúc; license không rõ, thương mại cần liên hệ. Nguồn từ vựng sản phẩm/tiếng lóng. ★2.

**Câu hỏi tương tự** — https://vjol.info.vn/halong/en/article/view/76720 : 7,911 cặp câu hỏi; không có link tải. ★1.

**Ghi chú**: trong các nguồn đã tra chưa thấy bộ NER sản phẩm/thương hiệu/size, query log hay autocomplete tiếng Việt công bố công khai (blog Cốc Cốc 2022 về trích xuất thuộc tính listing: https://careers.coccoc.com/blogs/attribution-extraction-of-e-commerce-product-listing-part-1 ). Với các tác vụ này dự án dùng dữ liệu của chính hệ thống (mục 5) và các bộ tiếng Anh ở mục 4(d) để chọn phương pháp. ViMedAQA và Zalo legal retrieval chưa xác minh được.

### (b) TMĐT / miền liên quan
- **ModelScope `viet-ecommerce-title` / `viet-ecommerce-query`** (DAMO Academy, XLM-R): gắn thẻ product, style, consumer group, brand, pattern; F1 tổng 77.8, brand 78.91 (title), 71.95 (query), theo mirror cyzone.cn (https://cyzone.cn/agi/data/model/581); kiểm tra trên ModelScope. Dùng làm baseline hoặc để gán nhãn mềm. ★2.
- Alibaba Cloud NER đa ngữ (trả phí) hỗ trợ tiếng Việt, nhận brand/product/model; không hỗ trợ từ điển tuỳ chỉnh.
- **Corpus văn bản thô**: OSCAR (https://huggingface.co/datasets/oscar-corpus/OSCAR-2201 ; tiếng Việt ~98.9 GB theo bảng một paper; số tài liệu thay đổi theo phiên bản), CC100-vi 138 GB (qua ViT5, https://arxiv.org/pdf/2205.06457). Dùng huấn luyện LM/khôi phục dấu; chất lượng web, có nội dung nhạy cảm. Wikipedia/tin tức VN: chưa tìm được link xác minh trong phiên.

### (c) Benchmark / paper học thuật
- **VN-MTEB** — https://arxiv.org/pdf/2507.21500 (Findings EACL 2026, GreenNode AI + VNU-HCMC): dịch bằng LLM các mẫu MTEB sang tiếng Việt, 41 bộ, 6 loại tác vụ; kết quả: mô hình lớn dùng RoPE tốt hơn. Dùng chọn mô hình embedding. Phù hợp ★5 (với nhu cầu embedding), lưu ý bản dịch máy.
- **mMARCO** — https://arxiv.org/abs/2108.13897 (v5: 13 ngôn ngữ; bản đầu 8 ngôn ngữ; README https://github.com/unicamp-dl/mMARCO ghi 14 ngôn ngữ gồm bản tiếng Anh gốc, tải bằng `load_dataset('unicamp-dl/mmarco', 'english')`; danh sách ngôn ngữ cụ thể không thấy trong kết quả search nên chưa xác nhận có tiếng Việt, hãy xem thẻ HF); có dữ liệu hard negatives `vietnamese_bge-reranker-v2-m3` (hotchpotch/mmarco-hard-negatives-reranker-score, quan hệ chưa xác minh). Dịch máy, miền web, không phải sản phẩm. ★3.
- **Mô hình tham chiếu (không phải dữ liệu)**: PhoBERT (https://cs.paperswithcode.com/paper/phobert-pre-trained-language-models-for ; kích thước corpus chưa xác minh), BGE-M3 (đa ngữ, 8192 token, MIT theo aggregator; kiểm tra thẻ chính thức), ViSoBERT (tiếng Việt mạng xã hội, nghiên cứu).

### (d) Bộ dữ liệu tiếng Anh / đa ngôn ngữ (bổ sung, dùng để chọn phương pháp)

Số liệu dưới đây lấy từ kết quả WebSearch (không mở được trang gốc); license nào không thấy ghi là chưa xác minh.

**Tìm kiếm sản phẩm và truy vấn mua sắm**
- **Amazon Shopping Queries Dataset (ESCI)** — https://arxiv.org/abs/2206.06588 ; repo amazon-science/esci-data (KDD Cup 2022): ~130K truy vấn duy nhất, 2.6M nhãn thủ công, tối đa 40 ứng viên/truy vấn, nhãn Exact/Substitute/Complement/Irrelevant; truy vấn tiếng Anh, Nhật, Tây Ban Nha; cuộc thi chạy 15/3-20/7/2022, >1,600 người từ 65 nước (blog Amazon Science). License: một mirror ghi Apache-2.0, chưa đối chiếu repo chính thức, chưa rõ có áp cho cả dữ liệu. Dùng: huấn luyện/đánh giá xếp hạng truy vấn-sản phẩm, phân loại ESCI, truy vấn lỗi chính tả thực. Có thêm bản có ảnh SQID (https://arxiv.org/pdf/2405.15190). ★5.
- **AmazonQAC** — https://arxiv.org/html/2411.04129v1 (EMNLP 2024 industry): 395M mẫu train từ log tìm kiếm Amazon, test 20K (giai đoạn sau để mô phỏng triển khai); mỗi mẫu có truy vấn cuối, chuỗi prefix đã gõ, session ID, timestamp. Baseline: prefix tree Success@10 ~25.3%, trần lý thuyết 69.8%, LLM tinh chỉnh có ngữ cảnh tốt nhất. Dữ liệu Parquet trên HF `amazon/AmazonQAC`. Đây là bộ autocomplete thực tế công khai gần nhất. ★5.
- **Wayfair WANDS** — https://github.com/wayfair/wands (ECIR 2022): 42,994 sản phẩm, 480 truy vấn, 233,448 nhãn liên quan 3 mức; kèm tên, mô tả, phân cấp danh mục. License chưa xác minh. ★4.
- **Dataset of Natural Language Queries for E-Commerce** — https://arxiv.org/pdf/2302.06355 : truy vấn ngôn ngữ tự nhiên chú thích mức "key fact"/yêu cầu sản phẩm; schema nhãn chưa xác minh. ★3.
- **NER truy vấn TMĐT tiếng Anh**: không thấy bộ công khai có nhãn brand rõ ràng trong kết quả search. Có bài Home Depot (https://arxiv.org/pdf/2012.07553, brand/product type; dữ liệu không thấy công bố), AE-Pub (AliExpress, giám sát xa) và Ecommerce5PT (33 thuộc tính, 5 loại sản phẩm Amazon) trích trong https://arxiv.org/pdf/2205.05904 ; mô hình ModelScope NER truy vấn tiếng Anh F1 ~75.7 (chỉ mô hình). Hướng thực tế: gán nhãn yếu từ catalog (mục 5).
- **ORCAS** — https://microsoft.github.io/msmarco/ORCAS.html : 18,823,602 bản ghi click cho 1.4M tài liệu TREC DL, 10M truy vấn; chỉ dùng nghiên cứu phi thương mại. Web search, không phải sản phẩm; dùng học đồng nghĩa/click. ★3.
- **AOL query log** (2006): ~20M truy vấn (21,011,340 truy vấn mới theo README), 657,426 user ID; đã bị thu hồi do cho phép nhận diện cá nhân, README gốc chỉ cho nghiên cứu phi thương mại; bản làm sạch AOLIA tồn tại. Nên tránh vì riêng tư/đạo đức, dùng ORCAS thay thế. ★1.

**Sửa chính tả truy vấn (tiếng Anh)**
- **Webis-QSpell-17** — https://zenodo.org/record/3570912 ; paper SIGIR 2017 (Hagen et al., DOI 10.1145/3077136.3080749): 54,772 truy vấn web, 9,171 có biến thể sửa; v2 có chú thích loại lỗi; zip 2.3 MB; tiếng Anh; ~16% truy vấn có lỗi theo một paper NAACL 2025; companion Webis-QSeC-10 có phân đoạn truy vấn. Dùng làm khuôn tập kiểm thử sửa chính tả truy vấn (lớn hơn Microsoft Speller Challenge ~6,000 truy vấn). ★4.

**Intent / slot đa ngôn ngữ**
- **MASSIVE** — https://arxiv.org/pdf/2204.08582 ; https://github.com/alexa/massive : >1M câu song song, 51 ngôn ngữ (bản 1.1 thêm Catalan = 52), 18 domain, 60 intent, 55 slot; dịch chuyên nghiệp từ SLURP. Locale **vi-VN có trong danh sách 51 locale** (nguồn: loader/thẻ HF qua search). License chưa xác minh. Dùng làm bộ intent/slot đa ngữ có tiếng Việt, phong cách trợ lý ảo (không phải mua sắm). ★4.
- **BANKING77** (13,083 câu, 77 intent, CC BY 4.0 theo TFDS) và **CLINC150** (kích thước, license chưa xác minh trong phiên): mẫu phân loại intent tiếng Anh một miền. ★2.

**Embedding đa ngôn ngữ**
- **MTEB / MMTEB** — https://arxiv.org/pdf/2502.13595 (ICLR 2025): >500 tác vụ chất lượng, 10 nhóm tác vụ; số ngôn ngữ ghi 250+ (paper) hoặc 1,000+ (poster ICLR) tuỳ nguồn; license theo từng tác vụ (metadata tác vụ trong tài liệu mteb). Dùng chọn mô hình embedding đa ngữ cho truy vấn↔sản phẩm; VN-MTEB là phần tiếng Việt. ★5.
- **BGE-M3**: 100+ ngôn ngữ, 8,192 token, nền XLM-RoBERTa mở rộng, MIT theo hai trang aggregator; nDCG@10 MIRACL 70.0 (tất cả chế độ, theo trang DeepInfra); chưa đối chiếu thẻ HF chính thức.

### (e) Blog / cuộc thi / công ty
- Blog Cốc Cốc về trích xuất thuộc tính listing bằng từ điển (liên kết ở trên): nhắc dữ liệu NER sản phẩm hạn chế; hướng dictionary-based là gợi ý thực tế.
- Zalo AI Challenge: không xác minh được trang dữ liệu; bỏ qua.

## 5. Dữ liệu tự tạo / synthetic từ chính hệ thống

- **Khôi phục dấu**: lấy tên sản phẩm/tiêu đề/review có dấu trong catalog, tự bỏ dấu → cặp (không dấu, có dấu); thêm biến thể gõ nhanh ("khong dau" bỏ cả khoảng trắng, viết tắt "k", "dc", "ko"). Bắt buộc có tập kiểm thử người gắn nhãn để tránh thiên lệch do tự sinh.
- **Lỗi chính tả**: sinh lỗi Telex/VNI, dính phím, lẫn l/n, s/x, ch/tr, d/gi; ghép với lỗi thật từ `events_raw` (truy vấn không kết quả rồi được gõ lại thành truy vấn có kết quả = cặp sửa lỗi tự nhiên).
- **NER sản phẩm**: gán nhãn yếu bằng từ điển thương hiệu/màu/size từ catalog + regex (size, dung lượng, khoảng giá "dưới 500k", "2 triệu"), 500-1,000 truy vấn người sửa tay; huấn luyện lặp (self-training).
- **Intent**: 6-8 nhóm (tìm hàng, so sánh, tồn kho, giá, đơn của tôi, chính sách, gợi ý, ngoài phạm vi), khớp tool trong `docs/ai/05_AI_DRAFT_ORIGINAL.md` (shopper: `search_products`, `check_stock`, `get_my_orders`, `ask_policy`, `get_recommendations`).
- **Autocomplete**: thống kê tiền tố từ `events_raw` + tên sản phẩm + doanh số `fact_order_items`; không đưa truy vấn chứa PII (SĐT, địa chỉ) vào gợi ý.
- **Đồng nghĩa/viết tắt**: từ click chung (truy vấn khác nhau cùng dẫn tới một sản phẩm).
- Dùng `SEED_DEMO_DATA=true` cho dữ liệu demo; production dùng log thật đã ẩn danh.

## 6. Lộ trình dùng dữ liệu đề xuất

1. VN-MTEB (và mMARCO nếu có tiếng Việt) để chọn embedding; thử BGE-M3 vs PhoBERT-based.
2. Sinh cặp không dấu/có dấu và lỗi chính tả từ catalog; đối chiếu với VSEC/Viwiki-Spelling làm kiểm thử ngoài miền.
3. Dựng gold set 500-1,000 truy vấn nội bộ (dấu, chính tả, thực thể, intent); PhoATIS làm khuôn nhãn intent/slot.
4. VnCoreNLP (VLSP) để tách từ; so sánh có/không tách từ khi dùng PhoBERT.
5. Autocomplete từ log khi đủ dữ liệu; trước đó dùng catalog.

## 7. Rủi ro, license, thiên lệch, lưu ý pháp lý/PII

- **Lệch miền**: các bộ tiếng Việt công khai là tin tức/wiki/COVID/hàng không, lệch so với truy vấn mua sắm (viết tắt, teencode, không dấu); bộ tiếng Anh (ESCI, AmazonQAC) gần miền hơn nhưng khác ngôn ngữ. Kết luận cuối cùng cần gold set nội bộ.
- **Dữ liệu truy vấn thật**: AOL đã bị thu hồi vì riêng tư; ORCAS/MS MARCO chỉ phi thương mại.
- **Nhập nhằng không dấu**: khôi phục dấu sai làm hỏng kết quả; nên giữ cả truy vấn gốc và truy vấn khôi phục, tìm trên cả hai.
- **Bản dịch máy** (VN-MTEB, mMARCO) có lỗi dịch; không thay thế đánh giá nội bộ.
- **License**: UIT (ViQuAD, ViCTSD...) cần ký thoả thuận; CC BY-NC-SA của corpus dấu hạn chế thương mại; OSCAR/CC100 là web crawl (bản quyền nội dung gốc, nội dung nhạy cảm); ViSFD thương mại cần liên hệ.
- **PII**: truy vấn và log có thể chứa tên, SĐT, địa chỉ; ẩn danh trước khi huấn luyện hoặc gợi ý; tuân thủ pháp luật bảo vệ dữ liệu cá nhân VN.
- **Công bằng ngôn ngữ**: phương ngữ vùng miền, người dùng gõ không dấu nhiều hơn (thiết bị, thói quen) không được bị kết quả kém hơn.

## 8. Nguồn tham khảo

- PhoATIS: https://arxiv.org/pdf/2104.02021
- PhoNER_COVID19: https://arxiv.org/pdf/2104.03879
- VnCoreNLP / VLSP: https://arxiv.org/html/1801.01331 ; https://arxiv.org/pdf/1708.07241
- VSEC: https://arxiv.org/pdf/2111.00640 ; Viwiki-Spelling: https://arxiv.org/pdf/2105.13578 ; https://arxiv.org/pdf/2405.02573 ; https://huggingface.co/nrl-ai/vn-spell-correction-base
- Khôi phục dấu: https://arxiv.org/pdf/1709.07104 ; https://huggingface.co/datasets/olaverse/diacbench ; https://b2find.eudat.eu/dataset/6fd801dc-09c6-57b8-b56a-d8db0c299cab ; https://github.com/arahusky/diacritics_restoration
- UIT datasets: https://nlp.uit.edu.vn/datasets ; ViQuAD 2.0: https://arxiv.org/html/2203.11400v3 ; ViSFD: https://arxiv.org/pdf/2105.15079
- VN-MTEB: https://arxiv.org/pdf/2507.21500 ; MMTEB: https://arxiv.org/pdf/2502.13595
- ESCI: https://arxiv.org/abs/2206.06588 ; AmazonQAC: https://arxiv.org/html/2411.04129v1 ; WANDS: https://github.com/wayfair/wands ; MASSIVE: https://arxiv.org/pdf/2204.08582 ; ORCAS: https://microsoft.github.io/msmarco/ORCAS.html ; Webis-QSpell-17: https://zenodo.org/record/3570912 ; NL e-commerce queries: https://arxiv.org/pdf/2302.06355
- mMARCO: https://arxiv.org/abs/2108.13897
- OSCAR: https://huggingface.co/datasets/oscar-corpus/OSCAR-2201 ; ViT5/CC100: https://arxiv.org/pdf/2205.06457
- Cốc Cốc: https://careers.coccoc.com/blogs/attribution-extraction-of-e-commerce-product-listing-part-1
- ModelScope mirror: https://cyzone.cn/agi/data/model/581 (chưa xác minh trên ModelScope)
- PhoBERT: https://cs.paperswithcode.com/paper/phobert-pre-trained-language-models-for ; BGE-M3 (aggregator): https://docs.ionos.com/cloud/ai/ai-model-hub/models/embedding-models/bge-m3
