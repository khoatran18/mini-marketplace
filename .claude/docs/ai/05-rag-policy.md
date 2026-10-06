# 05 · Hỏi đáp điều luật & chính sách sản phẩm (RAG)

> Cảnh báo: đây là **công cụ tra cứu thông tin**, không phải tư vấn pháp lý. Mọi câu trả lời phải kèm trích dẫn nguồn, ngày hiệu lực và dòng miễn trừ. Danh sách văn bản mẫu bên dưới chỉ là *gợi ý nguồn cần nạp* – chủ dự án/pháp chế phải xác minh văn bản và hiệu lực hiện hành trước khi dùng thật.

## 1. Phạm vi tri thức
| Nhóm (`kind`) | Ví dụ | Đối tượng hỏi | Ghi chú |
|---|---|---|---|
| `law` | Luật Bảo vệ quyền lợi người tiêu dùng; Luật Giao dịch điện tử; quy định về thương mại điện tử; quy định ghi nhãn hàng hoá; an toàn thực phẩm; quảng cáo; bảo vệ dữ liệu cá nhân | buyer, seller | văn bản nhà nước, có số hiệu, ngày hiệu lực, **bị sửa đổi/thay thế theo thời gian** |
| `platform_policy` | Điều khoản sử dụng, chính sách đổi trả/hoàn tiền, phí, vận chuyển, bảo mật | buyer, seller | do sàn soạn → admin sở hữu |
| `category_rule` | Quy định hàng cấm/hạn chế, yêu cầu chứng nhận theo ngành (mỹ phẩm, thực phẩm, điện tử…) | seller (đăng bán), admin (kiểm duyệt) | gắn `category_id` |
| `faq` | Câu hỏi thường gặp | buyer | ngắn, trả lời nhanh |

## 2. Mô hình dữ liệu (đã có ở 02 §5)
- `policy_documents`: metadata + `status` + `effective_from/to` + `sha256` (chống nạp trùng).
- `policy_chunks`: văn bản đã tách + `heading_path` (vd. `Chương II > Điều 10 > Khoản 2`) + `embedding` + `tsv` (full-text) + `metadata` (`jurisdiction`, `kind`, `category_id`, `article`, `doc_number`).
- Phiên bản: văn bản mới thay thế cũ ⇒ cũ `status=retired` + `effective_to`; chunk cũ **không xoá** (cần cho câu hỏi "tại thời điểm X").

## 3. Pipeline nạp (ingestion)
1. Admin upload (PDF/DOCX/MD/HTML) tại `/admin/policies` ⇒ lưu file, tính `sha256`, tạo bản ghi `pending`.
2. `Ingest()` (ai-service – **bạn code**): trích text (PDF có thể cần OCR) → chuẩn hoá → **tách theo cấu trúc pháp lý** (Chương/Điều/Khoản/Điểm) thay vì cắt cố định → chunk 200–500 token, mỗi chunk giữ tiêu đề cha → embed → ghi pgvector → `indexed`.
3. Kiểm duyệt của admin: xem trước chunk, sửa metadata, bấm "Xuất bản". Chỉ `indexed` + được xuất bản mới được truy vấn.
4. Re-index khi đổi embedding model (cột `model` trong chunk, chạy nền, chuyển đổi nguyên tử bằng `active_index_version`).

## 4. Truy hồi (retrieval) – hợp đồng
`PolicyRetriever.Retrieve(question, scope, k, as_of?) → [{chunk_id, text, heading_path, doc_title, doc_number, article, effective_from, url, score}]`
- Bộ lọc cứng bằng metadata: `status=indexed`, `as_of ∈ [effective_from, effective_to)`, `kind` theo `scope`, `category_id`.
- Gợi ý chiến lược (bạn quyết): hybrid (BM25/`tsvector` + vector) → rerank → mở rộng sang điều/khoản kế cận → ghép tối đa N token.
- Tìm theo **số hiệu/điều khoản** ("Điều 5 Nghị định …") nên có đường tìm chính xác (regex/metadata) song song với ngữ nghĩa.

## 5. Yêu cầu về câu trả lời (contract cho agent)
- **Luôn có `citation`** (tên văn bản, số hiệu, điều/khoản, ngày hiệu lực, link nội bộ tới đoạn gốc). Không có nguồn ⇒ nói "không tìm thấy trong kho văn bản" (không bịa).
- Nêu **ngày hiệu lực/ văn bản có thể đã đổi**; nếu chunk có `effective_to` đã qua ⇒ cảnh báo "đã hết hiệu lực".
- Phân biệt "luật nói gì" và "chính sách sàn quy định"; khi mâu thuẫn, ưu tiên luật và báo rõ.
- Dòng miễn trừ cố định ở UI: "Thông tin tham khảo, không thay thế tư vấn pháp lý."
- Câu hỏi cá nhân hoá ("đơn của tôi có được hoàn tiền không?") ⇒ agent kết hợp **tool đơn hàng** (dữ kiện của user) + RAG (quy tắc) – và nêu giả định.
- Từ chối đúng: câu hỏi ngoài phạm vi (tư vấn kiện tụng, vụ việc cụ thể) ⇒ gợi ý liên hệ chuyên gia/CSKH.

## 6. Tính năng cho người bán (kiểm tra listing)
- Khi đăng sản phẩm: nút "Kiểm tra quy định" ⇒ agent đối chiếu `category_rule` + mô tả (ví dụ thiếu nhãn, thiếu giấy tờ). Kết quả là **gợi ý**, không chặn đăng (chặn là quyết định của rule/ kiểm duyệt viên).
- Quét định kỳ listing (alert loại `ai_scan`, xem 06): phát hiện từ khoá hàng cấm, tuyên bố y tế quá đà… ⇒ tạo alert cho admin duyệt.

## 7. Đánh giá RAG
- Bộ câu hỏi vàng (30–100) do người hiểu luật/chính sách soạn: câu hỏi → điều khoản đúng.
- Metric: **Recall@k của chunk đúng**, citation precision (trích dẫn có thực sự hỗ trợ câu trả lời?), tỉ lệ "không bịa" (câu hỏi không có đáp án ⇒ từ chối), độ trễ, chi phí.
- Regression khi cập nhật văn bản/đổi model: chạy lại suite trong `eval_runs`.

## 8. Bảo mật & vận hành
- Chỉ admin (và role `legal` nếu sau này có) tạo/xuất bản/retire văn bản; mọi thao tác ghi `ai_audit`.
- Văn bản upload là dữ liệu **không tin cậy** (prompt injection) – đi qua khối `<untrusted>`.
- Văn bản luật là công khai → hỏi đáp mở cho khách vãng lai ở mức `law`+`platform_policy` (rate-limit chặt); `category_rule` nội bộ chỉ cho seller/admin.
- Lưu `file_key` + `sha256` để chứng minh nguồn; giữ lịch sử phiên bản.

## 9. UI liên quan (xem [07](07-ui-design.md))
`/policies` (thư viện + hỏi đáp công khai), panel nguồn trích dẫn trong chat, `/admin/policies` (upload, xem trước chunk, xuất bản, retire, thử truy vấn).
