# 05 · Analytics và giám sát hệ thống (Grafana)

## 1. Analytics (analytics-service + ClickHouse)
### 1.1 Số liệu cung cấp
| Nhóm | Số liệu | Scope |
|---|---|---|
| Doanh thu | doanh thu ghi nhận, GMV, số đơn, AOV, tỉ lệ huỷ/hết hạn/hoàn, doanh thu theo phương thức thanh toán, theo shop/ danh mục/ ngày-giờ, so kỳ trước | platform; **store (tự ép store_id)** |
| Sản phẩm | top theo doanh thu/ số lượng/ xem/ chuyển đổi; hiệu quả: impression→click→view→cart→order; sản phẩm xem nhiều không bán; bán chậm | platform; store |
| Tồn kho | tồn thấp, **ngày còn bán ước tính** (tồn / tốc độ bán 14 ngày), lịch sử biến động | store |
| Traffic | page view, phiên, người dùng duy nhất (UV), mới/quay lại, top trang, nguồn (`referrer`), thiết bị, quốc gia, theo giờ | platform |
| Funnel | view → cart → checkout → paid (theo ngày, theo thiết bị) | platform; store (lọc theo sản phẩm của shop) |
| Thanh toán | tỉ lệ thành công, top `failure_code`, thời gian xử lý, số đơn AWAITING_PAYMENT, hoàn tiền | platform |
| Tìm kiếm | top truy vấn, truy vấn 0 kết quả, tỉ lệ click | platform |
| Vận hành | mức độ trễ pipeline (events→ClickHouse), số event/phút, tỉ lệ event bị loại | admin (data health) |
Mọi phản hồi: `{as_of, source, data}`; cache Redis 30–60 s; giới hạn khoảng ≤ 400 ngày; nhóm theo `hour|day|week|month` (múi giờ **cố định `Asia/Ho_Chi_Minh` (UTC+7, không DST)**; DB lưu UTC, gom nhóm/ hiển thị theo +7; không có bộ chọn múi giờ).
### 1.2 Phân quyền
Gateway điền `Scope{role, store_id}` từ JWT, **không** nhận `store_id` từ client. analytics-service từ chối truy vấn `store` mà thiếu `store_id`. Có test chéo shop (A không đọc B).
### 1.3 Pipeline & chất lượng
Consumer idempotent (ReplacingMergeTree + `event_id`); batch insert 1–5 s hoặc 10k dòng; lỗi insert ⇒ retry, vượt ngưỡng ⇒ DLQ; backfill script từ order-service/ product-service để dựng lại fact; trang `/admin/data-health`.

## 2. Giám sát hệ thống (CPU, RAM…)
- Nguồn: cAdvisor (CPU/RAM/net/IO **theo container/service**), node-exporter (máy), metrics ứng dụng (02 §8), exporters Postgres/Redis/Kafka/ClickHouse.
- Trong app: `GET /admin/system/health` (bảng service × replica × ready/degraded × version × uptime) và `GET /admin/system/metrics?q=<tên>` (series CPU%, RAM MB/limit, p95, 5xx, Kafka lag, outbox cũ nhất, disk). **Whitelist truy vấn**, không PromQL tự do.
- Grafana cho SRE: dashboards Overview, Services (RED), Containers, Nodes, Kafka & Outbox, Postgres, Redis, ClickHouse, Payments, Business.
- Cần đặt `resources.limits.memory`/`cpus` trong `services.yml` để "RAM % theo limit" có nghĩa.

## 3. Cảnh báo: CHƯA LÀM
Quyết định: **giai đoạn này không có cảnh báo.** Không có `alert-service`, Alertmanager, bảng `alerts/notifications`, chuông, SSE hay email. Chỉ có **Grafana để người vận hành tự nhìn metric**.
- Giá trị ngưỡng bên dưới chỉ để **tô màu panel** (xanh/vàng/đỏ) trên Grafana và trên bảng `/admin/system` – không phát thông báo.
- Khi sau này muốn cảnh báo: thêm Alertmanager + rule dựa trên đúng các metric này (không cần đổi code service). Bảng ngưỡng là đầu vào sẵn có.

### 3.1 Panel Grafana & ngưỡng tô màu tham khảo
| Dashboard | Panel (nguồn) | Vàng / Đỏ |
|---|---|---|
| Overview | service up/ready (`mm_ready`), replicas | ready=0 → đỏ |
| Services (RED) | RPS, lỗi 5xx %, p95 (`mm_http_*`, `mm_grpc_*`) | 5xx >1% / >2%; p95 >0,5 s / >1 s |
| Containers | CPU % / RAM % theo limit (cAdvisor), restart count | >75/85% CPU; >80/90% RAM; restart >0/>3 trong 15 phút |
| Nodes | CPU, RAM, disk, load (node-exporter) | disk >80/90% |
| Kafka & Outbox | lag theo group/topic, `mm_outbox_pending`, `mm_outbox_oldest_age_seconds` | lag >500/>1000; oldest >30 s/>120 s |
| Postgres / Redis | connections, cache hit, `mm_db_pool_*`, memory | connections >70/85% |
| ClickHouse | insert lag (`mm_ch_insert_lag_seconds`), disk, merges | lag >60/>120 s |
| Payments | `mm_payments_total` theo status/method, tỉ lệ thành công, webhook | thành công <90/<80% |
| Business | đơn tạo/ phút, trạng thái đơn, `mm_orders_awaiting_payment`, reservations hết hạn | |
| Gateway | rate-limited, events nhận/ bị loại | |
Provisioning dashboard bằng **file JSON** trong `deploy/grafana/provisioning/` (không chỉnh tay trong UI), có datasource Prometheus (và ClickHouse nếu muốn xem business trực tiếp).

### 3.2 "Cần chú ý" trong ứng dụng (không phải cảnh báo)
Là **truy vấn thường** của analytics API, người dùng mở trang mới thấy (không đẩy thông báo):
- Seller (`/seller`): sản phẩm tồn thấp / sắp hết (tồn ÷ tốc độ bán 14 ngày), đơn đã thanh toán chưa giao > 24 h, sản phẩm nhiều lượt xem 0 đơn.
- Admin (`/admin`): trạng thái `ready/degraded/not_ready` từng service, `/admin/data-health` (lag pipeline, lệch đối soát).

### 3.3 Runbook
Bổ sung vào [../runbook.md](../runbook.md) khi làm: đọc dashboard CPU/RAM, Kafka lag, ClickHouse lag, payment thất bại cao.
