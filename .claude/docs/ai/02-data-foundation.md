# 02 · Nền tảng dữ liệu

AI chỉ giỏi bằng dữ liệu nó thấy. Mục này liệt kê **cái còn thiếu** và **cái cần thu** từ ngày đầu.

## 1. Khoảng trống hiện tại (đối chiếu code)

| Cần cho | Hiện trạng | Cần bổ sung | Service |
|---|---|---|---|
| Tìm theo mô tả | `products(name, price, seller_id, inventory, attributes JSONB)` – **không có mô tả, danh mục, ảnh** | `description`, `category_id`, `brand`, `tags[]`, `image_urls[]`, `status` (`active/hidden/out_of_stock`), `created_at/updated_at` | product |
| Danh mục | Không có | bảng `categories(id, parent_id, name, slug)` (cây 2–3 cấp) | product |
| Doanh thu thật | Order chỉ `PENDING/SUCCESS/FAILED/CANCELED`; `SUCCESS` = "đã giữ hàng", **không phải đã thanh toán** | Vòng đời `PAID → SHIPPED → DELIVERED`, `REFUNDED` (roadmap #1,#4); trước mắt có thể coi `SUCCESS` là "đã đặt" và đánh dấu rõ trong dashboard | order |
| Chi tiết đơn | `order_items` không lưu tên/danh mục/`seller_id` | snapshot `product_name`, `seller_id`, `category_id` trên item (roadmap #5) → doanh thu theo shop/danh mục không cần join ngược | order |
| Doanh thu theo shop | order không có `seller_id` | tách đơn theo shop khi checkout hoặc lưu `seller_id` ở item | order |
| Giỏ hàng cho gợi ý | `CartProvider` + localStorage | bảng `carts`, `cart_items` + API; sự kiện `cart.updated` | order (hoặc service mới `cart`) |
| Admin | Không có role `admin` | role `admin`, seed bằng CLI/env, **không** cho `/auth/register` | auth |
| Đánh giá/thích | Không có | `reviews(product_id, user_id, rating, text)`, `wishlist` (explicit feedback cho recsys) | product |
| Tồn kho lịch sử | chỉ số hiện tại | `inventory_ledger(product_id, delta, reason, ref_id, at)` | product |
| Truy cập/lưu lượng | Không có | tracking events (mục 3) | analytics |
| CPU/RAM | Không có metrics | Prometheus (xem 06) | tất cả |
| Sự kiện thay đổi catalog | Không có | outbox `product.changed` để re-index embedding | product |
| Vùng giao hàng, giờ, thiết bị | Không có | trường `context` trong tracking; địa chỉ lấy từ profile (đã có `address`) | – |

> Các mục trên là *việc hạ tầng* (làm trước, không cần AI). Thứ tự ưu tiên ở [10](10-roadmap-phases.md).

## 2. Bản đồ lưu trữ

| Dữ liệu | Nơi lưu | Lý do |
|---|---|---|
| Giao dịch (orders, products, accounts) | Postgres `postgres` (hiện có) | ACID |
| Hội thoại, audit AI, văn bản, chunk, embedding, alert, feature flag | Postgres DB **`ai`** (cùng instance, DB riêng) | Cô lập khỏi dữ liệu nghiệp vụ; pgvector |
| Events thô, fact, rollup | ClickHouse | OLAP |
| Feature online, cache gợi ý, counter nhanh | Redis | ms latency |
| File gốc (PDF luật, ảnh) | MinIO (hoặc volume trong giai đoạn đầu) | Object storage |
| Mô hình huấn luyện / artifact | MinIO `models/` (versioned) | Reproducible |

## 3. Sự kiện tracking (clickstream)

### 3.1 Envelope chung
```jsonc
{
  "event_id": "uuid-v4",           // client sinh, dùng dedupe (at-least-once)
  "event_type": "product_click",   // xem bảng 3.2
  "ts_client": "2026-10-06T08:15:30.123Z",
  "anonymous_id": "a_9f2…",        // cookie 1st-party, tồn tại cả khi chưa đăng nhập
  "session_id": "s_41c…",          // 30 phút không hoạt động → phiên mới
  "surface": "home_for_you",       // nơi xảy ra (xem 3.3)
  "page": { "path": "/products/12", "referrer": "/", "title": "…" },
  "item": { "product_id": 12, "position": 3 },   // khi liên quan tới sản phẩm
  "reco": { "request_id": "r_77…", "model_version": "pop-v1" }, // khi đến từ gợi ý/tìm kiếm
  "props": { },                    // phần đặc thù theo event_type
  "device": { "type": "mobile", "os": "iOS", "viewport": "390x844" },
  "consent": { "analytics": true, "personalization": true },
  "app_version": "web-0.3.0"
}
```
**Server gắn thêm** (client *không* được gửi): `user_id`, `role`, `store_id`, `ts_server`, `ip_hash` (HMAC + xoay muối hằng ngày, không lưu IP thô), `ua_family`, `geo_country/city` (tra offline).

### 3.2 Danh mục event
| event_type | Khi nào | `props` chính | Dùng cho |
|---|---|---|---|
| `page_view` | mỗi lần vào trang | – | traffic, funnel |
| `impression` | thẻ sản phẩm ≥ 50% trong viewport ≥ 1 s | – (position, reco ở envelope) | CTR, **negative sample** |
| `product_click` | bấm thẻ sản phẩm | – | positive signal |
| `product_view` | mở trang chi tiết | `source`, `referrer_surface` | |
| `dwell` | rời trang chi tiết / mỗi 15 s | `ms`, `scroll_pct` | implicit strength |
| `search` | gửi truy vấn | `q`, `mode` (`keyword|semantic|assistant`), `filters`, `result_count`, `latency_ms` | search quality, nhu cầu chưa đáp ứng |
| `search_click` | bấm kết quả | `q`, `rank` | learning-to-rank |
| `filter_apply`, `sort_change` | | `filter`, `value` | |
| `add_to_cart` / `remove_from_cart` / `cart_update` | | `qty`, `price_seen` | intent mạnh |
| `wishlist_add/remove` | | | explicit |
| `checkout_start` / `checkout_submit` | | `cart_value`, `items` | funnel |
| `order_placed` | (server phát, không tin client) | `order_id`, `total`, `items` | label mua |
| `order_canceled` | (server) | `reason` | negative/return |
| `review_submit` | | `rating` | explicit |
| `assistant_open` / `assistant_message` / `assistant_feedback` | UI chat | `conversation_id`, `thumb`, `reason` | đánh giá agent |
| `reco_feedback` | "không quan tâm", "ẩn" | `reason` | negative explicit |
| `alert_ack` / `alert_open` | UI alert | `alert_id` | vận hành |
| `error_client` | lỗi JS/API | `code`, `where` | chất lượng |

`order_placed`, `order_canceled`, `payment_*` là **sự kiện server-side** lấy từ outbox nghiệp vụ; sự kiện client cùng tên bị gateway loại bỏ.

### 3.3 Danh sách `surface`
`home_for_you`, `home_trending`, `search_results`, `search_assistant`, `pdp_similar`, `pdp_bought_together`, `cart_addon`, `post_purchase`, `seller_dashboard`, `admin_dashboard`, `assistant_chat`.

## 4. ClickHouse (đề xuất)

```sql
-- sự kiện thô, partition theo ngày, TTL 13 tháng
CREATE TABLE events_raw (
  event_id UUID, event_type LowCardinality(String),
  ts DateTime64(3), ts_server DateTime64(3),
  anonymous_id String, session_id String, user_id UInt64, role LowCardinality(String), store_id UInt64,
  surface LowCardinality(String), path String,
  product_id UInt64, position UInt16, request_id String, model_version LowCardinality(String),
  props String /* JSON */, device_type LowCardinality(String), country LowCardinality(String),
  consent_personalization UInt8
) ENGINE = ReplacingMergeTree(ts_server)         -- dedupe theo event_id
  PARTITION BY toYYYYMMDD(ts) ORDER BY (event_type, anonymous_id, ts, event_id)
  TTL toDateTime(ts) + INTERVAL 13 MONTH;

-- fact đơn hàng (từ order.status_changed)
CREATE TABLE fact_order_items (
  order_id UInt64, item_id UInt64, buyer_id UInt64, store_id UInt64, product_id UInt64, category_id UInt32,
  qty UInt32, unit_price Decimal(18,2), status LowCardinality(String), status_at DateTime64(3), created_at DateTime64(3)
) ENGINE = ReplacingMergeTree(status_at) ORDER BY (order_id, item_id);

-- rollup ví dụ: doanh thu theo giờ/shop
CREATE MATERIALIZED VIEW mv_revenue_hourly … AS
  SELECT toStartOfHour(status_at) h, store_id, sumIf(qty*unit_price, status IN ('PAID','SHIPPED','DELIVERED')) revenue, uniqExact(order_id) orders
  FROM fact_order_items GROUP BY h, store_id;
```
Rollup có sẵn: `mv_revenue_hourly`, `mv_traffic_hourly` (page_view, sessions, uv), `mv_funnel_daily` (view→cart→checkout→order), `mv_product_stats_daily` (impression, click, view, cart, purchase theo sản phẩm), `mv_search_terms_daily`.

## 5. Postgres DB `ai` (bảng mới, migration có version)
```
conversations(id uuid, user_id, role, store_id, title, created_at, last_message_at)
messages(id, conversation_id, role[user|assistant|tool], content jsonb, tokens_in, tokens_out, model, created_at)
ai_audit(id, ts, user_id, role, store_id, conversation_id, kind[chat|tool|reco|rag], tool_name, args jsonb, status, latency_ms, tokens_in, tokens_out, cost_usd, error)
policy_documents(id, title, kind[law|platform_policy|category_rule], jurisdiction, issuer, doc_number, effective_from, effective_to, status[draft|pending|indexed|retired], source_url, file_key, sha256, created_by, created_at)
policy_chunks(id, document_id, version, ordinal, heading_path, text, token_count, embedding vector(<dim>), tsv tsvector, metadata jsonb)
product_embeddings(product_id PK, model, dim, embedding vector(<dim>), text_hash, updated_at)
alerts(id, fingerprint UNIQUE-while-open, rule_id, severity[info|warning|critical], scope[platform|store], store_id, title, description, evidence jsonb, status[open|ack|resolved|suppressed], ai_summary, suggested_actions jsonb, first_seen, last_seen, count, ack_by, resolved_at)
alert_rules(id, name, type[promql|sql|threshold|anomaly|ai_scan], definition jsonb, schedule, severity, scope, enabled, owner)
notifications(id, user_id, store_id, channel, alert_id, payload, read_at, sent_at)
feature_flags(key PK, enabled, rollout_pct, config jsonb, updated_at)
reco_requests(request_id PK, ts, user_id, anonymous_id, surface, model_version, experiment, item_ids int[], scores real[])   -- cũng ghi sang ClickHouse
eval_cases / eval_runs(…)   -- bộ test cho agent, xem 03 §9
```
`<dim>` chốt khi bạn chọn embedding model (ví dụ 768/1024/1536). Đổi model ⇒ cột `model` + re-index; **không** trộn vector của 2 model trong cùng truy vấn.

## 6. Tracking ẩn danh
- Gateway thêm nhóm route **công khai có rate-limit riêng**: `POST /events`, `GET /products` (đã có nhưng đang yêu cầu JWT – cân nhắc mở), `GET /recommendations` (anonymous → chỉ trending), `GET /search`.
- `anonymous_id` do client sinh (cookie `mm_aid`, `SameSite=Lax`, 13 tháng). Khi đăng nhập, SDK gửi `identify` → gateway ghi cặp `(anonymous_id, user_id)` (bảng `identity_links`) để nối lịch sử trước đăng nhập.
- Body `/events`: tối đa 64 KB, 50 event/batch; `event_type` ngoài whitelist bị bỏ; trường client không được ghi đè trường server.

## 7. Privacy, đồng thuận, vận hành dữ liệu
- **Consent**: banner 2 mức (`analytics`, `personalization`). Không đồng ý ⇒ chỉ thu `page_view` ẩn danh tổng hợp, không dùng để cá nhân hoá. Cờ `consent_*` lưu trên từng event.
- **PII**: không đưa PII vào `props`; truy vấn tìm kiếm có thể chứa PII ⇒ quét & che (email/số điện thoại) trước khi lưu; log LLM (prompt) cũng che.
- **Quyền xoá**: `DELETE /users/me/data` ⇒ xoá `identity_links`, hội thoại, đánh dấu ẩn danh events theo `user_id` (ClickHouse `ALTER … UPDATE`/mutation theo lô).
- **Retention**: events 13 tháng; `ai_audit` 6 tháng; hội thoại 90 ngày (người dùng xoá được); alert 12 tháng.
- Tuân thủ pháp luật VN về dữ liệu cá nhân (Nghị định 13/2023/NĐ-CP và văn bản thay thế/ cập nhật – *cần xác minh bản hiệu lực khi triển khai thật*).

## 8. Chất lượng dữ liệu (làm sớm, rẻ, cứu cả dự án)
- `event_id` + `ReplacingMergeTree` để dedupe; test đếm `events_raw` vs Kafka offset.
- Dashboard "data health": events/phút theo type, tỉ lệ thiếu `request_id` ở click có `reco`, tỉ lệ impression:click bất thường, độ trễ Kafka→ClickHouse.
- Khớp số: doanh thu ClickHouse ≡ doanh thu Postgres (job đối soát hằng đêm, lệch > 0.5% ⇒ alert).
- Bộ **dữ liệu giả có cấu trúc** (generator) để dev AI khi chưa có traffic: sinh người dùng theo persona, hành vi theo xác suất, đơn hàng có xu hướng theo danh mục. Đặt ở `scripts/datagen/` (sẽ làm ở Phase 1).
