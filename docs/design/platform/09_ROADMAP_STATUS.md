# 09 · Lộ trình, trạng thái triển khai, ADR, giới hạn đã biết

Thứ tự đã làm: nền vận hành (health/ready/metrics) → dữ liệu, thanh toán, vòng đời đơn → tracking/analytics → giám sát Grafana → giao diện. Tài liệu này đối chiếu **thiết kế** (01–08) với **code hiện tại**. Khi khác nhau, code đúng.

## Trạng thái triển khai
Chú thích: ✅ xong · 🟡 một phần · ❌ chưa làm.

### P0 – Thiết kế
✅ Đã duyệt; các tài liệu 01–09 (nay đã cập nhật theo code).

### P1 – Nền vận hành & danh tính

| Hạng mục | Trạng thái | Ghi chú (code) |
|---|---|---|
| Thư viện chung `pkg/ops`: admin HTTP `:8081` (`/health /healthz /ready /readyz /metrics /version`), `grpc.health.v1`, interceptor metrics, tắt êm theo SIGTERM | ✅ | 7 bản giống hệt (md5 trùng) ở gateway, auth, user, product, order, payment, analytics |
| Docker `HEALTHCHECK` (`/healthz`) + Traefik healthcheck gateway `/ready` + `stop_grace_period 30s` + `resources.limits` | ✅ | `deploy/services.yml` – **chưa chạy thử trên Swarm thật** |
| `update_config: start-first` / `failure_action: rollback`, `scripts/wait-ready.sh` | 🟡 | đã viết trong `services.yml` / `scripts/wait-ready.sh` (gọi từ `deploy.sh`); chưa chạy thử trên Swarm thật |
| Healthcheck container hạ tầng | 🟡 | postgres, redis, clickhouse, kafka, prometheus, grafana, cAdvisor, node-exporter, traefik, frontend có; postgres-exporter và kafka-exporter có; redis-exporter không khả thi (image scratch, không có shell/wget) |
| Prometheus + exporters + cAdvisor + node-exporter + Grafana | ✅ | `deploy/observability.yml`, 11 dashboard – chưa chạy thử trên Swarm thật |
| Role `admin` (bootstrap `ADMIN_BOOTSTRAP_*`, `/admin/*`) | ✅ | không đăng ký công khai |
| Khoá tạm đăng nhập (`LOGIN_MAX_FAILURES`), `audit_log`, `status/email` của tài khoản | ❌ | chưa có biến/bảng |
| Migration Postgres có version (goose v3.26.0, chạy lúc khởi động, advisory lock, bảng `goose_db_version_<svc>`) | ✅ | auth / user / product / order / payment; test trên Postgres 16; chưa chạy trên Swarm |
| Request-id (`X-Request-ID` ↔ gRPC `x-request-id`, `internal/requestid`, body lỗi có `request_id`) | 🟡 | chưa vào Kafka / outbox; `productclient` của gateway chưa truyền ctx |
| Deadline / retry gRPC ở gateway (`GRPC_TIMEOUT_*`, `GRPC_RETRY_MAX_ATTEMPTS`; retry chỉ lệnh đọc, `Unavailable`) | 🟡 | DeadlineExceeded→504; chưa retry lệnh ghi, chưa circuit breaker |
| Log tập trung Vector → Loki → Grafana (nhãn `service/stack/level/env`, che PII) | 🟡 | chạy thật cục bộ; chưa trên Swarm |
| Giới hạn body (`MAX_BODY_BYTES`, 413) + nhóm route công khai + rate-limit riêng `/events` | ✅ | auth gắn theo route (ADR-19) |

### P2 – Catalog, giỏ hàng, thanh toán mô phỏng, vòng đời đơn

| Hạng mục | Trạng thái | Ghi chú |
|---|---|---|
| Trường mới của product (sku, mô tả, danh mục, brand, tags, ảnh, status, ngưỡng tồn, weight) + `categories` + tìm kiếm full-text không dấu `/search` | ✅ | cây ≤ 3 cấp, `mm_unaccent`, GIN index; seed ≥ 100 sản phẩm demo (`SEED_DEMO_DATA`) |
| Upload ảnh (MinIO), `MAX_UPLOAD_MB` | ❌ | ảnh chỉ là URL (≤ 8/sản phẩm) |
| `inventory_ledgers` + `reserved`/`sold` + outbox `product.changed`/`inventory.changed` | ✅ | ADR-21; test đồng thời |
| Giỏ hàng server + merge + `checkout/preview` + địa chỉ (user-service) + idempotency key | ✅ | giỏ ở order-service (≤ 50 dòng, ≤ 99/dòng) |
| Máy trạng thái mới + `order_status_histories` + hết hạn giữ chỗ + seller ship/deliver + trả hàng | ✅ | không có bước "seller xác nhận" riêng; `SUCCESS` cũ → `CONFIRMED` |
| Tách đơn theo store (`checkout_id`) | ✅ | ADR-15 |
| `payment-service` mockpay (thẻ test, ví, chuyển khoản, webhook ký HMAC, trễ/trùng/sau hạn, hoàn tiền) | ✅ | ADR-16; provider là code trực tiếp, **chưa** có interface `Provider` |
| `checkout_id` gộp thanh toán, `shipping_fee` | ✅ | `SHIPPING_FLAT_FEE`, `FREE_SHIP_OVER` (không theo cân nặng) |
| Reviews / wishlist | ❌ | |
| `GET /products/:id/stock`: số chính xác cho chủ store | 🟡 | công khai chỉ trả `level`; chủ store xem số qua `/products/:id` hoặc `/seller/products` |

### P3 – Tracking & analytics

| Hạng mục | Trạng thái | Ghi chú |
|---|---|---|
| ClickHouse (24.8) + `analytics-service` (consume 6 topic Kafka → 7 bảng) | ✅ | ADR-13/14; CI chạy ClickHouse thật |
| `POST /events`, `POST /events/identify` (gateway → gRPC `IngestEvents`) | ✅ | không qua Kafka; consent + che PII + ip-hash |
| API analytics seller/admin | ✅ | seller: summary, timeseries, top-products, funnel, low-stock · admin: + traffic, payments, search-terms, data-health |
| Rollup (materialized view) | ❌ | cố ý: gom bảng fact lúc truy vấn (ADR-14) |
| Đối soát đêm Postgres ↔ ClickHouse; `mm_reconcile_diff_ratio` | 🟡 | CLI `go run ./cmd/backfill reconcile` (analytics-service) đã chạy thật cục bộ; chưa có lịch đêm, `data-health` vẫn trả `reconciliation: "not_implemented"`, chưa có metric |
| Backfill từ service | 🟡 | CLI `backfill` Postgres → bảng fact / dim đã chạy thật cục bộ (idempotent; không dựng lại `events_raw`); Kafka vẫn chỉ 7 ngày |
| `scripts/datagen` có seed | ✅ | `scripts/datagen` (synthetic + RetailRocket) ghi Postgres + `events_raw`; `scripts/dev/gen_traffic.py` vẫn chỉ sinh event client. Thêm `scripts/export` (xuất snapshot) 🟡 chạy tay |
| SDK tracking + trang `/admin/data-health` ở UI | 🟡 | phía frontend do nhánh UI phụ trách (07-ui-design) |
| `DELETE /users/me/data`, bảng `consents`, `identity_links` | ❌ | `identify` chỉ lưu thành event `event_type=identify` |
| `GET /config/features` | ❌ | |

### P4 – Giám sát hệ thống (Grafana, không cảnh báo)

| Hạng mục | Trạng thái | Ghi chú |
|---|---|---|
| Dashboard provision bằng file | 🟡 | **11 đã viết**: Overview, Services (RED), Containers & hosts (gồm Nodes), Kafka·Postgres·Redis·Traefik, Analytics pipeline, ClickHouse server, Outbox & Workers, Payments, Business, Kafka consumer lag, Logs (Loki). 6 cái cuối chưa xem trên Grafana thật, PromQL chưa chạy trên Prometheus thật |
| `GET /admin/system/health` | ✅ | gom `/ready` của 7 service |
| `GET /admin/system/metrics` (proxy Prometheus), `/admin/system/outbox`, `/admin/system/kafka`, requeue outbox qua API | ❌ | dùng Grafana/`03_RUNBOOK.md` |
| Metric nghiệp vụ | 🟡 | đã thêm và test đơn vị, chưa chạy stack thật: order `mm_checkouts_total{result}`, `mm_orders_created_total`, `mm_order_status_transitions_total{to_status}`, `mm_orders_payment_expired_total`, `mm_orders_auto_delivered_total`, `mm_order_cart_merges_total`; payment `mm_payments_created_total{method}`, `mm_payments_completed_total{method,result}`, `mm_payment_amount_minor_total{method}`, `mm_refunds_total{result}`, `mm_payment_webhooks_total{result}`, `mm_payment_processing_seconds`; product `mm_inventory_reservations_total{result}`, `mm_inventory_adjustments_total{reason}`, `mm_products_changed_total{op}` (chưa đếm `SetProductStatus`, `delete` luôn 0); gateway `mm_rate_limited_total{group}`, `mm_grpc_client_requests_total{service,method,code}`, `mm_grpc_client_duration_seconds{service,method}` (nhãn `service` = service đích). Lag consumer Kafka lấy từ `kafka_consumergroup_lag` của kafka-exporter (không có `mm_kafka_consumer_lag` tự phát). Đã có sẵn: `mm_outbox_*`, `mm_worker_last_tick_timestamp_seconds`, `mm_ch_*`, `mm_events_*`. Vẫn thiếu: request-id, tracing, deadline/retry gRPC |
| Runbook đọc dashboard, Kafka lag, ClickHouse, payment fail | ✅ | `03_RUNBOOK.md` |
| Cảnh báo (Alertmanager, thông báo) | ❌ **cố ý** | ADR-12 |

### P5 – Giao diện
🟡 Do nhánh UI phụ trách (xem `07_UI_DESIGN.md`, không kiểm chứng ở đây). Backend cho console seller/admin, checkout, `/pay/<id>`, timeline đơn, trang system đã sẵn.

### Tính năng quyền riêng tư/ tài khoản/ quản trị chưa làm (tóm tắt)
❌ bảng `consents` + `DELETE /users/me/data` · ❌ review/ wishlist · ❌ upload ảnh MinIO · ❌ khoá đăng nhập + `audit_log` · ❌ `/admin/users|stores|audit-log` · ❌ `/auth/me`, `/auth/logout` · ❌ `/admin/system/metrics` · ❌ `DataHealth` đối soát.

## Giới hạn đã biết
- **Chưa từng chạy thật**: Docker, Docker Swarm, Traefik, ClickHouse *server*, Prometheus/Grafana **chưa bao giờ chạy trong sandbox soạn tài liệu/ code**. Compose, nhãn Traefik, healthcheck Docker, dashboard chỉ được kiểm cú pháp (cấu hình Prometheus/PromQL bằng thư viện Prometheus).
- SQL ClickHouse được xác thực trên **chDB 26.9** (engine ClickHouse nhúng, qua `scripts/dev/chdb_server.py`) + unit test; **ClickHouse 24.8 thật chỉ chạy trong CI**. chDB mới hơn server 24.8 nên có thể lệch ở chi tiết.
- `scripts/e2e` đã viết lại cho API mới (11 kịch bản, cart/checkout/payment/analytics, `resilience.sh` A..I, `scripts/load`) nhưng **chưa chạy lần nào trên stack thật**; contract với gateway có thể lệch ở vài chỗ nhỏ (xem `04_TESTING.md` §5).
- Swagger (`services/api-gateway/docs`) đã cũ.
- Tracking đi qua gRPC: analytics-service down/restart thì **mất** event tracking đang đệm (ADR-13); ClickHouse down lâu hơn `ANALYTICS_BUFFER_MAX_ROWS` thì rớt dòng.
- `domain_events` là một bảng vật lý chung của product/order/payment (ADR-23): backlog không tách theo service, một dòng độc chặn đầu hàng.
- Hoàn tiền thủ công của admin không cập nhật `orders.payment_status`; đơn `REFUNDED` không tự nhập lại kho; hoàn tiền đơn COD khi duyệt trả hàng chỉ đổi nhãn `payment_status` (không có dòng tiền); payment `PROCESSING` không tự hết hạn.
- `fact_order_status` giữ dòng có `at` mới nhất cho mỗi (đơn, trạng thái): đơn bị từ chối trả hàng (`DELIVERED` lần hai) làm dịch ngày ghi nhận doanh thu COD.
- Migration Postgres đã chuyển sang goose có version (5 service, ADR-26); `accounts.username` vẫn unique toàn cục.
- Tạo payment bất đồng bộ nên `POST /orders` thường chưa kèm `payment` (client poll `GET /checkouts/:id`).
- Seed demo gán sản phẩm cho `seller_id=2` nhưng không tạo store nào cho `seller1`.

## ADR
Danh sách đầy đủ ở [01_ADR_LOG.md](../../decisions/01_ADR_LOG.md). Ánh xạ từ các đề xuất ban đầu:

| Đề xuất | Trạng thái | ADR |
|---|---|---|
| P-1 liveness/readiness tách; critical vs non-critical | ✅ đã nhận | ADR-9 |
| P-2 cổng admin :8081 riêng, không publish | ✅ | ADR-9 |
| P-3 thanh toán mô phỏng cùng hình dạng thật | ✅ | ADR-16 |
| P-4 `SUCCESS` → `AWAITING_PAYMENT`/`CONFIRMED`; doanh thu chỉ khi PAID hoặc COD đã giao | ✅ | ADR-21, ADR-22 |
| P-5 giữ chỗ có hạn + sổ cái bất biến | ✅ | ADR-21 |
| P-6 ClickHouse cho analytics, không là nguồn sự thật tiền; dựng lại từ Kafka/Postgres | 🟡 dựng lại từ Kafka (7 ngày); CLI backfill Postgres → ClickHouse đã có (chạy tay, chưa lịch) | ADR-14 |
| P-7 tracking ẩn danh có consent, ip băm, whitelist event, event nghiệp vụ chỉ từ server | ✅ | ADR-13 |
| P-8 chưa có cảnh báo, chỉ Grafana | ✅ | ADR-12 |
| P-9 cart ở order-service; payment tách service | ✅ | – |
| P-10 tách đơn theo shop | ✅ | ADR-15 |
| P-11 admin không đăng ký công khai | ✅ | ADR-10 |
| (mới) tracking qua gRPC thay Kafka; fact `ReplacingMergeTree` không MV; múi giờ cố định; `*_minor` trong event; auth theo route; chDB shim; outbox chung; payment async; analytics/payment non-critical | ✅ | ADR-13, 14, 17, 18, 19, 20, 23, 24, 25 |

## Rủi ro (còn hiệu lực)

| Rủi ro | Giảm thiểu |
|---|---|
| Hạ tầng chưa chạy thật (Swarm/ClickHouse/Grafana) | lần deploy đầu đi theo checklist `02_DEPLOYMENT.md`, kiểm Targets/`/ready`; chạy e2e mới (đã viết, chưa chạy) |
| Số liệu doanh thu lệch | định nghĩa duy nhất (ADR-22) + test; **đối soát mới là CLI chạy tay** (`reconcile`), chưa có lịch đêm / `data-health` |
| Metrics bùng nổ cardinality | route mẫu, không nhãn id (đang tuân thủ) |
| Thanh toán mô phỏng bị nhầm là thật | nhãn MÔ PHỎNG (`simulated:true`, `payments/methods`), `PAYMENTS_MODE=mock` bắt buộc, không lưu thẻ |
| Rò dữ liệu chéo shop | scope từ JWT ở gateway + kiểm lại ở analytics-service/ order-service; test chéo shop ở handler/service |
| Mất dữ liệu analytics khi ClickHouse/analytics down lâu | đệm 100k dòng; Kafka 7 ngày; xem `03_RUNBOOK.md` §8 |

## Câu hỏi còn mở
- Có cần materialized view/ rollup khi dữ liệu lớn (ADR-14)?
- Tách outbox theo service và thêm dead-letter cho dòng độc?
- Chạy thật lần đầu e2e mới và `resilience.sh` (payment/analytics down, ClickHouse down) rồi sửa chỗ lệch; kiểm chứng nhiều replica cho order/payment worker.
