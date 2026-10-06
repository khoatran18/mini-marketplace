# Architecture decision records

## ADR-1 Replace Patroni stack with stock Postgres (2026-10)
The repo vendored the whole Patroni project (+ etcd, haproxy, custom image) for HA. It was unmaintained here, not needed by the code, and dominated the repo. `deploy/infra.yml` now runs `postgres:16-alpine` with a volume and healthcheck. For HA use a managed Postgres or re-introduce Patroni as an external image, not vendored source.

## ADR-2 Secrets never in git
`.env`, keys, certs and binaries are ignored; `.env.example` files document the settings; images contain no configuration; compose injects env at deploy time.

## ADR-3 Authorization lives in the gateway, identity from the JWT
Protobuf contracts could not change without regenerating code in five modules, and the gateway is the only public entry point. Handlers therefore derive buyer/seller/store from the token (and the auth service for the store) and ignore client-supplied identity. Services still validate business rules (owner of product, order transitions).

## ADR-4 Price and state are server-side
order-service re-prices every order from product-service and owns the status state machine. The removed `PUT /orders/:id` allowed arbitrary rewrites.

## ADR-5 Cancel only confirmed orders, release stock via outbox event
> *Cập nhật:* trạng thái `SUCCESS` không còn; huỷ được phép ở `AWAITING_PAYMENT/CONFIRMED/PAID` (đã có kết quả giữ hàng) và vẫn không cho huỷ `PENDING`; xem ADR-21 và [data-model.md](data-model.md). Cơ chế `restored` giữ nguyên.

Canceling a `PENDING` order would race with the in-flight inventory reservation across two topics with no ordering. Allowing cancel only after the result arrived (`SUCCESS`) makes release safe, and `restored` makes it exactly-once.

## ADR-6 At-least-once delivery with idempotent consumers
Transactional outbox + `acks=all` + commit-after-handle + bounded retries. Every handler is a conditional/idempotent write; there is no distributed transaction.

## ADR-7 NUMERIC money, integer-cent arithmetic
> *Cập nhật:* event Kafka đã dùng đơn vị nhỏ (ADR-18); REST/gRPC vẫn `double` VND.

Changing the protobuf `double` fields to integer minor units needs regenerated code in five modules and a frontend change; for now precision is protected in storage and arithmetic, and the wire format stays `double`. Revisit with proto regeneration.

## ADR-8 One Go module per service (kept)
A shared module for generated protobuf and `kafkaimpl` would remove the copies in each service but requires building Docker images from the repository root and a `go.work`/`replace` setup. Deferred; the copies are kept identical (`kafkaimpl` was normalized in all five).

## ADR-9 Operational endpoints on a separate admin port (P-2)
Every Go service runs a second, internal-only HTTP server (`:8081`) with `/health`=`/healthz`, `/ready`=`/readyz`, `/metrics`, `/version`, next to its gRPC port, via one identical `pkg/ops` file per module (ADR-8 style). Liveness never touches dependencies; readiness distinguishes critical (503) and non-critical (degraded, 200) failures; on SIGTERM readiness flips to 503 for `DRAIN_SECONDS` before the server stops.

## ADR-10 `admin` role, never self-registered (P-11)
Needed for operations (`/admin/*`). Created only through `ADMIN_BOOTSTRAP_*`; `Register` keeps its role whitelist; only `Login`/`ChangePassword` protos accept `admin`.

## ADR-11 Regenerating protobuf code without buf.build
buf.build can be unreachable (proxy/offline). The generated Go code is reproduced byte-for-byte using a local `buf`, local `protoc-gen-go`/`protoc-gen-go-grpc` (same versions as in the file headers) and a `buf/validate/validate.proto` reconstructed from the compiled descriptor in the protovalidate Go module. See [protobuf.md](protobuf.md).

## ADR-12 Monitoring only, no alerting (P-8)
> *Cập nhật:* hiện có 5 dashboard (thêm *Analytics pipeline*); vẫn không có cảnh báo.

Prometheus + Grafana (dashboards provisioned from files). No Alertmanager, no alert-service, no notifications; thresholds only colour panels.

---
Các ADR dưới đây (ADR-13 trở đi) ghi bằng tiếng Việt, là những quyết định đã được **triển khai** sau P1 (xem [platform/09-roadmap.md](platform/09-roadmap.md)). ADR-1…12 giữ nguyên.

## ADR-13 Tracking đi qua gRPC `IngestEvents`, không qua Kafka
Thiết kế ban đầu: gateway → Kafka `tracking.events` → analytics. Thực tế: gateway gọi **gRPC `IngestEvents`** của analytics-service (≤ 50 event/lần, timeout 2 s) và analytics-service đệm trong RAM rồi ghi ClickHouse theo lô. Lý do: ít thành phần hơn (không topic/consumer/DLQ cho luồng best-effort), phản hồi `accepted/rejected` có lý do ngay cho client, validate/che PII/xử lý consent nằm ở một chỗ. Đánh đổi: nếu analytics-service down hoặc restart khi còn đệm thì event tracking **mất** (không có Kafka để phát lại); gateway luôn trả 202 (`dropped:N`) nên trang không bị ảnh hưởng. Event nghiệp vụ (đơn, thanh toán, sản phẩm, tồn kho) **vẫn** đi qua Kafka + outbox.

## ADR-14 Bảng fact `ReplacingMergeTree` + tính tổng lúc truy vấn (chưa có materialized view)
Mỗi bảng ClickHouse khoá theo id tự nhiên và dùng `ReplacingMergeTree(<cột thời gian>)`; mọi truy vấn dùng `FINAL`. Nhờ vậy consumer Kafka giao trùng chỉ ghi lại cùng một dòng (idempotent mà không cần bảng trạng thái). `fact_order_status` có **một dòng cho mỗi (đơn, trạng thái)** thay vì một dòng `fact_orders` luôn bị ghi đè, nên giữ được toàn bộ lịch sử và doanh thu được định nghĩa bằng "đã từng PAID/DELIVERED". Báo cáo gom thẳng bảng fact khi truy vấn (có cache Redis 30 s) – **chưa** có `mv_*` rollup: với quy mô hiện tại đủ nhanh và đơn giản hơn, thêm MV khi dữ liệu lớn (thêm migration mới, không sửa cái cũ). Hệ quả đã biết: một trạng thái đạt được hai lần (vd. `DELIVERED` sau khi từ chối trả hàng) ghi đè `at` bằng giá trị mới hơn.

## ADR-15 Tách đơn theo store khi checkout (`checkout_id`)
Một lần "đặt hàng" tạo **mỗi store một đơn** (cùng `checkout_id`), vì tồn kho, vận chuyển, huỷ, hoàn tiền và doanh thu độc lập theo store. Thanh toán gộp theo checkout (một `payments` cho mọi đơn đang `AWAITING_PAYMENT`); phí ship tính theo đơn-store. Idempotency theo `(buyer_id, Idempotency-Key)` trên `checkouts`. Hệ quả: đơn hết hàng chỉ `FAILED` đúng đơn đó, payment chỉ phủ phần còn lại; tiền đến cho đơn đã huỷ/hết hạn được hoàn tự động (`order_not_payable`).

## ADR-16 Chỉ có thanh toán mô phỏng, nhưng cùng hình dạng như thật
`payment-service` chỉ có nhà cung cấp `mockpay` (`PAYMENTS_MODE` khác `mock` ⇒ từ chối khởi động). Nó vẫn mô phỏng đủ độ khó của thật: bất đồng bộ, thẻ test với kết quả cố định, 3-D Secure, webhook ký HMAC trễ/trùng/đến sau hạn, retry mũ, idempotency theo `provider_event_id`/`refund_key`/`checkout_id`, hoàn tiền một phần, không bao giờ lưu số thẻ/CVV. UI/API gắn nhãn MÔ PHỎNG. Không có tích hợp cổng thật; khi cần, thêm một implementation provider.

## ADR-17 Múi giờ cố định `Asia/Ho_Chi_Minh`
Dữ liệu lưu UTC; mọi nhóm theo giờ/ngày/tuần/tháng và hiển thị dùng UTC+7 cố định (không DST, không bộ chọn múi giờ). Tham số `from/to=YYYY-MM-DD` đọc theo giờ VN, `to` bao gồm hết ngày; nhãn `bucket` là giờ VN. analytics-service nhúng `time/tzdata` để chạy trong image alpine tối giản; ClickHouse server `TZ=UTC`.

## ADR-18 Event dùng đơn vị tiền nhỏ (`*_minor`)
Event của order/payment mang số nguyên `*_minor` (1/100 VND) để không trôi số thực qua Kafka; ClickHouse lưu `Int64` và API analytics đổi lại thành VND. Postgres vẫn `NUMERIC(18,2)` và REST/gRPC đơn hàng vẫn `double` (ADR-7); payment lưu `amount_minor` trực tiếp. Ngoại lệ: `product.changed` còn mang `price` (VND, float) – analytics làm tròn sang `price_minor`.

## ADR-19 Xác thực gắn theo route, không dùng `router.Use` toàn cục
Một lần dùng `router.Use(authenticated)` làm **mọi route "công khai" trả 401** (catalog, `/payments/methods`, `/events`). Quy tắc: xác thực/ phân quyền gắn **theo nhóm hoặc theo route** (`authenticated`, `OptionalAuth` cho route token tuỳ chọn, `AuthorizationMiddleware([...])`), không bao giờ toàn cục. Token công khai nhưng **sai** vẫn 401 (không coi là ẩn danh). Có test hồi quy `TestEventsRoutes`, `TestPublicRoutesNeedNoToken` và bảng truy cập trong `router_test.go` ([testing.md](testing.md)).

## ADR-20 chDB shim để test ClickHouse không cần Docker
Sandbox không có Docker nhưng SQL báo cáo (`FINAL`, `argMax`, `minIf`, tham số `{x:Type}`, `toTimeZone`) phải chạy trên engine ClickHouse thật. `scripts/dev/chdb_server.py` bọc **chDB** (ClickHouse nhúng) thành giao diện HTTP tối thiểu (`/ping`, `/?query=&param_*`, thân INSERT) để analytics-service chạy test thật qua `TEST_CLICKHOUSE_URL`. Chỉ dùng cho test; CI vẫn chạy ClickHouse **24.8** thật. SQL đã được xác thực trên chDB 26.9 + unit test; chưa chạy trên ClickHouse server trong sandbox.

## ADR-21 Tồn kho = available / reserved / sold + sổ cái bất biến
`products.inventory` là **số còn bán được**; `reserved` giữ cho đơn chưa xuất kho; `sold` tăng khi đơn `SHIPPED`; mọi thay đổi chạy qua một câu `UPDATE … WHERE inventory + Δ >= 0` rồi ghi `inventory_ledgers` + `inventory.changed` cùng transaction. Giữ hàng all-or-nothing theo đơn, khoá theo thứ tự `product_id`; trả hàng và xuất kho mỗi cái đúng một lần (cờ `restored`, `shipped`). Hết hàng là suy ra (`stock_level`), không phải status. Thay cho "SUCCESS = đã trừ kho" cũ; huỷ đơn `PENDING` vẫn không cho (chờ kết quả giữ hàng) nhưng đơn chưa trả tiền tự hết hạn sau `ORDER_PAYMENT_TTL_MIN`.

## ADR-22 Định nghĩa doanh thu duy nhất
Doanh thu = `subtotal` (tiền hàng, không gồm ship) của đơn **PAID** (online) hoặc **DELIVERED** (COD), tính tại thời điểm đó; hoàn tiền trừ theo **thời điểm hoàn**; GMV loại đơn `EXPIRED/FAILED` và `CANCELED` chưa từng trả. Cài ở một CTE duy nhất (`ordersCTE`) dùng cho mọi báo cáo; có test `TestRevenueDefinition`.

## ADR-23 Outbox chung `domain_events` cho dịch vụ mới
product/order/payment dùng một bảng outbox tổng quát (`topic, key, payload, status, attempts`) và worker `events.Run` (`SKIP LOCKED`, lô 100, 2 s) thay cho một bảng cho mỗi loại event; bản sao `pkg/events` giống hệt nhau (ADR-8). auth/user giữ bảng outbox cũ. Vì chung một database nên đây là một bảng vật lý dùng chung (xem [data-model.md](data-model.md) §1.5): chấp nhận được, nhưng backlog không tách theo service và một dòng "độc" chặn đầu hàng.

## ADR-24 Payment tạo bất đồng bộ qua Kafka (`payment.requested`)
order-service không gọi gRPC payment-service; khi **mọi** đơn của một checkout online đã rời `PENDING` nó phát `payment.requested` (số tiền chỉ phủ đơn `AWAITING_PAYMENT`). Nhờ vậy đặt hàng không phụ thuộc payment-service đang sống, và tiền luôn khớp với các đơn thực sự giữ được hàng. Đổi lại `payment` chưa có ngay trong phản hồi `POST /orders` – client poll `GET /checkouts/:id`.

## ADR-25 analytics và payment là phụ thuộc không-critical của gateway
Gateway đánh `PaymentClient`/`AnalyticsClient` là non-critical trong `/ready`: payment-service down thì COD vẫn đặt được; ClickHouse/analytics-service down thì dashboard 503 và tracking vẫn 202, cửa hàng không dừng. (Ngược lại analytics-service coi ClickHouse là critical cho readiness của chính nó.)
