# 03 · Mô hình dữ liệu – trường mới, tracking, ClickHouse

Bổ sung cho [../data-model.md](../data-model.md). Bảng mới dùng migration có version.

## 1. Trường mới theo service

### auth-service
| Bảng.trường | Mô tả |
|---|---|
| `accounts.role` | thêm giá trị **`admin`** (không có trong whitelist `/auth/register`; chỉ tạo bằng `ADMIN_BOOTSTRAP_*` hoặc CLI) |
| `accounts.status` | `active|locked|disabled`; `accounts.failed_logins`, `locked_until`, `last_login_at` (khoá tạm sau N lần sai – roadmap #8) |
| `accounts.email` (+ `email_verified_at`) | cần cho thông báo/ reset mật khẩu về sau (giai đoạn sau, tuỳ chọn) |
| `audit_log(id, ts, actor_id, actor_role, action, target, ip_hash, detail jsonb)` | hành động nhạy cảm của admin (khoá tài khoản, đổi rule, requeue…) |

### user-service
| `buyers` | `addresses` bảng riêng: `id, user_id, label, receiver_name, phone, line1, ward, district, city, is_default` (giao hàng nhiều địa chỉ) |
| `sellers` (store) | `status` (`pending|active|suspended`), `logo_url`, `rating_avg`, `rating_count`, `low_stock_default` (ngưỡng tồn mặc định), `created_at` |

### product-service
| Bảng.trường | Kiểu | Mô tả |
|---|---|---|
| `products.sku` | text, unique theo `(seller_id, sku)` | mã nội bộ shop |
| `products.description` | text | mô tả (markdown giới hạn, ≤ 5000 ký tự) |
| `products.category_id` | FK `categories` | |
| `products.brand` | text | |
| `products.tags` | text[] | ≤ 20 |
| `products.image_urls` | text[] | ≤ 8 ảnh (MinIO key) |
| `products.status` | enum | `draft|active|hidden|out_of_stock|banned` |
| `products.low_stock_threshold` | int null | ngưỡng cảnh báo (null → mặc định shop) |
| `products.weight_g`, `dimensions` | int, jsonb | phí vận chuyển mô phỏng |
| `products.reserved` | int | tồn đang giữ chỗ (inventory khả dụng = `inventory - reserved`) |
| `products.version` | int | optimistic lock |
| `products.created_at/updated_at` | timestamptz | |
| `categories(id, parent_id, name, slug UNIQUE, sort, active)` | | cây 2–3 cấp |
| `inventory_ledger(id, product_id, delta, reason, ref_type, ref_id, balance_after, at)` | | `reason`: `reserve|release|sale|restock|adjust|return`; bất biến (chỉ thêm) |
| `reviews(id, product_id, order_item_id UNIQUE, user_id, rating 1–5, text, status, created_at)` | | chỉ người đã `DELIVERED` mới được review |
| `wishlists(user_id, product_id, created_at)` | | |
| `product_changed_events`, `inventory_events` | outbox | cho `product.changed`, `inventory.changed` |

### order-service
| `orders` thêm | Mô tả |
|---|---|
| `status` mở rộng | xem 04 (state machine) |
| `payment_method` | `COD|MOCK_CARD|MOCK_WALLET|MOCK_BANK_TRANSFER` |
| `payment_status` | `UNPAID|PENDING|PAID|FAILED|REFUNDED|PARTIAL_REFUND` (đồng bộ từ payment-service) |
| `subtotal, shipping_fee, discount_total, total_price` | NUMERIC(18,2); server tính |
| `shipping_address jsonb` | **snapshot** địa chỉ tại thời điểm đặt (receiver, phone, line…) |
| `note`, `idempotency_key UNIQUE(buyer_id, key)` | chống đặt trùng (roadmap #3) |
| `expires_at` | hạn thanh toán (mặc định `now + ORDER_PAYMENT_TTL_MIN`) |
| `paid_at, shipped_at, delivered_at, canceled_at, cancel_reason, canceled_by` | mốc thời gian (phục vụ analytics, SLA) |
| `tracking_code`, `carrier` | giao hàng mô phỏng |
| `order_items` thêm | `product_name, sku, image_url, seller_id, category_id` (snapshot), `unit_price`, `line_total` |
| `order_status_history(id, order_id, from, to, actor_type[system|buyer|seller|admin|payment], actor_id, reason, at)` | lịch sử trạng thái (timeline UI) |
| `carts(user_id PK, updated_at)`, `cart_items(user_id, product_id, quantity, added_at)` | giỏ hàng server (`UNIQUE(user_id, product_id)`, quantity 1–99) |
| `shipments` *(tuỳ chọn)* | `order_id, seller_id, status, carrier, tracking_code` nếu tách đơn theo shop |
Tách đơn theo shop: checkout giỏ nhiều shop ⇒ tạo **một `orders` cha (`checkout_id`) + mỗi shop một đơn** để doanh thu/ tồn/ vận chuyển/ huỷ độc lập; thanh toán gộp theo `checkout_id`.

### payment-service (mới, DB `postgres`, bảng riêng) – xem 04
`payments(id, order_id/checkout_id, buyer_id, method, status, amount, currency, provider, provider_ref, idempotency_key UNIQUE, failure_code, created_at, updated_at, paid_at)` · `payment_attempts(id, payment_id, scenario, status, at, raw jsonb)` · `refunds(id, payment_id, amount, reason, status, at)` · `webhook_deliveries(id, payment_id, event, attempt, status_code, next_retry_at)` · outbox `payment_events`.

### alert-service (DB `ops`)
`alert_rules(id, key UNIQUE, name, type[promql|threshold|sql|anomaly], definition jsonb, schedule, severity, scope[platform|store], for_seconds, cooldown_s, enabled, runbook_url, owner, updated_by, updated_at)` ·
`alerts(id, fingerprint, rule_id, severity, scope, store_id, title, description, evidence jsonb, status[open|ack|resolved|silenced], count, first_seen, last_seen, ack_by, ack_at, resolved_at)` (unique `fingerprint` khi `status in (open,ack)`) ·
`silences(id, rule_id?, fingerprint?, store_id?, until, reason, created_by)` · `notifications(id, user_id, store_id, alert_id, channel, payload, created_at, read_at)` · `notification_prefs(user_id, channel, min_severity)`.

### gateway / chung
`identity_links(anonymous_id, user_id, first_seen)` (nối khách ẩn danh với tài khoản) · `consents(user_id, analytics, personalization, updated_at)` · `feature_flags(key, enabled, config, updated_at)`.

## 2. Tracking event (clickstream)

### 2.1 Envelope
```jsonc
{ "event_id":"uuid", "event_type":"product_click", "ts_client":"2026-10-06T08:15:30.123Z",
  "anonymous_id":"a_9f2", "session_id":"s_41c",
  "surface":"home_trending",          // nơi xảy ra
  "page":{"path":"/products/12","referrer":"/"},
  "item":{"product_id":12,"position":3},
  "props":{},                          // đặc thù từng loại
  "device":{"type":"mobile","os":"iOS","viewport":"390x844"},
  "consent":{"analytics":true},
  "app_version":"web-0.3.0" }
```
**Server gắn thêm** (client không được gửi): `user_id, role, store_id, ts_server, ip_hash` (HMAC + muối xoay hằng ngày; không lưu IP thô), `ua_family`, `country`.

### 2.2 Loại event
| event_type | Khi nào | `props` |
|---|---|---|
| `page_view` | vào trang | |
| `impression` | thẻ sản phẩm ≥ 50% viewport ≥ 1 s (1 lần/ phiên/ vị trí) | |
| `product_click`, `product_view`, `dwell` | bấm/ mở/ thời gian xem | `ms`, `scroll_pct` |
| `search`, `search_click` | tìm kiếm từ khoá | `q`, `filters`, `result_count`, `rank` |
| `filter_apply`, `sort_change` | | `filter`, `value` |
| `add_to_cart`, `remove_from_cart`, `cart_update` | | `qty`, `price_seen` |
| `wishlist_add/remove` | | |
| `checkout_start`, `checkout_submit`, `payment_page_view` | | `cart_value`, `method` |
| `error_client` | lỗi JS/API | `code`, `where` |
**Sự kiện server-side (client gửi cùng tên bị loại):** `order_placed`, `order_status_changed`, `payment_succeeded`, `payment_failed`, `order_canceled` – lấy từ outbox nghiệp vụ.

### 2.3 Surface
`home_trending, home_new, category_page, search_results, pdp, cart, checkout, orders, seller_console, admin_console`.

### 2.4 Tracking ẩn danh, consent, privacy
- `anonymous_id` cookie 1st-party `mm_aid` (13 tháng); đăng nhập ⇒ `POST /events/identify` ghi `identity_links`.
- `POST /events`: JWT tuỳ chọn, ≤ 50 event/batch, ≤ 64 KB, whitelist `event_type`, rate-limit riêng theo IP; client không ghi đè trường server.
- Banner consent (`analytics`); từ chối ⇒ chỉ thu `page_view`/`error_client` không định danh. Không PII trong `props` (che email/SĐT trong `q`).
- Retention: events 13 tháng; fact đơn hàng vô thời hạn; `notifications` 90 ngày. Quyền xoá: `DELETE /users/me/data`.

## 3. ClickHouse
```sql
CREATE TABLE events_raw (
  event_id UUID, event_type LowCardinality(String),
  ts DateTime64(3), ts_server DateTime64(3),
  anonymous_id String, session_id String, user_id UInt64, role LowCardinality(String), store_id UInt64,
  surface LowCardinality(String), path String, referrer String,
  product_id UInt64, position UInt16, props String,
  device_type LowCardinality(String), os LowCardinality(String), country LowCardinality(String), ua_family LowCardinality(String),
  analytics_consent UInt8
) ENGINE = ReplacingMergeTree(ts_server)
  PARTITION BY toYYYYMMDD(ts) ORDER BY (event_type, anonymous_id, ts, event_id)
  TTL toDateTime(ts) + INTERVAL 13 MONTH;

CREATE TABLE fact_orders (
  order_id UInt64, checkout_id String, buyer_id UInt64, store_id UInt64, status LowCardinality(String), payment_method LowCardinality(String), payment_status LowCardinality(String),
  subtotal Decimal(18,2), shipping_fee Decimal(18,2), discount_total Decimal(18,2), total Decimal(18,2),
  created_at DateTime64(3), paid_at Nullable(DateTime64(3)), delivered_at Nullable(DateTime64(3)), canceled_at Nullable(DateTime64(3)), status_at DateTime64(3)
) ENGINE = ReplacingMergeTree(status_at) ORDER BY order_id;

CREATE TABLE fact_order_items (
  order_id UInt64, item_id UInt64, store_id UInt64, product_id UInt64, category_id UInt32,
  qty UInt32, unit_price Decimal(18,2), line_total Decimal(18,2), status LowCardinality(String), status_at DateTime64(3)
) ENGINE = ReplacingMergeTree(status_at) ORDER BY (order_id, item_id);

CREATE TABLE fact_payments (payment_id UInt64, order_id UInt64, method LowCardinality(String), status LowCardinality(String), amount Decimal(18,2), failure_code LowCardinality(String), created_at DateTime64(3), updated_at DateTime64(3))
  ENGINE = ReplacingMergeTree(updated_at) ORDER BY payment_id;

CREATE TABLE dim_products (product_id UInt64, store_id UInt64, name String, category_id UInt32, price Decimal(18,2), status LowCardinality(String), updated_at DateTime64(3))
  ENGINE = ReplacingMergeTree(updated_at) ORDER BY product_id;

CREATE TABLE fact_inventory (product_id UInt64, store_id UInt64, delta Int32, reason LowCardinality(String), balance_after Int32, at DateTime64(3)) ENGINE = MergeTree ORDER BY (product_id, at);
```
**Rollup (materialized views)**: `mv_revenue_hourly` (theo giờ × shop × category × method: doanh thu, đơn, đơn hủy, AOV), `mv_traffic_hourly` (PV, session, UV, new/returning, theo path/ referrer/ device), `mv_funnel_daily` (view→cart→checkout→paid), `mv_product_stats_daily` (impression, click, view, cart, order, units, revenue), `mv_search_terms_daily`, `mv_payment_hourly` (tỉ lệ thành công/ thất bại theo method & failure_code).
**Định nghĩa doanh thu** (một nơi duy nhất, dùng cho mọi dashboard): *doanh thu ghi nhận* = tổng `line_total` của đơn có `payment_status=PAID` (online) hoặc `status=DELIVERED` (COD), trừ hoàn tiền; **tính theo thời điểm `paid_at`/`delivered_at`**. GMV = đơn đã đặt (không tính EXPIRED/FAILED/CANCELED trước thanh toán). Phải ghi chú định nghĩa này trên UI (tooltip).
**Đối soát**: job đêm so tổng doanh thu theo ngày giữa ClickHouse và Postgres; lệch > 0,5% ⇒ alert `reconcile_mismatch`.
