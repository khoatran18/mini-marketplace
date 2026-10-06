# 06 · API contract (REST gateway, gRPC nội bộ, Kafka)

Quy ước cũ giữ nguyên: `Authorization: Bearer`, lỗi `{"error":"…"}`, map mã gRPC→HTTP (`handler/helpers.go`). Ký hiệu quyền: 🌐 công khai (rate-limit riêng) · 🔑 đăng nhập · B buyer · S seller_admin/employee · SA seller_admin · A admin.
Tiền ở API mới: `{ "amount": "125000", "currency": "VND" }` (chuỗi số nguyên VND, không float).

## 1. Vận hành (mọi service, cổng :8081 – xem 02)
`GET /healthz` · `GET /readyz` · `GET /startupz`(tuỳ chọn) · `GET /metrics` · `GET /version`
Gateway public (cổng 8080): `GET /health` (giữ nguyên) và `GET /version`.
Frontend: `GET /api/health`, `GET /api/ready`.

## 2. Gateway REST mới / thay đổi

### 2.1 Tài khoản & quản trị người dùng
| API | Quyền | Ghi chú |
|---|---|---|
| POST `/auth/login` | 🌐 | thêm `role:"admin"` được phép; khoá tạm khi sai nhiều lần (429/423) |
| GET `/auth/me` | 🔑 | `{user_id, username, role, store_id}` (nay UI giải mã JWT phía client) |
| POST `/auth/logout` | 🔑 | thu hồi refresh (deny-list) – roadmap #8 |
| GET `/admin/users?role=&q=&page=` · GET `/admin/users/:id` | A | |
| POST `/admin/users/:id/lock` · `/unlock` | A | ghi `audit_log` |
| GET `/admin/audit-log` | A | lọc actor/action/time |
| GET/POST/PUT `/admin/stores` · `/admin/stores/:id/suspend` | A | duyệt/ khoá shop |
| GET `/admin/orders` · `/admin/orders/:id` | A | xem mọi đơn (hỗ trợ CSKH) |
| PUT `/users/me/consent` · DELETE `/users/me/data` | 🔑 | quyền riêng tư |
| GET/POST/PUT/DELETE `/users/me/addresses[/:id]` | B | địa chỉ giao |

### 2.2 Catalog & tìm kiếm (từ khoá, không AI)
| API | Quyền | Ghi chú |
|---|---|---|
| GET `/categories` · `/categories/:slug` | 🌐 | cây danh mục |
| POST/PUT/DELETE `/admin/categories[/:id]` | A | |
| GET `/search` | 🌐 | `q`(FTS Postgres `tsvector` tiếng Việt không dấu + unaccent), `category_id`, `price_min/max`, `in_stock`, `brand`, `seller_id`, `sort=relevance|price_asc|price_desc|newest|best_selling`, `page`, `page_size≤50` |
| GET `/search/suggest?q=` | 🌐 | gợi ý gõ (prefix) |
| GET `/products`, `/products/:id`, `/products/seller/:id` | 🌐 *(đổi từ 🔑)* | trả trường mới; chỉ `status=active` với khách; chủ shop thấy mọi trạng thái của mình |
| POST `/products`, PUT `/products/:id` | S | body mở rộng (03 §1); **PATCH** `/products/:id/status` đổi trạng thái |
| POST `/products/:id/images` · DELETE `/products/:id/images/:key` | S | multipart ≤ 5 MB, jpeg/png/webp, kiểm MIME thật |
| GET `/products/:id/stock` | 🌐 → `{level: none|low|ok}`; S (đúng shop) → số chính xác | |
| POST `/seller/products/:id/inventory/adjust` | S | `{delta, reason}` ghi ledger |
| GET `/seller/products/:id/inventory/ledger` | S | |
| GET `/products/:id/reviews` | 🌐 · POST `/products/:id/reviews` B (đã nhận hàng) · DELETE (admin) | |
| POST/DELETE `/wishlist/:product_id` · GET `/wishlist` | B | |
| GET `/products/trending?category_id=` | 🌐 | **thống kê** (bán chạy 7 ngày từ analytics) – không phải gợi ý cá nhân |

### 2.3 Giỏ hàng & đơn hàng
| API | Quyền | Ghi chú |
|---|---|---|
| GET `/cart` · PUT `/cart/items/:product_id {quantity}` · DELETE `/cart/items/:product_id` · DELETE `/cart` | B | |
| POST `/cart/merge` | B | gộp giỏ localStorage khi đăng nhập |
| POST `/checkout/preview` | B | server tính subtotal, ship, giảm, tổng, cảnh báo hết hàng (không tạo đơn) |
| POST `/orders` | B | header `Idempotency-Key` (bắt buộc); body `{items?|from_cart:true, address_id, payment_method, coupon_code?, note?}` → `{checkout_id, orders:[…], payment?:{payment_id, status, pay_url}}`. Server tính giá/ship/giảm, snapshot địa chỉ; bỏ qua mọi giá/status client gửi |
| GET `/orders?status=&page=` · GET `/orders/:id` | B | + `payment`, `history[]`, `tracking` |
| POST `/orders/:id/cancel {reason}` | B | trước SHIPPED; thay `DELETE /orders/:id` (giữ DELETE như alias tương thích tạm) |
| POST `/orders/:id/confirm-received` | B | SHIPPED→DELIVERED |
| POST `/orders/:id/return {reason, items?}` | B | trong cửa sổ đổi trả |
| GET `/seller/orders?status=&page=` · GET `/seller/orders/:id` | S (chỉ đơn của shop) | hộp thư đơn |
| POST `/seller/orders/:id/confirm` · `/ship {carrier, tracking_code}` · `/deliver` · `/cancel {reason}` | S | theo bảng 04 §1 |
| POST `/seller/orders/:id/return/approve` · `/reject` | S | |
| GET `/orders/:id/history` | B/S/A (quyền theo đơn) | |

### 2.4 Thanh toán (mô phỏng)
| API | Quyền | Ghi chú |
|---|---|---|
| GET `/payments/:id` | B (chủ đơn) | trạng thái, `method`, `next_action` |
| POST `/payments/:id/confirm` | B | body theo method: `{card:{number,exp,cvc}}` / `{otp}` / `{approve:true}` – **số thẻ chỉ dùng để chọn kịch bản mô phỏng, không lưu** |
| POST `/payments/:id/cancel` | B | |
| POST `/payments/:id/refund` | A / S (theo luồng return) | `{amount?, reason}` |
| GET `/admin/payments?status=&method=&from=&to=` · GET `/admin/payments/:id` | A | |
| POST `/admin/dev/payments/:id/force {status}` | A (**chỉ `ENV=dev`**) | |
| POST `/internal/payments/webhook` | nội bộ (HMAC) | không route qua Traefik |
| GET `/payments/methods` | 🌐 | danh sách method đang bật (+ cờ MÔ PHỎNG) |

### 2.5 Tracking, cấu hình
| API | Quyền | Ghi chú |
|---|---|---|
| POST `/events` | 🌐 (JWT tuỳ chọn) | 202; `{accepted, rejected:[{index,reason}]}` |
| POST `/events/identify` | 🔑 | |
| GET `/config/features` | 🌐 | `{tracking:true, payments_mock:true, …}` |

### 2.6 Analytics (seller)
Tất cả `S`, ép `store_id` từ JWT:
`GET /seller/analytics/summary?period=today|7d|30d|mtd|custom&from=&to=&compare=prev` · `/timeseries?metric=revenue|orders|units|aov&granularity=hour|day|week` · `/top-products?by=revenue|units|views|conversion&k=` · `/low-stock` · `/stockout-forecast` · `/products/:id/funnel` · `/payments-breakdown` · `/export.csv?report=orders|products`.

### 2.7 Analytics & hệ thống (admin)
`GET /admin/analytics/summary` · `/timeseries` · `/traffic` · `/funnel` · `/top-stores` · `/top-categories` · `/payments` · `/search-terms` · `/data-health`
`GET /admin/system/health` · `/admin/system/metrics?q=&service=&range=&step=` · `/admin/system/outbox` · `/admin/system/kafka` (lag theo group) · `POST /admin/system/outbox/requeue`
Phản hồi chung: `{ "as_of": "...", "tz":"Asia/Ho_Chi_Minh", "source":"clickhouse|postgres|prometheus", "data": … }`.

### 2.8 Cảnh báo
| API | Quyền | |
|---|---|---|
| GET `/alerts?status=&severity=&since=&page=` | S (shop mình) / A (tất cả) | |
| GET `/alerts/:id` | như trên | + `evidence`, `runbook_url` |
| POST `/alerts/:id/ack` · `/resolve` · `/silence {until, reason}` | S/A | |
| GET `/alerts/stream` | S/A | SSE |
| GET `/notifications` · POST `/notifications/:id/read` · `/read-all` | 🔑 | chuông |
| GET/PUT `/notification-prefs` | 🔑 | |
| GET/POST/PUT `/admin/alert-rules[/:id]` · POST `/admin/alert-rules/:id/test` · `/enable` · `/disable` | A | |
| POST `/internal/alerts/prometheus` | nội bộ (secret) | webhook Alertmanager |

### 2.9 Coupons (P2.5, tuỳ chọn)
`POST/GET/PUT /seller/coupons` (S) · `POST /admin/coupons` (A) · `POST /checkout/preview` áp mã.

## 3. gRPC nội bộ (đề xuất proto mới)
| Service | RPC chính |
|---|---|
| `payment.PaymentService` :50057 | `CreatePayment`, `GetPayment`, `ConfirmPayment`, `CancelPayment`, `Refund`, `ListPayments`, `HandleWebhook` + `grpc.health.v1` |
| `analytics.AnalyticsService` :50055 | `Summary`, `TimeSeries`, `TopN`, `Funnel`, `Traffic`, `ProductStats`, `StockoutForecast`, `PaymentsBreakdown`, `DataHealth` (nhận `Scope{role,store_id}`) |
| `alert.AlertService` :50056 | `ListAlerts`, `GetAlert`, `Ack`, `Resolve`, `Silence`, `UpsertRule`, `ListRules`, `TestRule`, `IngestExternal`, `StreamNotifications` |
| `order.OrderService` mở rộng | `GetCart/SetCartItem/MergeCart`, `PreviewCheckout`, `Transition(order_id, to, actor, reason)`, `ListOrdersByStore`, `GetHistory`, `RequestReturn` |
| `product.ProductService` mở rộng | `Search`, `ListCategories`, `AdjustInventory`, `GetLedger`, `SetStatus`, `CreateReview`, `Wishlist*`, `GetStockLevel` |
| `auth.AuthService` mở rộng | `GetMe`, `ListUsers`, `LockUser`, `Unlock`, `Logout` |
Mọi gRPC service: `grpc.health.v1`, interceptor metrics, deadline từ gateway (mặc định 5 s). Sinh code bằng `buf` như hiện tại (không sửa `.pb.go` tay).

## 4. Kafka topic mới / đổi
| Topic | Producer → Consumer (group) | Key | Payload (có trường `v:1`) |
|---|---|---|---|
| `tracking.events` | gateway → analytics (`analytics-ingest`) | anonymous_id / user_id | envelope 03 §2.1 |
| `order.status_changed` | order (outbox) → analytics, alert, payment? | order id | `{order_id, checkout_id, buyer_id, store_id, status, prev, payment_method, totals, at, items:[{item_id, product_id, category_id, qty, unit_price}]}` |
| `order.create_order` / `order.cancel_order` | (giữ) | order id | thêm `reason` khi cancel/expire |
| `product.validate_order` | (giữ) | order id | thêm `reserved:true` |
| `payment.requested` | order → payment *(thay vì gRPC nếu muốn async)* | order id | `{order_id, amount, method}` |
| `payment.succeeded` / `payment.failed` / `payment.refunded` | payment (outbox) → order, analytics, alert | order id | `{payment_id, order_id, method, amount, failure_code?, at, provider_event_id}` |
| `product.changed` | product (outbox) → analytics | product id | `{product_id, op, store_id, name, category_id, price, status, updated_at}` |
| `inventory.changed` | product (outbox) → analytics, alert | product id | `{product_id, store_id, delta, available, level, reason, ref}` |
| `cart.updated` | order (outbox) → analytics | user id | `{user_id, items:[{product_id,qty}], at}` |
| `alert.created` / `alert.updated` | alert → notifier (nội bộ alert-service) | alert id | alert JSON |
| `*.dlq` | mọi consumer mới | giữ key gốc | `{original_topic, error, payload, attempts}` |
Quy tắc cũ giữ nguyên (outbox, `acks=all`, commit sau xử lý, idempotent). Tăng retention `tracking.events` 7 ngày; `EnsureTopicExist` tạo các topic mới, 3 partition.
