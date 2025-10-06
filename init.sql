-- Dùng database mặc định "postgres"
\c postgres;

-- Tạo bảng Account
CREATE TABLE IF NOT EXISTS accounts (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(255) UNIQUE NOT NULL,
    password VARCHAR(255) NOT NULL,
    role VARCHAR(100) NOT NULL,
    pwd_version BIGINT NOT NULL,
    deleted_at TIMESTAMP,
    store_id BIGINT DEFAULT 0
);

-- Tạo bảng Product
CREATE TABLE IF NOT EXISTS products (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    price DOUBLE PRECISION NOT NULL,
    seller_id BIGINT NOT NULL,
    inventory BIGINT NOT NULL,
    attributes JSONB NOT NULL,
    deleted_at TIMESTAMP
);

-- Dữ liệu mẫu cho bảng Account
INSERT INTO accounts (username, password, role, pwd_version, store_id)
VALUES
    ('buyer1', 'password', 'buyer', 0, 0),
    ('seller1', 'password', 'seller_admin', 0, 0),
ON CONFLICT (username) DO NOTHING;

-- Dữ liệu mẫu cho bảng Product
INSERT INTO products (name, price, seller_id, inventory, attributes)
VALUES
    ('Iphone 15 Pro', 29990000, 1, 50, '{"color": "gray", "storage": "256GB"}'),
    ('Macbook Air M3', 34990000, 1, 20, '{"color": "silver", "ram": "16GB"}'),
    ('AirPods Pro 2', 5990000, 1, 100, '{"color": "white", "noise_cancel": true}')
ON CONFLICT DO NOTHING;
