-- +goose Up
-- Baseline = schema do GORM AutoMigrate tạo trước đây. Mọi lệnh idempotent (IF NOT EXISTS) nên chạy an toàn
-- cả trên DB trống lẫn DB cũ đã có dữ liệu. domain_events là bảng chung của product/order/payment (ADR-23),
-- cấu trúc phải giống hệt pkg/events.

CREATE TABLE IF NOT EXISTS orders (
    id bigserial PRIMARY KEY,
    buyer_id bigint NOT NULL,
    status text NOT NULL DEFAULT 'PENDING',
    total_price numeric(18,2) NOT NULL DEFAULT 0,
    created_at timestamptz,
    updated_at timestamptz,
    checkout_id text NOT NULL DEFAULT '',
    store_id bigint NOT NULL DEFAULT 0,
    payment_method text NOT NULL DEFAULT 'COD',
    payment_status text NOT NULL DEFAULT 'UNPAID',
    subtotal numeric(18,2) NOT NULL DEFAULT 0,
    shipping_fee numeric(18,2) NOT NULL DEFAULT 0,
    shipping_address jsonb,
    note text NOT NULL DEFAULT '',
    expires_at timestamptz,
    paid_at timestamptz,
    shipped_at timestamptz,
    delivered_at timestamptz,
    canceled_at timestamptz,
    cancel_reason text NOT NULL DEFAULT '',
    canceled_by text NOT NULL DEFAULT '',
    carrier text NOT NULL DEFAULT '',
    tracking_code text NOT NULL DEFAULT '',
    return_reason text NOT NULL DEFAULT '',
    return_rejected boolean NOT NULL DEFAULT false
);
CREATE INDEX IF NOT EXISTS order_index ON orders USING btree (buyer_id, status);
CREATE INDEX IF NOT EXISTS idx_orders_checkout_id ON orders USING btree (checkout_id);
CREATE INDEX IF NOT EXISTS idx_orders_store_status ON orders USING btree (store_id);
CREATE INDEX IF NOT EXISTS idx_orders_expires_at ON orders USING btree (expires_at);

CREATE TABLE IF NOT EXISTS order_items (
    id bigserial PRIMARY KEY,
    order_id bigint,
    product_id bigint NOT NULL,
    quantity bigint NOT NULL,
    price numeric(18,2) NOT NULL,
    status text NOT NULL DEFAULT 'ACTIVE',
    created_at timestamptz,
    updated_at timestamptz,
    product_name text NOT NULL DEFAULT '',
    sku text NOT NULL DEFAULT '',
    image_url text NOT NULL DEFAULT '',
    store_id bigint NOT NULL DEFAULT 0,
    category_id bigint NOT NULL DEFAULT 0,
    line_total numeric(18,2) NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS order_item_index ON order_items USING btree (order_id, status);

-- FK giữ đúng như GORM từng tạo (không ON DELETE CASCADE)
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_orders_order_items') THEN
        ALTER TABLE order_items ADD CONSTRAINT fk_orders_order_items FOREIGN KEY (order_id) REFERENCES orders(id);
    END IF;
END $$;
-- +goose StatementEnd

CREATE TABLE IF NOT EXISTS order_status_histories (
    id bigserial PRIMARY KEY,
    order_id bigint NOT NULL,
    from_status text NOT NULL DEFAULT '',
    to_status text NOT NULL,
    actor_type text NOT NULL,
    actor_id bigint NOT NULL DEFAULT 0,
    reason text NOT NULL DEFAULT '',
    at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_order_status_histories_order_id ON order_status_histories USING btree (order_id);

CREATE TABLE IF NOT EXISTS checkouts (
    id text PRIMARY KEY,
    buyer_id bigint NOT NULL,
    idempotency_key text NOT NULL,
    payment_method text NOT NULL,
    total numeric(18,2) NOT NULL,
    created_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_checkout_idem ON checkouts USING btree (buyer_id, idempotency_key);

CREATE TABLE IF NOT EXISTS cart_items (
    user_id bigint NOT NULL,
    product_id bigint NOT NULL,
    quantity bigint NOT NULL,
    added_at timestamptz,
    updated_at timestamptz,
    PRIMARY KEY (user_id, product_id)
);

-- Outbox cũ (vẫn được worker xả cho các dòng do phiên bản trước ghi)
CREATE TABLE IF NOT EXISTS create_order_events (
    order_id bigserial PRIMARY KEY,
    items jsonb,
    status text
);
CREATE INDEX IF NOT EXISTS idx_co_kafka ON create_order_events USING btree (status);

CREATE TABLE IF NOT EXISTS cancel_order_events (
    order_id bigserial PRIMARY KEY,
    items jsonb,
    status text
);
CREATE INDEX IF NOT EXISTS idx_cancel_kafka ON cancel_order_events USING btree (status);

-- Outbox chung (giống pkg/events)
CREATE TABLE IF NOT EXISTS domain_events (
    id bigserial PRIMARY KEY,
    topic text NOT NULL,
    key text NOT NULL,
    payload jsonb NOT NULL,
    status text NOT NULL DEFAULT 'PENDING',
    attempts bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL,
    published_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_domain_events_status_id ON domain_events USING btree (status);

-- Dữ liệu cũ: "SUCCESS" (đã giữ hàng, chưa có khái niệm thanh toán) nay là CONFIRMED (COD)
UPDATE orders SET status = 'CONFIRMED' WHERE status = 'SUCCESS';

-- +goose Down
-- domain_events là bảng chung với service khác nên không xoá ở đây.
DROP TABLE IF EXISTS cancel_order_events;
DROP TABLE IF EXISTS create_order_events;
DROP TABLE IF EXISTS cart_items;
DROP TABLE IF EXISTS checkouts;
DROP TABLE IF EXISTS order_status_histories;
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;
