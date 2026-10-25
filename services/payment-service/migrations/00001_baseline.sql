-- +goose Up
-- Baseline = schema do GORM AutoMigrate tạo trước đây. Mọi lệnh dùng IF NOT EXISTS nên chạy an toàn
-- trên DB cũ. domain_events là bảng vật lý CHUNG (ADR-23): cấu trúc phải giống hệt pkg/events.

CREATE TABLE IF NOT EXISTS payments (
    id             BIGSERIAL PRIMARY KEY,
    checkout_id    TEXT NOT NULL,
    buyer_id       BIGINT NOT NULL,
    method         TEXT NOT NULL,
    status         TEXT NOT NULL,
    amount_minor   BIGINT NOT NULL,
    currency       TEXT NOT NULL DEFAULT 'VND',
    provider       TEXT NOT NULL DEFAULT 'mockpay',
    provider_ref   TEXT NOT NULL DEFAULT '',
    failure_code   TEXT NOT NULL DEFAULT '',
    card_last4     TEXT NOT NULL DEFAULT '',
    failed_count   BIGINT NOT NULL DEFAULT 0,
    three_ds_passed BOOLEAN NOT NULL DEFAULT false,
    order_ids      JSONB NOT NULL DEFAULT '[]'::jsonb,
    expires_at     TIMESTAMPTZ NOT NULL,
    paid_at        TIMESTAMPTZ,
    refunded_minor BIGINT NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ,
    updated_at     TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_payments_checkout_id ON payments (checkout_id);
CREATE INDEX IF NOT EXISTS idx_payments_buyer_id ON payments (buyer_id);
CREATE INDEX IF NOT EXISTS idx_payments_status ON payments (status);
CREATE INDEX IF NOT EXISTS idx_payments_expires_at ON payments (expires_at);

CREATE TABLE IF NOT EXISTS attempts (
    id           BIGSERIAL PRIMARY KEY,
    payment_id   BIGINT NOT NULL,
    scenario     TEXT NOT NULL,
    status       TEXT NOT NULL,
    failure_code TEXT NOT NULL DEFAULT '',
    at           TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_attempts_payment_id ON attempts (payment_id);

CREATE TABLE IF NOT EXISTS refunds (
    id           BIGSERIAL PRIMARY KEY,
    payment_id   BIGINT NOT NULL,
    order_id     BIGINT NOT NULL DEFAULT 0,
    refund_key   TEXT NOT NULL,
    amount_minor BIGINT NOT NULL,
    reason       TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL,
    at           TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_refunds_payment_id ON refunds (payment_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_refunds_refund_key ON refunds (refund_key);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id                BIGSERIAL PRIMARY KEY,
    payment_id        BIGINT NOT NULL,
    provider_event_id TEXT NOT NULL,
    type              TEXT NOT NULL,
    payload           JSONB NOT NULL,
    deliver_at        TIMESTAMPTZ NOT NULL,
    attempts          BIGINT NOT NULL DEFAULT 0,
    status            TEXT NOT NULL DEFAULT 'PENDING',
    last_code         BIGINT NOT NULL DEFAULT 0,
    delivered_at      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_payment_id ON webhook_deliveries (payment_id);
CREATE INDEX IF NOT EXISTS idx_webhook_due ON webhook_deliveries (status, deliver_at);

CREATE TABLE IF NOT EXISTS processed_events (
    provider_event_id TEXT PRIMARY KEY,
    at                TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS domain_events (
    id           BIGSERIAL PRIMARY KEY,
    topic        TEXT NOT NULL,
    key          TEXT NOT NULL,
    payload      JSONB NOT NULL,
    status       TEXT NOT NULL DEFAULT 'PENDING',
    attempts     BIGINT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL,
    published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_domain_events_status_id ON domain_events (status);

-- +goose Down
-- Baseline không hoàn tác (domain_events dùng chung với service khác, dữ liệu tiền).
SELECT 1;
