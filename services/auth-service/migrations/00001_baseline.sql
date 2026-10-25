-- +goose Up
-- Baseline = schema do GORM AutoMigrate tạo trước đây; idempotent để DB cũ vẫn lên được.
CREATE TABLE IF NOT EXISTS accounts (
    id          BIGSERIAL PRIMARY KEY,
    username    TEXT        NOT NULL,
    password    TEXT        NOT NULL,
    role        TEXT        NOT NULL,
    pwd_version BIGINT      NOT NULL,
    deleted_at  TIMESTAMPTZ,
    store_id    BIGINT      DEFAULT 0,
    CONSTRAINT uni_accounts_username UNIQUE (username)
);
CREATE INDEX IF NOT EXISTS idx_accounts_deleted_at ON accounts (deleted_at);

CREATE TABLE IF NOT EXISTS pwd_version_events (
    user_id     BIGSERIAL   PRIMARY KEY,
    pwd_version BIGINT      NOT NULL,
    status      TEXT        NOT NULL DEFAULT 'PENDING',
    created_at  TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_status_created_at ON pwd_version_events (status, created_at);

-- +goose Down
DROP TABLE IF EXISTS pwd_version_events;
DROP TABLE IF EXISTS accounts;
