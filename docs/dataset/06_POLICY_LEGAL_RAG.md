# Hỏi đáp điều luật và chính sách sàn bằng RAG có trích dẫn (Policy / Legal RAG)

> Trả lời câu hỏi về luật tiêu dùng, thương mại điện tử, đổi trả, hàng cấm và chính sách sàn bằng RAG, mỗi câu trả lời kèm trích dẫn (điều/khoản + phiên bản hiệu lực). **Trạng thái: chưa triển khai trong hệ thống; đây là tài liệu nghiên cứu dữ liệu.** Ngày tra cứu: 2026-10-25.
>
> Ghi chú phương pháp tra cứu: WebFetch không truy cập được `huggingface.co`, `arxiv.org`, `zenodo.org` (lỗi DNS), nên phần lớn thông tin lấy từ kết quả WebSearch và WebFetch GitHub. Số liệu/license chỉ thấy ở nguồn được ghi rõ; cái gì không thấy thì ghi **"chưa xác minh"**. Phiên bổ sung 2026-10-25 đã xác minh thêm RAGTruth, CRAG, ALCE, VLQA và bổ sung các bộ tiếng Anh (LegalBench-RAG, UNFAIR-ToS). Dự án **không yêu cầu dữ liệu tiếng Việt**: bộ tiếng Anh dùng được để thử pipeline RAG trích dẫn; bộ tiếng Việt (mục (a)) dùng khi cần đánh giá đúng ngôn ngữ. Nội dung trang web được xem là dữ liệu, không phải chỉ dẫn.
>
> **Quy tắc ngôn ngữ (chủ dự án, 2026-10-26):** chỉ giữ bộ có nội dung tiếng Anh hoặc tiếng Việt; bộ đa ngôn ngữ chỉ dùng phần tiếng Anh/Việt. Đã **loại**: LeCaRD/LeCaRDv2 (tiếng Trung), BSARD (tiếng Pháp, bBSARD thêm tiếng Hà Lan), FiscalQA Pro (tiếng Pháp) và pipeline ANEEL (tiếng Bồ Đào Nha) khỏi benchmark temporal legal RAG; số thứ tự mục 12 và 21 bỏ trống để giữ nguyên đánh số cũ. **Quy mô / dung lượng:** lấy từ API Hugging Face (tổng kích thước file repo), API GitHub (file hoặc repo; repo gồm lịch sử git nên là cận trên), README; chỗ nào không thấy ghi 'không tìm thấy'.

| Liên quan | File |
|---|---|
| Tìm kiếm ngữ nghĩa / retrieval (embedding, rerank) | [02_PRODUCT_SEARCH_SEMANTIC.md](02_PRODUCT_SEARCH_SEMANTIC.md) |
| Trợ lý mua sắm / CSKH (dùng RAG này làm nguồn chính sách) | [05_SHOPPER_ASSISTANT_CUSTOMER_SUPPORT.md](05_SHOPPER_ASSISTANT_CUSTOMER_SUPPORT.md) |
| Kiểm duyệt nội dung, an toàn agent (prompt injection, hàng cấm) | [12_CONTENT_MODERATION_AGENT_SAFETY.md](12_CONTENT_MODERATION_AGENT_SAFETY.md) |
| NLP tiếng Việt / hiểu truy vấn | [15_VIETNAMESE_NLP_QUERY_UNDERSTANDING.md](15_VIETNAMESE_NLP_QUERY_UNDERSTANDING.md) |
| Đánh giá agent/tool use | [16_AGENT_TOOL_USE_EVAL.md](16_AGENT_TOOL_USE_EVAL.md) |

## 1. Mục tiêu và vị trí trong hệ thống

- **Vai trò hưởng lợi:** buyer ("đổi trả trong bao lâu?", "shop không giao hàng thì sao?"); seller ("mặt hàng nào bị cấm đăng bán?", "nghĩa vụ công khai thông tin"); admin (soạn/duyệt văn bản chính sách, xem câu hỏi không trả lời được).
- **Input:** câu hỏi tiếng Việt tự do, (tuỳ chọn) ngữ cảnh đơn hàng (`order_id`) và **ngày áp dụng** (`as_of`, mặc định hôm nay; với đơn cũ là `orders.created_at`).
- **Output:** câu trả lời ngắn + danh sách trích dẫn `{doc_id, điều, khoản, version_id, hiệu lực từ–đến, đoạn trích}`; hoặc từ chối/chuyển người khi không có căn cứ (abstain). Không đưa tư vấn pháp lý thay luật sư — gắn disclaimer.
- **Dữ liệu của hệ thống nuôi nó** (xem `docs/design/02_DATA_MODEL.md`):
  - Hiện **chưa có** kho văn bản luật/chính sách trong repo; cần corpus mới (đề xuất bảng `policy_documents`, `policy_versions`, `policy_chunks` — chưa tồn tại).
  - Tham số chính sách đang có trong cấu hình: `RETURN_WINDOW_DAYS` (7), `AUTO_DELIVER_DAYS` (7), `ORDER_PAYMENT_TTL_MIN` (15), `SHIPPING_FLAT_FEE`; câu trả lời RAG phải **khớp** các giá trị này (nguồn sự thật cho tham số là config/service, không phải LLM).
  - Ngữ cảnh đơn: `orders` (`status`, `delivered_at`, `payment_method`), máy trạng thái `DELIVERED -> REFUND_REQUESTED` (chỉ trong `RETURN_WINDOW_DAYS`), `products.status = banned` (hàng cấm do admin).
  - Tracking: gateway -> analytics (không qua Kafka). Có thể thêm `event_type` mới (đề xuất, chưa có) cho `policy_question`, `policy_answer_feedback`; chỉ ghi khi `analytics_consent`.
- Luồng gọi phải đi qua gateway (JWT, rate limit); danh tính chỉ lấy từ JWT, không tin `buyer_id` trong body.

## 2. Hướng làm đề xuất (baseline → nâng cao)

1. **Baseline lexical:** BM25 trên chunk theo cấu trúc "Điều" (mỗi điều/khoản là một chunk, giữ metadata `doc_id, điều, khoản, version`); trả về đoạn trích nguyên văn, chưa sinh văn.
2. **Dense + hybrid:** embedding đa ngữ/tiếng Việt (fine-tune trên cặp câu hỏi–điều luật), kết hợp BM25 (RRF), rerank bằng cross-encoder; so sánh với benchmark Zalo/ALQAC (mục 4a).
3. **RAG có trích dẫn:** prompt buộc câu trả lời chỉ dựa trên chunk truy hồi, mỗi mệnh đề có `[n]`; kiểm tra sau sinh (citation verification: mỗi trích dẫn có thực sự entail câu không — kiểu ALCE/NLI).
4. **Quản lý phiên bản theo thời gian:** lưu mỗi điều theo `effective_from/effective_to`; truy vấn có `as_of`; lọc cứng theo hiệu lực trước khi rerank; báo "điều này đã được sửa bởi X" (xem nghiên cứu temporal misgrounding ở 4c, chỉ phần TIDE).
5. **Abstain/hỏi lại:** ngưỡng điểm retrieval + bộ phân loại "có đủ căn cứ không"; câu hỏi ngoài phạm vi chuyển sang CSKH (liên kết 05).
6. **Nâng cao:** multi-hop (kết hợp luật + chính sách sàn), agent gọi tool tra tham số cấu hình, đánh giá faithfulness tự động (RAGTruth/FaithBench-style judge), guard chống prompt injection trong văn bản người dùng nhập (liên kết 12).

## 3. Metric đánh giá

| Tầng | Metric | Ghi chú |
|---|---|---|
| Retrieval | Recall@k, MRR, nDCG@10, **F2** (như Zalo 2021) | F2 thưởng recall hơn precision vì thiếu điều luật nguy hiểm hơn thừa |
| Trích dẫn | Citation precision / recall (ALCE), exact-match điều/khoản | gold là tập `(doc_id, điều)` |
| Faithfulness | Tỉ lệ mệnh đề được hỗ trợ bởi context (Ragas faithfulness, HHEM/NLI), hallucination rate (RAGTruth-style) | cần pin judge model, hiệu chuẩn bằng nhãn người |
| Đúng đáp án | Accuracy cho câu Đúng/Sai (ALQAC), EM/F1 cho span, rubric LLM-judge cho câu mở | |
| Thời gian | Tỉ lệ chọn đúng phiên bản theo `as_of` (version accuracy), tỉ lệ trích dẫn bản hết hiệu lực | metric riêng của dự án |
| An toàn | Abstain precision/recall trên câu không có đáp án, tỉ lệ bị prompt injection | |
| Vận hành | p95 latency, chi phí/câu, tỉ lệ người dùng bấm "không hữu ích" | |

## 4. Bộ dữ liệu

### Bảng tổng hợp

| # | Tên | Nhóm | Ngôn ngữ | Quy mô (số mẫu) | Dung lượng tải/đĩa (nguồn) | License | ★ |
|---|---|---|---|---|---|---|---|
| 1 | [Zalo AI Challenge 2021 – Legal Text Retrieval (bản BEIR trên HF `GreenNode/zalo-ai-legal-text-retrieval-vn`)](https://huggingface.co/datasets/GreenNode/zalo-ai-legal-text-retrieval-vn) | a | vi | ~2.556 query train; corpus ~61 nghìn điều (số lệch giữa nguồn) | `corpus.jsonl` 115 MB + `queries.jsonl` 0,55 MB (HF API); cả repo gồm bản parquet ~265 MB | MIT (thẻ HF qua API) | 5 |
| 2 | [ALQAC 2021–2025 (Automated Legal QA Competition)](https://sites.google.com/view/ALQAC-2025) | a | vi | chưa xác minh kích thước từng năm | không tìm thấy | điều kiện cuộc thi; chưa xác minh | 5 |
| 3 | [VLQA (arXiv 2507.19995) / VLSP 2025 DRiLL](https://arxiv.org/html/2507.19995v1) | a | vi | 3.129 câu hỏi do chuyên gia gán nhãn (theo bài VLSP 2025 trích; chưa kiểm ở bài gốc) | không tìm thấy | chưa xác minh | 4 |
| 4 | [CATI-AI `vietnamese-legal-retrieval-with-negatives`](https://huggingface.co/datasets/CATI-AI/vietnamese-legal-retrieval-with-negatives) | a | vi | ~144 nghìn dòng (theo kết quả tìm kiếm) | không tìm thấy (API HF trả 401) | chưa xác minh | 3 |
| 5 | [NghiemAbe/dev_legal](https://huggingface.co/datasets/NghiemAbe/dev_legal) | a | vi | 1K–10K dòng; parquet | 4,8 MB (HF API) | chưa xác minh | 2 |
| 6 | [thangvip/legaldocuments-nli-test-v3](https://huggingface.co/datasets/thangvip/legaldocuments-nli-test-v3) | a | vi | nhỏ (NLI test) | 0,55 MB (HF API) | chưa xác minh | 2 |
| 7 | [AITeamVN/EvalRAGData](https://huggingface.co/datasets/AITeamVN/EvalRAGData) | a | vi | 300 mẫu, người viết | 1,4 MB (HF API) | Apache-2.0 (thẻ HF) | 3 |
| 8 | [ViRE (Vietnamese IR multi-domain, có EduCoQA, CSConDa)](https://github.com/longstnguyen/ViRE) | a | vi | chưa xác minh | repo ~58 MB (API GitHub, gồm lịch sử git) | chưa xác minh | 3 |
| 9 | [Văn bản luật VN thật (Luật BVQLNTD 2023, NĐ 52/2013, NĐ 85/2021…)](https://english.luatvietnam.vn/decree-no-52-2013-nd-cp-dated-may-16-2013-of-the-government-on-e-commerce-78550-doc1.html) | a | vi | tự thu thập (3–5 văn bản dự kiến) | nhỏ: mỗi văn bản vài trăm KB văn bản thuần (ước tính) | **bản quyền/điều khoản của nguồn: chưa xác minh** | 5 |
| 10 | [LegalBench (+ CUAD tasks)](https://huggingface.co/datasets/nguha/legalbench) | b | en | nhiều task (tổng số mẫu: không tìm thấy) | 82 MB tổng file (HF API) | CC BY 4.0 (thẻ HF); CUAD task CC BY 4.0; theo từng task | 2 |
| 11 | [COLIEE](https://sites.ualberta.ca/%7erabelo/COLIEE2022/COLIEE2021_summary.pdf) | c | en (case law Canada); nhánh luật Nhật chỉ dùng nếu có bản tiếng Anh | chưa xác minh | không tìm thấy | cần đăng ký (chưa xác minh) | 1 |
| 13 | [BEIR](https://arxiv.org/pdf/2104.08663) | c | en | 18 dataset công khai | tổng: không tìm thấy; từng bộ khác nhau (ví dụ ArguAna ~12 MB, HF API) | theo từng dataset | 3 |
| 14 | [RAGTruth](https://github.com/particlemedia/RAGTruth) | c | en | 2.965 instance, 17.790 phản hồi, 7.664 phản hồi ảo giác, 14.289 span (đã xác minh, GitHub) | ~36,6 MB: `response.jsonl` 21,5 MB + `source_info.jsonl` 15,1 MB (API GitHub) | MIT (đã xác minh, GitHub) | 4 |
| 15 | [CRAG (Meta, KDD Cup 2024)](https://github.com/facebookresearch/CRAG) | c | en | 4.409 cặp QA, 5 miền, 8 loại câu (số QA theo bài báo) | không tìm thấy (docs chỉ liệt kê file `.jsonl.bz2`, không nêu dung lượng) | CC BY-NC 4.0 (đã xác minh, GitHub; phi thương mại) | 3 |
| 16 | [FRAMES (Google)](https://huggingface.co/datasets/google/frames-benchmark) | c | en | 824 câu multi-hop | 0,48 MB (`test.tsv`, HF API) | Apache-2.0 (thẻ HF qua API) | 3 |
| 17 | [ALCE (ASQA, QAMPARI, ELI5)](https://github.com/princeton-nlp/ALCE) | c | en | 3 dataset; tải bằng `download_data.sh` | dữ liệu: không tìm thấy; chỉ mục GTR Wikipedia ~31 GB (tuỳ chọn) | repo MIT (đã xác minh, GitHub); license từng dataset chưa xác minh | 4 |
| 18 | [FaithBench / Vectara HHEM](https://arxiv.org/pdf/2410.13210) | c | en | tóm tắt của 10 LLM, có nhãn người (số mẫu: không tìm thấy) | không tìm thấy (API HF trả 401) | chưa xác minh | 3 |
| 19 | [Benchmark temporal legal RAG – chỉ phần TIDE](https://arxiv.org/pdf/2608.09393) | c | chưa xác minh (văn bản hải quan Bangladesh; khả năng cao là tiếng Anh) | 3.050 QA / 644 văn bản (TIDE) | không tìm thấy | chưa xác minh | 3 |
| 20 | [Ragas (thư viện metric)](https://github.com/explodinggradients/ragas) | d | – | – | – (thư viện, không phải dataset) | chưa xác minh | 3 |
| 22 | [LegalBench-RAG](https://arxiv.org/pdf/2408.10343) | b | en | 6.858 (blog) hoặc 6.889 (bảng bài báo) query; 714 tài liệu; bản mini 776 query | không tìm thấy | theo các nguồn con; chưa xác minh | 3 |
| 23 | [UNFAIR-ToS / CLAUDETTE (qua LexGLUE, LegalBench)](https://hazyresearch.stanford.edu/legalbench/tasks/unfair_tos.html) | b | en | 50 hợp đồng ToS (YouTube, eBay, Facebook…); bản LegalBench 3.850 mẫu | không tìm thấy (nhỏ) | CC BY 4.0 (theo trang LegalBench) | 3 |

### (a) Tiếng Việt / Việt Nam

**1. Zalo AI Challenge 2021 – Legal Text Retrieval**
- Link: bản BEIR-format `GreenNode/zalo-ai-legal-text-retrieval-vn` trên Hugging Face (tên lấy từ repo GitHub [Messiac24/vietnamese-legal-retrieval-khong-dau](https://github.com/Messiac24/vietnamese-legal-retrieval-khong-dau); trang HF chưa mở được); dữ liệu cũng được nói là có trên Kaggle (chưa xác minh). Tin tức cuộc thi: [znews](https://znews.vn/di-len-tu-that-bai-quan-quan-zalo-ai-challenge-2021-la-ai-post1286490.html).
- Nguồn: Zalo AI Challenge 2021 (ba thử thách: 5K Compliance, Hum to Song, Legal Text Retrieval). Đội nhất 3PU: F2 = 0.64804.
- Kích thước: ~2.556 query train, corpus ~61 nghìn điều (một bảng ghi 61.060, bảng khác 61.425); dung lượng: `corpus.jsonl` 115 MB + `queries.jsonl` 0,55 MB (HF API, đã mở được qua API dù WebFetch không mở trang). Ngôn ngữ vi. License: MIT (thẻ HF qua API).
- Cột/nhãn: query (câu hỏi) – danh sách điều luật liên quan (`law_id`, `article_id`) – corpus văn bản luật theo điều. Tên file `legal_corpus.json`/`train_question_answer.json`: **chưa xác minh**.
- Dùng vào đâu: huấn luyện/đánh giá retriever pháp lý tiếng Việt (bước 2 mục 2); chuẩn hoá schema chunk theo "Điều" cho kho `policy_chunks` của ta.
- ★5. Hạn chế: một dự án cá nhân báo có 24 câu hỏi xuất hiện ở cả train và test với nhãn khác nhau (chưa xác minh độc lập) — tự kiểm tra trùng lặp trước khi chia tập; miền luật chung, không phải TMĐT; không có trường phiên bản.

**2. ALQAC (Automated Legal Question Answering Competition)**
- Link: [ALQAC 2021 summary (arXiv 2204.10717)](https://ar5iv.labs.arxiv.org/html/2204.10717), [ALQAC 2022](https://kse2022.tbd.edu.vn/?p=726), [2023](https://sites.google.com/view/ALQAC2023), [2024](https://sites.google.com/view/alqac-2024), [2025](https://sites.google.com/view/ALQAC-2025), [2026](https://sites.google.com/view/alqac2026).
- Nguồn: thi thường niên gắn với hội nghị KSE; mô phỏng COLIEE cho tiếng Việt. 2021 có 3 task (retrieval, entailment, QA); các năm sau còn Document Retrieval và Legal QA. Có câu Đúng/Sai và factoid (2022). Luật 2025 cấm dùng dataset QA pháp lý gán nhãn ngoài.
- Kích thước/license: không thấy trong kết quả — **chưa xác minh**; dung lượng: không tìm thấy; thường cần đăng ký đội để tải.
- Cột/nhãn: id câu hỏi, văn bản câu hỏi, điều liên quan (law ID + article ID), nhãn Đúng/Sai hoặc span đáp án.
- Dùng vào đâu: mẫu chuẩn để dựng bộ vàng của ta (định dạng gold article + nhãn yes/no); một bài báo 2026 (arXiv 2609.27376) tái dùng 80 bản ghi luật lao động của ALQAC 2021 làm benchmark citation.
- ★5. Hạn chế: luật thành văn tổng quát (dân sự, lao động…), ít TMĐT; điều khoản sử dụng phụ thuộc năm.

**3. VLQA và VLSP 2025 DRiLL**
- Link: [VLQA arXiv 2507.19995](https://www.emergentmind.com/papers/2507.19995); DRiLL là shared task VLSP 2025 dựng từ VLQA (hơn 50 đội, 1.255 bài nộp theo bài tổng quan).
- Quy mô: 3.129 câu hỏi chuyên gia gán nhãn (số lấy từ bài VLSP 2025 trích dẫn VLQA; chưa kiểm ở bài gốc). Bài VLQA đăng arXiv ngày 26/7/2025 (Nguyen và cs.). Câu hỏi lấy từ nền tảng tư vấn pháp luật công cộng; DRiLL dựng từ VLQA.
- License/link tải: **chưa xác minh** (tìm không thấy trang tải hay license); dung lượng: không tìm thấy.
- Dùng: bộ câu hỏi đời thường (giọng người dùng) — gần với cách buyer hỏi hơn là câu hỏi dạng luật; đánh giá retrieval + QA.
- ★4. Hạn chế: chưa thấy trang dữ liệu chính thức trong phiên.

**4. CATI-AI/vietnamese-legal-retrieval-with-negatives**
- Link: [tổ chức CATI-AI trên HF](https://huggingface.co/CATI-AI). Quy mô ~144 nghìn dòng (kết quả tìm kiếm; không thấy dataset card); dung lượng: không tìm thấy (API HF trả 401). Dùng: cặp query–passage kèm hard negative để fine-tune embedding. ★3. Hạn chế: schema/license/nguồn gốc dữ liệu chưa xác minh; có thể dựng từ Zalo (nguy cơ lẫn tập test).

**5–6. NghiemAbe/dev_legal, thangvip/legaldocuments-nli-test-v3**
- [dev_legal](https://huggingface.co/datasets/NghiemAbe/dev_legal/tree/df919379d19e7a6406320c824d12be0c9541f321): dataset text retrieval tiếng Việt, 1K–10K dòng, parquet, 4,8 MB (HF API). [legaldocuments-nli-test-v3](https://huggingface.co/datasets/thangvip/legaldocuments-nli-test-v3/tree/main): NLI pháp lý nhỏ (0,55 MB, HF API), dùng thử bộ kiểm tra entailment cho citation verification. ★2 mỗi bộ. Hạn chế: ít tài liệu, license chưa xác minh.

**7. AITeamVN/EvalRAGData**
- Link: [README](https://huggingface.co/datasets/AITeamVN/EvalRAGData/raw/e4e9b75cf3506ada4c8afac76207b801359068a3/README.md). 300 mẫu do người viết, 1,4 MB (HF API), Apache-2.0, đánh giá QA dựa trên context tiếng Việt. Dùng: smoke test khả năng "trả lời chỉ từ context" của LLM tiếng Việt. ★3. Hạn chế: nhỏ, không phải pháp lý.

**8. ViRE**
- Link: [bài EACL 2026 Findings](https://preview.aclanthology.org/new-sigs/2026.findings-eacl.110/), mã/dữ liệu GitHub `longstnguyen/ViRE`. Benchmark IR tiếng Việt đa miền (giáo dục, pháp lý, y tế, CSKH), so sánh lexical/dense/hybrid; tặng thêm EduCoQA, CSConDa (hội thoại tư vấn/CSKH). Dùng: so sánh BM25 vs dense trên tiếng Việt; phần CSKH liên quan scope 05. ★3. Kích thước/license: chưa xác minh; repo ~58 MB (API GitHub, gồm lịch sử git).

**9. Văn bản luật Việt Nam thật (tự thu thập)**
- Văn bản liên quan: Nghị định 52/2013/NĐ-CP về TMĐT (ban hành 16/5/2013, hiệu lực 1/7/2013; sửa đổi bởi NĐ 85/2021/NĐ-CP) — [bản tiếng Anh trên luatvietnam](https://english.luatvietnam.vn/decree-no-52-2013-nd-cp-dated-may-16-2013-of-the-government-on-e-commerce-78550-doc1.html), [thuvienphapluat](https://thuvienphapluat.vn/van-ban/thuong-mai/decree-no-52-2013-nd-cp-on-e-commerce-190231.aspx). Một bài của hãng luật nói quyền người tiêu dùng nay nằm ở Luật Bảo vệ quyền lợi người tiêu dùng 2023 (nguồn thứ cấp; cần đối chiếu văn bản gốc). Trạng thái hiệu lực của NĐ 52 khác nhau giữa các CSDL ("còn hiệu lực" vs "hết hiệu lực một phần") — chính là ví dụ cần quản lý phiên bản.
- Phiên này **không tra được điều khoản sử dụng/bản quyền của vbpl.vn** hay thuvienphapluat (tìm kiếm không trả về trang liên quan). Thuvienphapluat là site thương mại, thường cấm thu thập hàng loạt; cần đọc điều khoản trước khi crawl. Nguyên tắc chung (cần xác minh pháp lý): văn bản quy phạm pháp luật do cơ quan nhà nước ban hành không thuộc đối tượng bảo hộ bản quyền, nhưng bản biên tập/chú thích của site thương mại có thể.
- Quy mô/dung lượng: 3–5 văn bản, mỗi văn bản vài trăm KB văn bản thuần (ước tính, chưa đo).
- ★5 (đúng miền) nhưng chỉ dùng sau khi xác minh nguồn; khuyến nghị lấy từ cổng chính thức và lưu URL + ngày thu thập.

### (b) TMĐT / pháp lý công khai nước ngoài

**10. LegalBench / CUAD.** [LegalBench (arXiv 2308.11462)](https://arxiv.org/pdf/2308.11462), [HF `nguha/legalbench`](https://huggingface.co/datasets/nguha/legalbench): benchmark lập luận pháp lý tiếng Anh (82 MB tổng file repo HF, theo API; thẻ HF ghi CC BY 4.0), license có thể khác theo task; các task CUAD là CC BY 4.0 (Atticus Project). Dùng: tham chiếu cách thiết kế task (rule recall, interpretation) khi dựng test của ta; không dùng trực tiếp cho tiếng Việt. ★2. Trang gốc CUAD: chưa mở trong phiên.

**22. LegalBench-RAG.** [arXiv 2408.10343](https://arxiv.org/pdf/2408.10343), blog [ZeroEntropy](https://www.zeroentropy.dev/articles/legalbench-rag-the-first-open-source-retrieval-benchmark-for-the-legal-domain/). Đã xác minh qua WebSearch: benchmark retrieval cho RAG pháp lý, ghép từ 4 nguồn: ContractNLI (977 query), MAUD (1.676), CUAD (4.042), PrivacyQA (194) — tổng 6.889 query theo bảng bài báo (blog ghi 6.858; số lệch, kiểm lại trước khi trích), 714 tài liệu; bản LegalBench-RAG-mini 776 query/72 tài liệu; chấm retrieval ở mức đoạn ký tự (precise snippet). PrivacyQA (chính sách quyền riêng tư của công ty tiêu dùng) gần nhất với văn bản chính sách sàn. Mỗi query chỉ có đáp án trong đúng một tài liệu (không test multi-doc). Repo/license: **chưa xác minh** (không thấy URL repo trong kết quả). ★3.

**23. UNFAIR-ToS / CLAUDETTE.** [Lippi và cs., Artificial Intelligence and Law 2019, arXiv 1805.01217](https://arxiv.org/pdf/1805.01217); [trang LegalBench](https://hazyresearch.stanford.edu/legalbench/tasks/unfair_tos.html). Đã xác minh qua WebSearch: 50 hợp đồng điều khoản dịch vụ trực tuyến (YouTube, eBay, Facebook…) gán nhãn mức câu theo luật tiêu dùng EU, 8 loại điều khoản có thể bất công (arbitration, unilateral change, content removal, jurisdiction, choice of law, limitation of liability, unilateral termination, contract by using) + other; bản LegalBench 3.850 mẫu, license CC BY 4.0 theo trang đó (license LexGLUE chưa xác minh). Các corpus CLAUDETTE khác (100–142 ToS; 9.414 điều khoản) là bộ riêng. Dùng: kiểm tra điều khoản "Quy chế sàn" tự soạn có điều khoản bất lợi người tiêu dùng không. ★3. Hạn chế: ToS nền tảng lớn, không phải chính sách đổi trả.

### (c) Benchmark / bài báo học thuật

**11. COLIEE.** Cuộc thi Canada/Nhật (case law, statute law); chỉ dùng phần tiếng Anh (án lệ Canada; nhánh luật Nhật chỉ khi có bản dịch tiếng Anh), số mẫu và dung lượng: không tìm thấy. Tóm tắt [COLIEE 2021](https://sites.ualberta.ca/%7erabelo/COLIEE2022/COLIEE2021_summary.pdf). Nhắc tới corpus Federal Court of Canada do Compass Law cung cấp (license chưa xác minh; cần đăng ký). Nguồn cảm hứng thiết kế task retrieval + entailment. ★1.

**13. BEIR.** [arXiv 2104.08663](https://arxiv.org/pdf/2104.08663): 18 dataset zero-shot; BM25 là baseline mạnh. Dùng làm tham chiếu cho quy trình đánh giá retrieval và để kiểm tra mô hình embedding không bị overfit miền. ★3.

**14. RAGTruth.** [arXiv 2401.00396](https://arxiv.org/pdf/2401.00396); mã tại GitHub `particlemedia/ragtruth`. Theo README GitHub (đã xác minh; dung lượng ~36,6 MB: `response.jsonl` 21,5 MB + `source_info.jsonl` 15,1 MB, API GitHub): 2.965 instance và 17.790 phản hồi (7.664 phản hồi ảo giác, 14.289 span ảo giác) từ 6 mô hình (GPT-3.5/4, Llama-2 7B/13B/70B-chat, Mistral-7B-Instruct) trên 3 tác vụ (QA, data-to-text, tóm tắt CNN/DM và Recent News); người gán nhãn ở mức câu và mức từ kèm mức độ nghiêm trọng; fine-tune mô hình nhỏ cạnh tranh với GPT-4 prompt. Dùng: huấn luyện/hiệu chuẩn bộ phát hiện hallucination; mẫu schema để ta gán nhãn trả lời RAG của mình. ★4. License MIT (README GitHub). Hạn chế: tiếng Anh.

**15. CRAG.** [arXiv 2406.04744](https://arxiv.org/pdf/2406.04744), mã/dữ liệu `facebookresearch/CRAG` (dung lượng file dữ liệu: không tìm thấy), cuộc thi trên [AIcrowd](https://aicrowd.com/challenges/meta-comprehensive-rag-benchmark-kdd-cup-2024). 4.409 QA, 5 miền, 8 loại câu hỏi, có trường `query time` (README GitHub xác nhận 5 miền/8 loại câu, license CC BY-NC 4.0 — phi thương mại; số 4.409 theo bài báo); RAG công nghiệp chỉ trả lời 63% không hallucination. Dùng: mẫu phân loại câu hỏi và chấm "missing/hallucination" (đúng +1, bỏ qua 0, sai −1). ★3.

**16. FRAMES.** [arXiv 2409.12941](https://arxiv.org/html/2409.12941v3), HF `google/frames-benchmark` (0,48 MB, Apache-2.0 theo API HF). 824 câu multi-hop trên 2–15 bài Wikipedia; không retrieval ~0,40, pipeline nhiều bước ~0,66. Dùng: ý tưởng test multi-hop (luật + chính sách sàn). ★3.

**17. ALCE.** [arXiv 2305.14627](https://arxiv.org/abs/2305.14627v2), mã [`fallcat/ALCE`](https://github.com/fallcat/ALCE). Repo MIT (GitHub `princeton-nlp/ALCE`, license từng dataset chưa xác minh); tải dữ liệu kèm kết quả retrieval top-100 (DPR/GTR cho ASQA, QAMPARI; BM25 cho ELI5) bằng `download_data.sh`; chỉ mục GTR Wikipedia ~31GB (tuỳ chọn). Chấm citation recall/precision bằng NLI (TRUE T5-11B). Dung lượng dữ liệu: không tìm thấy (repo mã ~1,7 MB, API GitHub). Dùng: định nghĩa metric trích dẫn chuẩn. ★4.

**18. FaithBench / Vectara HHEM / FaithJudge.** [FaithBench arXiv 2410.13210](https://arxiv.org/pdf/2410.13210): benchmark hallucination khi tóm tắt, nhãn gồm consistent/questionable/benign/unfaithful. Dùng: hiệu chuẩn judge faithfulness; ★3. HaluEval và FACTS Grounding: **không tìm thấy trang trong phiên** nên không liệt kê.

**19. Temporal legal RAG (chỉ phần TIDE).** Bài [arXiv 2608.09393](https://arxiv.org/pdf/2608.09393) gồm nhiều benchmark; theo quy tắc ngôn ngữ **chỉ giữ TIDE**: 3.050 QA/644 văn bản hải quan Bangladesh 1969–2025 (nhận diện phiên bản chi phối 26,7%); ngôn ngữ văn bản: chưa xác minh (khả năng cao là tiếng Anh); dung lượng: không tìm thấy. Đã bỏ FiscalQA Pro (luật thuế Pháp, tiếng Pháp) và pipeline ANEEL (Brazil, tiếng Bồ Đào Nha). Dùng: mẫu thiết kế bộ test "as_of" cho văn bản đổi phiên bản; là preprint chưa kiểm chứng phản biện. ★3.

### (d) Blog / công cụ

**20. Ragas.** Công cụ metric faithfulness / answer relevancy / context precision dùng LLM judge. Phiên này chỉ thấy bài viết bên thứ ba, không thấy trang tài liệu chính thức; API thay đổi theo phiên bản, cần ghim version. Lưu ý các metric không kiểm chứng sự thật của chính context. ★3.

## 5. Dữ liệu tự tạo / synthetic từ chính hệ thống

Dự án định tự soạn văn bản luật MÔ PHỎNG (đặt tên rõ là mô phỏng, không dùng số hiệu văn bản thật để tránh nhầm lẫn là luật thật).

**5.1 Dựng corpus mô phỏng**
- 3–5 văn bản (ví dụ "Quy chế sàn", "Chính sách đổi trả", "Danh mục hàng cấm", "Quy định thanh toán mô phỏng", "Luật TMĐT mô phỏng"), cấu trúc Chương/Điều/Khoản/Điểm, mỗi điều có `doc_id, article_id, version_id, effective_from, effective_to, amended_by, text`.
- Tạo **ít nhất 2–3 phiên bản** cho 30–40% điều (ví dụ đổi `RETURN_WINDOW_DAYS` từ 7 lên 15 ngày kể từ ngày X; thêm mặt hàng cấm) để có câu hỏi phụ thuộc thời gian. Giá trị trong văn bản phải khớp với config thực của service tại thời điểm tương ứng (có test đối chiếu).
- Cài vài cái bẫy có chủ ý: điều tham chiếu chéo ("theo Điều 12"), ngoại lệ ("trừ hàng tươi sống"), điều khoản mâu thuẫn giữa văn bản chung và quy chế sàn (ưu tiên văn bản cấp cao hơn).

**5.2 Dựng bộ câu hỏi vàng (gold set)**
1. Sinh nháp bằng LLM từ từng điều (3–5 câu/điều), giọng buyer/seller thật (không dấu, viết tắt, lẫn từ ngữ đơn hàng); **người duyệt** 100% với bộ test (≥ 300 câu) và lấy mẫu ≥ 20% bộ dev.
2. Mỗi bản ghi: `question, as_of, gold_articles[(doc,article,version)], gold_answer, must_cite (bool), answerable (bool), type, role`.
3. Phân bố loại câu hỏi: đơn điều (40%), nhiều điều/multi-hop (15%), **phụ thuộc phiên bản** (15%), không có đáp án (10%), tham số theo đơn hàng cần tool (`orders.delivered_at` + `RETURN_WINDOW_DAYS`) (10%), đối kháng/prompt injection hoặc yêu cầu tư vấn ngoài phạm vi (10%).
4. Thêm câu hỏi thật sau khi chạy thử: lấy từ log câu hỏi CSKH (đã ẩn danh, có consent) để cập nhật bộ vàng theo vòng.
5. Câu "Đúng/Sai" theo kiểu ALQAC để chấm accuracy rẻ.

**5.3 Chia tập**
- Chia **theo điều/văn bản**, không theo câu hỏi, để tránh rò rỉ (cùng điều không xuất hiện ở train và test); riêng nhóm phụ thuộc phiên bản chia theo `version_id`: phiên bản cũ vào dev, phiên bản mới nhất vào test để thử khả năng tổng quát.
- Giữ một bộ "canary" cố định không dùng để tinh chỉnh prompt; ghi hash bộ test trong repo.
- Trộn thêm test ngoài (Zalo/ALQAC) để đo retriever không overfit corpus mô phỏng.

## 6. Lộ trình dùng dữ liệu đề xuất

1. **Tuần 1–2:** dựng pipeline retrieval và đo trên Zalo 2021 (kiểm tra trùng train/test trước) — chọn embedding/reranker; so BM25/dense/hybrid theo ViRE.
2. **Tuần 2–3:** soạn corpus mô phỏng + 150 câu vàng đầu tiên; chạy baseline BM25 -> hybrid; đo Recall@k, F2.
3. **Tuần 3–4:** RAG sinh có trích dẫn; chấm bằng ALCE-style citation + judge faithfulness (hiệu chuẩn với RAGTruth/FaithBench rồi với nhãn tay ~100 mẫu của ta).
4. **Tuần 4–5:** versioning và `as_of` (thiết kế test theo TIDE); thêm abstain.
5. **Sau đó:** tích hợp tool tra config/đơn hàng, red-team prompt injection (liên kết 12), vòng cải tiến bằng feedback thật.

## 7. Rủi ro, license, thiên lệch, lưu ý pháp lý/PII

- **Bản quyền nguồn luật:** chưa xác minh điều khoản của vbpl.vn/thuvienphapluat; ưu tiên cổng chính thức, ghi nguồn + ngày; không phân phối lại bản biên tập có thương hiệu.
- **Không phải tư vấn pháp lý:** gắn disclaimer, ưu tiên trích nguyên văn, tránh kết luận chắc chắn; rủi ro trách nhiệm nếu hệ thống trả lời sai về quyền đổi trả/hoàn tiền.
- **Hiệu lực văn bản:** luật VN thường xuyên sửa đổi/bãi bỏ; corpus phải có quy trình cập nhật và cảnh báo bản hết hiệu lực.
- **Thiên lệch bộ dữ liệu:** Zalo/ALQAC thiên về luật dân sự/lao động, không phải TMĐT; VLQA lấy từ nền tảng tư vấn công cộng (có thể lệch chủ đề).
- **Rò rỉ:** dataset retrieval trên HF có thể dựng lại từ Zalo -> nguy cơ test lẫn train; kiểm tra bằng hash văn bản.
- **LLM judge:** thiên lệch và phụ thuộc phiên bản judge; ghim model + hiệu chuẩn người.
- **PII:** câu hỏi thật của người dùng có thể chứa tên/SĐT/địa chỉ; ẩn danh trước khi đưa vào bộ vàng; chỉ dùng log khi có `analytics_consent`. Thanh toán trong hệ thống là mô phỏng — câu trả lời không được hứa hẹn quyền lợi tài chính thật.
- **Prompt injection:** văn bản do seller nhập (mô tả, chính sách shop) không được đưa vào kho RAG mà không lọc/gắn nhãn nguồn.

## 8. Nguồn tham khảo (URL)

- Zalo AI Challenge 2021 (nguồn thứ cấp): https://github.com/Messiac24/vietnamese-legal-retrieval-khong-dau ; https://znews.vn/di-len-tu-that-bai-quan-quan-zalo-ai-challenge-2021-la-ai-post1286490.html ; trang HF `GreenNode/zalo-ai-legal-text-retrieval-vn`: chưa xác minh
- ALQAC: https://ar5iv.labs.arxiv.org/html/2204.10717 ; https://kse2022.tbd.edu.vn/?p=726 ; https://sites.google.com/view/ALQAC2023 ; https://sites.google.com/view/alqac-2024 ; https://sites.google.com/view/ALQAC-2025 ; https://sites.google.com/view/alqac2026
- VLQA: https://www.emergentmind.com/papers/2507.19995 ; labour-law QA: https://arxiv.org/pdf/2609.27376 ; đánh giá LLM trên văn bản pháp luật VN: https://arxiv.org/pdf/2604.16270
- HF: https://huggingface.co/CATI-AI ; https://huggingface.co/datasets/NghiemAbe/dev_legal/tree/df919379d19e7a6406320c824d12be0c9541f321 ; https://huggingface.co/datasets/thangvip/legaldocuments-nli-test-v3/tree/main ; https://huggingface.co/datasets/AITeamVN/EvalRAGData/raw/e4e9b75cf3506ada4c8afac76207b801359068a3/README.md
- ViRE: https://preview.aclanthology.org/new-sigs/2026.findings-eacl.110/
- NĐ 52/2013: https://english.luatvietnam.vn/decree-no-52-2013-nd-cp-dated-may-16-2013-of-the-government-on-e-commerce-78550-doc1.html ; https://thuvienphapluat.vn/van-ban/thuong-mai/decree-no-52-2013-nd-cp-on-e-commerce-190231.aspx
- LegalBench: https://arxiv.org/pdf/2308.11462 ; https://huggingface.co/datasets/nguha/legalbench ; COLIEE 2021: https://sites.ualberta.ca/%7erabelo/COLIEE2022/COLIEE2021_summary.pdf
- BEIR: https://arxiv.org/pdf/2104.08663
- RAGTruth: https://arxiv.org/pdf/2401.00396 ; CRAG: https://arxiv.org/pdf/2406.04744 ; https://aicrowd.com/challenges/meta-comprehensive-rag-benchmark-kdd-cup-2024 ; FRAMES: https://arxiv.org/html/2409.12941v3 ; ALCE: https://arxiv.org/abs/2305.14627v2 ; https://github.com/fallcat/ALCE ; FaithBench: https://arxiv.org/pdf/2410.13210
- Temporal legal RAG: https://arxiv.org/pdf/2608.09393 ; https://arxiv.org/pdf/2608.08512
- Ragas (chỉ nguồn bên thứ ba): https://docs.datadoghq.com/llm_observability/ragas_evaluations — tài liệu chính thức: chưa xác minh
- Phiên bổ sung (2026-10-25): https://github.com/particlemedia/RAGTruth ; https://github.com/facebookresearch/CRAG ; https://github.com/princeton-nlp/ALCE  ; https://arxiv.org/pdf/2408.10343 (LegalBench-RAG) ; https://arxiv.org/pdf/1805.01217 (CLAUDETTE) ; https://arxiv.org/html/2507.19995v1 (VLQA)
- Không tìm thấy trong phiên: ViRAG (không có dataset tên này), HaluEval, FACTS Grounding, trang gốc CUAD. Chưa xác minh được: kích thước/license ALQAC, CATI-AI, FaithBench/COLIEE license; dung lượng ALQAC, VLQA, CATI-AI, CRAG, FaithBench, TIDE, LegalBench-RAG, UNFAIR-ToS: không tìm thấy.
