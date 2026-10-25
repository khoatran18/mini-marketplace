# OCR và trích xuất trường hoá đơn / biên lai

> Đọc ảnh/PDF hoá đơn, biên lai (tên cửa hàng, ngày, tổng tiền, dòng hàng) thành dữ liệu có cấu trúc để seller nhập hàng/đối soát và để admin kiểm tra chứng từ. **Trạng thái: chưa triển khai trong hệ thống; đây là tài liệu nghiên cứu dữ liệu.** Ngày tra cứu: 2026-10-25.
>
> Ghi chú phương pháp: không mở được Kaggle/HuggingFace/arXiv/Zenodo qua công cụ fetch; các repo GitHub được đọc trực tiếp (CORD, SROIE, DocILE, Kleister-NDA, VinText, VietOCR, Donut, LayoutLMv3, PaddleOCR, MMOCR/WildReceipt), phần bổ sung (MC-OCR, FATURA, WildReceipt, XFUND, ReceiptSense, DocILE) lấy từ kết quả WebSearch lượt 2. Bộ nào không đọc được trang ghi **"chưa xác minh"**. Phạm vi: tiếng Anh/đa ngôn ngữ dùng được để chọn phương pháp KIE; dữ liệu tiếng Việt: MC-OCR 2021 (nếu truy cập được), hoặc tự tạo/tổng hợp (mục 5).

| Liên quan | File |
|---|---|
| Phân loại/thuộc tính sản phẩm (nhập hàng từ hoá đơn) | [10_PRODUCT_CATEGORIZATION_ATTRIBUTES.md](10_PRODUCT_CATEGORIZATION_ATTRIBUTES.md) |
| Phát hiện gian lận / chứng từ giả | [08_FRAUD_ANOMALY_RISK.md](08_FRAUD_ANOMALY_RISK.md) |
| Kiểm duyệt nội dung/ảnh tải lên | [12_CONTENT_MODERATION_AGENT_SAFETY.md](12_CONTENT_MODERATION_AGENT_SAFETY.md) |
| Tìm kiếm bằng ảnh / đa phương thức | [11_VISUAL_SEARCH_MULTIMODAL.md](11_VISUAL_SEARCH_MULTIMODAL.md) |
| NLP tiếng Việt | [15_VIETNAMESE_NLP_QUERY_UNDERSTANDING.md](15_VIETNAMESE_NLP_QUERY_UNDERSTANDING.md) |
| Pháp lý / chính sách | [06_POLICY_LEGAL_RAG.md](06_POLICY_LEGAL_RAG.md) |

## 1. Mục tiêu và vị trí trong hệ thống

- **Vai trò:** seller (tải ảnh hoá đơn nhập hàng -> gợi ý điền phiếu nhập kho / `inventory/adjust` lý do `restock`), seller/admin (đối chiếu chứng từ khi xác minh store: `sellers.tax_code`, `bank_account`), buyer/admin (chứng từ trả hàng, bằng chứng thanh toán chuyển khoản trong luồng refund).
- **Input:** ảnh chụp bằng điện thoại hoặc PDF hoá đơn/biên lai tiếng Việt (VND, ngày dd/mm/yyyy, dấu phân cách nghìn "." và thập phân ",").
- **Output:** JSON trường chuẩn hoá: `seller_name`, `address`, `tax_code` (MST), `invoice_no`, `date` (múi giờ Asia/Ho_Chi_Minh), `line_items[{name, qty, unit_price, amount}]`, `subtotal`, `vat`, `total` (đổi sang số nguyên đồng/`*_minor` cho khớp quy ước tiền của hệ thống), kèm độ tin cậy từng trường; người dùng xác nhận trước khi ghi.
- **Dữ liệu của hệ thống liên quan:** Postgres `products(sku, name, price, inventory)`, `inventory_ledgers(delta, reason='restock'|'adjust', ref_type, ref_id)`, `sellers(tax_code, bank_account, name, address)`, `payments/refunds` (đối chiếu số tiền). **Hiện chưa có bảng lưu chứng từ/tệp tải lên** — cần thiết kế riêng (lưu tệp ngoài DB, mã hoá, thời hạn lưu). Hệ thống chỉ có thanh toán mô phỏng nên không có hoá đơn thật phát sinh nội bộ; dữ liệu OCR thật chỉ đến từ tệp seller tải lên.
- ClickHouse không phải nguồn dữ liệu cho scope này (dùng để đo chất lượng: tỉ lệ sửa tay, thời gian xử lý).

## 2. Hướng làm đề xuất (baseline → nâng cao)

1. **Baseline OCR + quy tắc:** PaddleOCR (Apache-2.0; hỗ trợ nhiều ngôn ngữ Latin — hỗ trợ tiếng Việt cần xác nhận trong tài liệu) hoặc VietOCR (Apache-2.0, nhận dạng dòng chữ in/viết tay tiếng Việt) + regex/heuristic cho ngày, tổng tiền, MST (10 hoặc 13 chữ số), kiểm tra checksum số học (tổng dòng = subtotal).
2. **KIE trên token:** LayoutLMv3 (CC BY-NC-SA 4.0 cho repo — xem mục 7) hoặc mô hình tương đương fine-tune trên CORD/SROIE/dữ liệu Việt tự gán nhãn.
3. **End-to-end không OCR:** Donut (MIT code; ảnh → JSON), huấn luyện thêm bằng dữ liệu tổng hợp (SynthDoG) tiếng Việt.
4. **VLM / LLM đa phương thức:** gợi ý trích xuất theo schema JSON, rồi **kiểm chứng bằng quy tắc** (tổng, thuế, định dạng MST) thay vì tin tuyệt đối; dùng khi thiếu dữ liệu huấn luyện.
5. **Human-in-the-loop:** hiển thị ô đã điền + bbox, seller sửa; lưu bản sửa làm dữ liệu huấn luyện (với sự đồng ý).

## 3. Metric đánh giá

- OCR: **CER/WER** (nhận dạng dòng), **Hmean (precision/recall/F1)** cho phát hiện chữ (IoU ≥ 0,5).
- KIE: **F1 theo trường** (khớp chính xác sau chuẩn hoá: ngày, số tiền), F1 micro/macro, **field-level accuracy** cho `total`, `date`; **Normalized Edit Distance (NED)** cho chuỗi (tên shop, địa chỉ).
- Dòng hàng: **line-item F1 / Tree-edit-distance accuracy** (Donut), AP cho LIR (DocILE).
- Nghiệp vụ: tỉ lệ ghi tự động không cần sửa, thời gian xử lý trung bình, tỉ lệ sai lệch tiền (đồng), % hoá đơn bị từ chối vì OCR kém chất lượng.

## 4. Bộ dữ liệu

### Bảng tổng hợp

| # | Bộ dữ liệu | Nhóm | Quy mô (theo nguồn) | Ngôn ngữ | License | ★ |
|---|---|---|---|---|---|---|
| 1 | MC-OCR 2021 (RIVF) | (a)/(d) | 2.436 ảnh biên lai Việt chụp bằng điện thoại (đã xác minh, search) | Việt | chưa xác minh | 5 (nếu truy cập được) |
| 2 | UIT-MLReceipts | (a) | chưa xác minh | Việt (tên dự án gợi ý) | chưa xác minh | 4 |
| 3 | VinText | (a)/(d) | 2.000 ảnh, ~56.000 instance | Việt | chỉ nghiên cứu/giáo dục, cấm phân phối lại | 3 (cảnh chữ, không phải hoá đơn) |
| 4 | VietOCR (mô hình + dữ liệu mẫu) | (a) | ~10 triệu ảnh dòng chữ (huấn luyện); mẫu 1 triệu ảnh tổng hợp | Việt | Apache-2.0 (code) | 3 |
| 5 | SROIE (ICDAR 2019) | (c) | 1.000 biên lai (600 trainval + 400 test) | Anh/số (Malaysia) | dữ liệu: chưa xác minh; code MIT | 4 |
| 6 | CORD | (c) | 1.000 biên lai (800/100/100) dù mô tả ghi >11.000 | Indonesia | CC BY 4.0 | 5 |
| 7 | WildReceipt (MMOCR) | (c) | 1.740 ảnh, 68.975 bbox, 25 lớp (đã xác minh, search) | Anh (chưa xác minh) | chưa xác minh | 3 |
| 8 | DocILE | (c)/(d) | 6.680 annotated, 932K unlabeled, 100K synthetic (đã xác minh, search) | Anh | cần token đăng ký; code MIT | 4 |
| 9 | Kleister (NDA, Charity) | (c) | NDA: 540 tài liệu (254/83/203) | Anh | chưa nêu | 1 |
| 10 | FUNSD | (c) | chưa xác minh | Anh | chưa xác minh | 2 |
| 11 | RVL-CDIP | (c) | 400.000 ảnh, 16 lớp (mô tả repo thứ cấp) | Anh | chưa xác minh | 2 (phân loại loại chứng từ) |
| 12 | SynthDoG (Donut) | (c) | 4 bộ × 0,5M (en/zh/ja/ko) | không có Việt sẵn | chưa nêu | 3 (công cụ sinh) |
| 13 | Train Ticket (Donut) | (c) | chưa xác minh | Trung | chưa nêu | 1 |
| 14 | FATURA | (c) | 10.000 hoá đơn, 50 layout (200 ảnh/layout), 24 lớp (đã xác minh, search) | Anh | chưa xác minh (Zenodo, ~690,7 MB) | 4 |
| 15 | XFUND | (c) | form 7 ngôn ngữ (zh, ja, es, fr, it, de, pt); số mẫu chưa xác minh | đa ngôn ngữ | CC BY-NC-SA 4.0 (theo bản mirror HF, search) | 2 |
| 16 | ReceiptSense (CORU) | (c) | 20.000 biên lai annotate, 30.000 ảnh OCR, 10.000 item-level (search) | Ả Rập/Anh | chưa xác minh | 3 |
| 17 | Viet-Receipt-VQA (5CD-AI, HF) | (a) | dẫn xuất từ MC-OCR: bản thấy trong search ghi 2.034 biên lai (bản trước 1.589) | Việt | chưa xác minh | 3 |

### (a) Tiếng Việt / Việt Nam

**MC-OCR 2021 — Mobile-Captured Image Document Recognition for Vietnamese Receipts (RIVF 2021)** — ghi nhận qua tên dự án `WyzanDev/VietReceiptAI` (GitHub) có dùng MC-OCR; không đọc được trang cuộc thi/paper. Nhiệm vụ nhận biết: đánh giá chất lượng ảnh, trích trường (SELLER, ADDRESS, TIMESTAMP, TOTAL_COST là 4 trường dự án trên dùng). Đã xác minh (nguồn: WebSearch — preprint MCOCR của tác giả Xuan-Son Vu, DOI 10.1109/RIVF51545.2021.9642077): **2.436 ảnh** biên lai Việt chụp bằng điện thoại, 50 người thu thập, 2 pha gán nhãn (IQA chất lượng ảnh + KIE), 4 trường SELLER / SELLER ADDRESS / TIMESTAMP / TOTAL COST; thi trên CodaLab 05/12/2020–23/01/2021 (105 người tham gia, ~1.285 lượt nộp). License, nơi tải hiện nay, việc có còn công khai hay phải đăng ký: **chưa xác minh**. Bộ dẫn xuất: HF `5CD-AI/Viet-Receipt-VQA` (VQA trên MC-OCR; số mẫu các bản khác nhau: 1.589 → 2.034; không phải bộ gốc). Ánh xạ: 4 trường -> `seller_name, address, date, total`. Dùng làm bộ chính nếu truy cập được vì đúng miền (biên lai chụp bằng điện thoại ở Việt Nam). ★5 (tiềm năng). Hạn chế: không xác minh được tính sẵn có.

**UIT-MLReceipts** — chỉ biết tên qua `WyzanDev/VietReceiptAI` ("cleaned UIT-MLReceipts, MC-OCR, and SROIE"). Nguồn gốc (thường là nhóm UIT), kích thước, license: **chưa xác minh**. ★4 (tiềm năng). Hạn chế: README repo không nêu chi tiết; repo chỉ mới tạo 2026-09 (0 sao), không dùng làm nguồn dữ liệu chính thức.

**VinText (VinAI, CVPR 2021)** — https://github.com/VinAIResearch/dict-guided (đã đọc). 2.000 ảnh, ~56.000 text instance, tiếng Việt, ảnh cảnh đường phố (không phải hoá đơn). Điều khoản dữ liệu: chỉ cho nghiên cứu/giáo dục, **không được phân phối lại**, phải trích dẫn bài "Dictionary-guided Scene Text Recognition"; repo mã AGPL-3.0. Định dạng gốc `x1,y1,...,x4,y4,TRANSCRIPT` hoặc COCO. Dùng để thử bộ nhận dạng chữ Việt có dấu (detector/recogniser), không cho KIE. ★3. Hạn chế: miền khác, hạn chế license.

**VietOCR** — https://github.com/pbcquoc/vietocr (đã đọc). Thư viện Transformer OCR (`vgg_transformer` precision 0,8800, `vgg_seq2seq` 0,8701 trên tập test nội bộ 10M ảnh dòng chữ); huấn luyện trên ~10M ảnh gồm tổng hợp, viết tay, tài liệu quét; Apache-2.0. Mẫu dữ liệu 1M ảnh tổng hợp (Google Drive). Dùng làm recogniser cho từng dòng sau khi detect bằng PaddleOCR/DBNet. ★3. Hạn chế: số liệu tự báo cáo, không phải benchmark hoá đơn.

> **Hoá đơn điện tử Việt Nam:** dữ liệu tiếng Việt: MC-OCR 2021 (biên lai) hoặc tự tạo từ mẫu (mục 5); chưa xác minh bộ hoá đơn điện tử (XML/PDF theo Nghị định 123/2020/NĐ-CP) công khai. Đừng dùng hoá đơn thật của người khác.

### (b) TMĐT công khai

> Ghi chú: chưa xác minh được bộ "hoá đơn TMĐT" công khai riêng; dùng các bộ hoá đơn/biên lai ở mục (c) và dữ liệu tổng hợp (mục 5).

### (c) Benchmark / paper học thuật

**CORD** — https://github.com/clovaai/cord (đã đọc); HF `naver-clova-ix/cord-v1`, `cord-v2`. Biên lai Indonesia; README quảng cáo ">11.000" nhưng các bản phát hành v0/v1/v2 đều chứa **1.000** biên lai (800 train/100 dev/100 test). **CC BY 4.0**. Cột `valid_line` (quad, text, category, group_id, row_id, is_key), `meta`, `roi`; nhãn: 5 siêu lớp (menu, void_menu, subtotal, void_total, total) với 42 lớp con (trang cũng ghi "30" mâu thuẫn). `gt_parse` ở v1+, v2 sửa nhãn. Gần miền nhất về cấu trúc **dòng hàng + tổng** -> ánh xạ `line_items`, `subtotal`, `total`. ★5. Hạn chế: Indonesia, ảnh sạch, 1.000 mẫu.

**SROIE (ICDAR 2019)** — https://github.com/zzzDavid/ICDAR-2019-SROIE (đã đọc). 1.000 biên lai quét (600 trainval, 400 test), 3 tác vụ: text localisation, OCR, KIE (company, address, date, total). Phần lớn chữ số và chữ Anh. License: code MIT (repo), **dữ liệu chưa nêu**; bản gốc có lỗi nhãn, repo có `scripts/check_data.py`. Ánh xạ trực tiếp `seller_name, address, date, total`. ★4. Hạn chế: biên lai Malaysia, không có dòng hàng.

**WildReceipt (MMOCR)** — hướng dẫn https://github.com/open-mmlab/mmocr (docs `data_prepare/kie.md`; đã đọc): tải `wildreceipt.tar` (gồm `class_list.txt`, `train.txt`, `test.txt`, `openset_*`). Đã xác minh (nguồn: WebSearch, paper SDMG-R arXiv 2103.14470 và tài liệu MMOCR): 1.740 ảnh biên lai, 68.975 bbox, 25 lớp (12 key + 12 value + 1 others); MMOCR báo Macro F1 ~0,89 cho `sdmgr_unet16`. License: **chưa xác minh**. ★3.

**DocILE** — https://github.com/rossumai/docile (đã đọc); paper arXiv 2302.05658; https://docile.rossum.ai . Hai track KILE (key info localisation & extraction) và LIR (line item recognition) trên hoá đơn/tài liệu kinh doanh, có tập annotated-trainval, test, unlabeled, synthetic. Truy cập qua **token đăng ký**; repo MIT, license dữ liệu riêng chưa nêu. Số tài liệu (nguồn: WebSearch, paper arXiv 2302.05658): 6.680 tài liệu doanh nghiệp thật có nhãn, ~932K chưa nhãn, 100K tổng hợp có nhãn đầy đủ. Hữu ích nhất cho **line items** của hoá đơn doanh nghiệp. ★4. Hạn chế: tiếng Anh, bố cục phương Tây.

**Kleister NDA / Charity** — https://github.com/applicaai/kleister-nda (đã đọc): 540 NDA (254/83/203) từ SEC EDGAR, thuộc tính effective_date, jurisdiction, party, term; tiếng Anh; license không nêu. Tài liệu dài kiểu hợp đồng, ít liên quan hoá đơn. ★1.

**FUNSD** — dùng trong LayoutLMv3 (README https://github.com/microsoft/unilm/tree/master/layoutlmv3: F1 base 0,9059, large 0,9215). Số mẫu, license của FUNSD: **chưa xác minh** (trang guillaumejaume.github.io không mở được). Form quét tiếng Anh. ★2.

**RVL-CDIP** — 400.000 ảnh tài liệu, 16 lớp (có "invoice") theo mô tả repo thứ cấp; Donut dùng làm benchmark phân loại (95,3). License: **chưa xác minh**; ~100 GB nén chưa tải theo mô tả thứ cấp. Dùng để tiền huấn luyện bộ phân loại loại chứng từ. ★2.

**SynthDoG / Train Ticket (Donut)** — https://github.com/clovaai/donut : công cụ sinh dữ liệu tổng hợp (4 bộ 0,5M en/zh/ja/ko); Donut đạt 91,3 trên CORD, 98,7 Train Ticket. Không có tiếng Việt sẵn; có thể sinh bằng font/văn bản Việt. ★3 (SynthDoG), ★1 (Train Ticket).

**FATURA** — arXiv 2311.11856 ; Zenodo https://zenodo.org/records/10371464 (v2, 13/12/2023, "Dataset Open", một archive ~690,7 MB). Đã xác minh (nguồn: WebSearch): 10.000 hoá đơn, 50 layout (mỗi layout 200 ảnh), 24 lớp đối tượng (bbox + text), annotation ở 3 định dạng (gốc, COCO, tương thích HF Transformers); lớp mất cân bằng. License: **chưa xác minh** (không thấy trong kết quả search). Một nguồn lưu ý nhiều bộ hoá đơn công khai là dữ liệu sinh tổng hợp nên đa dạng nhà cung cấp có thể thiếu. Hữu ích để thử KIE/layout trên hoá đơn doanh nghiệp. ★4.

**XFUND** — ACL 2022 Findings (LayoutXLM). Đã xác minh (nguồn: WebSearch): form có nhãn người ở 7 ngôn ngữ (zh, ja, es, fr, it, de, pt), 2 tác vụ semantic entity recognition và relation extraction; bản mirror HF `FrancophonIA/XFUND` ghi CC BY-NC-SA 4.0 (phi thương mại). Không có tiếng Việt; dùng để thử KIE đa ngôn ngữ. ★2.

**ReceiptSense (trước đây CORU)** — arXiv 2406.04493. Đã xác minh (nguồn: WebSearch): biên lai Ả Rập–Anh (Ả Rập ~53,6%, Anh ~26,2%, lẫn ~20,3% theo một aggregator), 20.000 biên lai annotate, 30.000 ảnh có OCR annotation, 10.000 annotation mức dòng hàng, cộng tập Receipt QA (1.265 ảnh, 40 cặp QA/ảnh); có bản HF `Timokerr/CORU`. License: **chưa xác minh**. Có dòng hàng nên dùng thử line items. ★3.

### (d) Blog / cuộc thi / công ty

- **MC-OCR 2021** (RIVF) và **VinText** (VinAI) ở mục (a). Ghi chú: chưa xác minh bộ hoá đơn công khai từ Shopee/Tiki/Lazada/Zalo/FPT; **FATURA** (Zenodo), **DocILE** (Rossum) ở mục (c) là bộ hoá đơn từ công bố học thuật/công ty.

### Mô hình tham chiếu (chỉ gợi ý, không phải dữ liệu)

| Mô hình | Ghi chú đã xác minh từ README | License |
|---|---|---|
| PaddleOCR (PP-OCRv5/v6, PP-StructureV3, PaddleOCR-VL) | Đa ngôn ngữ (PP-OCRv6: 50 ngôn ngữ trong một mô hình; README không nêu tên Việt) | Apache-2.0 |
| VietOCR | OCR dòng chữ Việt, Transformer/seq2seq | Apache-2.0 |
| Donut | Ảnh -> JSON không OCR, CORD 91,3 | MIT (code) |
| LayoutLMv3 | KIE trên token + layout, FUNSD F1 0,9059/0,9215 | CC BY-NC-SA 4.0 (hạn chế thương mại) |
| VLM đa phương thức | Trích xuất theo schema; kiểm chứng bằng quy tắc | tuỳ nhà cung cấp |

## 5. Dữ liệu tự tạo / synthetic từ chính hệ thống

- **Tạo hoá đơn giả có nhãn chính xác:** sinh HTML/PDF hoá đơn VN từ `products` + `orders/order_items` demo (`SEED_DEMO_DATA=true`): tên shop (`sellers.name`), MST, địa chỉ, dòng hàng (`product_name, qty, price, line_total`), VAT 8%/10%, tổng; render với nhiều font, bố cục (A4, K80 nhiệt, VAT điện tử), thêm nhiễu (xoay, mờ, nhăn, ngược sáng, chụp bằng điện thoại). Nhãn chính xác 100% vì sinh từ DB. Cẩn thận: dữ liệu giả phải gắn nhãn rõ là mẫu, không dùng tên/MST thật (tránh bị hiểu là giả mạo chứng từ).
- **Dùng SynthDoG** với từ điển/tên thương hiệu/sản phẩm tiếng Việt để tăng đa dạng.
- **Bộ vàng thật:** xin ~300–500 hoá đơn từ seller thí điểm **có đồng ý bằng văn bản**, che thông tin nhạy cảm, hai người gán nhãn độc lập, đo đồng thuận; **chia train/valid/test theo seller (không theo ảnh)** để đo khả năng tổng quát sang mẫu hoá đơn lạ.
- **Học từ sửa tay:** lưu cặp (dự đoán, bản sửa) của seller làm dữ liệu active learning.
- Kiểm tra nội tại (không cần nhãn): tổng dòng ≈ subtotal; subtotal + VAT = total; ngày hợp lệ; MST đúng định dạng.

## 6. Lộ trình dùng dữ liệu đề xuất

1. **Bước 1 (dựng pipeline, miễn phí license thương mại rõ):** CORD (CC BY 4.0) + SROIE để thử OCR→KIE, chọn Donut vs. LayoutLMv3 vs. VLM; đo F1 theo trường.
2. **Bước 2 (miền Việt):** xin/xác minh MC-OCR 2021 và UIT-MLReceipts (đọc kỹ điều khoản); VietOCR + PaddleOCR đo CER trên chữ Việt có dấu; VinText để thử detector/recogniser chữ Việt (chỉ nghiên cứu).
3. **Bước 3 (line items):** DocILE (đăng ký token) để thử LIR, sau đó fine-tune trên dữ liệu tổng hợp Việt.
4. **Bước 4:** tự dựng bộ vàng từ seller thí điểm; đo tỉ lệ tự động hoá, tối ưu ngưỡng tin cậy để quyết định khi nào bắt người dùng xác nhận.
5. **Bước 5:** vận hành human-in-the-loop, giám sát drift theo mẫu hoá đơn.

## 7. Rủi ro, license, thiên lệch, lưu ý pháp lý/PII

- **License:** VinText cấm phân phối lại và chỉ nghiên cứu; LayoutLMv3 mã/mô hình theo CC BY-NC-SA 4.0 -> không phù hợp triển khai thương mại nếu không có giấy phép riêng; SROIE/MC-OCR/UIT-MLReceipts/FUNSD/RVL-CDIP/WildReceipt/FATURA/ReceiptSense chưa xác minh license; XFUND CC BY-NC-SA 4.0 (theo mirror); chỉ CORD (CC BY 4.0) và code Donut/PaddleOCR/VietOCR (MIT/Apache) được xác minh rõ.
- **PII/pháp lý:** hoá đơn chứa tên, địa chỉ, số điện thoại, MST, số tài khoản. Cần cơ sở pháp lý và thông báo theo Nghị định 13/2023/NĐ-CP (bảo vệ dữ liệu cá nhân; xác nhận với chuyên gia pháp lý), mã hoá tệp lưu trữ, giới hạn thời hạn lưu, không gửi ảnh cho API bên ngoài khi chưa có thoả thuận xử lý dữ liệu. Hệ thống chưa có `DELETE /users/me/data` (roadmap) -> cần xử lý quyền xoá trước khi thu thập.
- **Thiên lệch:** dữ liệu công khai là biên lai Indonesia/Malaysia/Mỹ; hoá đơn Việt Nam khác (dấu tiếng Việt, định dạng số, hoá đơn VAT, bản in nhiệt mờ).
- **Gian lận:** OCR không xác thực tính thật của chứng từ; không dùng làm căn cứ duy nhất để duyệt tiền. Tránh để mô hình sinh ảnh/hoá đơn giả có thể bị lạm dụng.
- **LLM/VLM:** nguy cơ ảo giác số tiền -> luôn kiểm chứng bằng quy tắc số học.

## 8. Nguồn tham khảo

Đã đọc trực tiếp (GitHub):
- https://github.com/clovaai/cord
- https://github.com/zzzDavid/ICDAR-2019-SROIE
- https://github.com/rossumai/docile
- https://github.com/applicaai/kleister-nda
- https://github.com/VinAIResearch/dict-guided
- https://github.com/pbcquoc/vietocr
- https://github.com/clovaai/donut
- https://github.com/open-mmlab/mmocr/blob/main/docs/en/user_guides/data_prepare/kie.md
- https://github.com/microsoft/unilm/tree/master/layoutlmv3
- https://github.com/PaddlePaddle/PaddleOCR
- https://github.com/WyzanDev/VietReceiptAI (chỉ tên các bộ dữ liệu)

Nhắc qua kết quả công cụ (chưa mở trang gốc): https://docile.rossum.ai ; arXiv 2302.05658 (DocILE, qua README) ; RVL-CDIP qua mô tả repo https://github.com/sambalshikhar/Document-Image-Classification-with-Intra-Domain-Transfer-Learning-and-Stacked-Generalization-of-Deep .

Chưa xác minh: trang cuộc thi/dataset MC-OCR 2021, UIT-MLReceipts, FUNSD, RVL-CDIP chính thức; số liệu và license của WildReceipt, DocILE; hỗ trợ tiếng Việt cụ thể của PaddleOCR.
