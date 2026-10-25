# Mini Marketplace – Frontend

Next.js 14 (App Router, React 18, TypeScript, Tailwind CSS 3, `output: 'standalone'`). Giao diện **chỉ nói chuyện với api-gateway** (REST + JWT); không gọi trực tiếp service nào khác. Tham chiếu REST: [`docs/design/04_API_REFERENCE.md`](../../docs/design/04_API_REFERENCE.md).

## Chạy cục bộ

```bash
cd apps/frontend
npm install
npm run dev        # http://localhost:3000 (cần gateway ở http://localhost:8080)
npm run build && npm start   # bản production (standalone, cổng 3000)
npm run lint
```

Gateway cục bộ: dùng stack Swarm (`./scripts/deploy.sh`, xem `docs/instruction/01_QUICKSTART.md`) hoặc chạy `services/api-gateway`. Gateway phải cho phép origin của UI trong `ALLOWED_ORIGINS` (CORS).

## Biến môi trường

Các biến `NEXT_PUBLIC_*` được **nướng vào bundle lúc `next build`** (đổi giá trị thì phải build lại image).

| Biến | Mặc định | Ý nghĩa |
|---|---|---|
| `NEXT_PUBLIC_API_BASE_URL` | `http://localhost:8080` | URL gốc của api-gateway mà trình duyệt gọi (`lib/api.ts`, `lib/tracking.ts`) |
| `NEXT_PUBLIC_TRACKING_ENABLED` | bật | `false` để tắt hẳn SDK tracking |
| `NEXT_PUBLIC_APP_VERSION` | `0.1.0` | gửi trong event dưới dạng `app_version: web-<ver>` |
| `NEXT_PUBLIC_APP_ENV` | – | `dev` để hiện mục **Dev** (`/dev/tracking`) trong bản production; ở `npm run dev` luôn hiện |
| `GATEWAY_INTERNAL_URL` | `http://api-gateway:8080` | (runtime, phía server) địa chỉ gateway trong mạng Swarm cho `/api/ready` |
| `SERVICE_VERSION` | `dev` | (runtime) phiên bản báo trong `/api/ready` |

Không còn `NEXT_PUBLIC_JWT_SECRET`: frontend **không** biết secret, chỉ giải mã (không xác thực) payload JWT bằng `jose.decodeJwt` để lấy `UserID`, `Role`, hạn token. Mọi kiểm tra quyền thật nằm ở gateway.

## Build & deploy

- `apps/frontend/Dockerfile` (3 tầng: deps `npm ci` -> build -> runtime `node:20-alpine`, user `node`, cổng 3000) nhận `ARG NEXT_PUBLIC_API_BASE_URL`.
- `scripts/build-images.sh frontend` truyền `--build-arg NEXT_PUBLIC_API_BASE_URL="${API_BASE_URL:-$PUBLIC_SCHEME://api.$PUBLIC_HOST}"`. `PUBLIC_SCHEME`/`PUBLIC_HOST` lấy từ môi trường hoặc `deploy/.env` (mặc định `http` + `marketplace.swarm.localhost`). Dùng `https` thì phải `./scripts/gen-dev-certs.sh` trước; ghi đè trực tiếp bằng `API_BASE_URL=https://api.example.com`.
- `deploy/services.yml`: service `frontend` (image `marketplace/frontend`), Traefik route `Host(${PUBLIC_HOST})` -> cổng 3000; healthcheck `GET /api/healthz`.
- Route vận hành của chính frontend: `/api/health`, `/api/healthz` (liveness, không phụ thuộc gì), `/api/ready`, `/api/readyz` (gateway là phụ thuộc không critical: gateway chết -> `degraded`, không 503).

## Trang (app router)

| Nhóm | Route |
|---|---|
| Cửa hàng | `/` (trang chủ), `/products`, `/products/[id]`, `/categories/[slug]` (tìm kiếm + lọc qua `GET /search`) |
| Tài khoản | `/login`, `/register`, `/profile` (hồ sơ buyer/seller, địa chỉ giao hàng), `/dashboard` (chuyển hướng theo role) |
| Buyer | `/cart`, `/checkout`, `/orders`, `/orders/[id]`, `/pay/[id]` (trang thanh toán **mô phỏng**) |
| Seller | `/seller` (tổng quan), `/seller/products` (+ `/new`, `/[id]/edit`), `/seller/inventory`, `/seller/orders` (+ `/[id]`), `/seller/analytics`; `/products/mine` |
| Admin | `/admin`, `/admin/orders` (+ `/[id]`), `/admin/payments`, `/admin/categories`, `/admin/analytics`, `/admin/system` (health các service) |
| Dev | `/dev/tracking` (xem/ gửi event thử; ẩn ở production trừ khi `NEXT_PUBLIC_APP_ENV=dev`) |

## Cấu trúc mã

- `app/` – các route ở trên (+ `globals.css`, layout theo role cho `admin/` và `seller/`).
- `components/` – `auth/AuthProvider` (phiên), `cart/CartProvider`, `tracking/*`, `checkout/*`, `orders/*`, `seller/*`, `shop/*`, `analytics/*` (KPI, biểu đồ, bảng), `layout/*`, `ui/*`.
- `lib/api.ts` – client REST duy nhất (`apiFetch`, gắn `Authorization: Bearer`, `ApiError`); `lib/types.ts` – kiểu response; `lib/jwt.ts`; `lib/tracking.ts`; `lib/analytics.ts`; `lib/ops.ts`; `lib/theme.ts` (sáng/tối/hệ thống, lưu `localStorage` + cookie `mm_theme`).

## Cách nói chuyện với gateway

- Token: sau `POST /auth/login` lưu `access_token` + `refresh_token` trong `localStorage` (khoá `mini-marketplace-auth`); `AuthProvider` tự gọi `POST /auth/refresh-token` khi access token sắp hết hạn (mặc định 5 phút). Đổi mật khẩu làm token cũ bị vô hiệu (401 -> đăng nhập lại).
- Danh tính luôn lấy từ JWT phía server; UI không gửi buyer/store id, giá hay tổng. Tiền là VND dạng số thực, định dạng ở `lib/format.ts`.
- Giỏ hàng: khách lưu `localStorage` (`mini-marketplace-cart`); sau khi đăng nhập được gộp vào giỏ server bằng `POST /cart/merge`, từ đó dùng `/cart*`.
- Checkout: `POST /checkout/preview` -> `POST /orders` với header `Idempotency-Key` (sinh phía client, retry an toàn) -> poll `GET /checkouts/:id` cho tới khi có `payment` -> `/pay/[id]` gọi `POST /payments/:id/confirm` (xem thẻ test trong tài liệu REST).
- Lỗi: `ApiError.status` tương ứng mã HTTP của gateway (401 đăng xuất, 403 sai role, 422 sai trạng thái nghiệp vụ, 503/504 service phía sau); kèm `X-Request-ID` để tra log.

## Tracking (clickstream)

`lib/tracking.ts` + `components/tracking/*` gửi sự kiện tới `POST {API_BASE}/events` (và `POST /events/identify` sau khi đăng nhập để nối `anonymous_id` với tài khoản). Gateway chuyển sang analytics-service qua gRPC (không qua Kafka).

- Batch: flush mỗi 5 giây hoặc 20 event (tối đa 50/lần), `fetch(keepalive)` / `sendBeacon` khi tab ẩn, retry back-off rồi bỏ; không bao giờ chặn UI.
- Client không gửi `user_id/role/store_id/ip`: gateway lấy từ JWT. Chỉ gửi event loại client được phép (`page_view`, `impression`, `product_view`, `search`, `add_to_cart`, `checkout_start`, `checkout_submit`, `payment_page_view`, `error_client`, …); loại server-side bị từ chối.
- Consent: `consent.analytics=false` thì chỉ giữ `page_view`/`error_client` và bỏ mọi định danh. Tắt hẳn bằng `NEXT_PUBLIC_TRACKING_ENABLED=false`.
- Đặc tả event/giới hạn: `docs/design/04_API_REFERENCE.md` mục 11 và `docs/design/07_DATA_PIPELINE.md`.

## Giao diện

Tailwind CSS; các pattern dùng lại (`card`, nút, input) khai báo bằng `@apply` trong `app/globals.css`. Hỗ trợ chế độ sáng/tối (`components/layout/ThemeToggle`).
