# Roadmap – nợ kỹ thuật và việc còn lại

Trạng thái chi tiết từng hạng mục (xong / một phần / chưa làm) nằm ở bảng **"Trạng thái triển khai"** trong [platform/09-roadmap.md](platform/09-roadmap.md). Tệp này liệt kê phần còn lại.

## Đã làm từ danh sách cũ
Vòng đời đơn đầy đủ (`PAID → SHIPPED → DELIVERED`, `REFUNDED`, lịch sử trạng thái, ai được chuyển gì) · giữ hàng có hạn (đơn chưa trả tiền hết hạn) · idempotency key cho `POST /orders` · thanh toán (mô phỏng, webhook, hoàn tiền) · giỏ hàng + checkout nhiều store (tách đơn theo store, địa chỉ, phí ship) · công cụ seller (hộp thư đơn, tồn kho, thống kê) · tìm kiếm/ lọc (Postgres full-text không dấu) · graceful shutdown + readiness + metrics · giới hạn body · user-service có test (địa chỉ).

## Nợ kỹ thuật
- Module dùng chung cho protobuf + `kafkaimpl` + `ops` + `events` (ADR-8; hiện là bản sao giống hệt ở 7 service); `buf` registry thay cho các điểm copy `buf.gen.yaml`.
- Migration có version (goose/atlas) thay `AutoMigrate`; `accounts.username` UNIQUE toàn cục dù login theo (username, role); bảng thừa `create_seller_kafka_events` ở user-service.
- Tách outbox `domain_events` theo service (hiện một bảng vật lý chung – ADR-23) và có "dead letter" cho dòng độc thay vì chặn đầu hàng.
- Tiền: `double` trên REST/gRPC (ADR-7); event đã dùng `*_minor`.
- Observability còn thiếu: request-id, tracing, metric nghiệp vụ (`mm_orders_*`, `mm_payments_*`), metric lag consumer của chính service, panel Grafana cho outbox/payment/ClickHouse server.
- gRPC deadline/retry/circuit breaker từ gateway và order-service; TLS cho gRPC/Postgres; token trong cookie httpOnly.
- Kafka: 3 broker, RF 3, DLQ thay vì "log và bỏ qua", schema registry; consumer `AddChaPwdVerToRedis`/`UpdateStoreIDFromKafka` nên bỏ JSON hỏng thay vì thử 5 lần.
- Swagger (`services/api-gateway/docs`) **đã cũ** – chỉ mô tả auth/products/users/orders cũ; cần `swag init` lại và chú thích handler mới ([api.md](api.md) là bản đầy đủ).
- e2e `scripts/e2e/*.mjs` viết cho API cũ (xem [testing.md](testing.md) §5); cần viết lại cho checkout/payment/cart và thêm e2e analytics.
- Sửa các phát hiện trong "Giới hạn đã biết" của [platform/09-roadmap.md](platform/09-roadmap.md).

## Tính năng nghiệp vụ chưa làm (ưu tiên giảm dần)
1. **Reviews / wishlist** (chỉ người đã nhận hàng review); gợi ý/trending từ analytics.
2. **Upload ảnh sản phẩm** (MinIO, kiểm MIME/kích thước) – hiện ảnh chỉ là URL.
3. **Quyền riêng tư**: bảng `consents`, `PUT /users/me/consent`, `DELETE /users/me/data` (consent hiện chỉ đi kèm từng event).
4. **An toàn tài khoản**: khoá tạm khi sai mật khẩu nhiều lần, `audit_log`, email xác thực, đặt lại mật khẩu, đăng xuất (deny-list token), `GET /auth/me`.
5. **Quản trị**: `/admin/users`, `/admin/stores` (duyệt/khoá), `/admin/system/metrics` (proxy Prometheus whitelist), `/admin/system/outbox` + requeue qua API.
6. **Analytics nâng cao**: đối soát đêm Postgres↔ClickHouse (`DataHealth`), backfill, materialized views khi dữ liệu lớn, so sánh theo danh mục/shop, xuất CSV.
7. **Thanh toán thật** (provider thật + webhook thật) nếu muốn – hiện chỉ mô phỏng (ADR-16).
8. Thông báo (email/in-app qua Kafka), cảnh báo (Alertmanager) – cố ý chưa làm (ADR-12).
