# 10 · Lộ trình, nghiệm thu, ADR, câu hỏi mở

Phân chia: **P1–P4 = hạ tầng/giao diện/hợp đồng** (nền tảng, làm trước, chưa cần AI). **P5+ = ứng dụng AI do bạn tự code**, cắm vào các seam ở [01 §5](01-platform-architecture.md#5-điểm-cắm-ai-ai-seams).

## Giai đoạn

### P0 – Thiết kế (hiện tại)
- [x] Bộ tài liệu `.claude/docs/ai/` · [ ] Chủ dự án duyệt & chốt các câu hỏi mở (mục cuối) → **không code trước khi duyệt**.

### P1 – Nền dữ liệu nghiệp vụ *(điều kiện cho mọi thứ khác)*
| Việc | Tiêu chí nghiệm thu |
|---|---|
| Role `admin` + bootstrap + `Role` ở frontend | admin đăng nhập được; `/auth/register` từ chối `admin`; test RBAC |
| Catalog mở rộng: `description, category_id, brand, tags, image_urls, status`; bảng `categories` | CRUD qua gateway; validate; seed demo có ≥ 100 sản phẩm có mô tả thật |
| Upload ảnh (MinIO hoặc volume) | giới hạn size/MIME; ảnh hiện ở UI |
| Cart phía server + merge | giỏ giữ qua thiết bị; sự kiện `cart.updated` |
| Order: snapshot tên/`seller_id`/`category_id`; vòng đời tối thiểu (xem roadmap #1) | doanh thu theo shop/ danh mục tính được từ dữ liệu order |
| Outbox mới: `product.changed`, `inventory.changed`, `order.status_changed` (+ `inventory_ledger`) | test idempotent + outbox như mẫu `runOutboxWorker` |
| Body-size limit ở gateway; `/events` & public route rate-limit riêng | test |
| Migration có version cho bảng mới | |

### P2 – Tracking & analytics
| Việc | Nghiệm thu |
|---|---|
| ClickHouse + `analytics-service` (consume → events_raw, fact, rollups) | gửi 1000 event giả → khớp số trong ClickHouse; dedupe đúng |
| `POST /events` + `identify` + consent | 202 nhanh (< 50 ms p95); event sai bị loại; trường server không bị ghi đè |
| Tracking SDK frontend + impression/click ở `ProductCard`, search, cart, checkout | `/dev/tracking` thấy event; không chặn UI khi gateway lỗi |
| Analytics API (seller/admin) | shop A không đọc được shop B (test); `as_of`; cache |
| `scripts/datagen` (dữ liệu giả có cấu trúc) | sinh user/đơn/events lặp lại được (seed) |
| Đối soát doanh thu ClickHouse ↔ Postgres | lệch < 0.5% |

### P3 – Observability & cảnh báo
| Việc | Nghiệm thu |
|---|---|
| `/metrics` ở mọi service Go (RED + outbox + Kafka lag) | Grafana dashboard Overview/Services |
| Stack `observability.yml` (Prometheus, Alertmanager, Grafana, cAdvisor, node-exporter, exporters) | thấy CPU/RAM từng service |
| `GET /admin/system/*` (proxy whitelist) | admin xem được; role khác 403 |
| `alert-service`: rule threshold/sql/anomaly + Alertmanager webhook + vòng đời + SSE + notifications | gây tải/ hạ tồn/ chặn consumer ⇒ có alert đúng, dedupe, tự resolve |
| Bộ rule ban đầu (06 §3.2) + runbook cho từng rule | mỗi rule có `runbook_url` |

### P4 – Giao diện & khung AI (mock)
| Việc | Nghiệm thu |
|---|---|
| Console layout seller/admin + dashboard thật (analytics + system) | theo wireframe 07 §4.6–4.8 |
| Alert center, bell | |
| Trang `/policies` + `/admin/policies` (upload, trạng thái, xuất bản; chưa embedding) | quản lý được vòng đời văn bản |
| `ai-service` **stub** (Python) + `ai.proto` + gateway `/ai/chat` SSE, `/recommendations`, `/search?mode=semantic`, `/policies/ask` | UI chạy end-to-end với dữ liệu mock, badge `Mock` |
| Widget chat, RecoCarousel, SearchModeToggle, citation panel, action proposal | contract `card` được render đúng; flag tắt ⇒ fallback |
| Feature flags + `/admin/ai` khung (usage/audit/eval rỗng) | |
| pgvector + bảng `ai` + `ai_audit` + eval harness rỗng | `Health()` trả version |
**Cổng P4→P5**: toàn bộ test E2E ở 09 §6 xanh với stub; tài liệu `ai-service/README` hướng dẫn "viết seam đầu tiên".

### P5+ – Ứng dụng AI (bạn tự code; thứ tự gợi ý dựa trên rủi ro/ giá trị)
1. **Tìm theo mô tả** (Embedder + ProductRetriever + indexer từ `product.changed`) – ít rủi ro, thấy giá trị ngay.
2. **Agent seller/admin hỏi số liệu** (tool chỉ đọc, đúng/sai kiểm chứng được bằng dashboard).
3. **Recsys v0→v2** (trending → co-visit → content), rồi v3+ khi có dữ liệu thật; luôn đo bằng baseline.
4. **RAG chính sách/luật** (cần dữ liệu văn bản sạch + người kiểm chứng).
5. **Agent shopper** đầy đủ (tool đơn/giỏ, action proposal).
6. **AI giải thích cảnh báo + ai_scan** (sau cùng vì cần nhiều alert thật để đánh giá).
Mỗi bước: bật flag cho % nhỏ/ nội bộ → đo → mở rộng; chạy eval suite trước khi bật.

## Ma trận phụ thuộc (rút gọn)
```
role admin ─┬─▶ admin dashboard ─▶ agent admin
catalog ────┼─▶ semantic search ─▶ agent shopper
order fact ─┼─▶ analytics ─▶ agent seller/admin, recsys v0/v1
tracking ───┴─▶ funnel, recsys v3+ (cần impression log)
metrics ────▶ system dashboard ─▶ alert rules ─▶ AI explain
policy docs ─▶ RAG
```

## ADR
Ghi tiếp vào [../decisions.md](../decisions.md) sau khi duyệt; đây là bản đề xuất.

- **ADR-A1 – `ai-service` bằng Python, tách khỏi các service Go.** Hệ sinh thái LLM/ML; giao tiếp qua gRPC nên thay được. Đánh đổi: thêm một runtime để vận hành/CI.
- **ADR-A2 – ClickHouse cho events/analytics.** Hợp clickstream và aggregation. *Phương án lùi:* Postgres + bảng rollup (partition theo ngày) nếu máy yếu — giữ nguyên `analytics-service` API để đổi backend không ảnh hưởng.
- **ADR-A3 – Thêm role `admin` không đăng ký công khai.** Cần cho vận hành/ AI admin. Tạo bằng bootstrap env/CLI; không nằm trong whitelist `/auth/register`; token & rate-limit riêng.
- **ADR-A4 – Agent đi qua gateway bằng JWT của người dùng (không DB trực tiếp, không SQL tự do).** Tái dùng RBAC/ownership (ADR-3); tool = semantic layer.
- **ADR-A5 – pgvector trước, vector DB riêng sau (nếu cần).** Ít thành phần; ẩn sau `Retriever`.
- **ADR-A6 – Cart nằm trong `order-service`.** Tránh thêm service; cùng DB với order. (Tách khi cần.)
- **ADR-A7 – Rule phát hiện, AI giải thích.** Cảnh báo tin cậy không phụ thuộc LLM; AI chỉ bổ sung, luôn có nhãn.
- **ADR-A8 – Mọi tính năng AI sau feature flag + fallback.** Đảm bảo vận hành khi AI lỗi/hết ngân sách.
- **ADR-A9 – Log impression + request_id + model_version ngay từ đầu.** Điều kiện để huấn luyện & A/B sau này.
- **ADR-A10 – Tool ghi dùng `action_proposal` + xác nhận của người dùng.** Chống hành động ngoài ý muốn do hallucination/injection.
- **ADR-A11 – Tiền VND dạng `{amount, currency}` ở API mới; không `float` mới.** Không đụng ADR-7 cũ, nhưng tránh nhân rộng nợ kỹ thuật.

## Rủi ro chính
| Rủi ro | Giảm thiểu |
|---|---|
| Không có dữ liệu thật để dựng/đánh giá recsys | `datagen` có cấu trúc; kỳ vọng thấp; đo với baseline; thu log đúng từ đầu |
| Chi phí LLM mất kiểm soát | ngân sách + rate-limit + flag tự tắt + dashboard chi phí |
| Rò dữ liệu chéo shop qua agent | tool ép `store_id` ở server + test chéo shop + audit |
| Prompt injection qua mô tả/PDF | khối `<untrusted>`, không tool tự do, action cần xác nhận, eval red-team |
| Scope quá lớn | chia P1–P4 có nghiệm thu rõ; AI làm từng seam; không làm tất cả cùng lúc |
| Máy dev không đủ RAM | compose lite (09 §2), bỏ Grafana/exporter, ClickHouse→Postgres |
| Văn bản luật lỗi thời | `effective_from/to`, retire, cảnh báo hiệu lực, miễn trừ, người duyệt |
| Quyền riêng tư | consent, che PII, retention, quyền xoá, băm IP |

## Câu hỏi mở – cần bạn chốt trước khi code
1. **Phạm vi P1**: có làm vòng đời đơn `PAID→SHIPPED→DELIVERED` + thanh toán giả (COD) ngay, hay tạm coi `SUCCESS` là "đã đặt" để tính doanh thu? *(Đề xuất: làm tối thiểu PAID/SHIPPED/DELIVERED + COD vì doanh thu/ gợi ý "đã mua" cần nghĩa rõ.)*
2. **ClickHouse hay Postgres-only** cho analytics (máy của bạn bao nhiêu RAM)? *(Đề xuất: ClickHouse; lùi về Postgres nếu < 8 GB.)*
3. **Nhà cung cấp LLM/embedding** (API ngoài hay model local)? Ảnh hưởng RAM `ai-service`, egress, dim embedding. *(Bạn quyết ở P5; hạ tầng chỉ cần biến môi trường.)*
4. **Ngôn ngữ UI/dữ liệu**: chỉ tiếng Việt hay song ngữ? (ảnh hưởng embedding đa ngữ, i18n).
5. **Văn bản pháp lý nào** nạp đầu tiên, ai chịu trách nhiệm kiểm duyệt nội dung?
6. **Vãng lai được dùng chat không** (chi phí)? *(Đề xuất: cho hỏi chính sách + tìm hàng, giới hạn nghiêm; tool đơn/giỏ cần đăng nhập.)*
7. **Một dự án đa shop có cần tách đơn theo shop khi checkout** (ảnh hưởng doanh thu theo shop)? *(Đề xuất: có — roadmap #5.)*
8. **Thư viện biểu đồ**: Recharts có ổn không? Có muốn dark mode ngay không?
9. **Python cho ai-service** có ổn không, hay bạn muốn Go/TypeScript?
10. **Mức độ "self-healing"**: chỉ cảnh báo (đề xuất) hay cho phép hành động tự động có điều kiện sau này?
