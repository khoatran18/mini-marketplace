-- +goose Up
-- R10: bảng không được code nào đọc/ghi (outbox dùng create_seller_events).
DROP TABLE IF EXISTS create_seller_kafka_events;

-- +goose Down
CREATE TABLE IF NOT EXISTS create_seller_kafka_events (
    seller_id BIGINT,
    user_id   BIGINT
);
