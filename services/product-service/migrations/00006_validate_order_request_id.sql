-- +goose Up
-- ADR-36: id của request gây ra kết quả validate tồn kho, gửi đi dưới dạng Kafka header x-request-id để order-service
-- (payment.requested, status_changed) giữ cùng id. NULL với dòng cũ.
ALTER TABLE validate_order_events ADD COLUMN IF NOT EXISTS request_id text;

-- +goose Down
-- Không hoàn tác (mất thông tin truy vết); thêm migration mới nếu cần sửa schema.
SELECT 1;
