# 01 · Kiến trúc đích (đã triển khai phần lớn)

> **Trạng thái: ✅ backend ĐÃ TRIỂN KHAI** (7 service Go + Postgres/Redis/Kafka/ClickHouse + Prometheus/Grafana). Chưa có: MinIO, cảnh báo (cố ý), reviews/wishlist, đối soát analytics. Xem bảng "Trạng thái triển khai" ở [09-roadmap.md](09-roadmap.md). Bản mô tả luồng ngắn gọn: [../architecture.md](../architecture.md).

## 1. Sơ đồ
```
                       Traefik (TLS)   health check LB: /ready (cổng 8081)
Browser ───────────────▶ frontend (Next.js :3000)   [/api/health, /api/ready]
  │ tracking SDK            │
  └──────── HTTPS ─────────▶ api-gateway (gin :8080 public, :8081 admin)
                                │ gRPC (kèm grpc.health.v1)
   ┌───────────┬────────────┬───┴────────┬────────────┬────────────┐
   ▼           ▼            ▼            ▼            ▼            ▼
 auth        user        product       order       payment      analytics
 :50051      :50054      :50053        :50052      :50057       :50055
 (+admin     (+địa chỉ   (+catalog,    (+cart,     (mockpay     (Kafka +
  bootstrap)  giao hàng)  tồn kho,      checkout,   MÔ PHỎNG,    gRPC tracking
                          ledger)       lifecycle,  webhook      → ClickHouse,
                                        expiry)     nội bộ)      báo cáo)
   └───────────┴────────────┴─────┬──────┴────────────┴─────┬──────┘
                                  ▼                         ▼
                     Postgres   Kafka   Redis         ClickHouse         (MinIO: ❌ chưa có)
Giám sát: Prometheus ◀─ scrape :8081/metrics (7 service) + cAdvisor + node-exporter + postgres/redis/kafka exporters
          Grafana đọc Prometheus (chỉ theo dõi, KHÔNG có cảnh báo)
```
Mỗi service Go mở **hai cổng**: nghiệp vụ (gRPC; gateway HTTP :8080) và **admin HTTP :8081** (`/health /healthz /ready /readyz /metrics /version`) – xem [02](02-health-ready-metrics.md). Cổng admin **không publish, không route qua Traefik**. Địa chỉ gRPC giữa service cứng trong code (`grpc_config.go`).
Phụ thuộc gRPC thực tế: gateway → cả 6 service; order → product (giá/tồn); user → auth. **order-service không gọi payment-service bằng gRPC**: giao tiếp order ↔ payment hoàn toàn qua Kafka (`payment.requested`, `payment.succeeded/refunded`, `order.refund_requested`).

## 2. Service mới / mở rộng
| Service | Ngôn ngữ | Cổng | Trách nhiệm | Trạng thái |
|---|---|---|---|---|
| `payment-service` | Go | gRPC 50057 (+ webhook HTTP nội bộ trên :8081) | thanh toán **mô phỏng** (`mockpay`), idempotency, webhook ký HMAC, hoàn tiền; phát `payment.*` (outbox) – xem 04 | ✅ |
| `analytics-service` | Go | gRPC 50055 | consume Kafka (`order.status_changed`, `payment.{succeeded,failed,refunded}`, `product.changed`, `inventory.changed`) **và** nhận tracking qua gRPC `IngestEvents` → ClickHouse; `Query` theo scope (admin/store) | ✅ (tracking **không** qua Kafka `tracking.events`) |
| order-service (mở rộng) | | | giỏ hàng, checkout tách theo store, vòng đời đơn mở rộng, hết hạn giữ chỗ/ tự giao, yêu cầu thanh toán/ hoàn tiền | ✅ |
| product-service (mở rộng) | | | danh mục, trường catalog, tồn kho + sổ cái, tìm kiếm, outbox `product.changed`/`inventory.changed` | ✅ (ảnh/MinIO ❌, reviews/wishlist ❌) |
| auth-service (mở rộng) | | | role `admin` + bootstrap | ✅ (khoá đăng nhập, `audit_log` ❌) |
| user-service (mở rộng) | | | nhiều địa chỉ giao hàng | ✅ |
| api-gateway (mở rộng) | | | route mới, `/events`, `/admin/system/health`, giới hạn body, metrics | ✅ (proxy metrics hệ thống ❌) |

## 3. Luồng dữ liệu chính
1. **Đặt hàng + thanh toán** – xem 04 (reserve → `AWAITING_PAYMENT`/`CONFIRMED` → thanh toán mô phỏng → `PAID` → `SHIPPED` → `DELIVERED`; hết hạn nếu không trả). ✅
2. **Tracking** – SDK → `POST /events` (gateway gắn `user_id/role/store_id` từ JWT, `ip_hash`) → **gRPC `IngestEvents`** → analytics-service (whitelist, consent, che PII) → bộ đệm → ClickHouse `events_raw`. Không có materialized view; báo cáo gom bảng với `FINAL`. ✅
3. **Doanh thu** – `order.status_changed` + `payment.*` → `fact_order_status`/`fact_order_items`/`fact_payment_events`/`fact_refunds` → báo cáo seller/admin. Đối soát đêm với Postgres: ❌ chưa làm.
4. **Metrics hệ thống** – Prometheus scrape `:8081/metrics`; xem bằng **Grafana** (5 dashboard); admin xem tóm tắt trong app qua `GET /admin/system/health` ✅ (`/admin/system/metrics` proxy ❌).
5. **Cảnh báo: chưa làm (cố ý).** Chỉ Grafana; không alert-service, Alertmanager, thông báo.

## 4. Nguyên tắc (kế thừa + thực tế)
- Gateway là cổng công khai duy nhất; danh tính chỉ từ JWT (ADR-3). Route công khai (`/search`, `/categories`, `/products*`, `/payments/methods`, `/events`) có xác thực **gắn theo route** (ADR-19); `/events` rate-limit riêng.
- Mọi hiệu ứng liên service qua **outbox** + consumer idempotent (ADR-6). product/order/payment dùng bảng chung `domain_events`; auth/user giữ outbox cũ. **Topic `*.dlq`: ❌ chưa có** – consumer thử 5 lần rồi ghi log và bỏ qua.
- **Mỗi service/container Go có health/ready/metrics** theo chuẩn 02.
- Tiền: `NUMERIC(18,2)` trong Postgres, cent nguyên khi tính (ADR-7). Thiết kế "API mới dùng `{amount, currency}` chuỗi" **không** áp dụng: REST/gRPC trả VND dạng số thực (+ `amount_minor` ở payment), event Kafka dùng `*_minor` (ADR-18).
- ClickHouse là nơi *phân tích*, không phải nguồn sự thật tiền; dựng lại một phần từ Kafka (retention 7 ngày); backfill từ Postgres ❌.
- Migration có version: ClickHouse ✅ (`schema_migrations`); Postgres vẫn `AutoMigrate` (goose ❌ chưa làm).

## 5. Ràng buộc từ hệ thống cũ (đã xử lý)
- Gateway từng bắt JWT ở mọi route trừ auth/`/health` → đã thêm nhóm route công khai tường minh (và bài học: không dùng `router.Use` toàn cục – ADR-19).
- Body JSON đã có giới hạn (`MAX_BODY_BYTES`, `/events` ≤ 64 KB).
- Graceful shutdown (readiness 503 khi drain) đã có ở `pkg/ops`; chưa dừng có chủ đích consumer/worker.
- Giỏ hàng đã chuyển lên server (order-service); `POST /cart/merge` gộp giỏ localStorage khi đăng nhập.
