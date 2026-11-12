-- +goose Up
-- Đợt 3: thực thi có kiểm soát. Trạng thái quyết định (state) giữ nguyên; kết quả thực thi nằm ở cột riêng.
ALTER TABLE ops_proposals
    ADD COLUMN IF NOT EXISTS exec_state       TEXT,          -- EXECUTING | EXECUTED | FAILED (NULL = chưa thực thi)
    ADD COLUMN IF NOT EXISTS exec_by          TEXT,
    ADD COLUMN IF NOT EXISTS exec_started_at  TIMESTAMPTZ,   -- chỉ đặt khi ops-service tự chạy (đếm hạn mức); ghi tay thì NULL
    ADD COLUMN IF NOT EXISTS exec_finished_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS exec_result      JSONB;
ALTER TABLE ops_proposals ADD CONSTRAINT ops_proposals_exec_chk CHECK (exec_state IS NULL OR exec_state IN ('EXECUTING', 'EXECUTED', 'FAILED'));
-- chỉ đề xuất đã được duyệt (ACCEPTED, hoặc ALTERNATIVE: hành động người duyệt chọn) mới có thể có kết quả thực thi
ALTER TABLE ops_proposals ADD CONSTRAINT ops_proposals_exec_state_chk CHECK (exec_state IS NULL OR state IN ('ACCEPTED', 'ALTERNATIVE'));
CREATE INDEX IF NOT EXISTS ops_proposals_exec_idx ON ops_proposals (exec_started_at) WHERE exec_started_at IS NOT NULL;

-- +goose Down
ALTER TABLE ops_proposals DROP CONSTRAINT IF EXISTS ops_proposals_exec_state_chk, DROP CONSTRAINT IF EXISTS ops_proposals_exec_chk,
    DROP COLUMN IF EXISTS exec_result, DROP COLUMN IF EXISTS exec_finished_at, DROP COLUMN IF EXISTS exec_started_at,
    DROP COLUMN IF EXISTS exec_by, DROP COLUMN IF EXISTS exec_state;
