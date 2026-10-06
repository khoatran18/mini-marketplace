# Kiến trúc

```
Browser ──HTTPS──▶ Traefik ──▶ frontend (Next.js :3000)
                        └────▶ api-gateway (gin :8080) ──gRPC──▶ auth-service      :50051
                                   │  │  │                  ├───▶ order-service     :50052
                                   │  │  │                  ├───▶ product-service   :50053
                                   │  │  │                  ├───▶ user-service      :50054
                                   │  │  │                  ├───▶ analytics-service :50055   (IngestEvents, Query)
                                   │  │  │                  └───▶ payment-service   :50057   (SIMULATED provider)
                                   │  │  └─ Kafka (consumer auth.change_password)
                                   │  └──── Redis (rate limit, pwd_version)
                                   └─────── /admin/system/health ──HTTP──▶ :8081/ready của mọi service
Kafka ◀── outbox ── order · product · payment · auth · user ──▶ consumer: order · product · payment · analytics · auth · gateway
analytics-service ──HTTP :8123──▶ ClickHouse        order-service ──gRPC──▶ product-service (giá/tồn)        user-service ──gRPC──▶ auth-service
Mỗi service Go: cổng nghiệp vụ (gRPC; gateway HTTP :8080) + cổng admin nội bộ :8081 (/health /healthz /ready /readyz /metrics /version)
Quan sát: Prometheus ◀─ scrape :8081/metrics + cAdvisor + node-exporter + exporters ──▶ Grafana (chỉ xem, không cảnh báo)
```

## Service
| Service | Trách nhiệm | File chính |
|---|---|---|
| api-gateway | REST, JWT (`Type=="access"`), RBAC, kiểm sở hữu, CORS, rate limit, giới hạn body, **tracking `/events`** → analytics (gRPC), proxy báo cáo analytics, `/admin/system/health`, metrics HTTP | `internal/router` (danh sách route + quyền), `internal/middleware`, `internal/handler` |
| auth-service | đăng ký/ đăng nhập/ refresh/ đổi mật khẩu, role (`buyer`, `seller_admin`, `seller_employee`, `admin`), JWT, tra store của tài khoản, bootstrap admin | `internal/service/*` |
| user-service | hồ sơ buyer, store (seller), **địa chỉ giao hàng**; tạo store phát `user.create_seller` | `internal/service/{buyer,seller,address}_service.go` |
| product-service | catalog (danh mục, tìm kiếm không dấu, trạng thái), **tồn kho available/reserved/sold + sổ cái**, giữ/trả/xuất kho theo Kafka, outbox `product.changed`/`inventory.changed` | `internal/service`, `internal/repository` |
| order-service | **giỏ hàng**, checkout tách theo store (idempotent), định giá phía server, **máy trạng thái đơn mở rộng** + lịch sử, worker hết hạn/ tự giao, yêu cầu thanh toán/ hoàn tiền | `internal/service/{checkout,cart,orders,kafka_service}.go`, `internal/repository/{orders,statemachine,domain_events}.go` |
| payment-service | thanh toán **MÔ PHỎNG** (`mockpay`): thẻ test, ví, chuyển khoản, webhook ký HMAC, hoàn tiền | `internal/service/{service,confirm,queries}.go` |
| analytics-service | nhận tracking (gRPC) + event Kafka → ClickHouse; trả báo cáo theo scope platform/store; cache Redis | `internal/{ingest,store,service,ch}` |
Lớp trong một service: `server` (gRPC, protovalidate, ánh xạ lỗi) → `service` (nghiệp vụ) → `repository` (GORM); `client/*` bọc gRPC gửi đi; `config/messagequeue/kafkaimpl` bọc kafka-go; `pkg/ops` (admin :8081, bản sao giống hệt ở mọi service), `pkg/events` (outbox chung ở product/order/payment).

## Mô hình danh tính
- Tài khoản (`accounts`) có `role` và `store_id` (0 đến khi seller tạo store). Một *store* là một dòng `sellers` của user-service; `store_id` của tài khoản = id seller; `products.seller_id` = id store.
- Gateway tra store của người gọi qua auth-service (`GetStoreIDRoleById`) khi cần kiểm sở hữu; mọi route seller dùng store **từ token**, không từ request.
- Buyer là người duy nhất đặt hàng/ thanh toán; seller (`seller_admin`, `seller_employee`) quản lý catalog/ tồn/ đơn của store; admin quản trị nền tảng (không tự đăng ký được).

## Luồng đặt hàng → thanh toán → giao hàng (tóm tắt, chi tiết: [platform/04](platform/04-orders-payments.md))
1. `POST /orders` (header `Idempotency-Key`): gateway lấy buyer từ token, dựng snapshot địa chỉ; order-service định giá từ product-service, **tách đơn theo store**, trong **một transaction** ghi `checkouts`, `orders` (`PENDING`), lịch sử và outbox (`order.create_order`, `order.status_changed`).
2. product-service giữ hàng (`inventory−`, `reserved+`, ledger `reserve`) cho từng đơn, ghi kết quả → `product.validate_order`.
3. order-service: `PENDING → AWAITING_PAYMENT` (online, `expires_at = +ORDER_PAYMENT_TTL_MIN`) / `CONFIRMED` (COD) / `FAILED` (hết hàng). Khi mọi đơn của checkout online rời `PENDING` ⇒ `payment.requested`.
4. payment-service tạo payment (`REQUIRES_ACTION`); buyer xác nhận trên trang `/pay/<id>` (thẻ test/ ví/ chuyển khoản) → `payment.succeeded` → đơn `PAID`. Không trả kịp ⇒ worker `EXPIRED` và nhả kho; tiền về muộn ⇒ hoàn phần dư.
5. Seller `ship` (`PAID|CONFIRMED → SHIPPED`: kho chuyển `reserved → sold`), `deliver` hoặc buyer `confirm-received` hoặc tự giao sau `AUTO_DELIVER_DAYS` (COD: thu tiền = `PAID`). Trong cửa sổ trả hàng buyer `return` → seller/admin duyệt (`REFUNDED` + hoàn tiền online) hoặc từ chối.
6. Huỷ (`AWAITING_PAYMENT/CONFIRMED/PAID`): `CANCELED` + `order.cancel_order` (nhả kho đúng một lần) + hoàn tiền nếu đã trả online.
7. Mọi bước phát `order.status_changed` ⇒ analytics ghi `fact_order_status`.
Đơn `PENDING` không huỷ được (kết quả giữ hàng đang bay) – bỏ race reserve/cancel mà không cần timeout; đơn chưa trả tiền tự hết hạn.

## Tracking & analytics
Trình duyệt `POST /events` → gateway (gắn user/role/store/ip-hash từ JWT/request, giới hạn 50 event/64 KB, rate limit riêng) → **gRPC `IngestEvents`** → analytics-service (whitelist, consent, che PII) → bộ đệm RAM → ClickHouse `events_raw`. Event nghiệp vụ đi Kafka → bảng fact. Báo cáo (`/seller/analytics/*`, `/admin/analytics/*`) gọi gRPC `Query` → SQL trên bảng fact (`FINAL`), cache Redis 30 s, múi giờ cố định `Asia/Ho_Chi_Minh`. ClickHouse down ⇒ báo cáo 503, cửa hàng vẫn chạy.

## Nơi dữ liệu nằm
Một database Postgres chung (bảng thuộc từng service, không join chéo; riêng `domain_events` là bảng outbox dùng chung), Redis (bộ đếm rate limit, `<userId>:pwd_version`, cache báo cáo `analytics:v1:*`), Kafka một broker (3 partition/ topic, retention 7 ngày), ClickHouse (dẫn xuất, dựng lại được tối đa ~7 ngày từ Kafka). Chi tiết: [data-model.md](data-model.md), [events-kafka.md](events-kafka.md).
