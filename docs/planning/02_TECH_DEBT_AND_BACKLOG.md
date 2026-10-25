# Roadmap – nợ kỹ thuật và việc còn lại

Trạng thái chi tiết từng hạng mục (xong / một phần / chưa làm) nằm ở bảng **"Trạng thái triển khai"** trong [09_ROADMAP_STATUS.md](../design/platform/09_ROADMAP_STATUS.md). Tệp này liệt kê phần còn lại.

## Đã làm từ danh sách cũ
Vòng đời đơn đầy đủ (`PAID → SHIPPED → DELIVERED`, `REFUNDED`, lịch sử trạng thái, ai được chuyển gì) · giữ hàng có hạn (đơn chưa trả tiền hết hạn) · idempotency key cho `POST /orders` · thanh toán (mô phỏng, webhook, hoàn tiền) · giỏ hàng + checkout nhiều store (tách đơn theo store, địa chỉ, phí ship) · công cụ seller (hộp thư đơn, tồn kho, thống kê) · tìm kiếm/ lọc (Postgres full-text không dấu) · graceful shutdown + readiness + metrics · giới hạn body · user-service có test (địa chỉ).

## Nợ kỹ thuật
- ✅ `kafkaimpl` + `ops` + `events` đã dùng chung (`shared/`, ADR-33); còn `.pb.go` sao chép và `ops` của gateway/product; `buf` registry thay cho các điểm copy `buf.gen.yaml`.
- ~~Migration có version~~ ✅ đã dùng goose (ADR-26). Còn lại: `accounts.username` UNIQUE toàn cục dù login theo (username, role); bảng thừa `create_seller_kafka_events` ở user-service (giữ trong baseline).
- Tách outbox `domain_events` theo service (hiện một bảng vật lý chung – ADR-23). (Dead-letter và lỗi theo từng dòng đã xong, R2.)
- Tiền: `double` trên REST/gRPC (ADR-7); event đã dùng `*_minor`.
- Observability còn thiếu: tracing OpenTelemetry, request-id trong Kafka / outbox, log JSON thống nhất. Đã có (🟡): request-id, log tập trung Vector → Loki. Đã thêm (chưa chạy stack thật): metric nghiệp vụ `mm_orders_*`/`mm_payments_*`/`mm_inventory_*`/`mm_grpc_client_*`, dashboard Payments/Business/Kafka lag/Outbox/ClickHouse server; lag consumer dùng `kafka_consumergroup_lag` của kafka-exporter.
- gRPC deadline/retry/circuit breaker từ gateway và order-service; TLS cho gRPC/Postgres; token trong cookie httpOnly.
- Kafka: ✅ 3 broker, RF 3, min ISR 2 trong `infra.yml` (ADR-32); ✅ `createTopic` dùng RF `min(3, số broker)` (`shared/`); còn lại: DLQ thay vì "log và bỏ qua", schema registry; consumer `AddChaPwdVerToRedis`/`UpdateStoreIDFromKafka` nên bỏ JSON hỏng thay vì thử 5 lần.
- Swagger (`services/api-gateway/docs`) đã cập nhật **viết tay** đủ 90 route (không phải kết quả `swag init`; handler chưa có chú thích swag, `/swagger` chưa được đăng ký). Nếu muốn sinh tự động: chú thích handler rồi `swag init` ([04_API_REFERENCE.md](../design/04_API_REFERENCE.md) là bản đầy đủ).
- e2e `scripts/e2e` đã viết lại cho checkout/payment/cart/analytics (xem [04_TESTING.md](../instruction/04_TESTING.md) §5) nhưng **chưa chạy thật** trên stack; cần chạy lần đầu và chỉnh chỗ lệch.
- Sửa các phát hiện trong "Giới hạn đã biết" của [09_ROADMAP_STATUS.md](../design/platform/09_ROADMAP_STATUS.md).

## Tính năng nghiệp vụ chưa làm (ưu tiên giảm dần)
1. **Reviews / wishlist** (chỉ người đã nhận hàng review); gợi ý/trending từ analytics.
2. **Upload ảnh sản phẩm** (MinIO, kiểm MIME/kích thước) – hiện ảnh chỉ là URL.
3. **Quyền riêng tư**: bảng `consents`, `PUT /users/me/consent`, `DELETE /users/me/data` (consent hiện chỉ đi kèm từng event).
4. **An toàn tài khoản**: khoá tạm khi sai mật khẩu nhiều lần, `audit_log`, email xác thực, đặt lại mật khẩu, đăng xuất (deny-list token), `GET /auth/me`.
5. **Quản trị**: `/admin/users`, `/admin/stores` (duyệt/khoá), `/admin/system/metrics` (proxy Prometheus whitelist), `/admin/system/outbox` + requeue qua API.
6. **Analytics nâng cao**: đối soát đêm Postgres↔ClickHouse (CLI `backfill`/`reconcile` đã có, còn lịch đêm và nối `DataHealth`), materialized views khi dữ liệu lớn, so sánh theo danh mục/shop, xuất CSV.
7. **Thanh toán thật** (provider thật + webhook thật) nếu muốn – hiện chỉ mô phỏng (ADR-16).
8. Thông báo (email/in-app qua Kafka), cảnh báo (Alertmanager) – cố ý chưa làm (ADR-12).
