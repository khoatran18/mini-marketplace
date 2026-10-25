-- +goose Up
-- R6: đơn đã giao bị trả hàng (REFUNDED) thì hàng nhập lại kho đúng một lần.
ALTER TABLE validate_order_events ADD COLUMN IF NOT EXISTS returned boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE validate_order_events DROP COLUMN IF EXISTS returned;
