# Hiểu truy vấn tiếng Việt cho tìm kiếm

> Tìm KHÔNG DẤU (khôi phục dấu), sửa lỗi chính tả, tách từ, NER (tên sản phẩm, thương hiệu, size), intent, autocomplete/suggest, embedding tiếng Việt. **Trạng thái: chưa triển khai trong hệ thống; đây là tài liệu nghiên cứu dữ liệu.** Ngày tra cứu: 2026-10-25.
>
> Ghi chú phương pháp: WebFetch không mở được HuggingFace/arXiv/GitHub-pages (DNS); thông tin lấy từ WebSearch. Quan trọng: tìm không thấy bộ công khai nào cho NER sản phẩm tiếng Việt, query log hay autocomplete tiếng Việt; phần này phải tự tạo (mục 5). Chỗ không kiểm chứng ghi "chưa xác minh".

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
| 12 | VN-MTEB | a/c | 41 bộ, 6 loại tác vụ | HF; license chưa xác minh | ★★★★★ |
| 13 | mMARCO (có Vietnamese?) | c | 13 ngôn ngữ (số liệu Vietnamese chưa xác minh) | Chưa xác minh | ★★★ |
| 14 | OSCAR / CC100 (tiếng Việt) | b | CC100-vi 138 GB (qua ViT5); OSCAR ~98.9 GB | Web crawl | ★★ |
| 15 | ModelScope viet-ecommerce-title/query | b | Mô hình NER TMĐT VN (F1 77.8; brand F1 78.91) | Chưa xác minh; chỉ mô hình, không phải dữ liệu | ★★ |
| 16 | Mô hình tham chiếu: PhoBERT, BGE-M3, ViSoBERT | c | — | PhoBERT (VinAI); BGE-M3 MIT (theo aggregator) | tham chiếu |

### (a) Tiếng Việt / Việt Nam

**PhoATIS** — https://arxiv.org/pdf/2104.02021 ; code/data VinAIResearch JointIDSF (GitHub); bản SEACrowd trên HF. 5,871 câu dịch tay từ ATIS (4,478 train / 500 val / 893 test), 28 intent, 82 slot; giữ đặc điểm nói (lặp từ, ngắt quãng). Miền đặt vé máy bay, không phải mua sắm; dùng như **khuôn** để dựng bộ intent+slot của sàn. Mô hình JointIDSF. ★4.

**Spelling: VSEC** — https://arxiv.org/pdf/2111.00640 : test thật 618 tài liệu Tailieu (9,341 câu, 11,202 lỗi, 3 người sửa), train tổng hợp từ 5M câu tin tức. **Viwiki-Spelling** — https://arxiv.org/pdf/2105.13578 : ~14K câu Wikipedia nháp, ~1,500 lỗi sửa tay, JSONL, CC BY 4.0. **BERT+Transformer (2405.02573)** — dùng Binhvq News Corpus (~15M câu) sinh lỗi vùng miền, fat-finger, Telex. Mô hình HF: nrl-ai/vn-spell-correction-base (corpus tổng hợp 459K cặp). Hạn chế: lỗi văn bản dài, không phải truy vấn ngắn; cần sinh lỗi riêng cho query. ★3-4.

**Khôi phục dấu** — không có một bộ chuẩn Việt-only; các lựa chọn: DiacBench https://huggingface.co/datasets/olaverse/diacbench (~1,000 cặp stripped/original mỗi ngôn ngữ gồm tiếng Việt), corpus đa ngữ LINDAT/UFAL (12 ngôn ngữ, Wikipedia + web, CC BY-NC-SA 4.0; repo huấn luyện https://github.com/arahusky/diacritics_restoration), bài MT-based https://arxiv.org/pdf/1709.07104 (phrase-based 97.32% vs neural 96.15% trên bộ lớn), công cụ https://github.com/kanjirz50/restore-tonemark ; vina2vi (PyPI). Có thể tự tạo gần như vô hạn bằng cách bỏ dấu corpus có dấu (mục 5). ★3.

**VLSP 2013/2016 + VnCoreNLP** — https://arxiv.org/html/1801.01331 : tách từ & POS từ VLSP 2013 (WordSeg train 75,389 câu, test 2,120; POS ~27K câu), NER từ VLSP 2016. Tải thường qua đăng ký VLSP; license chưa xác minh. Dùng: tách từ, baseline NER nhưng nhãn (PER/LOC/ORG...) không phải thương hiệu/size. ★4.

**PhoNER_COVID19** — https://arxiv.org/pdf/2104.03879 ; VinAIResearch/PhoNER_COVID19 : NER miền COVID (quan trọng: mô hình mức từ > mức âm tiết, PhoBERT > XLM-R). Chỉ làm tham khảo kỹ thuật. ★2.

**UIT-ViQuAD** — https://nlp.uit.edu.vn/datasets ; COLING 2020 : 23K+ cặp QA, 5,109 đoạn, 174 bài; 2.0 (https://arxiv.org/html/2203.11400v3) có câu không trả lời, 28,457 cặp QA train. Truy cập: email tác giả ký thoả thuận corpus. Dùng cho FAQ/policy RAG hơn là truy vấn. ★2.

**UIT-ViSFD** — https://arxiv.org/pdf/2105.15079 : 11,122 comment về điện thoại từ sàn TMĐT VN, 10 khía cạnh, 3 mức cảm xúc; license không rõ, thương mại cần liên hệ. Nguồn từ vựng sản phẩm/tiếng lóng. ★2.

**Câu hỏi tương tự** — https://vjol.info.vn/halong/en/article/view/76720 : 7,911 cặp câu hỏi; không có link tải. ★1.

**Không tìm thấy (đã tìm)**: bộ NER sản phẩm/thương hiệu/size tiếng Việt công khai (blog Cốc Cốc 2022 nói thiếu: https://careers.coccoc.com/blogs/attribution-extraction-of-e-commerce-product-listing-part-1 ), query log hay dữ liệu autocomplete tiếng Việt, ViMedAQA và Zalo legal retrieval (không xác minh được trong phiên).

### (b) TMĐT / miền liên quan
- **ModelScope `viet-ecommerce-title` / `viet-ecommerce-query`** (DAMO Academy, XLM-R): gắn thẻ product, style, consumer group, brand, pattern; F1 tổng 77.8, brand 78.91 (title), 71.95 (query), theo mirror cyzone.cn (https://cyzone.cn/agi/data/model/581); kiểm tra trên ModelScope. Dùng làm baseline hoặc để gán nhãn mềm. ★2.
- Alibaba Cloud NER đa ngữ (trả phí) hỗ trợ tiếng Việt, nhận brand/product/model; không hỗ trợ từ điển tuỳ chỉnh.
- **Corpus văn bản thô**: OSCAR (https://huggingface.co/datasets/oscar-corpus/OSCAR-2201 ; tiếng Việt ~98.9 GB theo bảng một paper; số tài liệu thay đổi theo phiên bản), CC100-vi 138 GB (qua ViT5, https://arxiv.org/pdf/2205.06457). Dùng huấn luyện LM/khôi phục dấu; chất lượng web, có nội dung nhạy cảm. Wikipedia/tin tức VN: chưa tìm được link xác minh trong phiên.

### (c) Benchmark / paper học thuật
- **VN-MTEB** — https://arxiv.org/pdf/2507.21500 (Findings EACL 2026, GreenNode AI + VNU-HCMC): dịch bằng LLM các mẫu MTEB sang tiếng Việt, 41 bộ, 6 loại tác vụ; kết quả: mô hình lớn dùng RoPE tốt hơn. Dùng chọn mô hình embedding. Phù hợp ★5 (với nhu cầu embedding), lưu ý bản dịch máy.
- **mMARCO** — https://arxiv.org/abs/2108.13897 (v5: 13 ngôn ngữ; bản đầu 8 ngôn ngữ). Chưa xác nhận có tách tiếng Việt từ repo unicamp-dl; có dữ liệu hard negatives `vietnamese_bge-reranker-v2-m3` (hotchpotch/mmarco-hard-negatives-reranker-score, quan hệ chưa xác minh). Dịch máy, miền web, không phải sản phẩm. ★3.
- **Mô hình tham chiếu (không phải dữ liệu)**: PhoBERT (https://cs.paperswithcode.com/paper/phobert-pre-trained-language-models-for ; kích thước corpus chưa xác minh), BGE-M3 (đa ngữ, 8192 token, MIT theo aggregator; kiểm tra thẻ chính thức), ViSoBERT (tiếng Việt mạng xã hội, nghiên cứu).

### (d) Blog / cuộc thi / công ty
- Blog Cốc Cốc về trích xuất thuộc tính listing bằng từ điển (liên kết ở trên): nhắc thiếu dữ liệu NER sản phẩm; hướng dictionary-based là gợi ý thực tế.
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

- **Thiếu dữ liệu thật miền sản phẩm**: bộ công khai là tin tức/wiki/COVID/hàng không, lệch miền so với truy vấn mua sắm (viết tắt, teencode, không dấu).
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
- VN-MTEB: https://arxiv.org/pdf/2507.21500
- mMARCO: https://arxiv.org/abs/2108.13897
- OSCAR: https://huggingface.co/datasets/oscar-corpus/OSCAR-2201 ; ViT5/CC100: https://arxiv.org/pdf/2205.06457
- Cốc Cốc: https://careers.coccoc.com/blogs/attribution-extraction-of-e-commerce-product-listing-part-1
- ModelScope mirror: https://cyzone.cn/agi/data/model/581 (chưa xác minh trên ModelScope)
- PhoBERT: https://cs.paperswithcode.com/paper/phobert-pre-trained-language-models-for ; BGE-M3 (aggregator): https://docs.ionos.com/cloud/ai/ai-model-hub/models/embedding-models/bge-m3
