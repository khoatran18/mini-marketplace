-- +goose Up
-- Sự cố do poller tạo từ cảnh báo Prometheus. Một dedupe_key chỉ có tối đa MỘT sự cố chưa resolved.
CREATE TABLE IF NOT EXISTS ops_incidents (
    id          BIGSERIAL PRIMARY KEY,
    dedupe_key  TEXT        NOT NULL,
    alert_name  TEXT        NOT NULL,
    severity    TEXT        NOT NULL DEFAULT '',
    component   TEXT        NOT NULL DEFAULT '',
    labels      JSONB       NOT NULL DEFAULT '{}',
    summary     TEXT        NOT NULL DEFAULT '',
    runbook     TEXT        NOT NULL DEFAULT '',
    action      TEXT        NOT NULL DEFAULT 'observe',   -- observe | low | high (annotation `action` của luật)
    state       TEXT        NOT NULL DEFAULT 'open',      -- open | investigating | resolved
    fire_count  INTEGER     NOT NULL DEFAULT 1,
    first_seen  TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen   TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ,
    CONSTRAINT ops_incidents_state_chk CHECK (state IN ('open', 'investigating', 'resolved'))
);
CREATE UNIQUE INDEX IF NOT EXISTS ops_incidents_one_active ON ops_incidents (dedupe_key) WHERE state <> 'resolved';
CREATE INDEX IF NOT EXISTS ops_incidents_state_idx ON ops_incidents (state, last_seen DESC);

-- Nhật ký chỉ thêm: UPDATE / DELETE / TRUNCATE đều bị trigger từ chối (kể cả với chủ bảng; chỉ DROP TRIGGER mới gỡ được, và việc đó để lại dấu vết DDL).
CREATE TABLE IF NOT EXISTS ops_audit_log (
    id           BIGSERIAL PRIMARY KEY,
    ts           TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor        TEXT        NOT NULL,               -- poller | admin:<id> | agent
    event        TEXT        NOT NULL,               -- incident_opened | incident_resolved | incident_investigating | ...
    subject_type TEXT        NOT NULL DEFAULT '',
    subject_id   TEXT        NOT NULL DEFAULT '',
    details      JSONB       NOT NULL DEFAULT '{}',
    request_id   TEXT        NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS ops_audit_subject_idx ON ops_audit_log (subject_type, subject_id, id);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION ops_audit_immutable() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'ops_audit_log is append-only (% not allowed)', TG_OP;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS ops_audit_no_update ON ops_audit_log;
CREATE TRIGGER ops_audit_no_update BEFORE UPDATE OR DELETE ON ops_audit_log
    FOR EACH ROW EXECUTE FUNCTION ops_audit_immutable();
DROP TRIGGER IF EXISTS ops_audit_no_truncate ON ops_audit_log;
CREATE TRIGGER ops_audit_no_truncate BEFORE TRUNCATE ON ops_audit_log
    FOR EACH STATEMENT EXECUTE FUNCTION ops_audit_immutable();

-- +goose Down
DROP TABLE IF EXISTS ops_audit_log;
DROP FUNCTION IF EXISTS ops_audit_immutable();
DROP TABLE IF EXISTS ops_incidents;
