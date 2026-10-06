# 01 · Kiến trúc đích

## 1. Sơ đồ
```
                       Traefik (TLS)   health check LB: /readyz
Browser ───────────────▶ frontend (Next.js :3000)   [/api/health, /api/ready]
  │ tracking SDK            │
  └──────── HTTPS ─────────▶ api-gateway (gin :8080 public, :8081 admin)
                                │ gRPC (kèm grpc.health.v1)
   ┌───────────┬────────────┬───┴────────┬────────────┬────────────┬────────────┐
   ▼           ▼            ▼            ▼            ▼            ▼            ▼
 auth       user        product       order       payment*    analytics*    alert*
 :50051     :50054      :50053        :50052      :50057      :50055        :50056
 (+admin)              (+catalog,    (+cart,     (mock       (Kafka→       (rule engine,
                        media,        lifecycle,  provider,   ClickHouse,   scheduler,
                        inventory)    expiry)     webhook)    query API)    notify)
   └───────────┴────────────┴─────┬──────┴────────────┴─────┬──────┴────────────┘
                                  ▼                         ▼
             Postgres (db `postgres`: nghiệp vụ; db `ops`: alert, notification)   Kafka   Redis   ClickHouse   MinIO
Observability: Prometheus ◀─ scrape :8081/metrics ─ mọi service + cAdvisor + node-exporter + postgres/redis/kafka exporters
               Alertmanager ─webhook─▶ alert-service     Grafana (SRE)
```
Mỗi service Go mở **hai cổng**: nghiệp vụ (gRPC; gateway thì HTTP :8080) và **admin HTTP :8081** (`/healthz /readyz /metrics /version`). Cổng admin **không publish, không route qua Traefik**.

## 2. Service mới
| Service | Ngôn ngữ | Cổng | Trách nhiệm |
|---|---|---|---|
| `payment-service` | Go | gRPC 50057 | Tạo/ghi nhận thanh toán **mô phỏng** (provider `mockpay`), idempotency, webhook giả lập có chữ ký, hoàn tiền; phát `payment.*` (outbox) – chi tiết 04 |
| `analytics-service` | Go | gRPC 50055 | Consume Kafka (`tracking.events`, `order.status_changed`, `payment.*`, `product.changed`, `inventory.changed`) → ClickHouse; API truy vấn doanh thu/traffic/funnel theo scope (platform/store) |
| `alert-service` | Go | gRPC 50056 | Rule engine + scheduler (khoá leader bằng Redis), nhận webhook Alertmanager, vòng đời alert, thông báo trong app (+ email/webhook tuỳ chọn) |
Mở rộng service hiện có (không thêm service): **order-service** (giỏ hàng, vòng đời đơn mở rộng, hết hạn reservation, địa chỉ giao), **product-service** (danh mục, ảnh, trường mới, sổ cái tồn kho, outbox `product.changed`/`inventory.changed`), **auth-service** (role `admin`, bootstrap), **user-service** (nhiều địa chỉ giao – tuỳ chọn), **api-gateway** (route mới, `/events`, proxy metrics hệ thống, giới hạn body, metrics).

## 3. Luồng dữ liệu chính
1. **Đặt hàng + thanh toán** – xem 04 (saga mở rộng: reserve → thanh toán mô phỏng → PAID → SHIPPED → DELIVERED; hết hạn nếu không trả).
2. **Tracking** – SDK → `POST /events` (gateway gắn `user_id/role/store_id` từ JWT) → Kafka `tracking.events` → analytics-service → ClickHouse `events_raw` → materialized views.
3. **Doanh thu** – `order.status_changed` + `payment.succeeded` → ClickHouse `fact_orders`/`fact_order_items` → dashboard seller/admin. Đối soát đêm với Postgres.
4. **Metrics hệ thống** – Prometheus scrape; admin xem qua `GET /admin/system/*` (proxy whitelist) hoặc Grafana.
5. **Cảnh báo** – (a) Prometheus rules → Alertmanager → alert-service; (b) scheduler của alert-service chạy rule nghiệp vụ trên ClickHouse/Postgres → bảng `alerts` → thông báo in-app (SSE).

## 4. Nguyên tắc (kế thừa + bổ sung)
- Gateway là cổng công khai duy nhất; danh tính chỉ từ JWT (ADR-3). Route công khai mới (`/events`, `/search`, `/categories`) có rate-limit riêng.
- Mọi hiệu ứng liên service qua **outbox** + consumer idempotent (ADR-6). Topic mới có `*.dlq`.
- **Mỗi service/container có health/ready/metrics** theo chuẩn 02. Không merge service mới thiếu 3 thứ này.
- Tiền: `NUMERIC(18,2)`, tính bằng cent nguyên (ADR-7); API mới dùng `{amount, currency:"VND"}` dạng chuỗi/ số nguyên, không thêm `float`.
- ClickHouse là nơi *phân tích*, không phải nguồn sự thật tiền bạc; dựng lại được từ Kafka (retention 7 ngày) + backfill từ Postgres.
- Migration có version cho mọi bảng mới (goose) – service cũ giữ `AutoMigrate` cho đến khi làm nợ kỹ thuật tương ứng.

## 5. Ràng buộc từ hệ thống hiện tại
- Gateway hiện bắt JWT ở mọi route trừ auth/`/health` → thêm nhóm route công khai tường minh.
- Chưa giới hạn body JSON (security.md) → bắt buộc trước khi mở `/events` & upload.
- Token access sống 5 phút; SSE (`/alerts/stream`) phải xử lý hết hạn (UI refresh rồi mở lại kết nối).
- Chưa có graceful shutdown → là điều kiện để `/readyz` trả 503 khi drain (02 §5): làm cùng P1.
- Cart đang ở localStorage → chuyển lên server (gộp khi đăng nhập).
