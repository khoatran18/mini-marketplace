# 00 · Tổng quan

## 1. Mục tiêu
Dựng một **nền tảng thương mại điện tử có đủ dữ liệu, quan sát được và có điểm cắm rõ ràng** để sau này thêm 5 nhóm ứng dụng AI mà không phải đập lại kiến trúc:

| Nhóm | Ứng dụng AI (sẽ code sau) | Nền tảng phải có sẵn |
|---|---|---|
| A. Trợ lý người mua | Hỏi đáp "trong kho có gì", tìm hàng theo mô tả tự nhiên | Mô tả/danh mục/ảnh sản phẩm, embedding index, tool tìm kiếm, chat UI |
| B. Trợ lý người bán | Doanh thu, tồn kho, sản phẩm bán chậm, gợi ý nhập hàng | Analytics theo `store_id`, tool đọc số liệu, dashboard |
| C. Trợ lý quản trị | Tổng doanh thu, lưu lượng, RAM/CPU, sức khoẻ hệ thống | Metrics (Prometheus), analytics toàn sàn, role `admin` |
| D. Quét & cảnh báo tự động | Phát hiện bất thường, tồn thấp, đơn kẹt, tài nguyên cao, listing vi phạm | Rule engine, bảng alert, kênh thông báo, scheduler |
| E. Hỏi đáp điều luật/chính sách | RAG trên luật + quy định sàn + chính sách ngành hàng | Kho văn bản có version, ingestion, vector store, trích dẫn |
| F. Hệ gợi ý | Home "dành cho bạn", "sản phẩm tương tự", "hay mua cùng", rerank tìm kiếm | Pipeline tracking, impression log, feature store, serving API, A/B |

## 2. Persona và quyền
| Persona | Role trong JWT | Hiện có? | Ghi chú |
|---|---|---|---|
| Khách vãng lai | (không token) | có | Cần **tracking ẩn danh** (`anonymous_id`) để gợi ý và funnel |
| Người mua | `buyer` | có | |
| Chủ shop | `seller_admin` | có | Có `store_id` |
| Nhân viên shop | `seller_employee` | có | Quyền như shop nhưng không quản lý nhân sự |
| **Quản trị sàn** | **`admin`** | **CHƯA CÓ** | Phải thêm: không cho tự đăng ký qua `/auth/register`, tạo bằng seed/CLI. Xem [ADR-A3](10-roadmap-phases.md#adr) |

## 3. Use case chính (rút gọn – chi tiết ở 03, 07)
**Người mua**
- "Có áo khoác chống nước dưới 500k không?" → agent tìm (semantic + filter giá) → thẻ sản phẩm có nút thêm giỏ.
- "Đơn hàng hôm qua của tôi đang ở đâu?" → tool `get_my_orders`.
- "Hàng điện tử có được đổi trả 7 ngày không?" → RAG chính sách, có trích dẫn.
- Trang chủ/giỏ hàng/chi tiết sản phẩm hiển thị gợi ý, ghi lại click.

**Người bán**
- "Tuần này doanh thu shop tôi thế nào so với tuần trước?" · "Sản phẩm nào sắp hết hàng?" · "Mặt hàng nào xem nhiều nhưng ít mua?"
- Nhận cảnh báo: tồn < ngưỡng, đơn PENDING quá lâu, doanh thu tụt bất thường.

**Quản trị**
- "Tổng doanh thu tháng này? Lưu lượng truy cập? CPU/RAM service nào cao nhất?"
- Alert: CPU > 85% trong 5 phút, Kafka lag tăng, outbox kẹt, tỉ lệ lỗi 5xx tăng, listing có dấu hiệu vi phạm.
- Quản lý văn bản pháp lý, xem chi phí/chất lượng AI.

## 4. Nguyên tắc thiết kế
1. **Reuse, đừng đào mới**: AI đi qua gateway như mọi client; không cho AI đọc thẳng DB nghiệp vụ.
2. **Event-driven nhất quán với hiện tại**: mọi dữ liệu mới đi qua Kafka + outbox (xem [../events-kafka.md](../events-kafka.md)).
3. **Tách "serving" và "training"**: online (ms) đọc từ Redis/pgvector; offline (giờ/ngày) đọc từ ClickHouse.
4. **Log để học**: gợi ý/tìm kiếm phải log *impression + vị trí + model_version + request_id* ngay từ ngày đầu, nếu không sau này không huấn luyện/đánh giá được.
5. **Dữ liệu người dùng và nội dung người bán là *không tin cậy*** (prompt injection) → xem 03 §6.
6. **Có đường lui**: mỗi tính năng AI có flag và fallback.
7. **Chi phí & quan sát AI là hạng nhất**: token, latency, tool-call, lỗi đều được ghi (bảng `ai_audit`).

## 5. Phạm vi
**Trong phạm vi (giai đoạn "chuẩn bị hạ tầng")**
- Bổ sung dữ liệu nền: mô tả/danh mục/ảnh/thuộc tính sản phẩm, giỏ hàng phía server, vòng đời đơn hàng đủ để tính doanh thu, role `admin`.
- Pipeline tracking → Kafka → ClickHouse; analytics API theo vai trò.
- Observability: Prometheus/Grafana/Alertmanager, `/metrics` ở mọi service.
- Rule engine + alert center (không cần AI).
- Khung `ai-service` rỗng + hợp đồng gRPC/REST + UI chat/gợi ý chạy được với **mock**.
- Kho văn bản chính sách + trang quản trị (chưa embedding thật).
- Toàn bộ giao diện mới (xem 07).

**Ngoài phạm vi (bạn tự code sau)**: prompt/agent loop, chọn LLM, embedding & retrieval, huấn luyện/serving mô hình gợi ý, AI giải thích cảnh báo, đánh giá chất lượng AI.

**Ngoài phạm vi hẳn**: thanh toán thật, fine-tune LLM, multi-region, mobile app.

## 6. Thuật ngữ
- **Surface**: vị trí hiển thị gợi ý (`home`, `pdp_similar`, `cart_addon`, `search`…).
- **Impression**: một item được hiển thị trước mắt người dùng (khác với click).
- **Tool**: hàm có schema mà LLM có thể gọi (đọc tồn kho, tìm sản phẩm…).
- **Điểm cắm AI (AI seam)**: interface/contract nơi code AI của bạn thay thế bản mock. Liệt kê ở [01 §5](01-platform-architecture.md#5-điểm-cắm-ai-ai-seams).
