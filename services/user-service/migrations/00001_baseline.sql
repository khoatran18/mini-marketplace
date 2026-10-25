-- +goose Up
-- Baseline = schema do GORM AutoMigrate cũ tạo ra (sellers, buyers, addresses, outbox).
-- Dùng IF NOT EXISTS để chạy an toàn trên DB cũ đã có bảng.
-- Lưu ý: create_seller_kafka_events là bảng thừa (bỏ ở 00002); giữ trong baseline
-- cho DB cũ.

CREATE TABLE IF NOT EXISTS sellers (
    id            BIGSERIAL PRIMARY KEY,
    name          TEXT NOT NULL,
    bank_account  TEXT NOT NULL,
    tax_code      TEXT NOT NULL,
    description   TEXT NOT NULL,
    date_of_birth TIMESTAMPTZ,
    phone         TEXT NOT NULL,
    address       TEXT NOT NULL,
    deleted_at    TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_sellers_deleted_at ON sellers (deleted_at);

CREATE TABLE IF NOT EXISTS buyers (
    user_id       BIGSERIAL PRIMARY KEY,
    name          TEXT NOT NULL,
    gender        TEXT NOT NULL,
    date_of_birth TIMESTAMPTZ,
    phone         TEXT NOT NULL,
    address       TEXT NOT NULL,
    deleted_at    TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_buyers_deleted_at ON buyers (deleted_at);

CREATE TABLE IF NOT EXISTS addresses (
    id            BIGSERIAL PRIMARY KEY,
    user_id       BIGINT NOT NULL,
    label         TEXT NOT NULL DEFAULT '',
    receiver_name TEXT NOT NULL,
    phone         TEXT NOT NULL,
    line1         TEXT NOT NULL,
    ward          TEXT NOT NULL DEFAULT '',
    district      TEXT NOT NULL DEFAULT '',
    city          TEXT NOT NULL,
    is_default    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ,
    updated_at    TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_addresses_user_id ON addresses (user_id);

CREATE TABLE IF NOT EXISTS create_seller_events (
    seller_id  BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_status_created_at ON create_seller_events (status, created_at);

CREATE TABLE IF NOT EXISTS create_seller_kafka_events (
    seller_id BIGINT,
    user_id   BIGINT
);

-- +goose Down
DROP TABLE IF EXISTS create_seller_kafka_events;
DROP TABLE IF EXISTS create_seller_events;
DROP TABLE IF EXISTS addresses;
DROP TABLE IF EXISTS buyers;
DROP TABLE IF EXISTS sellers;
