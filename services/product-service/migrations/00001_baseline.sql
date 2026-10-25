-- +goose Up
-- Schema ban đầu của product-service (trước đây do GORM AutoMigrate tạo). Idempotent (IF NOT EXISTS) để
-- chạy an toàn trên DB cũ đã có sẵn bảng. Bảng domain_events là bảng CHUNG của product/order/payment
-- (ADR-23), cấu trúc giống hệt pkg/events.

-- Hàm tìm kiếm không dấu (thuần SQL, IMMUTABLE, không cần extension): hạ chữ thường và bỏ dấu tiếng Việt.
-- Phải có trước idx_products_search vì index dùng hàm này.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION mm_unaccent(t text)
 RETURNS text
 LANGUAGE sql
 IMMUTABLE PARALLEL SAFE
AS $function$ SELECT lower(translate(t, 'àÀáÁạẠảẢãÃâÂầẦấẤậẬẩẨẫẪăĂằẰắẮặẶẳẲẵẴèÈéÉẹẸẻẺẽẼêÊềỀếẾệỆểỂễỄìÌíÍịỊỉỈĩĨòÒóÓọỌỏỎõÕôÔồỒốỐộỘổỔỗỖơƠờỜớỚợỢởỞỡỠùÙúÚụỤủỦũŨưƯừỪứỨựỰửỬữỮỳỲýÝỵỴỷỶỹỸđĐ', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaeeeeeeeeeeeeeeeeeeeeeeiiiiiiiiiioooooooooooooooooooooooooooooooooouuuuuuuuuuuuuuuuuuuuuuyyyyyyyyyydd')) $function$;
-- +goose StatementEnd

CREATE TABLE IF NOT EXISTS categories (
    id bigserial PRIMARY KEY,
    parent_id bigint NOT NULL DEFAULT 0,
    name text NOT NULL,
    slug text NOT NULL,
    sort bigint NOT NULL DEFAULT 0,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz,
    updated_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_categories_parent_id ON categories USING btree (parent_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_categories_slug ON categories USING btree (slug);

CREATE TABLE IF NOT EXISTS products (
    id bigserial PRIMARY KEY,
    name text NOT NULL,
    price numeric(18,2) NOT NULL,
    seller_id bigint NOT NULL,
    inventory bigint NOT NULL,
    attributes jsonb NOT NULL,
    sku text NOT NULL DEFAULT '',
    description text NOT NULL DEFAULT '',
    category_id bigint,
    brand text NOT NULL DEFAULT '',
    tags jsonb NOT NULL DEFAULT '[]'::jsonb,
    image_urls jsonb NOT NULL DEFAULT '[]'::jsonb,
    status text NOT NULL DEFAULT 'active',
    low_stock_threshold bigint NOT NULL DEFAULT 0,
    weight_g bigint NOT NULL DEFAULT 0,
    reserved bigint NOT NULL DEFAULT 0,
    sold bigint NOT NULL DEFAULT 0,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz,
    updated_at timestamptz,
    deleted_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_products_category_id ON products USING btree (category_id);
CREATE INDEX IF NOT EXISTS idx_products_deleted_at ON products USING btree (deleted_at);
CREATE INDEX IF NOT EXISTS idx_products_seller_id ON products USING btree (seller_id);
CREATE INDEX IF NOT EXISTS idx_products_status ON products USING btree (status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_seller_sku ON products USING btree (seller_id, sku) WHERE (sku <> '');
-- Full-text không dấu: biểu thức phải khớp chính xác searchDocument trong internal/repository.
CREATE INDEX IF NOT EXISTS idx_products_search ON products USING gin (to_tsvector('simple', mm_unaccent(coalesce(name,'') || ' ' || coalesce(brand,'') || ' ' || coalesce(sku,'') || ' ' || coalesce(description,''))));

CREATE TABLE IF NOT EXISTS inventory_ledgers (
    id bigserial PRIMARY KEY,
    product_id bigint NOT NULL,
    delta bigint NOT NULL,
    reason text NOT NULL,
    ref_type text NOT NULL DEFAULT '',
    ref_id text NOT NULL DEFAULT '',
    balance_after bigint NOT NULL,
    at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ledger_product_at ON inventory_ledgers USING btree (product_id, at);

CREATE TABLE IF NOT EXISTS validate_order_events (
    order_id bigserial PRIMARY KEY,
    success boolean NOT NULL,
    status text NOT NULL DEFAULT 'PENDING',
    processed boolean NOT NULL DEFAULT false,
    restored boolean NOT NULL DEFAULT false,
    shipped boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_status_created_at ON validate_order_events USING btree (status, created_at);

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

-- +goose Down
-- Baseline không hỗ trợ rollback (xoá bảng sẽ mất dữ liệu); thêm migration mới nếu cần sửa schema.
SELECT 1;
