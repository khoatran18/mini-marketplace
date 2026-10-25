# Vấn đề còn tồn đọng

> Gom một chỗ những gì **chưa làm / chưa kiểm chứng / có rủi ro**, rút từ [../design/platform/09_ROADMAP_STATUS.md](../design/platform/09_ROADMAP_STATUS.md), [02_TECH_DEBT_AND_BACKLOG.md](02_TECH_DEBT_AND_BACKLOG.md), [../design/05_SECURITY.md](../design/05_SECURITY.md) và đối chiếu thêm với code / repo. Mức ưu tiên là **đề xuất** (P1 = nên làm trước khi demo thật hoặc trước khi gắn AI; P2 = quan trọng; P3 = để sau).

```mermaid
mindmap
  root((Tồn đọng))
    Chưa kiểm chứng
      Swarm Traefik Grafana chưa chạy thật
      e2e mới viết, chưa chạy thật
      chưa thử nhiều replica
    Tính năng thiếu
      review wishlist
      upload ảnh
      consent và xóa dữ liệu
      admin users stores
    Độ tin cậy
      outbox dùng chung, dòng độc chặn hàng
      mất tracking khi analytics restart
      backfill ClickHouse mới là CLI
      Kafka 3 broker; topic do service tạo có RF = min(3, số broker) (đã kiểm: RF 3, ISR đủ)
    Bảo mật
      xoay secret trong lịch sử git
      token trong localStorage
      gRPC plaintext
      thẻ img canary trong README
    Quan sát
      thiếu tracing, metric nghiệp vụ
      Grafana 11 dashboard, chưa xem thật
    Công cụ
      Swagger viết tay
      7 bản sao module dùng chung
```

## 1. Chưa kiểm chứng (rủi ro lớn nhất)

| # | Vấn đề | Ưu tiên | Ghi chú |
|---|---|---|---|
| V1 | 🟡 Lần chạy thật đầu tiên (2026-10-26): 22 service lên, Traefik / Prometheus (18 target) / Grafana (11 dashboard provision, datasource OK) chạy; đã sửa healthcheck frontend, lỗi consumer Kafka 0 partition, HTTPS tự ký ([nhật ký](../process/03_SESSION_LOG_2026-10-26.md)). Còn: xem từng panel Grafana, chạy trên Kafka rỗng, nhiều node | P1 | Lần deploy đầu theo [../instruction/02_DEPLOYMENT.md](../instruction/02_DEPLOYMENT.md) §1 và xem `docker service logs` từng service |
| V2 | SQL ClickHouse thử trên chDB 26.9; ClickHouse 24.8 thật chỉ chạy ở CI | P2 | Có thể lệch ở chi tiết hiếm |
| V3 | 🟡 E2E đã viết lại (11 kịch bản, `resilience.sh` A..I, `scripts/load`); trên stack thật mới chạy `resilience.sh A` (đạt). Còn B..I, bộ chính `run.mjs`, `load`; chạy rồi sửa chỗ lệch contract | P1 | [../instruction/04_TESTING.md](../instruction/04_TESTING.md) §5 |
| V4 | Chạy nhiều replica cho service ngoài gateway chưa được kiểm chứng (worker timers của order-service, worker payment…) | P2 | Outbox dùng `SKIP LOCKED` nên có cơ sở, nhưng chưa có test |
| V5 | Frontend (nhánh UI) chưa được kiểm chứng end-to-end với backend thật | P2 | [../design/platform/07_UI_DESIGN.md](../design/platform/07_UI_DESIGN.md) |

## 2. Tính năng nghiệp vụ chưa làm

| # | Vấn đề | Ưu tiên |
|---|---|---|
| F1 | Review / wishlist (event `wishlist_add` đã whitelist nhưng chưa có API) — **cần cho gợi ý và sentiment** | P1 nếu làm AI |
| F2 | Upload ảnh sản phẩm (MinIO) — ảnh hiện chỉ là URL (≤ 8 / sản phẩm); cần cho visual search và OCR | P2 |
| F3 | Quyền riêng tư: bảng `consents`, `PUT /users/me/consent`, `DELETE /users/me/data` (consent chỉ đi kèm từng event) | P2 |
| F4 | An toàn tài khoản: khóa tạm khi sai mật khẩu nhiều lần, `audit_log`, xác thực email, đặt lại mật khẩu, logout (deny-list token), `GET /auth/me` | P2 |
| F5 | Quản trị: `/admin/users`, `/admin/stores` (duyệt / khóa), `/admin/system/metrics` (proxy Prometheus whitelist), `/admin/system/outbox` + requeue qua API | P2 |
| F6 | Analytics nâng cao: đối soát đêm Postgres ↔ ClickHouse: đã có CLI `backfill` / `reconcile` và `scripts/export` (xuất CSV / Parquet), chạy thật cục bộ; còn thiếu lịch đêm, nối `data-health` (vẫn trả `reconciliation: "not_implemented"`) và metric; materialized view khi dữ liệu lớn | P2 |
| F7 | Thông báo (email / in-app) và cảnh báo (Alertmanager) — **cố ý chưa làm** (ADR-12) | P3 |
| F8 | Thanh toán thật — hiện chỉ mô phỏng (ADR-16) | P3 |
| F9 | `GET /config/features` (feature flag) — cần nếu bật / tắt tính năng AI | P2 nếu làm AI |

## 3. Độ tin cậy và dữ liệu

| # | Vấn đề | Ưu tiên |
|---|---|---|
| R1 | 🟡 Tracking đi qua gRPC: đã có spool bền (`ANALYTICS_SPOOL_DIR`, volume `analytics-spool`): dòng tràn bộ đệm RAM hoặc còn đệm lúc SIGTERM được ghi file và replay khi khởi động / khi ClickHouse về (metric `mm_ch_rows_spooled_total`, `mm_ch_rows_replayed_total`, `mm_ch_spool_bytes`). Còn lại: crash cứng (SIGKILL / OOM) vẫn mất phần còn trong RAM; spool đầy (`ANALYTICS_SPOOL_MAX_MB`) hoặc tắt thì rớt dòng; chưa chạy trên Swarm | P2 |
| R2 | ✅ Đã xử lý (migration `00002_outbox_dead_letter`): lỗi theo từng dòng (không chặn key khác, giữ thứ tự cùng key), `attempts` + backoff `next_attempt_at`, quá `OUTBOX_MAX_ATTEMPTS` (10) thì `DEAD` (dead-letter, requeue bằng SQL ở runbook §5), metric `mm_outbox_dead` / `mm_outbox_publish_failures_total`. Còn lại: `domain_events` vẫn là một bảng chung của product / order / payment (backlog không tách theo service) | ~~P1~~ P3 |
| R3 | 🟡 Consumer Kafka thử 5 lần rồi chuyển message sang topic `<topic>.dlq` (đủ key / value / header + `dlq.*`), chỉ commit sau khi publish DLQ xong; metric `mm_kafka_dlq_total{topic}`; runbook §6. Còn lại: chưa có công cụ replay tự động (làm tay), chưa cảnh báo, chưa schema registry; Kafka 1 broker, RF 1; chưa chạy trên Swarm | P2 |
| R4 | 🟡 Đã có CLI `backfill` Postgres → ClickHouse cho bảng fact / dim (không dựng lại được `events_raw`; nhớ chỉ backfill vào khoảng trống để khỏi nhân đôi `fact_payment_events` / `fact_inventory`); Kafka vẫn chỉ giữ ~7 ngày | P2 |
| R5 | ✅ 2026-10-26 · ~~`AutoMigrate`~~ → goose có version (5 service, chưa chạy trên Swarm, ADR-26). `accounts` giờ UNIQUE `(username, role)` (auth migration `00002`), khớp với register / login / refresh vốn đã tra theo cặp này; cùng username ở role khác là hợp lệ | ~~P2~~ |
| R6 | ✅ 2026-10-26 (trừ COD) · `payment.refunded` mang thêm `payment_status`; admin hoàn **toàn bộ** payment (không gắn đơn) ⇒ order-service đặt `payment_status=REFUNDED` cho các đơn `PAID` của checkout (hoàn một phần không quy được cho đơn nào ⇒ giữ `PAID`). Đơn `REFUNDED` (duyệt trả hàng) nhập lại kho đúng một lần (product-service nghe `order.status_changed`, ledger `return`, migration `00003`). Payment `PROCESSING` quá `expires_at` + 1 giờ ⇒ `EXPIRED` (tiền về muộn vẫn được nhận và hoàn qua luồng cũ). **Cố ý chưa làm:** hoàn tiền COD vẫn chỉ đổi nhãn (tiền mặt trả ngoài hệ thống, không có payment để hoàn) | ~~P2~~ |
| R7 | ✅ 2026-10-26 · consumer `order.status_changed` bỏ qua lần vào lại `DELIVERED` từ `REFUND_REQUESTED` (trả hàng bị từ chối) nên `fact_order_status` giữ thời điểm `DELIVERED` đầu tiên; backfill vốn đã dùng `min(at)`. Chưa sửa dữ liệu đã ghi trước đó (chạy lại `backfill` cho khoảng ngày cần) | ~~P2~~ |
| R8 | ✅ 2026-10-26 · giữ thiết kế bất đồng bộ (ADR-24), đã ghi rõ ở API reference §Orders; frontend checkout giờ poll `GET /checkouts/:id` tối đa ~8 s để chuyển thẳng tới trang thanh toán | ~~P3~~ |
| R9 | ✅ 2026-10-26 · với `SEED_DEMO_DATA=true` user-service tạo store id 2 (id cố định, khớp `seller_id` của sản phẩm demo) cho `seller_admin` demo đầu tiên chưa có store và đẩy event `user.create_seller` để auth gán `store_id`; chỉ chạy khi chưa có seller nào, bỏ qua nếu đã có | ~~P3~~ |
| R10 | ✅ 2026-10-26 · user-service migration `00002` xoá bảng `create_seller_kafka_events` (không code nào dùng; struct `outbox.CreateSellerKafkaEvent` chỉ là payload / DTO nên giữ) | ~~P3~~ |

## 4. Bảo mật

| # | Vấn đề | Ưu tiên |
|---|---|---|
| S1 | Mọi bí mật từng commit trước khi dọn repo (JWT secret cũ, mật khẩu DB / Elastic, htpasswd Traefik, khóa TLS) **vẫn còn trong lịch sử git** → phải xoay (rotate) | P1 |
| S2 | Username / password bị giới hạn `^[a-zA-Z0-9_]{3,16}$` bởi hợp đồng protobuf — mật khẩu tối đa 16 ký tự là yếu | P2 |
| S3 | Token lưu `localStorage` (rủi ro XSS); nên chuyển cookie httpOnly | P2 |
| S4 | gRPC và Postgres giữa các service đang plaintext (`sslmode=disable`) | P3 |
| S5 | ClickHouse, Prometheus, cổng `:8081` không có xác thực (chỉ an toàn nhờ không publish cổng). **Đã làm một phần (🟡 chưa chạy trên Swarm)**: `METRICS_TOKEN` (bearer, tuỳ chọn) khoá `/metrics`, `/version`, pprof ở cả 7 service, Prometheus gửi token; ClickHouse bắt buộc mật khẩu và chỉ nhận kết nối từ loopback + dải private (`deploy/clickhouse/users-networks.xml`, đã thử bằng container thật). **Còn lại**: Prometheus chưa bật web basic auth (cố ý: không publish, chỉ Grafana truy cập); `/health`/`/ready` vẫn mở; webhook thanh toán đã ký HMAC | P2 |
| S6 | gRPC: đã có deadline + retry lệnh đọc ở **gateway** (🟡 chưa chạy thật); còn thiếu retry lệnh ghi có `Idempotency-Key`, circuit breaker, và deadline / retry ở order-service | P2 |
| S7 | `README.md` gốc có 2 thẻ `<img>` ẩn (kích thước 0×0, `display:none`) trỏ tới `canarytokens.com` — là ảnh theo dõi (canary token) chứ không phải tài liệu; nếu không chủ ý thì nên gỡ | P2 |

## 5. Quan sát (observability)

- Đã có request-id (`X-Request-ID`, 🟡 chưa vào Kafka / outbox), deadline / retry đọc ở gateway, log tập trung Vector → Loki (🟡 chưa chạy trên Swarm). Còn thiếu: distributed tracing, circuit breaker, chuẩn hoá log JSON trong service. Metric nghiệp vụ (`mm_orders_*`, `mm_payments_*`, `mm_inventory_*`, `mm_rate_limited_total`, `mm_grpc_client_*`) đã thêm và test đơn vị, chưa chạy stack thật; lag Kafka dùng `kafka_consumergroup_lag` của kafka-exporter.
- Grafana có **11** dashboard (ClickHouse server, Outbox & Workers, Payments, Business, Kafka consumer lag, Logs mới viết, chưa xem trên Grafana thật; PromQL chưa chạy trên Prometheus thật).
- 🟡 `update_config: start-first` / `failure_action: rollback` và `scripts/wait-ready.sh` đã viết, chưa chạy thử trên Swarm thật.
- postgres-exporter và kafka-exporter đã có healthcheck; redis-exporter không làm được (image scratch, không có shell/wget/curl), trạng thái xem qua `up{job="redis"}`.

## 6. Công cụ và tài liệu

- ✅ Swagger (`services/api-gateway/docs/swagger.{json,yaml}`, `docs.go`) đã cập nhật đủ 90 route/method hiện có (2026-10-26), nhưng **viết tay** (handler chưa có chú thích swag; chưa nối `/swagger` vào router). Chạy `swag init` sẽ ghi đè bằng spec thiếu; muốn sinh tự động phải chú thích handler. Nguồn sự thật vẫn là [../design/04_API_REFERENCE.md](../design/04_API_REFERENCE.md).
- ✅ `apps/frontend/README.md` đã viết lại theo app thật (Next.js 14, route, biến môi trường, build/deploy, tracking).
- ✅ 2026-10-26: `kafkaimpl`, `ops` (5 service), `events` đã gộp vào module `shared/` (ADR-33). Còn: protobuf `.pb.go` vẫn sao chép, `ops` của gateway/product (có metric riêng), chưa có `buf` registry.
- ✅ Đã xử lý: `payment-service` / `analytics-service` đã có `cmd/.env.example`; `ANALYTICS_FLUSH_*` / `ANALYTICS_BUFFER_MAX_ROWS` đã có trong `services.yml` và `.env.example`; địa chỉ gRPC đọc từ `*_SERVICE_ADDR`.
- Tài liệu trong `docs/` pha tiếng Việt / tiếng Anh (nhiều file kế thừa từ trước); chưa thống nhất.

## 7. Tồn đọng riêng cho việc gắn AI

Chi tiết và lý do: [../ai/04_AI_DATA_READINESS.md](../ai/04_AI_DATA_READINESS.md). Tóm tắt: chưa có review / wishlist, chưa có `request_id` + `model_version` trong envelope tracking (cần để đo CTR của gợi ý), chưa có feature flag, chưa có upload ảnh, chưa có bộ dữ liệu thật đủ lớn (dữ liệu demo là giả), chưa có `ai-service` và hạ tầng eval / quan sát LLM.
