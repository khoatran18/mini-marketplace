# 02 · Chuẩn Health / Ready / Metrics

Áp dụng cho **mọi service tự viết** (gateway, auth, user, product, order, payment, analytics), frontend, và **mọi container hạ tầng**. Mục tiêu: orchestrator (Swarm/Traefik), Prometheus và con người biết chính xác *"sống chưa", "nhận request được chưa", "đang chạy phiên bản nào", "đang tải thế nào"*.

> **Trạng thái: ✅ ĐÃ TRIỂN KHAI cho cả 7 service Go** (gateway, auth, user, product, order, payment, analytics). Chưa chạy thử trên Swarm thật (xem [09-roadmap.md](09-roadmap.md)).

## 0. Hiện trạng và quyết định
Ban đầu chỉ `api-gateway` có HTTP; các service nội bộ chỉ có gRPC nên Prometheus/Docker không quan sát được. Đã chọn **cách A: cổng admin HTTP `:8081` riêng** (ADR-9) – mỗi service chạy thêm một `http.Server` nhỏ (`/health /healthz /ready /readyz /metrics /version`) cạnh gRPC bằng gói `pkg/ops` (**7 bản giống hệt nhau**, kiểm bằng `md5sum services/*/pkg/ops/ops.go`), đồng thời đăng ký `grpc.health.v1`. Gateway giữ `:8080` công khai (`GET /health`, `/healthz`) và mở `:8081` cho `/ready`, `/metrics`, `/version`. Cổng admin **không publish, không route qua Traefik**; payment-service gắn thêm `POST /internal/payments/webhook` lên cổng này.

> **Tên endpoint**: hỗ trợ cả hai kiểu – `/health` = `/healthz` (liveness), `/ready` = `/readyz` (readiness), cùng kết quả. Docker/Traefik trong repo dùng `/healthz` và `/ready`.
> **Đã làm**: `pkg/ops`, `:8081`, `grpc.health.v1`, metrics gRPC/HTTP/DB pool/build info + Go/process (nhãn `service`), tắt êm theo SIGTERM (`DRAIN_SECONDS`), healthcheck Docker + nhãn Traefik, frontend `/api/health|healthz|ready|readyz`, `GET /admin/system/health` (gom `/ready` của 7 service) + trang `/admin/system`, stack Prometheus/Grafana (5 dashboard), metric outbox (product/order/payment), worker (order/payment), pipeline (analytics). **Chưa làm**: `mm_kafka_consumer_lag`/`mm_grpc_client_*`/metric nghiệp vụ, `/admin/system/metrics`, `update_config: start-first` + `wait-ready.sh`.

## 1. Khác nhau giữa các endpoint
| Endpoint | Câu hỏi | Kiểm tra gì | Khi lỗi | Ai dùng |
|---|---|---|---|---|
| `GET /health` · `/healthz` (liveness) | "Tiến trình còn sống, không bị treo?" | **Chỉ** bản thân process (HTTP server trả lời được; tuỳ chọn: goroutine chính/ worker heartbeat không quá cũ). **Không** gọi DB/Kafka/service khác | 200 `{"status":"ok"}`; không 200 ⇒ restart container | Docker `HEALTHCHECK`, Swarm |
| `GET /ready` · `/readyz` (readiness) | "Nhận được traffic và xử lý đúng chưa?" | Dependency **bắt buộc** (bảng 3) + trạng thái khởi động xong + không đang drain | 200 hoặc **503** kèm JSON chi tiết từng check | Traefik LB healthcheck, `deploy.sh` (chờ ready), e2e, UI trạng thái hệ thống |
| `GET /metrics` | "Số đo Prometheus" | – | – | Prometheus |
| `GET /version` | "Đang chạy build nào?" | – | – | người, CI, dashboard "đã deploy gì" |

Tại sao tách *liveness* và *readiness*: nếu `health` kiểm tra Postgres thì khi Postgres chậm, Swarm sẽ **restart cả đàn service** (cascade failure). Dependency lỗi ⇒ chỉ *rút khỏi load balancer* (ready 503), không giết process.

## 2. Hợp đồng phản hồi
```jsonc
// GET /health  → 200
{ "status": "ok" }

// GET /ready   → 200 (hoặc 503 nếu có check critical fail)
{
  "status": "ready",              // ready | degraded | not_ready
  "service": "order-service",
  "version": "1.4.2",
  "uptime_s": 3811,
  "checks": {
    "postgres":   { "status": "ok",        "latency_ms": 2,  "critical": true },
    "kafka":      { "status": "ok",        "latency_ms": 11, "critical": true },
    "product-service": { "status": "ok",   "latency_ms": 4,  "critical": true },
    "redis":      { "status": "fail",      "error": "timeout", "critical": false },
    "outbox_worker": { "status": "ok",     "last_tick_age_s": 1.2, "critical": true },
    "migrations": { "status": "ok",        "critical": true }
  }
}
// GET /version → 200
{ "service":"order-service", "version":"1.4.2", "commit":"2279fee", "built_at":"2026-10-06T08:00:00Z", "go":"go1.22" }
```
Quy tắc:
- `status`: `ready` (mọi check ok) · `degraded` (chỉ check **non-critical** fail → vẫn **HTTP 200**) · `not_ready` (≥1 check critical fail hoặc đang drain/chưa khởi động xong → **HTTP 503**).
- Mỗi check có **timeout 1 s**, chạy **song song**, kết quả **cache 3 s** (tránh DDoS chính DB bởi healthcheck dày).
- Không lộ thông tin nhạy cảm (DSN, mật khẩu, địa chỉ nội bộ chi tiết) trong `error`; chỉ mã ngắn (`timeout`, `refused`, `auth`).
- Không cần xác thực nhưng **chỉ lắng nghe cổng admin :8081 trong mạng nội bộ**. Riêng gateway/frontend có thêm bản công khai tối thiểu cho Traefik (mục 6).
- `/metrics` luôn 200 kể cả khi not_ready (để quan sát chính lúc đang lỗi).

## 3. Ma trận readiness theo service (thực tế, theo `cmd/main.go` từng service)
`C` = critical (fail ⇒ 503) · `N` = non-critical (fail ⇒ degraded, vẫn 200).

| Service | Postgres | Redis | Kafka | gRPC downstream | Khác |
|---|---|---|---|---|---|
| api-gateway | – | N | C | `AuthClient` C · `OrderClient` C · `ProductClient` C · `UserClient` C · `PaymentClient` **N** · `AnalyticsClient` **N** | chưa drain (SIGTERM ⇒ 503) |
| auth-service | C | N | C | – | |
| user-service | C | N | C | auth-service C | |
| product-service | C | N | C | – | |
| order-service | C | N | C | product-service C | `timers_worker` C (không tick quá 3 × 30 s) |
| payment-service | C | N | C | – | `workers` C (không tick quá 30 × 1 s) |
| analytics-service | – (không dùng Postgres) | N (chỉ khi có `REDIS_ADDR`) | C | – | **`clickhouse` C**; `insert_pipeline` N (có hàng đợi mà > 1 phút chưa insert được) |
| frontend | – | – | – | – | gateway N |
Lưu ý: check Kafka chỉ **dial TCP** tới broker (chưa kiểm consumer/lag); chưa có check MinIO/migrations/signer key (không cần hoặc chưa có). Khác thiết kế: payment-service **không** coi Redis là critical, product-service không có check outbox worker.

Cách kiểm tra từng loại:
| Dependency | Cách check (timeout 1 s) |
|---|---|
| Postgres | `db.PingContext` + `SELECT 1`; (tuỳ chọn) pool không cạn: `in_use < max_open` |
| Redis | `PING` |
| Kafka | dial TCP tới broker (chưa kiểm consumer/lag) |
| gRPC downstream | gọi `grpc.health.v1.Health/Check` của service đó (mỗi service gRPC **phải** đăng ký health server) |
| ClickHouse | `ch.Client.Ping` = HTTP `GET /ping` |
| MinIO | – (chưa có MinIO) |
| Worker (order timers, payment workers) | thời điểm tick cuối; fail nếu quá ngưỡng (order 3 × 30 s, payment 30 × 1 s); outbox worker **chưa** có check |
| Migration | – (service chỉ mở cổng sau khi migrate; `/ready` 503 cho tới `MarkStarted`) |

## 4. gRPC health
Mỗi service gRPC đăng ký `grpc.health.v1.Health` (service name rỗng = toàn bộ; thêm tên con như `order.OrderService`). `SERVING` khi ready; `NOT_SERVING` khi drain/critical fail. Gateway và `grpc_health_probe` dùng cái này. gRPC **reflection hiện được bật ở mọi môi trường** (`reflection.Register` trong `cmd/main.go`; thiết kế là chỉ dev – chưa tắt theo `ENV`).

## 5. Vòng đời & graceful shutdown
```
start ─▶ [startup: chưa ready: /health 200, /ready 503] ─▶ migrate, kết nối, nạp cache ─▶ READY
SIGTERM ─▶ đặt NOT_SERVING + /ready 503 ─▶ chờ `DRAIN_SECONDS` (mặc định 10 s) cho LB rút ─▶ GracefulStop gRPC (ép dừng sau 20 s) / http.Shutdown (timeout 20 s) ─▶ tắt cổng admin ─▶ thoát
```
Đã làm đúng tới đây (`pkg/ops` `handleSignals`). **Chưa làm**: dừng có chủ đích consumer Kafka (commit offset) và outbox/timer worker khi tắt (chúng dùng context nền và chết cùng process; an toàn vì outbox at-least-once, consumer idempotent). Swarm: `stop_grace_period: 30s`.

## 6. Cổng, docker healthcheck, Traefik
| Thành phần | Probe | Cấu hình |
|---|---|---|
| Container Go (mỗi service) | healthcheck compose → **`/healthz`** trên `:8081` (`wget -qO-` trong image alpine) | `interval 10s, timeout 3s, retries 3, start_period 20s` (analytics 30s), `stop_grace_period: 30s` |
| api-gateway qua Traefik | label `…healthcheck.path=/ready`, `.interval=5s`, `.timeout=2s`, `.port=8081` | ✅ rút replica not_ready khỏi LB; public `GET /health` trả `{"status":"ok"}` |
| frontend | `GET /api/health` (liveness), `GET /api/ready` (ready: gọi gateway `/health`) | Next route handler; nhớ `HOSTNAME=0.0.0.0` (đã ghi trong deployment.md) |
| Rolling update | **chưa làm**: `update_config: order: start-first, monitor: 30s, failure_action: rollback` và `scripts/wait-ready.sh` không có trong repo | thiết kế: task mới `healthy` rồi mới tắt task cũ |
| Hạ tầng | xem bảng 7 | |

## 7. Health/ready của container hạ tầng (thực tế trong compose)
| Container | Healthcheck (compose) | Metrics |
|---|---|---|
| postgres | `pg_isready -U $POSTGRES_USER -d $POSTGRES_DB` | `postgres-exporter :9187` |
| redis | `redis-cli ping` | `redis-exporter :9121` |
| kafka (KRaft) | `kafka-broker-api-versions.sh --bootstrap-server localhost:9092` | `kafka-exporter :9308` (lag, offset) |
| clickhouse | `wget -qO- http://localhost:8123/ping` | ❌ chưa bật endpoint metrics/exporter của ClickHouse server |
| minio | – (chưa có MinIO) | – |
| prometheus | `wget … :9090/-/ready` | tự scrape |
| grafana | `wget … :3000/api/health` | – (chưa scrape) |
| cadvisor | `wget … :8080/healthz` | `/metrics` |
| node-exporter | `wget … :9100/` | `/metrics` |
| traefik | `traefik healthcheck --ping` (`--ping=true`) | `--metrics.prometheus` entrypoint `metrics` `:8082` |
| postgres/redis/kafka-exporter | – (chưa có healthcheck) | – |
Mọi service Go: Docker healthcheck `/healthz` :8081 (xem §6). Frontend: `wget :3000/api/healthz`.

## 8. `/metrics` (Prometheus) – thực tế
Thư viện: `prometheus/client_golang`, registry riêng trong `pkg/ops`; collector Go + process (nhãn `service`). Tiền tố `mm_` cho metric tự định nghĩa.

**Có ở mọi service Go** (đã triển khai)
| Metric | Loại | Nhãn | Ghi chú |
|---|---|---|---|
| `mm_build_info` | gauge = 1 | `service, version, commit` | |
| `mm_ready` | gauge 0/1 | `service` | 1 khi `/ready` là ready hoặc degraded |
| `mm_http_requests_total` | counter | `service, method, route, status` | chỉ gateway; `route` là **mẫu** (`/orders/:id`), `unmatched` khi không khớp |
| `mm_http_request_duration_seconds` | histogram | `service, method, route` | bucket 5 ms … 10 s |
| `mm_grpc_server_handled_total` | counter | `service, grpc_method, grpc_code` | interceptor (unary + stream) |
| `mm_grpc_server_handling_seconds` | histogram | `service, grpc_method` | |
| `mm_db_pool_open_connections`, `_in_use_connections`, `_idle_connections` | gauge | `service` | từ `sql.DBStats` (service có Postgres) |
| `mm_db_pool_wait_seconds_total` | counter | `service` | |
| `go_*`, `process_*` | chuẩn | `service` | |
**Chỉ ở một số service**
| Metric | Service | Ghi chú |
|---|---|---|
| `mm_outbox_pending{table="domain_events"}`, `mm_outbox_oldest_age_seconds` | product, order, payment | đếm **cả bảng chung** `domain_events` (xem data-model §1.5) |
| `mm_worker_last_tick_timestamp_seconds{worker}` | order (`order_timers`), payment (`payment_workers`) | |
| `mm_ch_buffer_rows`, `mm_ch_insert_lag_seconds`, `mm_ch_rows_inserted_total`, `mm_ch_insert_failures_total`, `mm_ch_rows_dropped_total`, `mm_events_accepted_total`, `mm_events_rejected_total` | analytics | đăng ký là gauge (giá trị tăng dần) |
**CHƯA có** (thiết kế, chưa làm): `mm_grpc_client_*`, `mm_kafka_consumer_lag`, `mm_kafka_messages_processed_total`, `mm_outbox_published_total`, `mm_db_query_duration_seconds`, metric nghiệp vụ (`mm_orders_*`, `mm_payments_total`, `mm_refunds_total`, `mm_webhook_deliveries_total`, `mm_inventory_reserve_total`, `mm_products_low_stock`, `mm_rate_limited_total`), `mm_reconcile_diff_ratio`. Lag Kafka hiện lấy từ kafka-exporter (`kafka_consumergroup_lag`).
**Quy tắc**: không đặt `user_id`, `order_id`, `product_id` làm nhãn; chỉ lộ `/metrics` trong mạng nội bộ; middleware đo **sau** khi router khớp để có `route` mẫu.

## 9. API vận hành khác
| Endpoint | Trạng thái |
|---|---|
| `GET /version` | ✅ mọi service (`{service, version, commit, built_at}`) |
| `GET /admin/system/health` (gateway, role admin) | ✅ gom `/ready` của 7 service (timeout 2 s mỗi probe) |
| `GET /admin/system/metrics?q=…` (proxy Prometheus whitelist) | ❌ chưa làm |
| `GET /admin/system/outbox`, `/admin/system/kafka`, `POST /admin/system/outbox/requeue` | ❌ chưa làm (dùng SQL/runbook) |
| `GET /debug/pprof/*` | ✅ tắt mặc định; chỉ bật bằng `PPROF_ENABLED=true` trên :8081 |
| `GET /config/features` | ❌ |

## 10. Kiểm thử
- ✅ Unit `pkg/ops/ops_test.go` (7 bản): liveness không đụng dependency, ready/degraded/not_ready, lỗi đã làm sạch, timeout, cache, draining, nhãn metric, test socket thật cho cổng admin + gRPC health. Gateway: `TestAdminSystemHealthIsAdminOnly` (ready/not_ready/unreachable).
- ❌ E2E (`scripts/e2e`): dừng Postgres ⇒ `/ready` 503 mà container không restart; SIGTERM ⇒ 503 trước khi thoát; Prometheus target `up==1` – chưa viết/ chưa chạy (xem [../testing.md](../testing.md) §5).
- ❌ CI lint tên metric/nhãn; test chung "đủ 4 endpoint".
