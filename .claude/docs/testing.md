# Kiểm thử

## 1. Chạy test
Mỗi service là một Go module riêng (Go 1.25): chạy từ `services/<name>`.
```bash
cd services/<name> && gofmt -l . && go vet ./... && go build ./... && go test -race -count=1 ./...
cd apps/frontend && npx tsc --noEmit && npm run build
```
Bảy service: `api-gateway`, `auth-service`, `user-service`, `product-service`, `order-service`, `payment-service`, `analytics-service` (CI chạy đủ bảy trong ma trận `.github/workflows/ci.yml`).

### Biến môi trường
| Biến | Dùng cho | Khi thiếu |
|---|---|---|
| `TEST_POSTGRES_DSN` | auth, user, product, order, payment (mọi test chạm DB) | test **bị skip** |
| `TEST_CLICKHOUSE_URL` | analytics-service (`internal/ch`, `internal/store`, `internal/service/service_ch_test.go`) | test **bị skip** |
```bash
export TEST_POSTGRES_DSN="host=localhost port=5432 user=postgres password=postgres sslmode=disable"
export TEST_CLICKHOUSE_URL="http://127.0.0.1:8123"
```
Test DB tạo một database tạm (`CREATE DATABASE …_test_<rand>`) cho **mỗi test** rồi xoá, nên role phải có quyền tạo database. Test ClickHouse tạo database riêng cho mỗi test (`store_test.go`). Không test nào cần Kafka/Redis thật: Kafka được thay bằng handler gọi trực tiếp, Redis bằng `miniredis`, gRPC phụ thuộc bằng fake nhúng interface client.

### Postgres cục bộ không cần Docker
`initdb`/`pg_ctl` từ `/usr/lib/postgresql/16/bin` dưới user OS `postgres` (thư mục dữ liệu ngoài `/tmp/claude-*`), rồi `export TEST_POSTGRES_DSN="host=localhost port=5433 user=postgres sslmode=disable"`.

### ClickHouse cho test: server thật hoặc chDB shim
- **CI** chạy ClickHouse **24.8** thật (service container, `TEST_CLICKHOUSE_URL=http://localhost:8123`).
- **Máy không có Docker**: `scripts/dev/chdb_server.py` giả lập giao diện HTTP của ClickHouse (GET/POST `/ping`, `/?query=…&param_x=…`, phần thân nối vào câu lệnh cho `INSERT … FORMAT JSONEachRow`, tham số `database`) trên **chDB** (engine ClickHouse nhúng) để SQL thật của analytics-service chạy trên engine thật. Chỉ dùng cho TEST (bỏ qua auth, một session có khoá).
```bash
pip install chdb                         # SQL đã được xác thực trên chDB 26.9
python3 scripts/dev/chdb_server.py --port 8123 --path /var/tmp/chdb-data &
cd services/analytics-service && TEST_CLICKHOUSE_URL=http://127.0.0.1:8123 go test -race -count=1 ./...
```
Khác biệt chDB ↔ ClickHouse server có thể có ở chi tiết hiếm; bản 24.8 trong CI là phép thử cuối.
- Sinh traffic giả để xem dashboard (DEV): `python3 scripts/dev/gen_traffic.py --api https://api.marketplace.swarm.localhost --visitors 200 --products 1-60` (chỉ gửi loại event của client; đơn/thanh toán phải đặt thật qua UI/API).

## 2. Phạm vi từng bộ test
| Khu vực | File | Nội dung chính |
|---|---|---|
| Đơn: định giá & checkout | `order-service/internal/service/checkout_test.go` | `toMinor` làm tròn cent; tách đơn theo store; giá luôn từ catalog; idempotency theo (buyer, key); validate (key, method, địa chỉ, ghi chú); preview báo `issue` không lỗi; luật phí ship (flat / free-ship) |
| Đơn: vòng đời | `…/service/flow_test.go` | online happy path đến trả hàng; COD; hết hàng chỉ làm `FAILED` đúng đơn đó và payment chỉ phủ phần còn lại; đơn quá hạn `EXPIRED` + trả hàng; quy tắc huỷ & hoàn tiền; cửa sổ trả hàng & từ chối; tự giao hàng; đọc theo scope; migrate `SUCCESS→CONFIRMED`; `payment.refunded`; tuần tự hoá thao tác đồng thời |
| Đơn: giỏ | `…/service/cart_test.go` | vòng đời giỏ; vấn đề phát sinh sau khi thêm; merge giỏ khách; trần 50 sản phẩm; checkout từ giỏ chỉ xoá dòng đã đặt |
| Đơn: state machine | `…/repository/statemachine_test.go` | bảng `transitions` (ai được chuyển gì), terminal không có đường ra, `errors.Is(ErrNotAllowed)`, đơn vị nhỏ |
| Payment | `payment-service/internal/service/service_test.go` (20 test) | idempotent tạo payment; kết quả từng thẻ test; không lưu dữ liệu thẻ; validate thẻ; 5 lần lỗi ⇒ `FAILED`; 3-D Secure/OTP; thẻ timeout trả lời bằng webhook; webhook đôi chỉ trả một lần; webhook sau hạn vẫn `SUCCEEDED`; ví & chuyển khoản; chữ ký + idempotency webhook; retry/bỏ cuộc; payment thuộc đúng buyer; huỷ & hết hạn; hoàn tiền; admin refund/force; consumer; xác nhận đồng thời chỉ trả một lần; yêu cầu secret webhook |
| Kho & catalog | `product-service/internal/service/product_service_test.go`, `catalog_test.go` | giữ hàng all-or-nothing, giao trùng/đồng thời, không bán quá tồn, trả hàng đúng một lần, owner check, giá trị 0, phân trang; trường catalog + validate; tìm kiếm không dấu + bộ lọc; cây danh mục (đếm, quy tắc ≤3 cấp); quy tắc status (`banned` chỉ admin); reserved → sold; adjust & ledger (quyền xem); danh sách tồn thấp; `domain_events` được phát & xuất bản; `slugify` |
| Auth | `auth-service/internal/service/auth_service_test.go`, `admin_test.go` | whitelist role, bcrypt, lỗi login thống nhất, loại token, đổi mật khẩu vô hiệu token cũ, chỉ tạo employee; bootstrap admin (idempotent, không đè mật khẩu, admin không đăng ký được) |
| User | `user-service/internal/service/address_service_test.go` | địa chỉ đầu là mặc định & chỉ một mặc định; cô lập theo user; giới hạn 10 + validate |
| Analytics | `analytics-service/internal/store/store_test.go` (ClickHouse) | migrate idempotent; **định nghĩa doanh thu**; giao lại idempotent; bucket theo giờ VN; top sản phẩm/funnel/scope store; payments/traffic/search/health; low-stock; validate scope & range |
| | `internal/ingest/ingest_test.go`, `internal/store/sink_test.go`, `internal/service/service_test.go`, `internal/server/server_test.go`, `internal/service/service_ch_test.go`, `internal/ch/client_test.go` | whitelist event, giới hạn/che PII, đồng hồ client ±24 h, consent; map event Kafka → hàng; sink gom lô/retry/rớt khi đầy buffer; luật truy cập báo cáo; đếm accepted/rejected; map gRPC; cache; client HTTP |
| Gateway: handler | `api-gateway/internal/handler/*_test.go` | quyền sở hữu order/buyer/store/product; trường do client điều khiển bị bỏ qua; ánh xạ lỗi; search không lộ hàng ẩn & che tồn; endpoint seller dùng store từ token; checkout cần `Idempotency-Key`; actor từ token; payment nhãn MÔ PHỎNG, **không echo body có dữ liệu thẻ**, checkout kèm payment; analytics: scope lấy từ token, lỗi → HTTP; tracking: gắn danh tính phía server, best-effort, giới hạn, `identify`, ip-hash xoay theo ngày |
| Gateway: middleware & route | `internal/middleware`, `internal/router/router_test.go` | JWT loại/hạn/alg, thu hồi theo `pwd_version`, kiểm role, rate limit nguyên tử, body limit 413, CORS allow-list, **bảng truy cập route** (§3), `/admin/system/health` |
| `pkg/ops` | `services/*/pkg/ops/ops_test.go` | liveness không đụng dependency; ready/degraded/not-ready; lỗi đã làm sạch; timeout; cache; draining; nhãn metric; test socket thật cho cổng admin + gRPC health. **Bảy bản phải giống hệt**: `md5sum services/*/pkg/ops/ops.go` (cũng `pkg/events/events.go` ở product/order/payment và `kafkaimpl/*`) |

### Bảng kiểm tra truy cập trong `router_test.go`
Test dạng bảng, mỗi dòng là `{method, path, wrongRole}` (`TestCatalogRouteAccessRules`, `TestOrderRouteAccessRules`):
- **không token ⇒ 401** và **token đúng cú pháp nhưng sai role ⇒ 403** – cả hai bị middleware chặn trước khi chạm handler/gRPC (nên không cần fake service);
- `TestPublicRoutesNeedNoToken`: các route công khai (`/search`, `/categories`, `/products*`, `/payments/methods`, `/health`…) không bao giờ 401/403, còn `/users/buyers/1`, `/users/sellers/1` vẫn 401;
- token công khai nhưng **sai** (`Bearer not-a-jwt`) ⇒ 401 chứ không bị coi là ẩn danh;
- `TestEventsRoutes` là **test hồi quy** cho lỗi dùng `router.Use(authenticated)` toàn cục làm mọi route công khai trả 401 (ADR-19): `/events` rỗng ⇒ 400 (không phải 401), `/events/identify` ẩn danh ⇒ 401;
- `TestOrderUpdateEndpointIsNotExposed`: `PUT /orders/:id` phải 404.
**Thêm route mới** ⇒ thêm một dòng vào bảng (role *sai* điển hình) và, nếu route công khai, vào `TestPublicRoutesNeedNoToken`.

## 3. Thẻ test thanh toán (MÔ PHỎNG)
Danh sách đầy đủ + hành vi: [api.md §7](api.md). Tóm tắt: `4242 4242 4242 4242` thành công · `4000 0000 0000 0002` declined · `…9995` thiếu tiền · `…3220` 3-D Secure (OTP `123456`) · `…0119` provider_error · `…0341` timeout (webhook sau 30 s) · `…0259` webhook đôi · `…0067` webhook sau hạn. Hằng số nằm ở `payment-service/internal/service/confirm.go`; thẻ khác ⇒ `do_not_honor`. Với test nhanh đặt `MOCK_WEBHOOK_DELAY_MS`, `MOCK_TRANSFER_DELAY_MS`, `MOCK_TIMEOUT_DELAY_MS` nhỏ; `ENV=dev` mở `POST /admin/dev/payments/:id/force`.

## 4. Viết test
- Giả một gRPC dependency bằng struct nhúng interface client được sinh ra, chỉ ghi đè method cần (`fakeProductClient`, `fakeOrderClient`, …).
- Test gateway dựng engine gin với middleware giả set `userID`/`userRole`; Redis là `miniredis`.
- Ưu tiên test tầng service với Postgres thật (khoá hàng, `UPDATE` có điều kiện, `NUMERIC` khác hẳn mock) và ClickHouse thật/chDB cho SQL báo cáo.
- Mỗi thay đổi hành vi phải kèm test (CLAUDE.md). Hành vi bất đồng bộ qua Kafka được test bằng cách gọi trực tiếp handler consumer (`UpdateOrderStatusByKafka`, `HandlePaymentSucceeded`, …) và `events.PublishBatch`.

## 5. Kiểm thử trên hạ tầng thật (Docker Swarm) – **đang LỖI THỜI**
```bash
./scripts/deploy.sh                                   # sau build-images.sh
scripts/e2e/run-in-swarm.sh                           # từ container tạm trong mạng marketplace-net
RATE_LIMIT_TEST=1 scripts/e2e/run-in-swarm.sh         # cần AUTH_RATE_LIMIT_PER_MINUTE=20; bộ chính cần nâng (vd. 1000)
scripts/e2e/resilience.sh                             # tiêm lỗi: Kafka down, product/order down, rolling restart, Postgres restart
```
`scripts/e2e/e2e.mjs` và `resilience.mjs` được viết cho API **trước** P2: đặt hàng bằng `POST /orders {order:{order_items…}}` không có `Idempotency-Key`/địa chỉ, chờ trạng thái `SUCCESS`, huỷ bằng `DELETE`. Với API hiện tại (checkout theo store, `AWAITING_PAYMENT/CONFIRMED`, bắt buộc `Idempotency-Key` và địa chỉ) các bước đó sẽ **thất bại** cho tới khi viết lại. Chưa có e2e cho payment/analytics/cart. Các phần auth, edge (TLS/CORS), store/catalog, thu hồi token vẫn đúng ý tưởng nhưng cần chạy lại.

## 6. Chưa được kiểm thử
- Docker/Swarm/Traefik/ClickHouse server **chưa từng chạy** trong sandbox soạn tài liệu: deploy, healthcheck Docker, nhãn Traefik, dashboard Grafana là chưa chạy thật (CI chỉ build image, chạy ClickHouse 24.8 cho test).
- Browser-level frontend test; gRPC server adapter; `DataHealth` đối soát (chưa cài).
