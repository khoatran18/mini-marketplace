-- +goose Up
-- Người tạo rút lại đề xuất của mình khi còn PENDING (trạng thái WITHDRAWN). Dùng lại decided_by / decided_at / decision_reason.
ALTER TABLE ops_proposals DROP CONSTRAINT IF EXISTS ops_proposals_state_chk;
ALTER TABLE ops_proposals ADD CONSTRAINT ops_proposals_state_chk CHECK (state IN ('PENDING', 'ACCEPTED', 'REJECTED', 'NEEDS_REVIEW', 'ALTERNATIVE', 'EXPIRED', 'WITHDRAWN'));

-- +goose Down
UPDATE ops_proposals SET state = 'REJECTED' WHERE state = 'WITHDRAWN';
ALTER TABLE ops_proposals DROP CONSTRAINT IF EXISTS ops_proposals_state_chk;
ALTER TABLE ops_proposals ADD CONSTRAINT ops_proposals_state_chk CHECK (state IN ('PENDING', 'ACCEPTED', 'REJECTED', 'NEEDS_REVIEW', 'ALTERNATIVE', 'EXPIRED'));
