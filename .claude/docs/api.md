# REST API (api-gateway)

> Nguồn sự thật: `services/api-gateway/internal/router/router.go` (route + role), `internal/handler/*.go` (JSON). `services/api-gateway/docs/swagger.*` **đã cũ** (chỉ có auth/orders cũ/products/users, xem [roadmap.md](roadmap.md)); tài liệu này là bản đầy đủ.

## 0. Quy ước chung
| Mục | Giá trị |
|---|---|
| Base URL | `https://api.<PUBLIC_HOST>` (dev: `http://localhost:8080`) |
| Xác thực | `Authorization: Bearer <access_token>` (chỉ nhận `Type=="access"`) |
| Lỗi | `{"error": "message"}` |
| Content-Type | `application/json`; body tối đa `MAX_BODY_BYTES` (mặc định 1 MiB) → **413** |
| CORS | `ALLOWED_ORIGINS`; header cho phép `Origin, Content-Type, Authorization, Idempotency-Key`; method `GET POST PUT PATCH DELETE OPTIONS` |
| Rate limit (Redis, theo IP, fail-open) | toàn cục `RATE_LIMIT_PER_MINUTE` (300) · `/auth/*` `AUTH_RATE_LIMIT_PER_MINUTE` (20) · `/events*` `EVENTS_RATE_LIMIT_PER_MINUTE` (120) → **429** `{"error":"Too many requests"}` |
| JSON từ gRPC | các route cart/checkout/orders/catalog/seller/payments trả **tên trường proto (snake_case)**, có cả giá trị 0/rỗng, số nguyên 64-bit là số JSON, thời gian là chuỗi RFC 3339 (UTC) hoặc `""` |
| Tiền | **VND dạng số thực** (`price`, `unit_price`, `line_total`, `subtotal`, `shipping_fee`, `total_price`, `grand_total`, `amount`…). Chỉ payment có thêm đơn vị nhỏ `amount_minor`/`refunded_minor` (1/100 VND, số nguyên – ADR-7). Server tính bằng cent nguyên, không bao giờ tin giá client |
| Nhận diện | luôn từ JWT; `buyer_id`, `store_id`, `status`, giá, tổng trong body bị bỏ qua (store của seller lấy qua auth-service `GetStoreIDRoleById`) |
| Idempotency | `POST /orders` bắt buộc header `Idempotency-Key`; webhook thanh toán idempotent theo `provider_event_id` (nội bộ) |

Ký hiệu cột **Ai**: 🌐 công khai · 👁 token tuỳ chọn (có token nhưng sai/hết hạn → **401**, không coi là ẩn danh) · 🔑 mọi user đăng nhập · **B** buyer · **S** `seller_admin` hoặc `seller_employee` (phải đã có store, nếu không 403 `Create a store first`) · **SA** chỉ `seller_admin` · **A** `admin`.

### Ánh xạ lỗi (`handler/helpers.go`)
`InvalidArgument`/lỗi protovalidate→400 · `Unauthenticated`→401 · `PermissionDenied`→403 · `NotFound`→404 · `AlreadyExists`/`Aborted`→409 · `FailedPrecondition`→**422** · `Unavailable`/`DeadlineExceeded`→503 · còn lại 500. Middleware: thiếu/sai token → 401, sai role → 403 (`Do not have permission to access`).

### Phân trang
| Endpoint | `page` | `page_size` mặc định | Giới hạn |
|---|---|---|---|
| `GET /products` | 1 | 10 | >100 → 100 |
| `GET /search`, `GET /seller/products` | 1 | 20 | <1 hoặc >50 → 50 |
| `GET /orders`, `/seller/orders`, `/admin/orders`, `/admin/payments` | 1 | 20 | <1 hoặc >100 → **20** |
| `GET /seller/products/:id/inventory/ledger` | 1 | 50 | >100 → 50 |
| `GET /payments/…`, báo cáo analytics | – | – | analytics dùng `limit` (xem §8) |

---
## 1. Vận hành
| Method & path | Ai | Ghi chú |
|---|---|---|
| GET `/health`, `/healthz` | 🌐 | liveness của gateway `{"status":"ok"}` |
| GET `/admin/system/health` | A | gom `/ready` của mọi service (`SYSTEM_HEALTH_TARGETS`, mặc định 7 service): `{as_of, status, services:[{name,status,latency_ms,version,uptime_s,checks}]}`; `status` = tệ nhất của `ready<degraded<not_ready<unreachable`. Mỗi probe timeout 2 s |
| GET `/ready`, `/readyz`, `/metrics`, `/version` | nội bộ `:8081` | **không** nằm trên cổng công khai 8080 (xem [platform/02](platform/02-health-ready-metrics.md)) |

## 2. Xác thực & tài khoản
Username/password phải khớp `^[a-zA-Z0-9_]{3,16}$` (hợp đồng `auth.proto`); gateway thêm: mật khẩu **≥ 8** ký tự ⇒ thực tế 8–16.
| Method & path | Ai | Request | Response / lỗi |
|---|---|---|---|
| POST `/auth/register` | 🌐 | `{username,password,role}` – `role` ∈ `buyer`, `seller_admin` | 200 `{message,success}`; 400 sai định dạng hoặc role `admin`; 403 role `seller_employee`; 409 đã tồn tại (cùng username+role). `accounts.username` UNIQUE toàn cục nên trùng username khác role sẽ là lỗi DB (500) |
| POST `/auth/login` | 🌐 | `{username,password,role}` – role ∈ `buyer, seller_admin, seller_employee, admin` | 200 `{message,success,access_token,refresh_token}`; 401 `username or password is incorrect` (thông báo thống nhất) |
| POST `/auth/refresh-token` | 🌐 | `{refresh_token}` | 200 token mới; 401 nếu là access token / hết hạn / đã đổi mật khẩu |
| POST `/auth/change-password` | 🔑 | `{old_password,new_password}` (tài khoản lấy từ token; mới ≥ 8) | 200; token cũ bị vô hiệu qua Redis `<userId>:pwd_version` |
| POST `/auth/register-seller-roles` | SA | `{username,password}` | 200; tạo `seller_employee` trong store của admin; 403 nếu chưa có store |
Token: access `JWT_EXPIRE_TIME` phút (mặc định 5), refresh ×2; claims `UserID, Username, Role, PwdVersion, Type`. Role `admin` chỉ tạo bằng `ADMIN_BOOTSTRAP_*` (không đăng ký được).
**Chưa làm**: `GET /auth/me`, `POST /auth/logout`, khoá tạm đăng nhập, `audit_log`, `/admin/users*`, `/admin/stores*`.

## 3. Hồ sơ buyer / store (user-service)
| Method & path | Ai | Ghi chú |
|---|---|---|
| POST `/users/buyers` | B | body `{buyer:{name,gender,date_of_birth,phone,address}}`; luôn tạo cho chính người gọi |
| GET/PUT/DELETE `/users/buyers/:id` | B | `:id` phải bằng user id của token (403) |
| POST `/users/sellers` | SA | body `{seller:{name,bank_account,tax_code,description,date_of_birth,phone,address}}`; gắn store mới vào tài khoản (phát `user.create_seller` → auth-service gán `store_id`) |
| GET `/users/sellers/:id` | 🔑 | chủ store thấy đủ; người khác chỉ `id,name,description` |
| PUT/DELETE `/users/sellers/:id` | SA | chỉ store của chính mình (403) |
Response chung `{message,success}` (+ `buyer`/`seller`).

### Địa chỉ giao hàng (buyer, tối đa 10)
| Method & path | Ai | Ghi chú |
|---|---|---|
| GET `/users/me/addresses` | B | `{addresses:[…]}`, địa chỉ mặc định trước |
| POST `/users/me/addresses` | B | **201** `{message,success,address}` |
| PUT `/users/me/addresses/:id` | B | thay toàn bộ; 404 nếu không phải của mình |
| DELETE `/users/me/addresses/:id` | B | nếu xoá địa chỉ mặc định, địa chỉ cũ nhất còn lại thành mặc định |
Body: `{label≤50, receiver_name 1-100, phone (^\+?[0-9][0-9 .\-]{7,14}$), line1 1-200, ward≤100, district≤100, city 1-100, is_default}`. Địa chỉ đầu tiên luôn là mặc định; chỉ có một mặc định. Quá 10 → 422. Địa chỉ được **sao chụp (snapshot)** vào đơn lúc checkout.

---
## 4. Catalog & tìm kiếm
### 4.1 Đọc (công khai)
| Method & path | Ai | Ghi chú |
|---|---|---|
| GET `/search` | 👁 | tham số: `q` (không dấu, khớp tiền tố, ≤200 ký tự), `category_id` (gồm danh mục con), `price_min`, `price_max`, `brand`, `seller_id`, `in_stock=true|1`, `sort=relevance|price_asc|price_desc|newest|best_selling`, `page`, `page_size`. → `{message,success,products[],total}`. Khách chỉ thấy `status=active`; chủ store xem được `draft/hidden` của **chính store** khi lọc `seller_id` của mình (admin: của bất kỳ store) |
| GET `/categories` | 🌐 | `{categories:[{id,parent_id,name,slug,sort,active,product_count}]}` danh sách phẳng (dựng cây theo `parent_id`), chỉ `active`; `Cache-Control: public, max-age=30`. `product_count` = sản phẩm active của danh mục + con |
| GET `/products` | 👁 | danh sách phân trang, **luôn chỉ `active`** → `{message,success,products[]}` |
| GET `/products/:id` | 👁 | `{message,success,product}`; sản phẩm không `active` → **404** với người không phải chủ/admin |
| GET `/products/seller/:seller_id` | 👁 | chủ store thấy mọi trạng thái, người khác chỉ `active` |
| GET `/products/:id/stock` | 🌐 | `{product_id, level}` với `level` ∈ `none|low|ok`; 404 nếu không `active` |
**Che số tồn** với người không quản lý sản phẩm: `inventory` bị chặn ở **20** (hiển thị "20+"), `reserved`, `low_stock_threshold`, `version` = 0; `stock_level` giữ nguyên. Lưu ý `GET /products/:id`, `/products`, `/products/seller/:id` dùng DTO không có `category_name` (chỉ `/search` & `/seller/products` có).

Product JSON: `id, name, price, seller_id, inventory (= còn bán được), attributes{}, description, category_id, category_name, brand, tags[], image_urls[], status, sku, low_stock_threshold, weight_g, reserved, sold, version, created_at, updated_at, stock_level`.
`status` ∈ `draft | active | hidden | banned` (hết hàng **không** phải status: `stock_level` suy ra từ tồn: `none` nếu 0, `low` nếu ≤ ngưỡng, còn lại `ok`; ngưỡng = `low_stock_threshold` của sản phẩm hoặc 5).

### 4.2 Ghi (seller)
| Method & path | Ai | Ghi chú |
|---|---|---|
| POST `/products` | S | 200 `{message,success,id}`. Body: `name`(1-200), `price`>0, `inventory`≥0, `attributes{}`, `description`≤5000, `category_id` (phải tồn tại & active), `brand`≤100, `tags`≤20 (mỗi cái ≤40), `image_urls`≤8 (http/https hoặc `/path`, ≤500), `status` ∈ draft/active/hidden (mặc định `active`; **không** được `banned`), `sku`≤64 (duy nhất trong store → 409), `low_stock_threshold`≥0, `weight_g`≥0. `seller_id` luôn là store của token |
| PUT `/products/:id` | S | body `{product:{…các trường trên…}}` thay toàn bộ trường sửa được (`status` rỗng = giữ nguyên; sản phẩm `banned` giữ `banned`). Đổi `inventory` ghi ledger lý do `update`. 403 nếu không phải chủ; 404 nếu không có |
| PATCH `/products/:id/status` | S | `{status}` ∈ `draft|active|hidden`; đặt/gỡ `banned` → 403 |
Mỗi thay đổi phát `product.changed`/`inventory.changed` qua outbox ([events-kafka.md](events-kafka.md)).
**Chưa làm**: upload ảnh (MinIO) – ảnh chỉ là URL; reviews; wishlist; `/products/trending`; `/search/suggest`.

---
## 5. Giỏ hàng & checkout (buyer)
| Method & path | Ai | Request | Response |
|---|---|---|---|
| GET `/cart` | B | – | `{lines:[{product_id,quantity,name,image_url,store_id,unit_price,line_total,stock_level,available,issue}], subtotal, item_count}` – giá/tồn lấy **sống** từ catalog; `subtotal` chỉ cộng dòng `available` |
| PUT `/cart/items/:product_id` | B | `{quantity}` 0–99 (0 = xoá) | giỏ mới; 422 nếu sản phẩm không bán được / thiếu hàng (`issue`: `product not found`, `product is not available`, `out of stock`, `only N left`) hoặc giỏ đầy (**50** sản phẩm) |
| DELETE `/cart/items/:product_id` | B | – | giỏ mới |
| DELETE `/cart` | B | – | giỏ rỗng |
| POST `/cart/merge` | B | `{items:[{product_id,quantity}]}` (giỏ localStorage sau khi đăng nhập) | giỏ mới; bỏ qua sản phẩm không active; cộng dồn, trần 99/dòng |
| POST `/checkout/preview` | B | `{from_cart:true}` hoặc `{items:[{product_id,quantity}]}` | `{groups:[{store_id,lines[],subtotal,shipping_fee,total}], subtotal, shipping_fee, grand_total, can_order}`; `can_order=false` nếu có dòng `issue`; không tạo gì |
| POST `/orders` | B | header `Idempotency-Key` + body bên dưới | **201** (mới) / **200** (`replayed:true`) `{checkout_id, orders:[Order…], grand_total, payment_method, replayed, payment?}` |
| GET `/checkouts/:id` | B | – | cùng dạng; 404 nếu không phải của mình |

**Body `POST /orders`**: `{"from_cart":true}` hoặc `{"items":[{"product_id":12,"quantity":2}]}`; `address_id` (0 = địa chỉ mặc định), `payment_method` ∈ `COD | MOCK_CARD | MOCK_WALLET | MOCK_BANK_TRANSFER`, `note`≤500.
```bash
curl -X POST $API/orders -H "Authorization: Bearer $T" -H "Idempotency-Key: 7c1f-order-001" \
  -d '{"from_cart":true,"address_id":0,"payment_method":"MOCK_CARD","note":"Giao giờ hành chính"}'
```
Quy tắc: `Idempotency-Key` 1–64 ký tự `[A-Za-z0-9_-]`, duy nhất theo (buyer, key) → gọi lại trả đúng checkout cũ (200, `replayed:true`) mà không tạo thêm. Đơn được **tách theo store** (mỗi store một đơn, chung `checkout_id`); ≤ 50 sản phẩm khác nhau, mỗi dòng ≤ 100000; dòng trùng được gộp; `from_cart` chỉ xoá khỏi giỏ những dòng đã đặt. Phí ship mỗi đơn-store: `SHIPPING_FLAT_FEE` (30000 VND), miễn phí khi `subtotal ≥ FREE_SHIP_OVER` (500000). Lỗi: 400 thiếu/sai `Idempotency-Key`, giỏ/`items` rỗng, method không hỗ trợ, địa chỉ thiếu trường; **404** không có địa chỉ (`address not found`); **422** có dòng không đặt được (`<tên>: out of stock`); 503 product-service không sẵn sàng.
`payment` (nếu có) do gateway ghép từ payment-service: **payment được tạo bất đồng bộ** (sau khi kho đã giữ xong, qua Kafka `payment.requested`) nên ngay sau `POST /orders` thường **chưa có** `payment` – hãy poll `GET /checkouts/:id` (hoặc đọc đơn: `status` `PENDING → AWAITING_PAYMENT`). COD không có payment.

### Order JSON
`id, checkout_id, buyer_id, store_id, status, payment_method, payment_status (UNPAID|PAID|REFUNDED), subtotal, shipping_fee, total_price, shipping_address{label,receiver_name,phone,line1,ward,district,city}, note, expires_at, created_at, updated_at, paid_at, shipped_at, delivered_at, canceled_at, cancel_reason, canceled_by, carrier, tracking_code, return_reason, items[{id,order_id,product_id,name,sku,image_url,store_id,category_id,quantity,unit_price,line_total,status ACTIVE|CANCELED}], history[{from_status,to_status,actor_type,actor_id,reason,at}]` (`history` chỉ có khi đọc **một** đơn / sau một action). Giá, tên, sku là snapshot lúc đặt.

---
## 6. Đơn hàng
### 6.1 Buyer
| Method & path | Ai | Ghi chú |
|---|---|---|
| GET `/orders?status=&page=&page_size=` | B | `{orders[],total}` mới nhất trước; `status` sai → 400; `buyer_id` trong query bị bỏ qua |
| GET `/orders/summary` | B | `{by_status:{PAID:2,…}}` |
| GET `/orders/:id` | B | đơn của người khác → **404** |
| POST `/orders/:id/cancel` · DELETE `/orders/:id` (alias cũ) | B | body tuỳ chọn `{reason}`; chỉ khi `AWAITING_PAYMENT/CONFIRMED/PAID`; 422 nếu đã SHIPPED… |
| POST `/orders/:id/confirm-received` | B | `SHIPPED→DELIVERED` |
| POST `/orders/:id/return` | B | `{reason}` bắt buộc; `DELIVERED→REFUND_REQUESTED` trong `RETURN_WINDOW_DAYS` (7) tính từ `delivered_at`, chỉ **một lần** (sau khi bị từ chối → 422) |
Không có `PUT /orders/:id` (cố ý: cho phép sửa trạng thái/giá).

### 6.2 Seller (chỉ đơn của store mình; đơn người khác → 404)
| Method & path | Ghi chú |
|---|---|
| GET `/seller/orders?status=&page=&page_size=` · GET `/seller/orders/summary` · GET `/seller/orders/:id` | hộp thư đơn |
| POST `/seller/orders/:id/ship` | `{carrier≤50, tracking_code≤64}` bắt buộc; `PAID|CONFIRMED→SHIPPED` |
| POST `/seller/orders/:id/deliver` | `SHIPPED→DELIVERED` (COD: ghi `payment_status=PAID`) |
| POST `/seller/orders/:id/cancel` | `{reason}` bắt buộc |
| POST `/seller/orders/:id/return/approve` | `REFUND_REQUESTED→REFUNDED` (+ hoàn tiền nếu thanh toán online) |
| POST `/seller/orders/:id/return/reject` | `{reason}` bắt buộc; `REFUND_REQUESTED→DELIVERED`, đóng đường trả hàng |
Không có bước "seller xác nhận" riêng: đơn đã `PAID`/`CONFIRMED` đi thẳng sang `ship`.

### 6.3 Admin
`GET /admin/orders` · `GET /admin/orders/:id` · `POST /admin/orders/:id/cancel` (`{reason}` bắt buộc) · `/deliver` · `/return/approve` · `/return/reject` (`{reason}` bắt buộc). Không có admin `/summary` và admin không `ship`.

### 6.4 Máy trạng thái đơn (`order-service/internal/repository/statemachine.go`)
```
PENDING ─▶ AWAITING_PAYMENT (online) | CONFIRMED (COD) | FAILED        [system: kết quả giữ hàng]
AWAITING_PAYMENT ─▶ PAID [payment] | EXPIRED [system] | CANCELED [buyer, seller, admin]
CONFIRMED | PAID ─▶ SHIPPED [seller] | CANCELED [buyer, seller, admin]
SHIPPED ─▶ DELIVERED [seller, buyer, admin, system sau AUTO_DELIVER_DAYS]
DELIVERED ─▶ REFUND_REQUESTED [buyer]
REFUND_REQUESTED ─▶ REFUNDED | DELIVERED(từ chối) [seller, admin]
```
Terminal: `FAILED, EXPIRED, CANCELED, REFUNDED`. Chi tiết trong [data-model.md](data-model.md) và [platform/04](platform/04-orders-payments.md). Chuyển trạng thái không hợp lệ → 422 (`… can not move an order from X to Y`); thiếu `reason`/`carrier`/`tracking_code` → 400.

---
## 7. Thanh toán (MÔ PHỎNG – không có cổng thật)
| Method & path | Ai | Ghi chú |
|---|---|---|
| GET `/payments/methods` | 🌐 | `{methods:[{code,label,online,simulated,test_cards?}]}` (danh sách cố định ở gateway; `COD` offline, 3 method còn lại `simulated:true`) |
| GET `/payments/:id` | B | chỉ payment của mình (404 nếu khác) |
| POST `/payments/:id/confirm` | B | body theo method (dưới) |
| POST `/payments/:id/cancel` | B | chỉ khi `REQUIRES_ACTION`; ngược lại 422 |
Payment JSON: `id, checkout_id, buyer_id, method, status, amount_minor, currency ("VND"), provider ("mockpay"), provider_ref, failure_code, card_last4, expires_at, paid_at, created_at, refunded_minor, next_action, order_ids[]` + trường gateway thêm: `amount` (VND), `refunded` (VND), `pay_url` (`/pay/<id>`, trang UI giả), `simulated:true`; admin còn có `attempts[]`, `refunds[]`. Một payment **phủ cả checkout** (mọi đơn đang `AWAITING_PAYMENT`).
`status`: `REQUIRES_ACTION → PROCESSING → SUCCEEDED | FAILED | CANCELED | EXPIRED`; `SUCCEEDED → PARTIALLY_REFUNDED → REFUNDED`. `next_action`: `enter_card | otp | approve_in_wallet | transfer_and_confirm | wait | none`.

**Body `confirm`**: `{card_number, card_exp:"MM/YY", card_cvc, otp, approve}`
| Method | Cách xác nhận | Kết quả |
|---|---|---|
| `MOCK_CARD` | gửi `card_number`, `card_exp` (chưa hết hạn), `card_cvc` (3–4 số); số thẻ chỉ để chọn kịch bản, **không lưu** (chỉ 4 số cuối) | theo bảng thẻ test |
| `MOCK_WALLET` | `{"approve":true}` | `PROCESSING`, sau `MOCK_WEBHOOK_DELAY_MS` (3 s) webhook → `SUCCEEDED`; `approve:false` → lỗi `user_cancelled` |
| `MOCK_BANK_TRANSFER` | `{"approve":true}` (bắt buộc, nếu không 400); `provider_ref` = `MM`+8 ký tự cuối của checkout id (nội dung chuyển khoản) | `PROCESSING`, sau `MOCK_TRANSFER_DELAY_MS` (8 s) → `SUCCEEDED` |

### Thẻ test (cố định, dùng cho e2e)
| Số thẻ | Kết quả |
|---|---|
| `4242 4242 4242 4242` | thành công ngay |
| `4000 0000 0000 0002` | từ chối `card_declined` |
| `4000 0000 0000 9995` | `insufficient_funds` |
| `4000 0000 0000 3220` | 3-D Secure: lần 1 trả `next_action:"otp"`, lần 2 gửi `{"otp":"123456"}` → thành công; OTP sai → `otp_incorrect` |
| `4000 0000 0000 0119` | `provider_error` (thử lại được) |
| `4000 0000 0000 0341` | "timeout": `PROCESSING`, kết quả về qua webhook sau `MOCK_TIMEOUT_DELAY_MS` (30 s) |
| `4000 0000 0000 0259` | thành công nhưng webhook gửi **2 lần** (idempotent, chỉ trả tiền một lần) |
| `4000 0000 0000 0067` | thành công nhưng webhook đến **sau hạn thanh toán** (đơn có thể đã `EXPIRED` ⇒ order-service hoàn tiền phần dư) |
| số khác (≥12 chữ số hợp lệ) | từ chối `do_not_honor` |
Mỗi lần thất bại cộng `failed_count`; **5 lần** ⇒ `FAILED` (kết thúc). Lỗi: 400 thẻ sai định dạng/hết hạn; 404 payment của người khác; 422 trạng thái không cho phép (`the payment is being processed`, `already paid`, `the payment has expired`…).

### Admin
| Method & path | Ghi chú |
|---|---|
| GET `/admin/payments?status=&method=&page=&page_size=` | `{payments[],total}` |
| GET `/admin/payments/:id` | kèm `attempts[]`, `refunds[]` |
| POST `/admin/payments/:id/refund` | `{amount_minor>0, reason}` – hoàn ngay (mô phỏng); 422 nếu vượt số đã thu / chưa thanh toán |
| POST `/admin/dev/payments/:id/force` | `{status: SUCCEEDED|FAILED|EXPIRED}`; **chỉ khi `ENV=dev`** (nếu không 403). `FAILED` kết thúc luôn payment |
Webhook mô phỏng `POST /internal/payments/webhook` nằm trên cổng admin `:8081` của payment-service (không qua gateway/Traefik), ký `X-Signature: t=<unix>,v1=HMAC_SHA256(PAYMENT_WEBHOOK_SECRET, t + "." + body)`, lệch giờ > 5 phút bị 401.

---
## 8. Seller console
| Method & path | Ai | Ghi chú |
|---|---|---|
| GET `/seller/products` | S | như `/search` nhưng **mọi trạng thái** của store trong token (`seller_id` query bị bỏ qua) |
| POST `/seller/products/:id/inventory/adjust` | S | `{delta (≠0, \|delta\|≤1.000.000), reason (1–200, bắt buộc)}` → `{message,success,inventory}`; ledger `restock` (delta>0) hoặc `adjust`; tồn không được âm (422) |
| GET `/seller/products/:id/inventory/ledger?page=&page_size=` | S | `{entries:[{id,product_id,delta,reason,ref_type,ref_id,balance_after,at}]}` mới nhất trước; `reason` ∈ `initial, reserve, release, sale, restock, adjust, update` |
| GET `/seller/inventory/low-stock?threshold=` | S | `{products[]}` (≤200) của sản phẩm `active/hidden` có tồn ≤ ngưỡng riêng hoặc `threshold` (mặc định 5), từ **Postgres** |
| Đơn hàng | S | §6.2 |
| GET `/seller/analytics/{summary,timeseries,top-products,funnel,low-stock}` | S | §9 (scope = store của token) |
Sản phẩm của người khác → 403 (`store does not own the product`).

---
## 9. Analytics (ClickHouse, qua analytics-service gRPC `Query`)
Seller: `GET /seller/analytics/{summary|timeseries|top-products|funnel|low-stock}` (S, scope store từ JWT). Admin: `GET /admin/analytics/{summary|timeseries|top-products|funnel|low-stock|traffic|payments|search-terms|data-health}` (A, toàn nền tảng). Gateway **không** nhận `store_id` từ client; analytics-service kiểm lại scope.
| Query | Ý nghĩa |
|---|---|
| `from`, `to` | `YYYY-MM-DD` (hiểu theo `Asia/Ho_Chi_Minh`, `to` là **ngày bao gồm** = hết ngày) hoặc RFC 3339. Mặc định 30 ngày gần nhất; tối đa **400 ngày** (trừ `low-stock`, `data-health`); sai → 400 |
| `granularity` | `hour|day|week|month` (mặc định `day`; tuần bắt đầu thứ Hai) – cho `timeseries`, `traffic` |
| `sort` | `top-products`: `revenue` (mặc định), `units`, `views`, `conversion`, `viewed_unsold` (nhiều lượt xem, 0 bán) |
| `limit` | `top-products` mặc định 20 (≤100); `low-stock` 50 (≤200); `search-terms` 20 (≤100) |
Phản hồi: `{"as_of":"…","source":"clickhouse","cached":false,"timezone":"Asia/Ho_Chi_Minh","data":…}`. Cache Redis 30 s (`ANALYTICS_CACHE_TTL_SEC`; không cache `data_health`).
**Doanh thu** = `subtotal` (tiền hàng, **không** gồm ship) của đơn **PAID** (online) hoặc **DELIVERED** (COD), tính theo thời điểm đó; `refunds` trừ theo thời điểm hoàn tiền; `net_revenue = revenue − refunds`.
| Report | `data` |
|---|---|
| `summary` | `{current:{revenue,refunds,net_revenue,gmv,orders_placed,orders_recognized,aov,canceled,expired,refunded,cancel_rate}, previous:{…kỳ liền trước cùng độ dài}, from, to}` – VND |
| `timeseries` | `[{bucket:"YYYY-MM-DD HH:MM:SS" (giờ VN), revenue, refunds, orders, placed}]` – bucket rỗng được điền 0 |
| `top-products` | `[{product_id,name,status,units,revenue,impressions,clicks,views,carts,conversion}]` |
| `funnel` | `{steps:[{step:"view|cart|checkout|paid",count}], unit, by_device? }` – đếm **session**; store scope: `checkout` = `null`; `paid` = số đơn đã ghi nhận doanh thu; `by_device` chỉ admin |
| `low-stock` | `[{product_id,name,available,reserved,level,units_per_day,days_left|null,store_id?}]` – hết/ sắp hết, hoặc hết trong ≤ 7 ngày theo tốc độ bán 14 ngày |
| `traffic` (admin) | `{page_views,sessions,visitors,client_errors,new_visitors,returning_visitors,series[],top_paths,top_referrers,devices,countries}` |
| `payments` (admin) | `{succeeded,failed,success_rate,amount_succeeded,by_method[],failure_codes[],refunds{count,amount},orders_awaiting_payment}` (đếm theo **lần thử**) |
| `search-terms` (admin) | `{top:[{term,count,avg_results}], zero_results:[{term,count}]}` |
| `data-health` (admin) | `{tables:[{table,rows,last_event_at,avg_lag_seconds_15m}], events_last_hour_by_type[], reconciliation:"not_implemented"}` |
Lỗi: ClickHouse/analytics-service down → **503** (`analytics store error` / unavailable) – cửa hàng vẫn chạy bình thường.

---
## 10. Admin khác
| Method & path | Ghi chú |
|---|---|
| GET `/admin/categories` | gồm cả danh mục `active=false` |
| POST `/admin/categories` | **201**; `{parent_id, name (1-100), slug (tự sinh từ name nếu trống; a-z0-9 và `-`), sort, active (mặc định true)}`; tối đa **3 cấp**; slug trùng/vòng lặp → 400; cha không tồn tại → 404 |
| PUT `/admin/categories/:id` | 200, thay toàn bộ |
| PATCH `/admin/products/:id/status` | `{status}` ∈ `draft|active|hidden|banned` (chỉ admin được `banned`; seller không gỡ được) |
| Thanh toán, đơn, analytics, `/admin/system/health` | §1, §6.3, §7, §9 |
**Chưa làm**: `/admin/system/metrics` (proxy Prometheus), `/admin/system/outbox`, `/admin/system/kafka`, requeue outbox qua API, `/admin/users`, `/admin/stores`, `/admin/audit-log`, `/admin/dev` khác ngoài force-payment. Xem Grafana thay thế ([runbook.md](runbook.md)).

---
## 11. Tracking (clickstream)
| Method & path | Ai | Ghi chú |
|---|---|---|
| POST `/events` | 👁 | rate-limit riêng; token tuỳ chọn (chỉ để gắn user) |
| POST `/events/identify` | 🔑 | `{anonymous_id}` → ghi event `event_type=identify` (user từ token) để nối khách ẩn danh với tài khoản; 400 thiếu `anonymous_id`; 401 chưa đăng nhập |
Gateway chuyển event cho analytics-service qua **gRPC `IngestEvents`** (không qua Kafka). Body `POST /events`:
```json
{"events":[{"event_id":"uuid","event_type":"product_view","ts_client":"2026-10-06T08:15:30.123Z",
  "anonymous_id":"a_9f2","session_id":"s_41c","surface":"pdp",
  "page":{"path":"/products/12","referrer":"https://google.com/search?q=x"},
  "item":{"product_id":12,"position":3},"props":{"q":"áo thun","result_count":0},
  "device":{"type":"mobile","os":"iOS"},"consent":{"analytics":true},"app_version":"web-0.3.0"}]}
```
Giới hạn: ≤ **50** event và ≤ **64 KB** mỗi lần (400 nếu rỗng/ quá nhiều/ JSON hỏng; 413 nếu quá 64 KB). Phản hồi **202** `{accepted, rejected, reasons:["<index>:<lý do>"]}`; nếu analytics-service không gọi được: 202 `{accepted:0,rejected:0,dropped:N}` (tracking là best-effort, trang không được biết).
Client **không** gửi `user_id/role/store_id/ip` (không có trong envelope); gateway gắn `user_id, role, store_id` từ JWT, `ip_hash` = HMAC(`IP_HASH_SECRET`, ngày UTC|IP) (đổi mỗi ngày, không lưu IP thô), `ua_family` (chrome/firefox/safari/edge/bot/other), `country` (từ header `CF-IPCountry` nếu có).
`event_type` cho phép: `page_view, impression, product_click, product_view, dwell, search, search_click, filter_apply, sort_change, add_to_cart, remove_from_cart, cart_update, wishlist_add, wishlist_remove, checkout_start, checkout_submit, payment_page_view, error_client` (+ `identify` chỉ từ `/events/identify`). Loại server-side (`order_placed`, `payment_succeeded`…) bị từ chối `unknown_or_server_only_type`. `surface` ∈ `home_trending, home_new, category_page, search_results, pdp, cart, checkout, orders, seller_console, admin_console` (giá trị khác → rỗng); `device.type` ∈ `desktop|mobile|tablet` (khác → `other`).
Lý do từ chối: `unknown_or_server_only_type`, `id_too_long` (>64), `props_too_large` (>2 KB), `props_not_json_object`, `no_consent`, `identify_needs_user_and_anonymous_id`.
**Consent**: `consent.analytics=false` ⇒ chỉ giữ `page_view` và `error_client`, **xoá** `anonymous_id, session_id, user_id, ip_hash, product_id`, `props={}`; mọi loại khác bị `no_consent`. **PII**: email/số điện thoại trong chuỗi của `props` bị che `[email]`/`[phone]`; `path` bỏ query/fragment; `referrer` chỉ giữ host+path; `ts_client` chỉ tin khi lệch ≤ ±24 h.
**Chưa làm**: `PUT /users/me/consent`, `DELETE /users/me/data`, bảng `consents`/`identity_links` (chỉ lưu event `identify`), `GET /config/features`.

---
## 12. Bản đồ route → quyền (tóm tắt)
| Nhóm | Route | Ai |
|---|---|---|
| Công khai | `/health`, `/healthz`, `/auth/{login,register,refresh-token}`, `/categories`, `/payments/methods`, `/products/:id/stock`, `POST /events` | 🌐 |
| Token tuỳ chọn | `/search`, `GET /products`, `/products/:id`, `/products/seller/:id`, `POST /events` | 👁 |
| Đăng nhập | `/auth/change-password`, `GET /users/sellers/:id`, `POST /events/identify` | 🔑 |
| Buyer | `/users/buyers*`, `/users/me/addresses*`, `/cart*`, `/checkout/preview`, `/checkouts/:id`, `/orders*`, `/payments/:id*` | B |
| Seller admin | `/auth/register-seller-roles`, `POST/PUT/DELETE /users/sellers*` | SA |
| Seller | `POST /products`, `PUT /products/:id`, `PATCH /products/:id/status`, `/seller/*` | S |
| Admin | `/admin/*` | A |
Bảng test truy cập: `services/api-gateway/internal/router/router_test.go` ([testing.md](testing.md)).

## Token & lỗi thường gặp
| Tình huống | Mã |
|---|---|
| Thiếu/sai/hết hạn token; dùng refresh token như access | 401 |
| Sai role; sửa store người khác; sản phẩm người khác | 403 |
| Đơn/payment/địa chỉ của người khác; sản phẩm không active (khách) | 404 |
| Hành động không hợp lệ với trạng thái hiện tại (cancel đơn đã ship, dòng hết hàng…) | 422 |
| Trùng sku / tài khoản đã tồn tại | 409 |
| Service phía sau down (product, analytics…) | 503 |
