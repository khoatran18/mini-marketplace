# Nhật ký quyết định kiến trúc (ADR)

## ADR-1 Thay stack Patroni bằng Postgres thường (2026-10)
Repo từng vendor nguyên dự án Patroni (+ etcd, haproxy, image tuỳ biến) cho HA. Phần này không được bảo trì ở đây, code không cần đến, và chiếm phần lớn repo. `deploy/infra.yml` nay chạy `postgres:16-alpine` với volume và healthcheck. Cần HA thì dùng Postgres managed hoặc đưa Patroni trở lại dưới dạng image bên ngoài, không vendor mã nguồn.

## ADR-2 Secret không bao giờ nằm trong git
`.env`, khoá, chứng chỉ và binary đều bị ignore; các file `.env.example` mô tả cấu hình; image không chứa cấu hình; compose truyền biến môi trường lúc deploy.

## ADR-3 Phân quyền nằm ở gateway, danh tính lấy từ JWT
Hợp đồng protobuf không thể đổi mà không sinh lại mã ở năm module, và gateway là điểm vào công khai duy nhất. Vì vậy các handler suy ra buyer/seller/store từ token (và từ auth service cho store) và bỏ qua danh tính do client gửi. Các service vẫn kiểm tra quy tắc nghiệp vụ (chủ sản phẩm, chuyển trạng thái đơn).

## ADR-4 Giá và trạng thái do server quyết định
order-service tính lại giá mọi đơn từ product-service và sở hữu state machine trạng thái. `PUT /orders/:id` đã bị gỡ vì cho phép ghi đè tuỳ ý.

## ADR-5 Chỉ huỷ đơn đã xác nhận, trả tồn kho qua event outbox
> *Cập nhật:* trạng thái `SUCCESS` không còn; huỷ được phép ở `AWAITING_PAYMENT/CONFIRMED/PAID` (đã có kết quả giữ hàng) và vẫn không cho huỷ `PENDING`; xem ADR-21 và [02_DATA_MODEL.md](../design/02_DATA_MODEL.md). Cơ chế `restored` giữ nguyên.

Huỷ đơn `PENDING` sẽ chạy đua với việc giữ hàng đang diễn ra trên hai topic không có thứ tự. Chỉ cho huỷ sau khi có kết quả (`SUCCESS`) thì việc trả hàng an toàn, và cờ `restored` đảm bảo trả đúng một lần.

## ADR-6 Giao ít nhất một lần với consumer idempotent
Transactional outbox + `acks=all` + commit sau khi xử lý + retry có giới hạn. Mọi handler đều là ghi có điều kiện/idempotent; không có distributed transaction.

## ADR-7 Tiền dạng NUMERIC, tính toán bằng số nguyên cent
> *Cập nhật:* event Kafka đã dùng đơn vị nhỏ (ADR-18); REST/gRPC vẫn `double` VND.

Đổi các trường `double` của protobuf sang đơn vị nhỏ kiểu số nguyên cần sinh lại mã ở năm module và sửa frontend; hiện tại độ chính xác được bảo vệ ở khâu lưu trữ và tính toán, còn định dạng trên wire vẫn là `double`. Xem lại khi sinh lại proto.

## ADR-8 Mỗi service một Go module (giữ nguyên)
Một module dùng chung cho protobuf sinh ra và `kafkaimpl` sẽ loại bỏ các bản sao ở từng service nhưng đòi hỏi build image Docker từ thư mục gốc repo và cấu hình `go.work`/`replace`. Hoãn lại; các bản sao được giữ giống hệt nhau (`kafkaimpl` đã được chuẩn hoá ở cả năm service).

## ADR-9 Endpoint vận hành trên cổng admin riêng (P-2)
Mỗi service Go chạy thêm một HTTP server thứ hai chỉ dùng nội bộ (`:8081`) với `/health`=`/healthz`, `/ready`=`/readyz`, `/metrics`, `/version`, bên cạnh cổng gRPC, qua một file `pkg/ops` giống hệt nhau ở mỗi module (theo kiểu ADR-8). Liveness không bao giờ chạm vào dependency; readiness phân biệt lỗi critical (503) và non-critical (degraded, 200); khi nhận SIGTERM, readiness chuyển sang 503 trong `DRAIN_SECONDS` trước khi server dừng.

## ADR-10 Role `admin`, không bao giờ tự đăng ký (P-11)
Cần cho vận hành (`/admin/*`). Chỉ được tạo qua `ADMIN_BOOTSTRAP_*`; `Register` giữ nguyên whitelist role; chỉ proto `Login`/`ChangePassword` chấp nhận `admin`.

## ADR-11 Sinh lại mã protobuf khi không có buf.build
buf.build có thể không truy cập được (proxy/offline). Mã Go sinh ra được tái tạo giống từng byte bằng `buf` cục bộ, `protoc-gen-go`/`protoc-gen-go-grpc` cục bộ (cùng phiên bản như trong header file) và `buf/validate/validate.proto` dựng lại từ descriptor đã biên dịch trong Go module protovalidate. Xem [06_PROTOBUF.md](../design/06_PROTOBUF.md).

## ADR-12 Chỉ giám sát, không cảnh báo (P-8)
> *Cập nhật:* hiện có 11 dashboard (thêm *Analytics pipeline*, *ClickHouse server*, *Outbox & Workers*, *Payments*, *Business*, *Kafka consumer lag*, *Logs*; 6 cái mới nhất chưa xem trên Grafana thật); vẫn không có cảnh báo.

Prometheus + Grafana (dashboard được provision từ file). Không có Alertmanager, không có alert-service, không có thông báo; các ngưỡng chỉ tô màu panel.

---
Các ADR từ ADR-13 trở đi là những quyết định đã được **triển khai** sau P1 (xem [09_ROADMAP_STATUS.md](../design/platform/09_ROADMAP_STATUS.md)).

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
Một lần dùng `router.Use(authenticated)` làm **mọi route "công khai" trả 401** (catalog, `/payments/methods`, `/events`). Quy tắc: xác thực/ phân quyền gắn **theo nhóm hoặc theo route** (`authenticated`, `OptionalAuth` cho route token tuỳ chọn, `AuthorizationMiddleware([...])`), không bao giờ toàn cục. Token công khai nhưng **sai** vẫn 401 (không coi là ẩn danh). Có test hồi quy `TestEventsRoutes`, `TestPublicRoutesNeedNoToken` và bảng truy cập trong `router_test.go` ([04_TESTING.md](../instruction/04_TESTING.md)).

## ADR-20 chDB shim để test ClickHouse không cần Docker
Sandbox không có Docker nhưng SQL báo cáo (`FINAL`, `argMax`, `minIf`, tham số `{x:Type}`, `toTimeZone`) phải chạy trên engine ClickHouse thật. `scripts/dev/chdb_server.py` bọc **chDB** (ClickHouse nhúng) thành giao diện HTTP tối thiểu (`/ping`, `/?query=&param_*`, thân INSERT) để analytics-service chạy test thật qua `TEST_CLICKHOUSE_URL`. Chỉ dùng cho test; CI vẫn chạy ClickHouse **24.8** thật. SQL đã được xác thực trên chDB 26.9 + unit test; chưa chạy trên ClickHouse server trong sandbox.

## ADR-21 Tồn kho = available / reserved / sold + sổ cái bất biến
`products.inventory` là **số còn bán được**; `reserved` giữ cho đơn chưa xuất kho; `sold` tăng khi đơn `SHIPPED`; mọi thay đổi chạy qua một câu `UPDATE … WHERE inventory + Δ >= 0` rồi ghi `inventory_ledgers` + `inventory.changed` cùng transaction. Giữ hàng all-or-nothing theo đơn, khoá theo thứ tự `product_id`; trả hàng và xuất kho mỗi cái đúng một lần (cờ `restored`, `shipped`). Hết hàng là suy ra (`stock_level`), không phải status. Thay cho "SUCCESS = đã trừ kho" cũ; huỷ đơn `PENDING` vẫn không cho (chờ kết quả giữ hàng) nhưng đơn chưa trả tiền tự hết hạn sau `ORDER_PAYMENT_TTL_MIN`.

## ADR-22 Định nghĩa doanh thu duy nhất
Doanh thu = `subtotal` (tiền hàng, không gồm ship) của đơn **PAID** (online) hoặc **DELIVERED** (COD), tính tại thời điểm đó; hoàn tiền trừ theo **thời điểm hoàn**; GMV loại đơn `EXPIRED/FAILED` và `CANCELED` chưa từng trả. Cài ở một CTE duy nhất (`ordersCTE`) dùng cho mọi báo cáo; có test `TestRevenueDefinition`.

## ADR-23 Outbox chung `domain_events` cho dịch vụ mới
product/order/payment dùng một bảng outbox tổng quát (`topic, key, payload, status, attempts`) và worker `events.Run` (`SKIP LOCKED`, lô 100, 2 s) thay cho một bảng cho mỗi loại event; bản sao `pkg/events` giống hệt nhau (ADR-8). auth/user giữ bảng outbox cũ. Vì chung một database nên đây là một bảng vật lý dùng chung (xem [02_DATA_MODEL.md](../design/02_DATA_MODEL.md) §1.5): chấp nhận được, nhưng backlog không tách theo service và một dòng "độc" chặn đầu hàng.

## ADR-24 Payment tạo bất đồng bộ qua Kafka (`payment.requested`)
order-service không gọi gRPC payment-service; khi **mọi** đơn của một checkout online đã rời `PENDING` nó phát `payment.requested` (số tiền chỉ phủ đơn `AWAITING_PAYMENT`). Nhờ vậy đặt hàng không phụ thuộc payment-service đang sống, và tiền luôn khớp với các đơn thực sự giữ được hàng. Đổi lại `payment` chưa có ngay trong phản hồi `POST /orders` – client poll `GET /checkouts/:id`.

## ADR-25 analytics và payment là phụ thuộc không-critical của gateway
Gateway đánh `PaymentClient`/`AnalyticsClient` là non-critical trong `/ready`: payment-service down thì COD vẫn đặt được; ClickHouse/analytics-service down thì dashboard 503 và tracking vẫn 202, cửa hàng không dừng. (Ngược lại analytics-service coi ClickHouse là critical cho readiness của chính nó.)

## ADR-26 Migration Postgres bằng goose, chạy lúc khởi động, có khoá advisory
Thay `AutoMigrate` bằng `goose` v3 (Provider API) cho auth / user / product / order / payment: file SQL đánh số trong `services/<svc>/migrations`, nhúng bằng `go:embed`, bảng version riêng `goose_db_version_<svc>`, `lock.NewPostgresSessionLocker()` để chỉ một replica chạy migration. Baseline `00001` là schema hiện tại, `IF NOT EXISTS` nên DB cũ vẫn lên được; `domain_events` (bảng chung, ADR-23) có mặt ở ba baseline. Ghim goose v3.26.0 vì v3.29 đòi Go 1.26. *Hệ quả:* thay đổi schema chỉ qua file SQL mới; chạy lúc khởi động đủ cho quy mô hiện tại (job migration riêng trước deploy để dành khi có nhiều replica / tách quyền DDL). Không dùng một "migration service" riêng.

## ADR-27 Request-id tự làm trước, OpenTelemetry để sau
Một mã `X-Request-ID` (HTTP) ↔ `x-request-id` (gRPC metadata) do gateway tạo hoặc nhận, mỗi service có `internal/requestid` (interceptor + logger), đưa vào log và body lỗi. Rẻ, không cần collector / backend tracing; sau này dùng lại làm trace-id khi lên OpenTelemetry. *Chưa làm:* truyền qua Kafka / outbox (cần thêm cột).

## ADR-28 Log: stdout → Vector → Loki, `request_id` không là nhãn
Service ghi JSON ra stdout (không tự đẩy log, để backend log chết không ảnh hưởng service); Vector chạy global đọc `docker.sock`, che PII rồi gửi tới Loki (single binary, filesystem, retention cấu hình); Grafana là giao diện. Nhãn Loki chỉ `service`, `stack`, `level`, `env`; `request_id` nằm trong nội dung dòng để tránh bùng nổ chỉ mục. Chọn Loki thay vì ClickHouse để không trộn log vận hành vào kho analytics (2 GB). Docker `json-file` giới hạn 10m × 3 để không đầy đĩa node.

## ADR-29 Đường ống dữ liệu: công cụ chạy một lần, ghi SQL trực tiếp, backfill chỉ cho bảng dẫn xuất
Datagen (Python), backfill / reconcile (Go, trong analytics-service) và export (Python) là công cụ chạy một lần, chưa phải service lập lịch. Click / giỏ lịch sử chỉ có ở ClickHouse và gateway chỉ nhận `ts` trong ±24 giờ, nên datagen ghi thẳng `events_raw`; nghiệp vụ lịch sử ghi SQL trực tiếp vào Postgres (test dựng DB bằng chính migration goose để bắt lệch schema) rồi backfill sang ClickHouse (idempotent nhờ `ReplacingMergeTree`). Backfill không dựng lại được `events_raw` từ Postgres. Chi tiết: [../design/07_DATA_PIPELINE.md](../design/07_DATA_PIPELINE.md).
