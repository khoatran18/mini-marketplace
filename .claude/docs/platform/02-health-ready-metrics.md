# 02 · Chuẩn Health / Ready / Metrics

Áp dụng cho **mọi service tự viết** (gateway, auth, user, product, order, payment, analytics), frontend, và **mọi container hạ tầng**. Mục tiêu: orchestrator (Swarm/Traefik), Prometheus và con người biết chính xác *"sống chưa", "nhận request được chưa", "đang chạy phiên bản nào", "đang tải thế nào"*.

## 0. Hiện trạng: vì sao cần cổng HTTP
Kiểm tra code: **chỉ `api-gateway` có HTTP** (gin `:8080`, route `/health`). `auth/user/product/order` hiện chỉ có **gRPC server** (`:50051–50054`), không có HTTP, và chưa có healthcheck nào trong `deploy/services.yml` ngoài gateway và frontend. Hệ quả: Prometheus không scrape được (Prometheus cần HTTP `/metrics`), Docker không biết service có sống không, gateway không biết service nào sẵn sàng.

Có 3 cách; chọn **A**:
| Cách | Mô tả | Đánh giá |
|---|---|---|
| **A. Thêm cổng admin HTTP `:8081` riêng** (đề xuất) | mỗi service chạy thêm 1 `http.Server` nhỏ (`/health /ready /metrics /version`) cạnh gRPC | đơn giản, tách biệt nghiệp vụ và vận hành, không publish ra ngoài; chỉ ~50 dòng/ service (viết một lần, copy theo ADR-8) |
| B. Chỉ dùng `grpc.health.v1` | healthcheck bằng `grpc_health_probe` | đủ cho health/ready nhưng **không có `/metrics`** → vẫn phải mở HTTP |
| C. Gộp HTTP + gRPC chung một cổng (cmux/h2c) | | phức tạp, dễ lỗi; không đáng |
Vẫn đăng ký **`grpc.health.v1`** (cho gateway gọi kiểm tra downstream) **và** mở cổng HTTP `:8081` (cho Docker, Prometheus, con người). Gateway giữ `:8080` công khai (có `/health` hiện tại) và cũng mở `:8081` cho `/ready`, `/metrics`.

> **Tên endpoint**: hỗ trợ **cả hai kiểu** – `/health` = `/healthz` (liveness), `/ready` = `/readyz` (readiness). Hai tên trả cùng kết quả (alias); dùng tên nào cũng được (Docker/Traefik trong repo dùng `/healthz` và `/ready`).
> **Trạng thái triển khai (P1, đã code)**: gói `pkg/ops` (5 bản giống hệt nhau trong gateway/auth/user/product/order), cổng admin `:8081`, `/health /healthz /ready /readyz /metrics /version`, `grpc.health.v1`, metrics gRPC/HTTP/DB pool/build info, tắt êm theo SIGTERM, healthcheck Docker/Traefik, frontend `/api/health|healthz|ready|readyz`. **Chưa làm**: outbox/Kafka-lag metrics, Prometheus/Grafana, `/admin/system/*`, service payment/analytics.

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

## 3. Ma trận readiness theo service
`C` = critical (fail ⇒ 503) · `N` = non-critical (fail ⇒ degraded).

| Service | Postgres | Redis | Kafka | gRPC downstream | Khác |
|---|---|---|---|---|---|
| api-gateway | – | N *(rate limiter fail-open, ADR hiện có)* | C *(consumer `auth.change_password`)* | auth C · user C · product C · order C · payment N · analytics N | chưa drain |
| auth-service | C | N | C | – | migration, admin bootstrap xong |
| user-service | C | N | C | auth C | |
| product-service | C | N | C | – | outbox worker, MinIO N |
| order-service | C | N | C | product C · payment C | outbox worker, expiry worker |
| payment-service | C | C *(idempotency/lock)* | C | – | outbox worker, webhook signer key đã nạp |
| analytics-service | N *(chỉ đối soát)* | N | C | – | **ClickHouse C**, consumer đang chạy, lag < ngưỡng (non-critical: degraded) |
| frontend | – | – | – | – | gateway N (UI vẫn hiển thị trang lỗi thân thiện) |

Cách kiểm tra từng loại:
| Dependency | Cách check (timeout 1 s) |
|---|---|
| Postgres | `db.PingContext` + `SELECT 1`; (tuỳ chọn) pool không cạn: `in_use < max_open` |
| Redis | `PING` |
| Kafka | `Metadata`/dial tới broker; kiểm tra consumer goroutine còn heartbeat; lag chỉ cảnh báo (degraded) |
| gRPC downstream | gọi `grpc.health.v1.Health/Check` của service đó (mỗi service gRPC **phải** đăng ký health server) |
| ClickHouse | HTTP `GET /ping` + `SELECT 1` |
| MinIO | HTTP `/minio/health/ready` |
| Worker (outbox, expiry, scheduler) | gauge `last_tick_unix`; fail nếu quá `3 × interval` |
| Migration | cờ nội bộ đặt sau khi migrate xong |

## 4. gRPC health
Mỗi service gRPC đăng ký `grpc.health.v1.Health` (service name rỗng = toàn bộ; thêm tên con như `order.OrderService`). `SERVING` khi ready; `NOT_SERVING` khi drain/critical fail. Gateway và `grpc_health_probe` dùng cái này. Thêm gRPC **reflection chỉ ở dev**.

## 5. Vòng đời & graceful shutdown
```
start ─▶ [startup: chưa ready: /health 200, /ready 503] ─▶ migrate, kết nối, nạp cache ─▶ READY
SIGTERM ─▶ đặt NOT_SERVING + /ready 503 ─▶ chờ `DRAIN_SECONDS` (mặc định 10 s) cho LB rút ─▶ GracefulStop gRPC / http.Shutdown (timeout 20 s)
        ─▶ dừng consumer (commit offset) ─▶ flush outbox tick cuối ─▶ đóng DB ─▶ exit 0
```
Swarm: `stop_grace_period: 30s`. Đây cũng là bản sửa cho nợ "graceful shutdown" trong roadmap.

## 6. Cổng, docker healthcheck, Traefik
| Thành phần | Probe | Cấu hình |
|---|---|---|
| Container Go (mỗi service) | Docker `HEALTHCHECK` → **`/health`** trên `:8081` | `interval 10s, timeout 3s, retries 3, start_period 20s`. Image dùng binary nhỏ `healthcheck` (hoặc `wget -qO-`) vì image distroless không có shell |
| api-gateway qua Traefik | label `traefik.http.services.api.loadbalancer.healthcheck.path=/ready`, `.interval=5s`, `.timeout=2s`, port `8081` | rút replica not_ready khỏi LB; **public `GET /health` giữ nguyên** (đã có) trả `{"status":"ok"}` cho bên ngoài |
| frontend | `GET /api/health` (liveness), `GET /api/ready` (ready: gọi gateway `/health`) | Next route handler; nhớ `HOSTNAME=0.0.0.0` (đã ghi trong deployment.md) |
| Rolling update | `update_config: order: start-first, monitor: 30s, failure_action: rollback` | task mới phải `healthy` rồi mới tắt task cũ; thêm bước `scripts/wait-ready.sh` trong `deploy.sh` đợi mọi `/ready` = 200 |
| Hạ tầng | xem bảng 7 | |

## 7. Health/ready của container hạ tầng
| Container | Liveness | Readiness | Metrics |
|---|---|---|---|
| postgres | `pg_isready -U $POSTGRES_USER` (đã có) | `psql -c 'select 1'` | `postgres-exporter :9187` |
| redis | `redis-cli ping` | cùng lệnh (+ `INFO persistence` loading:0) | `redis-exporter :9121` |
| kafka (KRaft) | `kafka-broker-api-versions.sh --bootstrap-server localhost:9092` | cùng lệnh + topic cần thiết tồn tại (do service tự tạo) | `kafka-exporter :9308` (lag, offset) |
| clickhouse | `GET :8123/ping` → `Ok.` | `SELECT 1` qua HTTP | built-in `:9363/metrics` (bật `prometheus` trong config) |
| minio | `GET :9000/minio/health/live` | `…/health/ready` | `/minio/v2/metrics/cluster` |
| prometheus | `GET :9090/-/healthy` | `GET :9090/-/ready` | tự scrape |
| grafana | `GET :3000/api/health` | cùng | `/metrics` |
| cadvisor | `GET :8080/health` | – | `/metrics` |
| node-exporter | `GET :9100/` | – | `/metrics` |
| traefik | `traefik healthcheck --ping` (bật `--ping`) | cùng | `--metrics.prometheus` `:8082` |

## 8. Chuẩn `/metrics` (Prometheus)
Thư viện: `prometheus/client_golang`. Đăng ký collector Go + process. Quy ước tên: tiền tố `mm_` cho metric tự định nghĩa, `snake_case`, đơn vị cuối tên (`_seconds`, `_bytes`, `_total`).

**Bắt buộc ở mọi service**
| Metric | Loại | Nhãn | Ghi chú |
|---|---|---|---|
| `mm_build_info` | gauge=1 | `service, version, commit` | |
| `mm_http_requests_total` | counter | `service, method, route, status` | `route` là **mẫu** (`/orders/:id`), không phải đường dẫn thật (tránh bùng nổ cardinality) |
| `mm_http_request_duration_seconds` | histogram | `service, method, route` | bucket 5ms…10s |
| `mm_grpc_server_handled_total` / `_duration_seconds` | counter/hist | `service, grpc_service, grpc_method, grpc_code` | interceptor |
| `mm_grpc_client_handled_total` / `_duration_seconds` | | `service, target, grpc_method, grpc_code` | |
| `mm_db_pool_connections` | gauge | `service, state=open|in_use|idle`; `mm_db_pool_wait_seconds_total` | từ `sql.DBStats` |
| `mm_db_query_duration_seconds` | histogram | `service, op` | tuỳ chọn |
| `mm_kafka_consumer_lag` | gauge | `service, topic, group, partition` | |
| `mm_kafka_messages_processed_total` | counter | `service, topic, result=ok|retry|dropped` | |
| `mm_outbox_pending` / `mm_outbox_oldest_age_seconds` | gauge | `service, table` | cảnh báo đơn kẹt |
| `mm_outbox_published_total` | counter | `service, topic, result` | |
| `mm_worker_last_tick_timestamp_seconds` | gauge | `service, worker` | cho /ready |
| `mm_ready` | gauge 0/1 | `service` | phản ánh `/ready` |
| `process_*`, `go_*` | chuẩn | | CPU/RAM của process |
**Nghiệp vụ**
- order: `mm_orders_created_total`, `mm_order_status_transitions_total{from,to}`, `mm_orders_awaiting_payment`, `mm_reservations_expired_total`
- payment: `mm_payments_total{method,status}`, `mm_payment_amount_vnd_sum`, `mm_refunds_total`, `mm_webhook_deliveries_total{result}`
- product: `mm_inventory_reserve_total{result}`, `mm_products_low_stock`
- gateway: `mm_rate_limited_total{scope}`, `mm_events_ingested_total{result}`, `mm_events_rejected_total{reason}`
- analytics: `mm_ch_insert_rows_total`, `mm_ch_insert_lag_seconds`, `mm_events_consumed_total`
**Quy tắc**: không đặt `user_id`, `order_id`, `product_id` làm nhãn (cardinality); chỉ lộ `/metrics` trong mạng nội bộ; middleware đo **sau** khi router khớp để có `route` mẫu.

## 9. API vận hành khác (admin, nội bộ hoặc qua gateway cho role `admin`)
| Endpoint | Mục đích |
|---|---|
| `GET /version` | build info (mọi service) |
| `GET /admin/system/health` (gateway, role admin) | tổng hợp `ready` của mọi service + hạ tầng thành một bảng cho UI `/admin/system` |
| `GET /admin/system/metrics?q=…` (gateway, admin) | proxy Prometheus **theo tên truy vấn whitelist** (cpu_by_service, mem_by_service, p95_latency, error_rate, kafka_lag, node_cpu, node_mem, outbox_oldest) |
| `GET /admin/system/outbox` | backlog outbox từng service (đã có gauge) |
| `POST /admin/system/outbox/requeue` (admin, audit) | đặt lại hàng FAILED→PENDING có điều kiện (thay cho SQL tay trong runbook) – tuỳ chọn P4 |
| `GET /debug/pprof/*` | **tắt mặc định**; chỉ bật bằng `PPROF_ENABLED=true` trên :8081 |
| `GET /config/features` (gateway công khai) | cờ tính năng giao diện (vd. `tracking`, `payments.mock`) |

## 10. Kiểm thử bắt buộc
- Unit: handler health/ready với fake dependency (fail/ok/timeout) → đúng `status` & HTTP code; cache 3 s; timeout 1 s.
- E2E (`scripts/e2e`): dừng Postgres ⇒ `/ready` của service DB-bound = 503, `/health` vẫn 200, **container không bị restart**; dừng Redis ⇒ gateway `degraded` (200); SIGTERM ⇒ 503 trước khi thoát; Prometheus có đủ target `up==1`.
- CI: lint kiểm tra tên metric/nhãn (không nhãn id), service mới không merge nếu thiếu 4 endpoint.
