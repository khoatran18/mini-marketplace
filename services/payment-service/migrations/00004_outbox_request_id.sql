-- +goose Up
-- ADR-36: id của request gây ra event, để lần theo một đơn qua mọi service (gửi đi dưới dạng Kafka header x-request-id).
-- NULL với dòng cũ hoặc event không sinh từ request (timer, consumer cũ).
ALTER TABLE payment_domain_events ADD COLUMN IF NOT EXISTS request_id text;
-- bảng cũ chung (ADR-34) vẫn tồn tại và events.Event ghi cột này, nên thêm cả ở đó (không còn ai gửi từ bảng này)
ALTER TABLE domain_events ADD COLUMN IF NOT EXISTS request_id text;

-- +goose Down
-- Không hoàn tác (mất thông tin truy vết); thêm migration mới nếu cần sửa schema.
SELECT 1;
