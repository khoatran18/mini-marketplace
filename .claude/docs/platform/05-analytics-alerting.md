# 05 · Analytics, giám sát hệ thống, quy tắc cảnh báo

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
Mọi phản hồi: `{as_of, source, data}`; cache Redis 30–60 s; giới hạn khoảng ≤ 400 ngày; nhóm theo `hour|day|week|month` (múi giờ `Asia/Ho_Chi_Minh` cố định, ghi rõ).
### 1.2 Phân quyền
Gateway điền `Scope{role, store_id}` từ JWT, **không** nhận `store_id` từ client. analytics-service từ chối truy vấn `store` mà thiếu `store_id`. Có test chéo shop (A không đọc B).
### 1.3 Pipeline & chất lượng
Consumer idempotent (ReplacingMergeTree + `event_id`); batch insert 1–5 s hoặc 10k dòng; lỗi insert ⇒ retry, vượt ngưỡng ⇒ DLQ; backfill script từ order-service/ product-service để dựng lại fact; trang `/admin/data-health`.

## 2. Giám sát hệ thống (CPU, RAM…)
- Nguồn: cAdvisor (CPU/RAM/net/IO **theo container/service**), node-exporter (máy), metrics ứng dụng (02 §8), exporters Postgres/Redis/Kafka/ClickHouse.
- Trong app: `GET /admin/system/health` (bảng service × replica × ready/degraded × version × uptime) và `GET /admin/system/metrics?q=<tên>` (series CPU%, RAM MB/limit, p95, 5xx, Kafka lag, outbox cũ nhất, disk). **Whitelist truy vấn**, không PromQL tự do.
- Grafana cho SRE: dashboards Overview, Services (RED), Containers, Nodes, Kafka & Outbox, Postgres, Redis, ClickHouse, Payments, Business.
- Cần đặt `resources.limits.memory`/`cpus` trong `services.yml` để "RAM % theo limit" có nghĩa.

## 3. Hệ thống cảnh báo
### 3.1 Hai đường vào, một bảng `alerts`
```
Prometheus rules ─▶ Alertmanager ─ webhook (secret) ─▶ alert-service ─┐
alert-service scheduler (Redis leader lock) ─ rule threshold/sql/anomaly (ClickHouse|Postgres) ─┤
                                                                      ▼ upsert theo fingerprint
              alerts ─▶ notifications (in-app, SSE `/alerts/stream`) ─▶ [email/webhook – giai đoạn sau]
```
### 3.2 Loại rule
| type | Định nghĩa | Ghi chú |
|---|---|---|
| `promql` | biểu thức + `for` | thực thi trong Prometheus (rule file đặt `deploy/prometheus/rules/*.yml`); alert-service chỉ nhận |
| `threshold` | truy vấn đơn giản (SQL) + toán tử + ngưỡng + `for` | tồn thấp, đơn kẹt |
| `sql` | truy vấn **chỉ đọc, whitelist, timeout 5 s, LIMIT** trả danh sách vi phạm | mỗi dòng = một alert (fingerprint theo khoá dòng) |
| `anomaly` | giá trị hiện tại so baseline cùng giờ/ngày-trong-tuần: `median ± k·MAD` (k mặc định 3), cần ≥ 4 tuần dữ liệu (nếu thiếu thì dùng ngưỡng cứng) | doanh thu, traffic, tỉ lệ lỗi thanh toán |
(Không rule nào dùng AI trong giai đoạn này.)
### 3.3 Bộ rule khởi đầu
**Hạ tầng (scope platform → admin)**
| key | Điều kiện | Mức | Runbook |
|---|---|---|---|
| `svc_down` | `up{job=~"mm-.*"}==0` hoặc `mm_ready==0` > 1 phút | critical | restart/ logs |
| `container_cpu_high` | CPU container > 85% limit trong 5 phút | warning (critical > 95% 10 phút) | |
| `container_mem_high` | working-set > 90% limit 5 phút | warning/critical | |
| `container_mem_leak` | RAM tăng đơn điệu 30 phút (`deriv > 0` & > 20%) | warning | |
| `container_restarts` | > 3 lần/ 15 phút | critical | |
| `node_disk_high` | disk > 85% | warning/ critical 95% | |
| `node_cpu_high` / `node_mem_high` | > 85% 10 phút | warning | |
| `gateway_latency_high` | p95 > 1 s 5 phút | warning | |
| `gateway_5xx_high` | 5xx > 2% 5 phút | critical | |
| `grpc_error_high` | lỗi gRPC (non-OK trừ NotFound/InvalidArgument) > 5% | warning | |
| `kafka_lag_high` | lag > 1000 trong 5 phút | warning | runbook "consumer idle" |
| `outbox_stuck` | `oldest_age > 120s` hoặc `pending > 500` | critical | runbook PENDING |
| `db_conn_high` | connections > 80% | warning | |
| `db_pool_wait` | `mm_db_pool_wait_seconds_total` tăng nhanh | warning | |
| `clickhouse_lag` | `mm_ch_insert_lag_seconds > 120` | warning | |
| `tls_expiry` | cert < 14 ngày | warning | |
| `scrape_missing` | target mất > 2 phút | warning | |
| `rate_limit_spike` | `mm_rate_limited_total` tăng bất thường | info | |
**Nghiệp vụ (platform)**
| key | Điều kiện | Mức |
|---|---|---|
| `order_pending_stuck` | `PENDING` > 10 phút | critical |
| `awaiting_payment_backlog` | số đơn AWAITING_PAYMENT tăng > x2 so baseline | warning |
| `payment_failure_rate` | thất bại > 20% / 15 phút (≥ 20 giao dịch) | critical |
| `payment_timeout` | webhook/ kết quả không về sau 2 phút | warning |
| `refund_rate_high` | hoàn tiền > 10%/ ngày | warning |
| `cancel_rate_high` | huỷ > 15%/ ngày | warning |
| `revenue_anomaly` | doanh thu giờ < 50% baseline (anomaly) | warning |
| `traffic_anomaly` | PV tăng > 5× baseline (nghi bot) hoặc giảm > 70% | warning |
| `signup_spike` | đăng ký/ phút bất thường, trùng IP băm | warning |
| `reconcile_mismatch` | ClickHouse vs Postgres lệch > 0,5% | critical |
| `event_pipeline_drop` | events/phút giảm > 80% so baseline giờ cao điểm | warning |
**Shop (scope store → chủ/nhân viên shop đó)**
| key | Điều kiện | Mức |
|---|---|---|
| `low_stock` | `available <= threshold` | warning (critical khi = 0) |
| `stockout_soon` | tồn / tốc độ bán 14 ngày < 3 ngày | warning |
| `order_unprocessed` | đơn `PAID/CONFIRMED` chưa `SHIPPED` > 24 h | warning |
| `shipping_overdue` | `SHIPPED` > 7 ngày chưa DELIVERED | info |
| `sales_drop` | doanh thu 7 ngày giảm > 30% so tuần trước | info |
| `views_no_orders` | ≥ 100 lượt xem/7 ngày, 0 đơn | info |
| `rating_drop` | rating TB 30 ngày giảm ≥ 0,5 | info |
| `cancel_rate_store` | huỷ > 20% (≥ 10 đơn) | warning |
Rule lưu trong DB (`alert_rules`), admin bật/tắt/chỉnh ngưỡng qua UI (ghi `audit_log`); có nút "Chạy thử" trả các dòng vi phạm hiện tại; rule hạ tầng gốc nằm file Prometheus (read-only trên UI).
### 3.4 Vòng đời
`firing → open → ack → resolved` (tự resolved khi điều kiện hết đúng ≥ 2 chu kỳ); `silenced` theo thời hạn + lý do.
- **Dedupe**: `fingerprint = hash(rule_key, scope, store_id, khoá nhãn)`; đang open ⇒ tăng `count`, cập nhật `last_seen`.
- **Chống bão**: `cooldown`, group (Alertmanager `group_by`), `inhibit` (service down ⇒ ẩn latency/5xx của nó).
- **Định tuyến**: scope store ⇒ chỉ shop đó; platform ⇒ admin.
- **Mức độ**: `info|warning|critical`; critical ⇒ chuông đỏ + banner trong console.
- **Kênh**: in-app (bảng `notifications` + SSE); email/webhook = tuỳ chọn sau (biến `SMTP_*`, `ALERT_WEBHOOK_URL`).
- Mỗi rule có `runbook_url` → mục trong [../runbook.md](../runbook.md) (bổ sung: CPU/RAM, Kafka lag, ClickHouse, payment).
### 3.5 Bản thân alert-service phải được giám sát
`mm_rule_eval_errors_total`, `mm_alerts_open`; "dead-man switch": rule `Watchdog` luôn firing; nếu Alertmanager không nhận được ⇒ bên ngoài báo (ghi trong runbook).
