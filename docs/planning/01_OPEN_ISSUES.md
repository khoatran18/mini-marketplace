# Vấn đề còn tồn đọng

> Gom một chỗ những gì **chưa làm / chưa kiểm chứng / có rủi ro**, rút từ [../design/platform/09_ROADMAP_STATUS.md](../design/platform/09_ROADMAP_STATUS.md), [02_TECH_DEBT_AND_BACKLOG.md](02_TECH_DEBT_AND_BACKLOG.md), [../design/05_SECURITY.md](../design/05_SECURITY.md) và đối chiếu thêm với code / repo. Mức ưu tiên là **đề xuất** (P1 = nên làm trước khi demo thật hoặc trước khi gắn AI; P2 = quan trọng; P3 = để sau).

## 1. Chưa kiểm chứng (rủi ro lớn nhất)

| # | Vấn đề | Ưu tiên | Ghi chú |
|---|---|---|---|
| V1 | Docker Swarm, Traefik, ClickHouse server, Prometheus, Grafana **chưa từng chạy thật** trong môi trường phát triển; compose, nhãn Traefik, healthcheck, dashboard mới chỉ được kiểm cú pháp | P1 | Lần deploy đầu theo [../instruction/02_DEPLOYMENT.md](../instruction/02_DEPLOYMENT.md) §1 và xem `docker service logs` từng service |
| V2 | SQL ClickHouse thử trên chDB 26.9; ClickHouse 24.8 thật chỉ chạy ở CI | P2 | Có thể lệch ở chi tiết hiếm |
| V3 | E2E `scripts/e2e/*.mjs` viết cho API cũ → **sẽ hỏng**; chưa có e2e cho cart / checkout / payment / analytics, chưa chạy lại `resilience.sh` | P1 | [../instruction/04_TESTING.md](../instruction/04_TESTING.md) §5 |
| V4 | Chạy nhiều replica cho service ngoài gateway chưa được kiểm chứng (worker timers của order-service, worker payment…) | P2 | Outbox dùng `SKIP LOCKED` nên có cơ sở, nhưng chưa có test |
| V5 | Frontend (nhánh UI) chưa được kiểm chứng end-to-end với backend thật | P2 | [../design/platform/07_UI_DESIGN.md](../design/platform/07_UI_DESIGN.md) |

## 2. Tính năng nghiệp vụ chưa làm

| # | Vấn đề | Ưu tiên |
|---|---|---|
| F1 | Review / wishlist (event `wishlist_add` đã whitelist nhưng chưa có API) — **cần cho gợi ý và sentiment** | P1 nếu làm AI |
| F2 | Upload ảnh sản phẩm (MinIO) — ảnh hiện chỉ là URL (≤ 8 / sản phẩm); cần cho visual search và OCR | P2 |
| F3 | Quyền riêng tư: bảng `consents`, `PUT /users/me/consent`, `DELETE /users/me/data` (consent chỉ đi kèm từng event) | P2 |
| F4 | An toàn tài khoản: khóa tạm khi sai mật khẩu nhiều lần, `audit_log`, xác thực email, đặt lại mật khẩu, logout (deny-list token), `GET /auth/me` | P2 |
| F5 | Quản trị: `/admin/users`, `/admin/stores` (duyệt / khóa), `/admin/system/metrics` (proxy Prometheus whitelist), `/admin/system/outbox` + requeue qua API | P2 |
| F6 | Analytics nâng cao: đối soát đêm Postgres ↔ ClickHouse (`data-health` hiện trả `reconciliation: "not_implemented"`), backfill, materialized view khi dữ liệu lớn, xuất CSV | P2 |
| F7 | Thông báo (email / in-app) và cảnh báo (Alertmanager) — **cố ý chưa làm** (ADR-12) | P3 |
| F8 | Thanh toán thật — hiện chỉ mô phỏng (ADR-16) | P3 |
| F9 | `GET /config/features` (feature flag) — cần nếu bật / tắt tính năng AI | P2 nếu làm AI |

## 3. Độ tin cậy và dữ liệu

| # | Vấn đề | Ưu tiên |
|---|---|---|
| R1 | Tracking đi qua gRPC: analytics-service down / restart thì **mất** event đang đệm; ClickHouse down quá `ANALYTICS_BUFFER_MAX_ROWS` (100.000) thì rớt dòng cũ nhất | P2 |
| R2 | `domain_events` là một bảng chung của product / order / payment: backlog không tách theo service, **một dòng độc chặn đầu hàng** (lỗi đầu tiên dừng cả lô); chưa có dead-letter | P1 |
| R3 | Consumer Kafka thử 5 lần rồi bỏ message; chưa có DLQ / schema registry; Kafka 1 broker, RF 1 | P2 |
| R4 | ClickHouse không backfill từ Postgres; chỉ dựng lại được tối đa ~7 ngày từ Kafka | P2 |
| R5 | `AutoMigrate` thay vì migration có version (Postgres); `accounts.username` UNIQUE toàn cục dù đăng nhập theo (username, role) | P2 |
| R6 | Hoàn tiền thủ công của admin không cập nhật `orders.payment_status`; đơn `REFUNDED` không tự nhập lại kho; hoàn tiền COD chỉ đổi nhãn; payment `PROCESSING` không tự hết hạn | P2 |
| R7 | `fact_order_status` giữ dòng mới nhất cho mỗi (đơn, trạng thái): đơn bị từ chối trả hàng làm dịch ngày ghi nhận doanh thu COD | P2 |
| R8 | `POST /orders` thường chưa kèm `payment` (tạo bất đồng bộ) → client phải poll `GET /checkouts/:id` | P3 |
| R9 | Seed demo gán sản phẩm cho `seller_id=2` nhưng không tạo store cho `seller1` | P3 |
| R10 | Bảng thừa `create_seller_kafka_events` ở user-service | P3 |

## 4. Bảo mật

| # | Vấn đề | Ưu tiên |
|---|---|---|
| S1 | Mọi bí mật từng commit trước khi dọn repo (JWT secret cũ, mật khẩu DB / Elastic, htpasswd Traefik, khóa TLS) **vẫn còn trong lịch sử git** → phải xoay (rotate) | P1 |
| S2 | Username / password bị giới hạn `^[a-zA-Z0-9_]{3,16}$` bởi hợp đồng protobuf — mật khẩu tối đa 16 ký tự là yếu | P2 |
| S3 | Token lưu `localStorage` (rủi ro XSS); nên chuyển cookie httpOnly | P2 |
| S4 | gRPC và Postgres giữa các service đang plaintext (`sslmode=disable`) | P3 |
| S5 | ClickHouse, Prometheus, cổng `:8081` không có xác thực (chỉ an toàn nhờ không publish cổng) | P2 |
| S6 | gRPC chưa có deadline / retry / circuit breaker từ gateway và order-service | P2 |
| S7 | `README.md` gốc có 2 thẻ `<img>` ẩn (kích thước 0×0, `display:none`) trỏ tới `canarytokens.com` — là ảnh theo dõi (canary token) chứ không phải tài liệu; nếu không chủ ý thì nên gỡ | P2 |

## 5. Quan sát (observability)

- Thiếu request-id, distributed tracing, metric nghiệp vụ (`mm_orders_*`, `mm_payments_*`, `mm_inventory_*`, `mm_kafka_consumer_lag`, `mm_grpc_client_*`).
- Grafana mới có **5 / 10** dashboard theo thiết kế; chưa có Payments, Business, ClickHouse server, panel outbox riêng.
- `update_config: start-first` / `failure_action: rollback` và `scripts/wait-ready.sh` chưa có (deploy cập nhật có thể gián đoạn).
- 3 exporter (postgres / redis / kafka) chưa có healthcheck container.

## 6. Công cụ và tài liệu

- Swagger (`services/api-gateway/docs`) **đã cũ**; REST đầy đủ ở [../design/04_API_REFERENCE.md](../design/04_API_REFERENCE.md). Cần `swag init` lại và chú thích handler mới.
- `apps/frontend/README.md` đã cũ (nhắc `cd frontend`, `NEXT_PUBLIC_JWT_SECRET`, API cũ).
- Chưa có module Go dùng chung cho protobuf + `kafkaimpl` + `ops` + `events` (hiện là bản sao giống hệt ở 7 service, ADR-8); chưa có `buf` registry.
- `payment-service` / `analytics-service` thiếu `cmd/.env.example`; `ANALYTICS_FLUSH_*` có trong `services.yml` nhưng chưa có trong `.env.example`; `ANALYTICS_BUFFER_MAX_ROWS` chưa truyền qua `services.yml`.
- Tài liệu trong `docs/` pha tiếng Việt / tiếng Anh (nhiều file kế thừa từ trước); chưa thống nhất.

## 7. Tồn đọng riêng cho việc gắn AI

Chi tiết và lý do: [../ai/04_AI_DATA_READINESS.md](../ai/04_AI_DATA_READINESS.md). Tóm tắt: chưa có review / wishlist, chưa có `request_id` + `model_version` trong envelope tracking (cần để đo CTR của gợi ý), chưa có feature flag, chưa có upload ảnh, chưa có bộ dữ liệu thật đủ lớn (dữ liệu demo là giả), chưa có `ai-service` và hạ tầng eval / quan sát LLM.
