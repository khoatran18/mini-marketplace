# 04 · Thanh toán mô phỏng, vòng đời đơn, tồn kho

Mục tiêu: có dòng tiền/ trạng thái **giống thật** (bất đồng bộ, thất bại, hết hạn, hoàn tiền, webhook trễ/ trùng) để dashboard doanh thu có nghĩa và để luyện các tình huống vận hành, **mà không dùng cổng thật**. Mọi thứ gắn nhãn *MÔ PHỎNG* trên UI; `PAYMENTS_MODE=mock` là giá trị duy nhất ở giai đoạn này.

## 1. Vòng đời đơn hàng (state machine mới)
```
                    ┌───────────── FAILED (hết hàng/ không hợp lệ)
PENDING ────────────┤
 (đang giữ hàng)    └─▶ AWAITING_PAYMENT ──(payment.succeeded)──▶ PAID ──(seller xác nhận+giao)──▶ SHIPPED ──▶ DELIVERED
                          │ │                                       │                                              │
                          │ └─(quá expires_at)──▶ EXPIRED           └─(buyer/seller huỷ trước SHIPPED)─▶ CANCELED   └─(đổi trả)▶ REFUND_REQUESTED ▶ REFUNDED
                          └──(buyer huỷ)───────▶ CANCELED
COD:  PENDING ─▶ CONFIRMED (không cần trả trước) ─▶ SHIPPED ─▶ DELIVERED (thu tiền khi giao ⇒ payment PAID)
```
- **`SUCCESS` cũ** (= "đã giữ hàng") đổi tên thành `AWAITING_PAYMENT` (online) / `CONFIRMED` (COD). Migration: `SUCCESS → AWAITING_PAYMENT`; client cũ coi cả hai là "thành công đặt hàng" (UI cập nhật).
- Terminal: `FAILED, EXPIRED, CANCELED, REFUNDED, DELIVERED` (DELIVERED chỉ đi tiếp qua luồng hoàn trả).
- Chuyển trạng thái là `UPDATE … WHERE status IN (allowedPredecessors)` như hiện tại (idempotent). Mỗi lần chuyển: ghi `order_status_history` + outbox `order.status_changed` **trong cùng transaction**.
- Ai được chuyển (ép ở gateway + order-service):
| Chuyển | Ai | Điều kiện |
|---|---|---|
| PENDING→AWAITING_PAYMENT / CONFIRMED / FAILED | hệ thống (kết quả reserve từ product-service) | |
| AWAITING_PAYMENT→PAID | `payment-service` qua event | amount khớp tổng đơn |
| AWAITING_PAYMENT→EXPIRED | worker hết hạn | `now > expires_at` & chưa có payment thành công |
| (AWAITING_PAYMENT\|PAID\|CONFIRMED)→CANCELED | buyer (trước SHIPPED) / seller (kèm lý do) / admin | PAID ⇒ tự tạo refund |
| (PAID\|CONFIRMED)→SHIPPED | seller của đơn (kèm `carrier`, `tracking_code`) | |
| SHIPPED→DELIVERED | seller xác nhận, hoặc buyer bấm "đã nhận", hoặc tự động sau `AUTO_DELIVER_DAYS` (mô phỏng) | COD ⇒ ghi payment PAID |
| DELIVERED→REFUND_REQUESTED | buyer trong `RETURN_WINDOW_DAYS` (mặc định 7) | |
| REFUND_REQUESTED→REFUNDED | seller đồng ý / admin | gọi hoàn tiền |

## 2. Tồn kho & giữ chỗ (reservation)
- `products.inventory` = tồn vật lý; `products.reserved` = đang giữ cho đơn chưa kết thúc; khả dụng = `inventory - reserved`.
- Đặt hàng → **reserve** (tăng `reserved`, ghi `inventory_ledger reason=reserve`). Thanh toán xong/ giao → khi `SHIPPED` trừ `inventory` & giảm `reserved` (`sale`). Huỷ/hết hạn/ thất bại → `release`.
- Mọi thay đổi ghi `inventory_ledger` (bất biến) + outbox `inventory.changed` (để analytics & cảnh báo tồn thấp).
- **Hết hạn giữ chỗ**: worker trong order-service quét `status=AWAITING_PAYMENT AND expires_at < now()` mỗi 30 s (khoá `FOR UPDATE SKIP LOCKED`) ⇒ `EXPIRED` + `order.cancel`-tương đương ⇒ release. `ORDER_PAYMENT_TTL_MIN` mặc định 15.
- Ngưỡng tồn thấp: `available <= low_stock_threshold` ⇒ phát `inventory.changed{new_level:low}`; alert-service tạo alert cho shop (xem 05).
- Shop điều chỉnh tồn: `POST /seller/products/:id/inventory/adjust {delta, reason}` (ghi ledger, bắt buộc lý do).

## 3. `payment-service` (mô phỏng)
### 3.1 Phương thức
| Method | Hành vi mô phỏng |
|---|---|
| `COD` | Không tạo thanh toán trước; payment `PENDING_COD` → `PAID` khi `DELIVERED` |
| `MOCK_CARD` | Trang thanh toán giả (frontend `/pay/[payment_id]`) nhập số thẻ **test**; kết quả theo số thẻ (3.2) |
| `MOCK_WALLET` | "Ví" xác nhận bất đồng bộ: bấm đồng ý trên trang giả → webhook trễ 2–8 s |
| `MOCK_BANK_TRANSFER` | Hiển thị mã chuyển khoản; admin/ nút "giả lập đã nhận tiền" hoặc tự khớp sau X giây; quá hạn ⇒ không thanh toán → đơn EXPIRED |

### 3.2 Kịch bản theo số thẻ test (hoặc header `X-Mock-Scenario` ở môi trường dev)
| Số thẻ / kịch bản | Kết quả |
|---|---|
| `4242 4242 4242 4242` | thành công ngay |
| `4000 0000 0000 0002` | bị từ chối (`card_declined`) |
| `4000 0000 0000 9995` | không đủ tiền (`insufficient_funds`) |
| `4000 0000 0000 3220` | cần xác thực 3-D Secure giả (bước OTP `123456`) rồi thành công |
| `4000 0000 0000 0119` | lỗi nhà cung cấp tạm thời (`provider_error`) → cho phép thử lại |
| `4000 0000 0000 0341` | **timeout**: kết quả không về; webhook đến trễ sau 30 s |
| `4000 0000 0000 0259` | thành công nhưng webhook **gửi 2 lần** (kiểm tra idempotency) |
| `4000 0000 0000 0067` | thành công, webhook **đến sau khi đơn đã EXPIRED** (kiểm tra hoàn tiền tự động) |
Bảng này cố định, ghi trong repo để test E2E lặp lại được. Số thẻ lưu **chỉ 4 số cuối** và `scenario`; không bao giờ lưu số thẻ đầy đủ/CVV (tập thói quen đúng).

### 3.3 Luồng
```
POST /orders {payment_method:"MOCK_CARD", …}  ──▶ order PENDING ─▶ (reserve OK) AWAITING_PAYMENT
order-service ─gRPC CreatePayment(order_id, amount, method, idempotency_key)─▶ payment-service  (status=REQUIRES_ACTION, trả `payment_id`, `pay_url`)
Buyer ─▶ /pay/{payment_id}  (UI giả)  ─POST /payments/{id}/confirm {card…}─▶ gateway ─▶ payment-service
payment-service: chọn kịch bản → cập nhật payments/attempts → outbox `payment.succeeded|failed`
   + (mô phỏng provider) bắn **webhook nội bộ có chữ ký HMAC** tới `POST /internal/payments/webhook` của chính nó
     (retry mũ, có thể trễ/ trùng theo kịch bản) → xử lý idempotent theo `provider_event_id`
order-service consume `payment.succeeded` → AWAITING_PAYMENT→PAID  (nếu đơn đã EXPIRED/CANCELED ⇒ yêu cầu refund tự động)
```
- **Idempotency**: `payments.idempotency_key UNIQUE`; webhook trùng không đổi trạng thái hai lần.
- **Kiểm tra số tiền**: `amount` payment phải bằng `orders.total_price` (so bằng cent nguyên); lệch ⇒ `FAILED` + alert.
- **Chữ ký webhook**: `X-Signature: t=<ts>,v1=HMAC_SHA256(secret, ts + "." + body)`, từ chối nếu lệch giờ > 5 phút (tập đúng cách làm với cổng thật). Secret `PAYMENT_WEBHOOK_SECRET`.
- **Hoàn tiền**: `POST /payments/:id/refund` (nội bộ/ seller/ admin theo luồng đơn) → `refunds` → `payment.refunded`; hoàn một phần được; tổng hoàn ≤ đã thu.
- **Trạng thái payment**: `REQUIRES_ACTION → PROCESSING → SUCCEEDED | FAILED | CANCELED`; `SUCCEEDED → PARTIALLY_REFUNDED | REFUNDED`.
- **Tương lai**: thêm provider thật = thêm một `Provider` implementation (interface `Charge/Refund/VerifyWebhook`) – mockpay chỉ là một implementation.

### 3.4 Chủ động bất thường (để luyện vận hành)
Công cụ admin dev-only `POST /admin/dev/payments/:id/force {status}`; `PAYMENT_CHAOS_RATE` (0–1) tỉ lệ lỗi ngẫu nhiên khi chạy kiểm thử tải. Tắt hẳn ở production-like (`PAYMENTS_MODE` + `ENV`).

## 4. Phí/giảm giá (mô phỏng tối thiểu)
- `shipping_fee`: công thức đơn giản theo tổng khối lượng & vùng (cấu hình `SHIPPING_FLAT_FEE`, `FREE_SHIP_OVER`).
- `discount_total`: mã giảm giá (`coupons(code, type, value, min_total, max_discount, starts_at, ends_at, usage_limit, store_id?)`) – **tuỳ chọn P2.5**; server tính, không tin client.
- Tổng = `subtotal + shipping_fee - discount_total`, tính bằng cent nguyên.

## 5. Kiểm thử bắt buộc (bổ sung testing.md)
Unit (Postgres thật): chuyển trạng thái hợp lệ/không hợp lệ theo từng actor; reserve/release không lệch ledger; hết hạn giải phóng đúng một lần; webhook trùng/ trễ/ sau EXPIRED; số tiền lệch; hoàn tiền vượt mức bị từ chối; COD → PAID khi DELIVERED; cạnh tranh: 12 người mua 5 hàng (đã có) vẫn đúng với reserve mới.
E2E: bảng kịch bản thẻ ở 3.2, mỗi dòng một case; `resilience.sh`: payment-service down ⇒ tạo đơn online lỗi gọn (đơn giữ chỗ rồi hết hạn), Kafka down ⇒ outbox bù.
