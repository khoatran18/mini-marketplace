-- +goose Up
-- ADR-42: id của lượt chat AI khi hành động được người dùng XÁC NHẬN từ đề xuất của agent (gateway đặt qua metadata gRPC
-- x-agent-run-id, chỉ sau khi kiểm chữ ký gói đề xuất). NULL = người dùng tự làm trực tiếp (hoặc dòng cũ).
ALTER TABLE order_domain_events ADD COLUMN IF NOT EXISTS agent_run_id text;
-- bảng chung cũ (ADR-34) còn tồn tại và events.Event ghi cột này khi chưa gọi SetTable (test), nên thêm cả ở đó
ALTER TABLE domain_events ADD COLUMN IF NOT EXISTS agent_run_id text;
-- Tra "mọi hành động do lượt chat X": chỉ lập chỉ mục các dòng có giá trị (đa số là NULL).
CREATE INDEX IF NOT EXISTS idx_order_domain_events_agent_run ON order_domain_events (agent_run_id) WHERE agent_run_id IS NOT NULL;

-- +goose Down
-- Không hoàn tác (mất thông tin truy vết); thêm migration mới nếu cần sửa schema.
SELECT 1;
