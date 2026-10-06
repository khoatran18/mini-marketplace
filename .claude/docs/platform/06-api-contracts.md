# 06 · API contract (REST gateway, gRPC nội bộ, Kafka)

> **Trạng thái: ✅ phần lớn ĐÃ TRIỂN KHAI.** Tài liệu tham chiếu đầy đủ, theo code, của REST là **[../api.md](../api.md)** (method, path, role, request/response, lỗi, idempotency, phân trang, thẻ test). Tệp này là bản đồ "thiết kế → thực tế": cái gì đã có, đã đổi, hoặc chưa làm. Danh sách Kafka đầy đủ: [../events-kafka.md](../events-kafka.md).

Quy ước giữ nguyên: `Authorization: Bearer`, lỗi `{"error":"…"}`, ánh xạ gRPC→HTTP ở `handler/helpers.go`. Ký hiệu quyền trong [api.md](../api.md): 🌐 công khai · 👁 token tuỳ chọn · 🔑 đăng nhập · B buyer · S seller · SA seller_admin · A admin.
**Tiền**: khác thiết kế ("chuỗi số nguyên `{amount,currency}`"): REST/gRPC trả **VND dạng số thực** (`price`, `total_price`, `amount`…) và payment có thêm `amount_minor`/`refunded_minor` (số nguyên 1/100 VND); event Kafka dùng `*_minor` (ADR-7/18).

## 1. Vận hành
| Thiết kế | Thực tế |
|---|---|
| `GET /health /ready /metrics /version` trên :8081 mọi service | ✅ (+ `/healthz`, `/readyz`) |
| Gateway công khai: `GET /health` và `GET /version` | 🟡 công khai chỉ `GET /health` + `/healthz`; `/version` chỉ ở cổng admin :8081 |
| Frontend `/api/health`, `/api/ready` | ✅ (+ `healthz`, `readyz`) |

## 2. Gateway REST – đối chiếu
### 2.1 Tài khoản & quản trị người dùng
| Thiết kế | Trạng thái |
|---|---|
| `POST /auth/login` nhận `role:"admin"` | ✅; khoá tạm khi sai nhiều lần ❌ |
| `GET /auth/me`, `POST /auth/logout` | ❌ (UI giải mã JWT phía client) |
| `/admin/users*`, `lock/unlock`, `/admin/audit-log`, `/admin/stores*` | ❌ |
| `GET /admin/orders`, `/admin/orders/:id` | ✅ (+ `cancel`, `deliver`, `return/approve`, `return/reject`) |
| `PUT /users/me/consent`, `DELETE /users/me/data` | ❌ |
| `GET/POST/PUT/DELETE /users/me/addresses[/:id]` | ✅ (buyer, ≤ 10) |

### 2.2 Catalog & tìm kiếm
| Thiết kế | Trạng thái |
|---|---|
| `GET /categories`, `/categories/:slug` | 🟡 `GET /categories` (cây phẳng); **không có** `/categories/:slug` |
| `POST/PUT/DELETE /admin/categories[/:id]` | 🟡 `GET/POST/PUT` ✅; **không có DELETE** (tắt bằng `active=false`) |
| `GET /search` (q, category_id, price_min/max, in_stock, brand, seller_id, sort, page, page_size≤50) | ✅ (👁 – chủ store xem được `draft/hidden` của chính store) |
| `GET /search/suggest` | ❌ |
| `GET /products`, `/products/:id`, `/products/seller/:id` công khai | ✅ 👁 (khách chỉ `active`) |
| `POST /products`, `PUT /products/:id`, `PATCH /products/:id/status` | ✅ (S); `PATCH` không đặt được `banned` – admin dùng `PATCH /admin/products/:id/status` |
| `POST/DELETE /products/:id/images` (MinIO) | ❌ (ảnh = URL trong body) |
| `GET /products/:id/stock` → `{level}` | ✅ (số chính xác cho chủ store qua `/products/:id`/`/seller/products`) |
| `POST /seller/products/:id/inventory/adjust`, `GET …/inventory/ledger` | ✅ |
| (thêm) `GET /seller/products`, `GET /seller/inventory/low-stock` | ✅ |
| Reviews, wishlist, `/products/trending` | ❌ |

### 2.3 Giỏ hàng & đơn hàng
| Thiết kế | Trạng thái |
|---|---|
| `GET /cart`, `PUT/DELETE /cart/items/:product_id`, `DELETE /cart`, `POST /cart/merge` | ✅ (B) |
| `POST /checkout/preview` | ✅ (`{from_cart | items}` → nhóm theo store, `can_order`) |
| `POST /orders` (header `Idempotency-Key` bắt buộc; `{items?|from_cart, address_id, payment_method, note}`) → `{checkout_id, orders:[…], payment?}` | ✅ **201** mới / **200** replay; `payment` thường chưa có ngay (tạo bất đồng bộ); thêm `GET /checkouts/:id` |
| `GET /orders?status=&page=`, `GET /orders/:id` (+ `payment`, `history[]`) | 🟡 `history[]` ✅ khi đọc 1 đơn; `payment` nằm ở checkout, không ở đơn; thêm `GET /orders/summary` |
| `POST /orders/:id/cancel`, `DELETE /orders/:id` (alias) | ✅ |
| `POST /orders/:id/confirm-received`, `POST /orders/:id/return` | ✅ |
| `GET /seller/orders[/:id]`, `POST /seller/orders/:id/{ship,deliver,cancel}`, `…/return/{approve,reject}` | ✅ (+ `GET /seller/orders/summary`) |
| `POST /seller/orders/:id/confirm` | ❌ không có (đơn `PAID/CONFIRMED` đi thẳng sang `ship`) |
| `GET /orders/:id/history` | ❌ riêng (nằm trong `GET /orders/:id`) |

### 2.4 Thanh toán (mô phỏng)
| Thiết kế | Trạng thái |
|---|---|
| `GET /payments/:id`, `POST /payments/:id/confirm` (`{card|otp|approve}`), `POST /payments/:id/cancel` | ✅ (body phẳng `{card_number, card_exp, card_cvc, otp, approve}`) |
| `POST /payments/:id/refund` (A/S) | 🟡 chỉ admin: `POST /admin/payments/:id/refund {amount_minor, reason}`; hoàn tiền theo luồng đơn là tự động (event) |
| `GET /admin/payments`, `GET /admin/payments/:id` | ✅ |
| `POST /admin/dev/payments/:id/force` | ✅ chỉ `ENV=dev` |
| `POST /internal/payments/webhook` | ✅ ở cổng admin :8081 của payment-service (không qua gateway), ký HMAC |
| `GET /payments/methods` | ✅ (kèm danh sách thẻ test) |

### 2.5 Tracking, cấu hình
| Thiết kế | Trạng thái |
|---|---|
| `POST /events` → 202 `{accepted, rejected}` | ✅ (+ `reasons[]`; analytics down ⇒ 202 `{accepted:0,rejected:0,dropped:N}`) |
| `POST /events/identify` | ✅ |
| `GET /config/features` | ❌ |

### 2.6 Analytics (seller) – 🟡
Đã có: `GET /seller/analytics/{summary,timeseries,top-products,funnel,low-stock}`. Khác thiết kế: tham số `from`, `to`, `granularity`, `sort`, `limit` (không có `period`/`compare`/`metric`/`k`); `summary` luôn kèm kỳ trước (`previous`). Chưa có: `stockout-forecast`, `products/:id/funnel`, `payments-breakdown`, `export.csv`.

### 2.7 Analytics & hệ thống (admin) – 🟡
Đã có: `GET /admin/analytics/{summary,timeseries,top-products,funnel,low-stock,traffic,payments,search-terms,data-health}`, `GET /admin/system/health`. Chưa có: `top-stores`, `top-categories`, `/admin/system/metrics`, `/admin/system/outbox`, `/admin/system/kafka`, `POST /admin/system/outbox/requeue`.
Phản hồi báo cáo: `{as_of, source:"clickhouse", cached, timezone:"Asia/Ho_Chi_Minh", data}` (khác thiết kế: có `cached`, không có `source` Prometheus/Postgres).

## 3. gRPC nội bộ (thực tế)
| Service | RPC |
|---|---|
| `auth` :50051 | `Login`, `Register`, `RefreshToken`, `ChangePassword`, `RegisterSellerRoles`, `GetStoreIDRoleById` (**không có** `GetMe/ListUsers/LockUser/Logout`) |
| `user` :50054 | buyer/seller CRUD + `UpsertAddress`, `ListAddresses`, `GetAddress`, `DeleteAddress` |
| `product` :50053 | `CreateProduct`, `UpdateProduct`, `GetProductByID`, `GetProductsByID`, `GetProductsBySellerID`, `GetInventoryByID`, `GetAndDecreaseInventoryByID` (cũ), `GetProducts`, `SearchProducts`, `ListCategories`, `UpsertCategory`, `SetProductStatus`, `AdjustInventory`, `GetInventoryLedger`, `ListLowStock` |
| `order` :50052 | `Checkout`, `PreviewCheckout`, `GetCheckout`, `GetOrder`, `ListOrders`, `CountOrders`, `ApplyAction`, `GetCart`, `SetCartItem`, `ClearCart`, `MergeCart` (**không có** `Transition`/`RequestReturn` tách riêng: tất cả action qua `ApplyAction`) |
| `payment` :50057 | `GetPayment`, `GetPaymentByCheckout`, `ConfirmPayment`, `CancelPayment`, `ListPayments`, `RefundPayment`, `ForceStatus` (không có `CreatePayment`/`HandleWebhook` qua gRPC: payment tạo bằng Kafka, webhook là HTTP nội bộ) |
| `analytics` :50055 | `IngestEvents` (≤ 50 `TrackEvent`), `Query` (một RPC chung: `report`, `scope{role,store_id}`, `from`, `to`, `granularity`, `sort`, `limit`) |
Mọi service gRPC có `grpc.health.v1` + interceptor metrics `mm_grpc_server_*`. Deadline từ gateway: 2 s (tracking) – 10 s (product, user, payment, catalog) – 15 s (order) – 20 s (analytics); chưa có deadline chuẩn 5 s, chưa retry. Sinh code bằng `buf` / `scripts/protogen/gen.sh` (không sửa `.pb.go` tay).

## 4. Kafka (thực tế)
Danh sách, producer/consumer/group, payload: **[../events-kafka.md](../events-kafka.md)**. Khác thiết kế:
| Thiết kế | Thực tế |
|---|---|
| `tracking.events` (gateway → analytics) | ❌ **không có**: gateway gọi gRPC `IngestEvents` (ADR-13) |
| `order.status_changed` có `totals`, `items[]` | ✅ dạng phẳng `subtotal_minor, shipping_fee_minor, total_minor`, `items[{item_id, product_id, category_id, quantity, unit_price_minor}]`, `prev`, `actor_type`, `payment_status` |
| `order.create_order` / `order.cancel_order` thêm `reason` | ❌ giữ nguyên `{order_id, items}` |
| `product.validate_order` thêm `reserved:true` | ❌ giữ nguyên `{order_id, success}` |
| `payment.requested` order → payment | ✅ `{v, checkout_id, buyer_id, method, amount_minor, expires_at, order_ids}` (theo **checkout**) |
| `payment.succeeded/failed/refunded` | ✅ theo `payment_id`/`checkout_id` (không có `order_id` ở succeeded/failed); `payment.failed` phát mỗi lần thử, có `terminal`; thêm `order.refund_requested` order → payment |
| `product.changed`, `inventory.changed` | ✅ (thêm `reserved`, `ref_type`, `ref_id`) |
| `cart.updated` | ❌ |
| `*.dlq` | ❌ (quá 5 lần lỗi thì ghi log và bỏ qua) |
Quy tắc cũ giữ nguyên (outbox, `acks=all`, commit sau xử lý, idempotent). Topic do service tự `EnsureTopicExist` (3 partition, RF 1); retention Kafka 168 h.
