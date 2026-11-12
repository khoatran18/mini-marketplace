-- +goose Up
-- Đề xuất hành động (do agent hoặc người tạo, người duyệt). Nội dung (action, params, risk, incident) bất biến sau khi tạo;
-- chỉ các cột quyết định (state, decided_*, chosen_*) đổi, và chỉ từ PENDING.
CREATE TABLE IF NOT EXISTS ops_proposals (
    id              BIGSERIAL PRIMARY KEY,
    incident_id     BIGINT REFERENCES ops_incidents (id),
    action_id       TEXT        NOT NULL,
    params          JSONB       NOT NULL DEFAULT '{}',
    evidence        JSONB       NOT NULL DEFAULT '[]',
    rationale       TEXT        NOT NULL DEFAULT '',
    risk            TEXT        NOT NULL,                  -- do máy chủ lấy từ sổ hành động, không nhận từ người tạo
    state           TEXT        NOT NULL DEFAULT 'PENDING',
    content_hash    TEXT        NOT NULL,                  -- sha256(incident, action, params, risk)
    created_by      TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL,
    supersedes_id   BIGINT REFERENCES ops_proposals (id),  -- đề xuất mới thay cho một đề xuất NEEDS_REVIEW
    decided_by      TEXT,
    decided_at      TIMESTAMPTZ,
    decision_reason TEXT        NOT NULL DEFAULT '',
    chosen_action_id TEXT,                                 -- chỉ khi ALTERNATIVE
    chosen_params   JSONB,
    chosen_risk     TEXT,
    chosen_hash     TEXT,
    CONSTRAINT ops_proposals_state_chk CHECK (state IN ('PENDING', 'ACCEPTED', 'REJECTED', 'NEEDS_REVIEW', 'ALTERNATIVE', 'EXPIRED')),
    CONSTRAINT ops_proposals_risk_chk CHECK (risk IN ('observe', 'low', 'high')),
    CONSTRAINT ops_proposals_alt_chk CHECK ((state = 'ALTERNATIVE') = (chosen_action_id IS NOT NULL))
);
-- cùng một nội dung không được chờ duyệt hai lần cho cùng sự cố
CREATE UNIQUE INDEX IF NOT EXISTS ops_proposals_one_pending ON ops_proposals (COALESCE(incident_id, 0), content_hash) WHERE state = 'PENDING';
CREATE INDEX IF NOT EXISTS ops_proposals_state_idx ON ops_proposals (state, created_at DESC);
CREATE INDEX IF NOT EXISTS ops_proposals_incident_idx ON ops_proposals (incident_id, id);

-- +goose Down
DROP TABLE IF EXISTS ops_proposals;
