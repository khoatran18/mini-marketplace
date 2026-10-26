-- +goose Up
-- Outbox riêng cho order-service (ADR-34): trước đây product/order/payment dùng chung bảng domain_events nên
-- backlog và dòng DEAD không tách được theo service. Bảng mới cùng cấu trúc; chuyển sang đây các dòng CHƯA GỬI
-- hoặc DEAD của đúng các topic của service này (giữ thứ tự id). Lịch sử SUCCESS ở lại bảng cũ (không còn ai ghi).
-- Idempotent: chạy lại không chuyển thêm gì vì dòng đã bị xoá khỏi bảng cũ.
CREATE TABLE IF NOT EXISTS order_domain_events (
    id              bigserial PRIMARY KEY,
    topic           text NOT NULL,
    key             text NOT NULL,
    payload         jsonb NOT NULL,
    status          text NOT NULL DEFAULT 'PENDING',
    attempts        bigint NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL,
    published_at    timestamptz,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_error      text,
    dead_at         timestamptz
);
CREATE INDEX IF NOT EXISTS idx_order_domain_events_status_id ON order_domain_events (status);
CREATE INDEX IF NOT EXISTS idx_order_domain_events_unpublished ON order_domain_events (id) WHERE status IN ('PENDING', 'FAILED');
CREATE INDEX IF NOT EXISTS idx_order_domain_events_unpublished_key ON order_domain_events (key, id) WHERE status IN ('PENDING', 'FAILED');
CREATE INDEX IF NOT EXISTS idx_order_domain_events_dead ON order_domain_events (id) WHERE status = 'DEAD';

-- Bảng cũ có thể chưa tồn tại / chưa có cột R2 trên DB rất cũ: baseline + 00002 đã bảo đảm cả hai.
WITH moved AS (
    DELETE FROM domain_events
    WHERE status IN ('PENDING', 'FAILED', 'DEAD') AND topic IN ('order.create_order', 'order.cancel_order', 'order.status_changed', 'order.refund_requested', 'payment.requested')
    RETURNING id, topic, key, payload, status, attempts, created_at, published_at, next_attempt_at, last_error, dead_at
)
INSERT INTO order_domain_events (topic, key, payload, status, attempts, created_at, published_at, next_attempt_at, last_error, dead_at)
SELECT topic, key, payload, status, attempts, created_at, published_at, next_attempt_at, last_error, dead_at
FROM moved
ORDER BY id;

-- +goose Down
-- Không hoàn tác (dòng đã chuyển); thêm migration mới nếu cần sửa schema.
SELECT 1;
