# 03 · Mô hình dữ liệu – trường mới, tracking, ClickHouse

> **Trạng thái: ✅ phần lớn ĐÃ TRIỂN KHAI.** Bản đầy đủ, chính xác theo code ở [../data-model.md](../data-model.md); tệp này giữ thiết kế + đối chiếu "đã làm / chưa làm". Bảng Postgres vẫn dùng `AutoMigrate` (chưa có migration có version – "goose" trong thiết kế **chưa làm**); ClickHouse có migration có version.

## 1. Trường mới theo service
### auth-service
| Thiết kế | Trạng thái |
|---|---|
| `accounts.role` thêm `admin` (chỉ `ADMIN_BOOTSTRAP_*`) | ✅ |
| `accounts.status`, `failed_logins`, `locked_until`, `last_login_at` (khoá tạm đăng nhập) | ❌ chưa làm |
| `accounts.email`, `email_verified_at` | ❌ |
| `audit_log` | ❌ |

### user-service
| Thiết kế | Trạng thái |
|---|---|
| `addresses(id, user_id, label, receiver_name, phone, line1, ward, district, city, is_default)` | ✅ (≤ 10/user, một mặc định, + `created_at/updated_at`) |
| `sellers`: `status`, `logo_url`, `rating_*`, `low_stock_default`, `created_at` | ❌ |

### product-service
| Thiết kế | Trạng thái |
|---|---|
| `products.sku` (unique `(seller_id, sku)`), `description`, `category_id`, `brand`, `tags`, `image_urls` (≤ 8, **URL**, không MinIO), `low_stock_threshold`, `weight_g`, `reserved`, `version`, `created_at/updated_at` | ✅ (thêm `sold`) |
| `products.status` | ✅ `draft | active | hidden | banned`; **`out_of_stock` KHÔNG là status** – suy ra `stock_level` (`none|low|ok`) từ tồn (ADR-21) |
| `products.dimensions` | ❌ |
| `categories(id, parent_id, name, slug UNIQUE, sort, active)` cây ≤ 3 cấp | ✅ |
| `inventory_ledgers` (`reason`: `initial, reserve, release, sale, restock, adjust, update`) | ✅ (bảng tên GORM `inventory_ledgers`; `return` không dùng) |
| `reviews`, `wishlists` | ❌ chưa làm |
| outbox `product.changed`, `inventory.changed` | ✅ trong `domain_events` |

### order-service
| Thiết kế | Trạng thái |
|---|---|
| `orders` mở rộng: `status` (11 giá trị, xem 04), `payment_method`, `payment_status` (**chỉ** `UNPAID|PAID|REFUNDED`), `subtotal`, `shipping_fee`, `total_price`, `shipping_address` JSONB (snapshot), `note`, `expires_at`, `paid_at/shipped_at/delivered_at/canceled_at`, `cancel_reason`, `canceled_by`, `carrier`, `tracking_code` | ✅ (thêm `checkout_id`, `store_id`, `return_reason`, `return_rejected`) |
| idempotency `UNIQUE(buyer_id, key)` | ✅ trên bảng `checkouts` (không nằm ở `orders`) |
| `order_items` snapshot `product_name, sku, image_url, store_id, category_id, line_total` | ✅ |
| `order_status_history` | ✅ (bảng `order_status_histories`) |
| `carts`/`cart_items` | ✅ chỉ `cart_items(user_id, product_id, quantity 1–99)`; không có bảng `carts` |
| Tách đơn theo store (`checkout_id` cha) | ✅ bảng `checkouts` + mỗi store một `orders` |
| `shipments` | ❌ (carrier/tracking nằm trên `orders`) |
| `payment_status` `PENDING/FAILED/PARTIAL_REFUND` | ❌ (trạng thái chi tiết ở `payments.status`) |

### payment-service
`payments`, `attempts`, `refunds`, `webhook_deliveries`, `processed_events`, `domain_events` – ✅ ([../data-model.md](../data-model.md)). Khác thiết kế: `payments` khoá theo **`checkout_id` UNIQUE** (một payment cho cả checkout) thay vì `order_id`; không có `idempotency_key` riêng (idempotency theo `checkout_id`/`provider_event_id`/`refund_key`); số tiền là `amount_minor` (BIGINT).

### gateway / chung
`identity_links`, `consents`, `feature_flags` – ❌ **chưa làm** (identify chỉ ghi event `identify` vào `events_raw`; consent đi kèm từng event).

## 2. Tracking event (clickstream) – ✅
### 2.1 Envelope (JSON gửi tới `POST /events` – xem [../api.md](../api.md) §11)
```jsonc
{ "event_id":"uuid", "event_type":"product_click", "ts_client":"2026-10-06T08:15:30.123Z",
  "anonymous_id":"a_9f2", "session_id":"s_41c",
  "surface":"home_trending",
  "page":{"path":"/products/12","referrer":"/"},
  "item":{"product_id":12,"position":3},
  "props":{},
  "device":{"type":"mobile","os":"iOS"},
  "consent":{"analytics":true},
  "app_version":"web-0.3.0" }
```
**Server gắn thêm** (client không có trường này): `user_id, role, store_id` (JWT), `ts_server`, `ip_hash` (HMAC + ngày UTC, xoay hằng ngày, không lưu IP thô), `ua_family` (chrome/firefox/safari/edge/bot/other), `country` (header `CF-IPCountry` nếu có). (`device.viewport` trong thiết kế **không** được lưu.)

### 2.2 Loại event
Client được gửi: `page_view, impression, product_click, product_view, dwell, search, search_click, filter_apply, sort_change, add_to_cart, remove_from_cart, cart_update, wishlist_add, wishlist_remove, checkout_start, checkout_submit, payment_page_view, error_client`; `identify` chỉ từ `POST /events/identify`. **Event nghiệp vụ** (`order_placed`, `order_status_changed`, `payment_succeeded`…) **không** là event tracking: bị loại nếu client gửi, còn dữ liệu đơn/ thanh toán đi Kafka → bảng fact. `props` quy ước: `search` có `q`, `result_count` (dùng bởi báo cáo `search_terms`).
`surface`: `home_trending, home_new, category_page, search_results, pdp, cart, checkout, orders, seller_console, admin_console`.

### 2.3 Consent, privacy
- `anonymous_id` do client giữ (cookie 1st-party); đăng nhập ⇒ `POST /events/identify {anonymous_id}` ghi event `identify` (không có bảng `identity_links`).
- ≤ 50 event/lần, ≤ 64 KB, whitelist `event_type`, rate limit riêng theo IP; client không ghi đè trường server.
- Không consent ⇒ chỉ `page_view`/`error_client`, đã bỏ định danh; PII (email/SĐT) trong chuỗi `props` bị che.
- Retention: `events_raw` TTL `EVENTS_RETENTION_MONTHS` (13 tháng); fact vô thời hạn. **`DELETE /users/me/data` ❌ chưa làm.**

## 3. ClickHouse – ✅ (thực tế, `services/analytics-service/internal/store/schema.go`)
Khác thiết kế ban đầu: **không** có `fact_orders` gộp, không có `Decimal` (dùng `Int64 *_minor`), không có MV; bảng `fact_order_status` có **một dòng cho mỗi (đơn, trạng thái)**; có thêm `fact_payment_events`, `fact_refunds`; `fact_inventory` là `ReplacingMergeTree`. Mọi thời gian `DateTime64(3,'UTC')`.
```sql
events_raw(event_id String, event_type LowCardinality(String), ts, ts_server, anonymous_id, session_id, user_id UInt64, role, store_id UInt64,
  surface, path, referrer, product_id UInt64, position UInt16, props String, device_type, os, country, ua_family,
  analytics_consent UInt8, ip_hash String, app_version, ingested_at DEFAULT now64(3))
  ENGINE ReplacingMergeTree(ts_server) PARTITION BY toYYYYMM(ts) ORDER BY (event_type, anonymous_id, ts, event_id) TTL ts + INTERVAL <EVENTS_RETENTION_MONTHS> MONTH
fact_order_status(order_id, status, checkout_id, buyer_id, store_id, payment_method, payment_status, subtotal_minor, shipping_fee_minor, total_minor, at, ingested_at)
  ReplacingMergeTree(at) ORDER BY (order_id, status)
fact_order_items(order_id, item_id, store_id, product_id, category_id, qty, unit_price_minor, at, ingested_at)  ReplacingMergeTree(at) ORDER BY (order_id, item_id)
fact_payment_events(payment_id, kind succeeded|failed, checkout_id, buyer_id, method, amount_minor, failure_code, terminal, at, ingested_at)  ORDER BY (payment_id, kind, at)
fact_refunds(refund_id, payment_id, checkout_id, order_id, amount_minor, reason, at, ingested_at)  ORDER BY refund_id
dim_products(product_id, store_id, name, sku, category_id, brand, price_minor, status, updated_at, ingested_at)  ReplacingMergeTree(updated_at) ORDER BY product_id
fact_inventory(product_id, store_id, delta, available, reserved, level, reason, ref_type, ref_id, at, ingested_at)  ORDER BY (product_id, at, reason, ref_type, ref_id)
schema_migrations(version, applied_at)   -- 7 migration; không bao giờ sửa migration đã chạy
```
**Rollup (materialized view)** `mv_revenue_hourly`, `mv_traffic_hourly`, `mv_funnel_daily`, `mv_product_stats_daily`, `mv_search_terms_daily`, `mv_payment_hourly`: ❌ **chưa làm** – báo cáo gom bảng fact với `FINAL` lúc truy vấn (ADR-14).
**Định nghĩa doanh thu** (một nơi duy nhất, dùng cho mọi báo cáo): *doanh thu ghi nhận* = `subtotal` (tiền hàng, không gồm ship) của đơn có trạng thái **PAID** (online) hoặc **DELIVERED** (COD), tính theo thời điểm đó; trừ hoàn tiền theo **thời điểm hoàn**; GMV = đơn đặt trừ `EXPIRED/FAILED/CANCELED-chưa-PAID`. Ghi chú định nghĩa này trên UI (tooltip) do nhánh UI.
**Đối soát**: job đêm so Postgres ↔ ClickHouse và `mm_reconcile_diff_ratio` ❌ **chưa làm** (`DataHealth` trả `reconciliation:"not_implemented"`). Backfill ❌.
