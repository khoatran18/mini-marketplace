-- +goose Up
-- Outbox: xử lý lỗi theo từng dòng (R2). domain_events là bảng CHUNG của product/order/payment nên migration này
-- giống hệt nhau ở cả ba service và idempotent (IF NOT EXISTS): service nào chạy trước cũng được.
--   next_attempt_at : chưa thử lại trước thời điểm này (backoff mũ sau mỗi lần lỗi)
--   last_error      : lỗi publish gần nhất (để điều tra)
--   dead_at         : thời điểm chuyển sang DEAD (dead-letter, giữ lại, không tự thử lại; requeue tay theo runbook)
ALTER TABLE domain_events
    ADD COLUMN IF NOT EXISTS next_attempt_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS last_error text,
    ADD COLUMN IF NOT EXISTS dead_at timestamptz;

-- Hàng đợi chưa gửi (chỉ PENDING/FAILED) theo id; và theo (key, id) cho kiểm tra thứ tự cùng aggregate.
CREATE INDEX IF NOT EXISTS idx_domain_events_unpublished ON domain_events (id) WHERE status IN ('PENDING', 'FAILED');
CREATE INDEX IF NOT EXISTS idx_domain_events_unpublished_key ON domain_events (key, id) WHERE status IN ('PENDING', 'FAILED');
CREATE INDEX IF NOT EXISTS idx_domain_events_dead ON domain_events (id) WHERE status = 'DEAD';

-- +goose Down
-- Không hoàn tác (sẽ mất thông tin dead-letter); thêm migration mới nếu cần sửa schema.
SELECT 1;
