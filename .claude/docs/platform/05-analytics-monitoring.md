# 05 · Analytics và giám sát hệ thống (Grafana)

> **Trạng thái: ✅ analytics-service + ClickHouse ĐÃ TRIỂN KHAI; 🟡 Grafana 5/10 dashboard; ❌ cảnh báo (cố ý chưa làm).** Docker/Swarm/ClickHouse *server*/Grafana chưa từng chạy trong sandbox: SQL được xác thực trên chDB 26.9 + unit test, ClickHouse 24.8 thật chỉ chạy trong CI (xem [09-roadmap.md](09-roadmap.md) "Giới hạn đã biết").

## 1. Analytics (analytics-service + ClickHouse)
### 1.1 Pipeline thực tế
```
Trình duyệt ─ POST /events ─▶ gateway (gắn user/role/store/ip-hash, ≤50 event/64 KB) ─ gRPC IngestEvents ─▶ analytics-service ─┐
order · payment · product ─ outbox ─ Kafka (6 topic) ─▶ consumer `analytics-service-<topic>` ───────────────────────────────────┤
                                                                                       bộ đệm RAM (Sink) ─ lô 2 s / 5000 dòng ─▶ ClickHouse
gateway ─ gRPC Query (scope từ JWT) ─▶ analytics-service ─ SQL trên bảng fact (FINAL) + cache Redis 30 s ─▶ JSON
```
- **Tracking đi gRPC, không qua Kafka** (`tracking.events` không tồn tại – ADR-13).
- Bộ đệm `Sink`: gom theo bảng, insert `JSONEachRow` mỗi `ANALYTICS_FLUSH_INTERVAL_MS` (2000) hoặc khi đủ `ANALYTICS_FLUSH_ROWS` (5000); lỗi thì giữ lại thử vòng sau; vượt `ANALYTICS_BUFFER_MAX_ROWS` (100000) thì **rớt dòng cũ nhất** và đếm. Dòng idempotent nên retry vô hại. Consumer Kafka commit khi dòng đã vào đệm.
- Consumer idempotent nhờ `ReplacingMergeTree` khoá theo id tự nhiên (ADR-14). Bảng: `events_raw`, `fact_order_status` (một dòng cho mỗi **(đơn, trạng thái)**), `fact_order_items`, `fact_payment_events`, `fact_refunds`, `dim_products`, `fact_inventory` ([../data-model.md](../data-model.md) §3). **Không có materialized view**: mọi báo cáo gom bảng fact lúc truy vấn với `FINAL`.

### 1.2 Báo cáo (gateway: `/seller/analytics/*`, `/admin/analytics/*`; JSON: [../api.md](../api.md) §9)
| Report | Seller (store) | Admin (platform) | Nội dung |
|---|---|---|---|
| `summary` | ✅ | ✅ | `current` & `previous` (kỳ liền trước cùng độ dài): `revenue, refunds, net_revenue, gmv, orders_placed, orders_recognized, aov, canceled, expired, refunded, cancel_rate` |
| `timeseries` | ✅ | ✅ | doanh thu/ hoàn tiền/ đơn ghi nhận/ đơn đặt theo `hour|day|week|month` (giờ VN), điền bucket rỗng |
| `top-products` | ✅ | ✅ | xếp hạng `revenue|units|views|conversion|viewed_unsold`; kèm impressions/clicks/views/carts |
| `funnel` | ✅ (checkout = null) | ✅ (+ `by_device`) | `view → cart → checkout → paid`, đếm **session**; `paid` = đơn đã ghi nhận doanh thu |
| `low-stock` | ✅ | ✅ | tồn thấp/ hết, hoặc hết trong ≤ 7 ngày theo tốc độ bán 14 ngày |
| `traffic` | ❌ | ✅ | page view, session, visitor, mới/quay lại, series, top path/referrer/device/country |
| `payments` | ❌ | ✅ | thành công/ thất bại theo **lần thử**, theo method, top `failure_code`, hoàn tiền, đơn đang chờ thanh toán |
| `search_terms` | ❌ | ✅ | top truy vấn, truy vấn 0 kết quả (từ `props.q`/`props.result_count`) |
| `data_health` | ❌ | ✅ | số dòng, event cuối, độ trễ ingest 15 phút từng bảng; event 1 giờ qua theo loại; `reconciliation:"not_implemented"` |
Khác thiết kế: không có `stockout-forecast`/`payments-breakdown`/`export.csv`/`top-stores`/`top-categories`/`products/:id/funnel` riêng; "chuyển đổi" = units / product_view; `new_visitors` tính trên toàn bộ `events_raw` (không `FINAL`).
- **Phản hồi**: `{as_of, source:"clickhouse", cached, timezone:"Asia/Ho_Chi_Minh", data}`. Cache Redis `analytics:v1:*` **30 s** (`ANALYTICS_CACHE_TTL_SEC`; không cache `data_health`). Khoảng ≤ **400 ngày** (trừ `low_stock`, `data_health`), mặc định 30 ngày.
- **Múi giờ cố định `Asia/Ho_Chi_Minh` (UTC+7, không DST)**: dữ liệu UTC, nhóm/ nhãn theo giờ VN (`toTimeZone`), `from/to` dạng `YYYY-MM-DD` đọc theo giờ VN (`to` bao gồm); không có bộ chọn múi giờ (ADR-17).

### 1.3 Định nghĩa doanh thu (một nơi – `ordersCTE`)
- Đơn **được ghi nhận** khi từng có trạng thái `PAID` (phương thức online) hoặc `DELIVERED` (COD) trong `fact_order_status`; doanh thu = **`subtotal`** (tiền hàng, **không** gồm ship) tại thời điểm đó; số đơn ghi nhận = `orders_recognized`; `aov = revenue / orders_recognized`.
- **Hoàn tiền** (`fact_refunds`) trừ **theo thời điểm hoàn**; `net_revenue = revenue − refunds`. (Hoàn tiền không đưa doanh thu của kỳ gốc về lại.)
- **GMV** = `subtotal` của đơn đặt trong kỳ, trừ đơn `EXPIRED`, `FAILED`, và `CANCELED` chưa từng `PAID`. `cancel_rate = (canceled + expired) / orders_placed`.
- Seller chỉ thấy đơn/ sản phẩm của store mình: lọc `store_id` ở SQL (đơn) và `product_id IN dim_products(store)` (event sản phẩm); `refunds` lọc qua đơn của store.
- ClickHouse là nơi *phân tích*, **không** là nguồn sự thật tiền: dựng lại một phần từ Kafka (7 ngày); **backfill từ Postgres và đối soát đêm (`DataHealth`) CHƯA làm**.

### 1.4 Quyền & consent & PII
- Gateway điền `Scope{role, store_id}` từ JWT (`seller` + store của người gọi, hoặc `admin`); **không** nhận `store_id` từ client. analytics-service từ chối scope `seller` thiếu `store_id` (400), báo cáo chỉ-admin với seller (403), role khác (403). Có test chéo scope (handler gateway + `TestQueryAccessRules` + store test).
- **Consent**: `consent.analytics=false` ⇒ chỉ giữ `page_view`/`error_client`, **xoá** `anonymous_id`, `session_id`, `user_id`, `ip_hash`, `product_id`, `props`; loại khác bị từ chối `no_consent`. `identify` luôn tính là có consent (chỉ khi đã đăng nhập).
- **PII**: email/số điện thoại trong chuỗi `props` bị thay `[email]`/`[phone]`; `path` bỏ query/fragment; `referrer` giữ host+path; `props` ≤ 2 KB và phải là object JSON; `ts_client` chỉ tin trong ±24 h; `ip_hash` = HMAC(`IP_HASH_SECRET`, ngày UTC | IP) xoay mỗi ngày, IP thô không lưu. Server-only event (`order_placed`…) từ client bị loại (`unknown_or_server_only_type`).
- Retention `events_raw`: `EVENTS_RETENTION_MONTHS` (13) bằng TTL; bảng fact giữ vô hạn. **Chưa làm**: `DELETE /users/me/data` (quyền xoá), bảng `consents`, `identity_links`.

### 1.5 Quan sát pipeline
Metric (cổng :8081): `mm_ch_buffer_rows`, `mm_ch_insert_lag_seconds`, `mm_ch_rows_inserted_total`, `mm_ch_insert_failures_total`, `mm_ch_rows_dropped_total`, `mm_events_accepted_total`, `mm_events_rejected_total` (đăng ký dạng gauge nhưng giá trị là bộ đếm tăng dần – dùng `rate()/increase()`). Readiness: `clickhouse` critical; `insert_pipeline` non-critical (có hàng đợi mà > 1 phút không insert được). Dashboard **Analytics pipeline**.

## 2. Giám sát hệ thống (CPU, RAM…)
- Nguồn: cAdvisor (CPU/RAM/mạng theo container, nhãn swarm service), node-exporter (máy), metric ứng dụng (`mm_*`, [02 §8](02-health-ready-metrics.md)), postgres/redis/kafka-exporter, Traefik. **Chưa có** exporter/metric riêng cho ClickHouse server, MinIO (chưa có).
- Trong app: `GET /admin/system/health` (bảng service × `ready/degraded/not_ready/unreachable` × version × uptime) ✅. `GET /admin/system/metrics`, `/admin/system/outbox`, requeue ❌ chưa làm.
- Giới hạn bộ nhớ (`resources.limits.memory`) đã đặt cho service Go (256–384 MB), ClickHouse (2 GB) và container giám sát ⇒ panel "Memory % of limit" có nghĩa.

### 2.1 Đã triển khai
- `deploy/observability.yml` (stack `marketplace-obs`): Prometheus v2.55.1 (retention `PROMETHEUS_RETENTION`, mặc định 15d), Grafana 11.3.0 (`https://grafana.$PUBLIC_HOST`, đăng nhập `GRAFANA_ADMIN_*`, không anonymous), cAdvisor + node-exporter (global), postgres/redis/kafka-exporter. **Không có Alertmanager.**
- `deploy/prometheus/prometheus.yml`: scrape `tasks.<service>:8081/metrics` cho **7** service Go bằng DNS service discovery của Swarm (thêm replica không cần sửa cấu hình), cAdvisor, node-exporter, 3 exporter, Traefik (`:8082`).
- Grafana provisioning bằng file (`deploy/grafana/provisioning`, dashboard JSON `deploy/grafana/dashboards`, `allowUiUpdates:false`), folder *Marketplace*, làm mới 15 s:
| Dashboard (uid) | Panel chính |
|---|---|
| **Overview** (`mm-overview`) | targets up/down, services not ready (`mm_ready`), gateway 5xx %/p95/RPS, bảng `mm_build_info`, CPU/RAM theo swarm service |
| **Services (RED)** (`mm-services`, biến `service`) | gateway theo route (req/s, 5xx, p95, status, 429/unmatched); gRPC server theo service/method (calls, lỗi theo `grpc_code`, p95, method chậm); CPU, RSS, goroutine; pool DB |
| **Containers & hosts** (`mm-containers`) | cAdvisor: CPU, RAM, RAM % limit, mạng, restart 1 h; node-exporter: CPU %, RAM %, disk %, load, mạng, I/O |
| **Kafka · Postgres · Redis · Traefik** (`mm-infra`) | lag theo group/topic, msg/s; connections, tps, cache hit; RAM/ops/clients Redis; req/s & 5xx Traefik |
| **Analytics pipeline** (`mm-analytics`) | hàng chờ, tuổi hàng cũ nhất, batch lỗi, hàng rớt, rows/s, event/s, lag group `analytics-service-*`, p95 `Query` |
- Cấu hình và PromQL được kiểm cú pháp bằng thư viện Prometheus (`config.LoadFile` + `promql/parser`); **chưa chạy thử trên Swarm thật** – lần deploy đầu cần kiểm Targets và từng dashboard.
- Chạy: `scripts/deploy.sh observability` (hoặc `all`). Cách đọc dashboard & xử lý sự cố: [../runbook.md](../runbook.md).

## 3. Cảnh báo: CHƯA LÀM (cố ý)
Quyết định (ADR-12): **không có cảnh báo.** Không `alert-service`, Alertmanager, bảng `alerts/notifications`, chuông, SSE, email. Chỉ có Grafana để người vận hành tự nhìn. Ngưỡng ở bảng dưới chỉ là gợi ý **tô màu** (một số đã có trên ô stat của dashboard); khi sau này muốn cảnh báo, thêm Alertmanager + rule trên đúng các metric này, không cần đổi code service.

### 3.1 Panel/ngưỡng tham khảo – trạng thái
| Dashboard thiết kế | Panel | Vàng / Đỏ | Trạng thái |
|---|---|---|---|
| Overview | service ready (`mm_ready`), replicas | ready=0 → đỏ | ✅ |
| Services (RED) | RPS, 5xx %, p95 | 5xx >1 % / >2 %; p95 >0,5 s / >1 s | ✅ (màu trên Overview) |
| Containers | CPU/RAM % limit, restart | RAM >80/90 % | ✅ (panel, chưa tô màu theo ngưỡng) |
| Nodes | CPU, RAM, disk, load | disk >80/90 % | ✅ (gộp vào *Containers & hosts*) |
| Kafka & Outbox | lag theo group/topic | lag >500/>1000 | 🟡 lag ✅; `mm_outbox_pending`/`oldest_age` **chưa có panel** (metric đã có) |
| Postgres / Redis | connections, cache hit, pool | connections >70/85 % | ✅ |
| ClickHouse | insert lag, disk, merges | lag >60/>120 s | 🟡 insert lag ở *Analytics pipeline* (>60/>120 s); metric của ClickHouse server ❌ |
| Payments | `mm_payments_total`, tỉ lệ thành công | thành công <90/<80 % | ❌ (metric chưa có; dùng báo cáo `payments` của analytics) |
| Business | đơn/phút, `mm_orders_awaiting_payment` | – | ❌ (dùng báo cáo analytics) |
| Gateway | rate-limited, events nhận/loại | – | 🟡 429 ở *Services*; `mm_events_*` ở *Analytics pipeline* |

### 3.2 "Cần chú ý" trong ứng dụng (không phải cảnh báo)
Là **truy vấn thường**, người dùng mở trang mới thấy: seller – `low-stock` (tồn thấp/ sắp hết), sản phẩm nhiều lượt xem 0 đơn (`top-products?sort=viewed_unsold`); admin – trạng thái service (`/admin/system/health`) và `data-health` (độ trễ pipeline; **đối soát chưa có**). Đơn đã thanh toán chưa giao > 24 h: chưa có báo cáo riêng.

### 3.3 Runbook
Đã viết: [../runbook.md](../runbook.md) (đọc dashboard, Kafka lag, outbox, ClickHouse down, payment thất bại cao, sao lưu).
