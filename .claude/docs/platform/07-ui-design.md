# 07 · Thiết kế giao diện (không AI)

Stack giữ nguyên: Next.js 14 + Tailwind, chỉ gọi gateway. Thêm Recharts cho biểu đồ.

## 1. Nguyên tắc
Tiếng Việt mặc định · mỗi vùng dữ liệu đủ 4 trạng thái **loading / empty / error / stale** · số liệu luôn có `as_of` + tooltip định nghĩa (đặc biệt "doanh thu"); **múi giờ cố định `Asia/Ho_Chi_Minh`**, không có bộ chọn múi giờ · mọi thứ *MÔ PHỎNG* (thanh toán, vận chuyển) có nhãn rõ · ẩn UI theo role chỉ là tiện lợi, quyền thật ở gateway · không render HTML từ người dùng · truy cập được (focus, tương phản, `aria-live` cho thông báo).

## 2. Sitemap
```
Công khai/buyer:  /  ·  /products (lọc, sắp xếp, tìm kiếm từ khoá + gợi ý gõ)  ·  /products/[id] (ảnh, mô tả, thuộc tính, review, tồn "còn ít")
                  /categories/[slug]  ·  /cart  ·  /checkout (địa chỉ, phương thức, mã giảm, tóm tắt)  ·  /pay/[payment_id] (trang thanh toán MÔ PHỎNG)
                  /orders  ·  /orders/[id] (timeline trạng thái, huỷ, đã nhận, đổi trả)  ·  /wishlist  ·  /profile (+ địa chỉ, quyền riêng tư)  ·  /login /register
Người bán:        /seller (tổng quan)  ·  /seller/products (+ form đủ trường, ảnh, trạng thái)  ·  /seller/inventory  ·  /seller/orders (hộp thư đơn, ship)
                  /seller/analytics  ·  /seller/team (seller_admin)
Quản trị:         /admin  ·  /admin/system (CPU/RAM/health)  ·  /admin/analytics  ·  /admin/payments  ·  /admin/orders
                  /admin/users  ·  /admin/stores  ·  /admin/categories  ·  /admin/audit  ·  /admin/data-health
Dev (ENV=dev):    /dev/tracking (event realtime)  ·  /dev/payments (ép trạng thái)
```
`NavBar`: bảng cấu hình route→role ở một nơi; thêm `admin` vào `Role` (`lib/types.ts`); `/dashboard` cũ chuyển hướng theo role. Layout: `(shop)`, `(seller)`, `(admin)`; seller/admin dùng **ConsoleLayout** (sidebar + bộ chọn khoảng thời gian + nút đổi giao diện sáng/tối).

## 3. Wireframe chính
### 3.1 Checkout & thanh toán mô phỏng
```
/checkout                                                   ┌ Tóm tắt ──────────────┐
 1. Địa chỉ giao  ( ) Nhà – Nguyễn A, 09…  [Sửa]  [+ Thêm]    │ 2 sản phẩm   450.000₫ │
 2. Thanh toán    (•) Thẻ (MÔ PHỎNG)  ( ) Ví (MÔ PHỎNG)       │ Phí giao      20.000₫ │
                  ( ) Chuyển khoản (MÔ PHỎNG) ( ) COD          ⚠ Chế độ mô phỏng: không trừ tiền thật.                      │ [ Đặt hàng ]          │
                                                              └───────────────────────┘
/pay/{id}: banner "TRANG THANH TOÁN GIẢ LẬP"  Số thẻ test [4242…] hạn [..] CVC [..]  [Thanh toán]
           (bảng số thẻ kịch bản có thể mở ra: thành công / từ chối / 3-D Secure / timeout…) → trạng thái "Đang xử lý…" (poll) → kết quả
           Đếm ngược hết hạn giữ hàng:  ⏱ 14:32
```
### 3.2 Chi tiết đơn (buyer)
Timeline: `Đã đặt ● Chờ thanh toán ● Đã thanh toán ○ Đang giao ○ Đã giao`; thông tin thanh toán (method, trạng thái, mã), địa chỉ snapshot, mã vận đơn; nút theo trạng thái (Thanh toán lại / Huỷ / Đã nhận hàng / Yêu cầu đổi trả).
### 3.3 Seller tổng quan `/seller`
```
[Hôm nay|7 ngày|30 ngày|Tuỳ chọn]                                                   
┌Doanh thu┐ ┌Đơn┐ ┌AOV┐ ┌Tỉ lệ huỷ┐   (so kỳ trước ▲▼)       ┌ Cần chú ý ────────────┐
│ 12,4tr  │ │38 │ │326k│ │ 4,2%   │                           │ ⚠ 3 SP sắp hết hàng   │
└─────────┘ └───┘ └────┘ └────────┘                           │ ⚠ 2 đơn chờ giao >24h │
┌ Doanh thu theo ngày (line + kỳ trước) ─────────┐ ┌ Top SP ┐ │ ⓘ SP X: 120 xem 0 đơn │
┌ Funnel xem→giỏ→thanh toán ┐ ┌ Tồn thấp / ngày còn bán ┐     └───────────────────────┘
Cập nhật lúc 08:15 · Doanh thu = đơn đã thanh toán/ COD đã giao (ⓘ)
```
`/seller/orders`: tab theo trạng thái (Chờ xác nhận / Cần giao / Đang giao / Hoàn tất / Huỷ / Đổi trả), bảng + thao tác hàng loạt "Xác nhận & in phiếu", form nhập mã vận đơn.
`/seller/products` form: tên, SKU, danh mục, thương hiệu, mô tả, giá, tồn, ngưỡng cảnh báo, ảnh (kéo thả, sắp xếp), thuộc tính (key/value), tags, trạng thái; xem trước trang sản phẩm.
### 3.4 Admin `/admin` & `/admin/system`
```
/admin: KPI  Doanh thu | GMV | Đơn | Người dùng hoạt động | Phiên | Lỗi 5xx | Service not_ready/degraded
        Biểu đồ: doanh thu theo giờ · traffic (PV/Session/UV) · funnel · top shop · top danh mục · thanh toán thành công/thất bại
/admin/system:
 ┌ service           replicas  ready   version  CPU%  RAM (MB/limit)  p95    5xx   Kafka lag  outbox cũ nhất ┐
 │ api-gateway        2/2      ready   1.4.2    34    310/512         120ms  0.1%   –         –              │
 │ order-service      1/1      ready   1.4.2    12    180/512          45ms  0%     12        0.8s           │
 │ payment-service    1/1      ready   …                                                                     │
 ├ Hạ tầng: postgres ● redis ● kafka ● clickhouse ● minio ● prometheus ●  (từ /admin/system/health)          ┤
 ├ Node: CPU/RAM/Disk gauge · Biểu đồ chọn (service, metric, 1h/6h/24h/7d) · Hàng đợi outbox/ consumer lag  ┘
```
Hàng có trạng thái `degraded`/`not_ready` tô màu + hiện check lỗi (từ `checks{}` của `/ready`).
### 3.5 `/admin/payments`
Bảng giao dịch (lọc status/method/thời gian), chi tiết (attempts, webhook deliveries, hoàn tiền), nút hoàn tiền; thẻ tỉ lệ thành công & top `failure_code`.

## 4. Component & thư viện
```
components/layout/   ConsoleLayout · RoleGuard · FeatureGate · ThemeToggle (sáng/tối/theo hệ thống)
components/shop/     ProductCard(+impression/click) · ProductGallery · StockBadge · CategoryNav · SearchBar(+suggest) · FilterPanel · ReviewList
components/checkout/ AddressPicker · PaymentMethodPicker · OrderSummary · MockPayForm · CountdownTimer · OrderTimeline
components/seller/   OrderTable · ShipDialog · ProductForm · ImageUploader · InventoryTable
components/analytics/DateRangePicker · KpiRow · TimeSeries · FunnelChart · DataTable · AsOfBadge · DefinitionTooltip
components/system/   ServiceTable · ResourceGauge · HealthPill · ChecksPopover
lib/                 api.ts (mở rộng) · tracking.ts · flags.ts · money.ts (định dạng VND) · tz.ts (cố định Asia/Ho_Chi_Minh) · theme.ts
```
Biểu đồ: Recharts, một màu nhấn, kỳ trước nét đứt xám, trục có đơn vị, tooltip; lazy-load (`next/dynamic`).

## 5. Tracking SDK (`lib/tracking.ts`)
`track(type, props)`, `identify(userId)`, `setConsent()`; `<TrackImpression>` (IntersectionObserver ≥ 50%/1 s), `<TrackClick>`; hàng đợi, gửi batch 5 s/20 event, `sendBeacon` khi ẩn tab, retry mũ rồi bỏ, **không chặn UI**; sinh `event_id`, `anonymous_id` (`mm_aid`), `session_id` (30 phút); tôn trọng consent; `NEXT_PUBLIC_TRACKING_ENABLED`. Trang `/dev/tracking` hiển thị event; test đơn vị batching/dedupe.

## 6. Bảo mật UI
Token localStorage là nợ đã ghi (security.md) → chuyển cookie httpOnly trước khi mở thêm bề mặt (SSE/ upload). Markdown mô tả sản phẩm chỉ cho tập thẻ an toàn, sanitize; link ngoài `rel="noopener nofollow"`.

## 7. Dark mode & token thiết kế
- **Tailwind `darkMode: 'class'`**; màu định nghĩa bằng **biến CSS** (`--bg`, `--surface`, `--text`, `--muted`, `--border`, `--brand`, `--success/--warning/--danger/--info`) ở `:root` và `.dark`; component dùng class ánh xạ biến (`bg-surface text-text`) thay vì hardcode `slate-*` (code hiện tại đang dùng `text-slate-900` ở nhiều nơi → cần chuyển dần).
- Chế độ: **Sáng / Tối / Theo hệ thống** (mặc định theo `prefers-color-scheme`); lưu lựa chọn ở `localStorage` **và cookie `mm_theme`** để server render đúng class `<html class="dark">` ngay (tránh nháy trắng); `ThemeToggle` nằm ở NavBar/ConsoleLayout.
- Biểu đồ (Recharts): palette riêng cho từng theme (lưới/ trục/ tooltip theo biến CSS); kỳ trước nét đứt xám; kiểm tra độ tương phản ≥ 4.5:1 cho chữ, ≥ 3:1 cho thành phần đồ hoạ ở cả hai theme.
- Ảnh sản phẩm nền trong suốt: nền khung `surface`; logo/ icon dùng `currentColor`.
- Trạng thái đơn/ thanh toán/ tồn kho: một bảng ánh xạ duy nhất `lib/status.ts` → token màu (có biến thể sáng/tối).
- Bo `xl`, bóng nhẹ (tối: dùng viền thay bóng), font hệ thống.
- Kiểm thử: snapshot hai theme cho component chính; không có chữ/ icon "mất hút" ở theme tối (kiểm tra thủ công + axe).

## 9. Trạng thái triển khai (frontend)
**Đã làm** (`apps/frontend`, `tsc` + `eslint` + `next build` sạch; chưa chạy cùng gateway thật trong trình duyệt):
- Nền: dark mode (Tailwind `class`, CSS variables, cookie `mm_theme` + script chống nháy, `ThemeToggle`), `lib/format.ts` (vi-VN, VND, múi giờ cố định Asia/Ho_Chi_Minh), `lib/status.ts` (một nơi ánh xạ trạng thái đơn/thanh toán/tồn), `lib/api.ts` (ApiError, Idempotency-Key), role `admin`.
- Người mua: `/`, `/products` (tìm kiếm, lọc, sắp xếp lưu trên URL), `/categories/[slug]`, `/products/[id]` (mức tồn), `/cart` (giỏ server + giỏ khách gộp khi đăng nhập), `/checkout` (địa chỉ, phương thức, tóm tắt tính ở server), `/pay/[id]` (trang thanh toán MÔ PHỎNG: thẻ, 3-D Secure OTP, ví, chuyển khoản, bảng thẻ test), `/orders`, `/orders/[id]` (timeline, huỷ, đã nhận, đổi trả), `/profile` (địa chỉ).
- Người bán `/seller/*`: tổng quan, hộp thư đơn (ship/giao/huỷ/duyệt trả), sản phẩm (form đủ trường), tồn kho (điều chỉnh + sổ cái), analytics.
- Quản trị `/admin/*`: tổng quan, đơn, thanh toán (hoàn tiền, ép trạng thái ở dev), danh mục, `/admin/system`, analytics (doanh thu, traffic, thanh toán, tìm kiếm, data-health).
- Tracking SDK `lib/tracking.ts` (gộp lô, `sendBeacon`, `mm_aid`, phiên 30 phút, chống trùng impression) + `/dev/tracking`. Biến build: `NEXT_PUBLIC_APP_ENV=dev` (hiện công cụ dev), `NEXT_PUBLIC_TRACKING_ENABLED=false` (tắt tracking) — xem `apps/frontend/.env.example`.
- Biểu đồ là SVG thuần (không thêm Recharts).

**Chưa làm:** gợi ý khi gõ, review, wishlist, mã giảm giá (đã loại bỏ), upload ảnh (nhập URL https), `/seller/team`, `/admin/{users,stores,audit}`, banner đồng ý (consent mặc định "đồng ý"), test tự động cho frontend (chưa có test runner).
