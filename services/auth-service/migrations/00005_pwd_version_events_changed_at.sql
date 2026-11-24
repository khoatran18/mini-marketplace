-- +goose Up
-- changed_at = lần đổi mật khẩu GẦN NHẤT của tài khoản (created_at chỉ là lần đầu, bảng này mỗi user một dòng và được ghi đè).
-- Gateway hỏi "ai đã đổi mật khẩu trong N phút gần đây" để dựng lại khóa deny trong Redis khi Redis mất dữ liệu (R25).
-- Dòng cũ nhận now(): không biết lần đổi thật nên coi như vừa đổi (thừa an toàn, chỉ làm danh sách dựng lại dài hơn một lần).
ALTER TABLE pwd_version_events ADD COLUMN IF NOT EXISTS changed_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE INDEX IF NOT EXISTS idx_pwd_version_events_changed_at ON pwd_version_events (changed_at);

-- +goose Down
DROP INDEX IF EXISTS idx_pwd_version_events_changed_at;
ALTER TABLE pwd_version_events DROP COLUMN IF EXISTS changed_at;
