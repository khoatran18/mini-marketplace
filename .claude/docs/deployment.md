# Triển khai

Ba file compose nhắm tới Docker Swarm (cũng chạy được bằng `docker compose` cục bộ). Mạng overlay dùng chung tên cố định **`marketplace-net`** (`infra.yml` tạo, hai file kia khai `external`).
| Stack (tên) | File | Thành phần |
|---|---|---|
| `marketplace-infra` | `deploy/infra.yml` | `postgres:16-alpine`, `redis:8.2.1-alpine` (AOF), **`clickhouse/clickhouse-server:24.8`**, Kafka `apache/kafka:3.9.0` một node KRaft (`broker1`, 3 partition mặc định, retention 168 h); volume `postgres-data redis-data kafka-data clickhouse-data`; healthcheck đủ cả bốn |
| `marketplace-obs` | `deploy/observability.yml` | Prometheus v2.55.1, Grafana 11.3.0, cAdvisor (global), node-exporter (global), postgres/redis/kafka-exporter; volume `prometheus-data grafana-data` |
| `marketplace` | `deploy/services.yml` | Traefik v3.6, frontend, `api-gateway` (2 replica), `auth/user/product/order-service`, **`payment-service`**, **`analytics-service`** (mỗi cái 1 replica) |
Env lấy từ shell/`deploy/.env` qua nội suy compose; **không `.env` nào được nhúng vào image**. Image build bằng `scripts/build-images.sh` (7 service Go + frontend; truyền `SERVICE_VERSION`, `GIT_COMMIT`, `BUILD_TIME` để `/version` đúng).

## 1. Checklist lần deploy đầu
1. `cp deploy/.env.example deploy/.env`, đặt tối thiểu: `POSTGRES_PASSWORD`, `JWT_SECRET` (≥16 ký tự), `TRAEFIK_DASHBOARD_USERS` (htpasswd, `$`→`$$`), `GRAFANA_ADMIN_PASSWORD`, **`CLICKHOUSE_PASSWORD`**, **`PAYMENT_WEBHOOK_SECRET`** (≥16 ký tự – thiếu thì payment-service không khởi động), `IP_HASH_SECRET` (khuyến nghị đặt để hash IP khớp giữa các replica), `ADMIN_BOOTSTRAP_USERNAME/PASSWORD` nếu cần admin. Giữ `ENV=prod` và `SEED_DEMO_DATA=false` ở môi trường thật.
2. `./scripts/gen-dev-certs.sh` (hoặc đặt cert thật vào `deploy/traefik/certs/{local.crt,local.key}`).
3. `API_BASE_URL=https://api.marketplace.swarm.localhost ./scripts/build-images.sh` (frontend nướng URL API lúc build).
4. `docker swarm init`.
5. `./scripts/deploy.sh` (= `infra` → `observability` → `services`; chạy riêng: `deploy.sh infra|observability|services`). Script render compose bằng `docker compose config`, bỏ `name:` và ép cổng số vì `docker stack deploy` không hiểu.
6. Thêm `/etc/hosts`: `marketplace.swarm.localhost`, `api.marketplace.swarm.localhost`, `dashboard.marketplace.swarm.localhost`, `grafana.marketplace.swarm.localhost`.
7. Kiểm tra: `docker stack services marketplace-infra` / `marketplace-obs` / `marketplace` đều `N/N`; `docker exec` một container trong mạng và `wget -qO- http://<service>:8081/ready` (mọi service `ready`); đăng nhập admin → `/admin/system` (hoặc `GET /admin/system/health`) toàn `ready`; Prometheus → Status → Targets mọi job `UP` (service Go qua DNS `tasks.<service>:8081`); mở Grafana, 5 dashboard trong thư mục *Marketplace* có số liệu ([runbook.md](runbook.md) §Grafana).
8. Thứ tự khởi động không bắt buộc (service tự chờ/ thử lại): analytics-service **chờ ClickHouse tối đa ~2 phút** (60 lần × 2 s) rồi mới migrate schema và mở cổng; nếu quá hạn nó thoát và Swarm khởi động lại.
> **Chưa được kiểm chứng**: toàn bộ các bước Docker/Swarm/Traefik/ClickHouse/Grafana ở trên chưa từng chạy trong sandbox soạn tài liệu (xem [platform/09-roadmap.md](platform/09-roadmap.md), "Giới hạn đã biết"). Lần deploy đầu hãy theo dõi `docker service logs` từng service.

Host: UI `https://marketplace.swarm.localhost`, API `https://api.<host>`, Traefik dashboard `https://dashboard.<host>` (basic auth), Grafana `https://grafana.<host>`.

## 2. ClickHouse + analytics-service
- ClickHouse **không publish cổng**, chỉ trong overlay (`clickhouse:8123` HTTP); user/mật khẩu qua `CLICKHOUSE_USER` (mặc định `default`) / `CLICKHOUSE_PASSWORD` (bắt buộc); `TZ=UTC`; giới hạn RAM 2 GB; `nofile` 262144. Volume `clickhouse-data`.
- analytics-service tự `CREATE DATABASE` (`CLICKHOUSE_DB`=`marketplace`) và tạo 7 bảng bằng migration có version ([data-model.md](data-model.md) §3). Không cần chạy SQL tay.
- Tiêu thụ 6 topic Kafka (`order.status_changed`, `payment.{succeeded,failed,refunded}`, `product.changed`, `inventory.changed`) và nhận tracking qua gRPC `IngestEvents` từ gateway. Ghi vào ClickHouse theo lô trong bộ đệm bộ nhớ (`ANALYTICS_FLUSH_INTERVAL_MS` 2000, `ANALYTICS_FLUSH_ROWS` 5000); ClickHouse down ⇒ giữ lại tới `ANALYTICS_BUFFER_MAX_ROWS` (100000) rồi **rớt dòng cũ nhất** (đếm ở `mm_ch_rows_dropped_total`).
- **ClickHouse là phần phụ**: gateway coi payment/analytics là non-critical; ClickHouse hay analytics-service down thì dashboard analytics trả **503** nhưng cửa hàng (xem, đặt hàng, thanh toán) và `POST /events` (vẫn 202) chạy bình thường.
- Lần đầu chạy trên dữ liệu đã có sẵn: ClickHouse **không** được backfill từ Postgres (chưa làm); chỉ có event còn nằm trong Kafka (retention 7 ngày): group mới (`analytics-service-<topic>`) đọc từ offset đầu tiên còn giữ (mặc định `FirstOffset` của kafka-go), nên dựng lại được tối đa ~7 ngày gần nhất bằng cách xoá bảng/ đổi group.

## 3. Biến môi trường
(`deploy/.env.example` là mẫu; `services.yml` truyền xuống container.)
| Biến | Dùng bởi | Mặc định / ghi chú |
|---|---|---|
| `POSTGRES_USER/PASSWORD/DB` | infra, services | mật khẩu bắt buộc |
| `JWT_SECRET` | mọi service Go | ≥ 16 ký tự, giống nhau |
| `JWT_EXPIRE_TIME` | auth, gateway | phút, mặc định 5 |
| `POSTGRES_DSN`, `REDIS_ADDR`, `KAFKA_BROKERS_ADDR` | service | dựng sẵn trong `services.yml`; chạy local xem `services/<svc>/cmd/.env.example` (payment/analytics chưa có file mẫu) |
| `KAFKA_PRODUCER_RETRY/BACKOFF`, `KAFKA_CONSUMER_BACKOFF` | service | 2 / 100 ms / 100 ms |
| `PUBLIC_HOST`, `TRAEFIK_DASHBOARD_USERS` | services.yml | định tuyến + auth dashboard |
| `ALLOWED_ORIGINS` (= `https://$PUBLIC_HOST`), `RATE_LIMIT_PER_MINUTE` (300), `AUTH_RATE_LIMIT_PER_MINUTE` (20), `TRUSTED_PROXIES`, `MAX_BODY_BYTES` (1048576) | gateway | xem security.md |
| `EVENTS_RATE_LIMIT_PER_MINUTE` (120), `IP_HASH_SECRET` (rỗng = khoá ngẫu nhiên mỗi replica) | gateway | tracking |
| `SYSTEM_HEALTH_TARGETS` | gateway | `name=http://host:8081,…`; mặc định 7 service |
| `ADMIN_BOOTSTRAP_USERNAME/PASSWORD` | auth | tạo admin một lần (3–16 ký tự `[a-zA-Z0-9_]`, mật khẩu ≥ 8); trống = không tạo |
| `ORDER_PAYMENT_TTL_MIN` (15), `RETURN_WINDOW_DAYS` (7), `AUTO_DELIVER_DAYS` (7, 0 = tắt), `SHIPPING_FLAT_FEE` (30000 VND), `FREE_SHIP_OVER` (500000, 0 = không bao giờ miễn phí) | order-service | quy tắc nghiệp vụ |
| `PAYMENTS_MODE` | payment | chỉ `mock` (giá trị khác ⇒ service từ chối chạy) |
| `PAYMENT_WEBHOOK_SECRET` | payment | HMAC webhook, ≥ 16 ký tự, **bắt buộc** |
| `PAYMENT_WEBHOOK_URL` | payment | mặc định `http://127.0.0.1:$ADMIN_PORT/internal/payments/webhook` |
| `MOCK_WEBHOOK_DELAY_MS` (3000), `MOCK_TRANSFER_DELAY_MS` (8000), `MOCK_TIMEOUT_DELAY_MS` (30000) | payment | độ trễ mô phỏng |
| `PAYMENT_CHAOS_RATE` (0–1), `ENV` (`prod`/`dev`) | payment | `ENV=dev` mới có `POST /admin/dev/payments/:id/force` và chaos |
| `CLICKHOUSE_URL` (`http://clickhouse:8123`), `CLICKHOUSE_DB` (`marketplace`), `CLICKHOUSE_USER` (`default`), `CLICKHOUSE_PASSWORD` | analytics, infra | mật khẩu bắt buộc |
| `ANALYTICS_CACHE_TTL_SEC` (30), `EVENTS_RETENTION_MONTHS` (13) | analytics | cache Redis; TTL `events_raw` (fact giữ vô hạn) |
| `ANALYTICS_FLUSH_INTERVAL_MS` (2000), `ANALYTICS_FLUSH_ROWS` (5000) | analytics | có trong `services.yml`, **chưa** có trong `.env.example` |
| `ANALYTICS_BUFFER_MAX_ROWS` (100000) | analytics | có trong code, **chưa** truyền qua `services.yml` (không chỉnh được khi deploy) |
| `GRAFANA_ADMIN_USER/PASSWORD`, `PROMETHEUS_RETENTION` (15d) | observability | mật khẩu bắt buộc |
| `SEED_DEMO_DATA` | auth, product | `true`: `buyer1`/`seller1` (mật khẩu `password`, **không** tạo store) + ~100 sản phẩm demo gán `seller_id=2` vào catalog rỗng. Không bao giờ bật ở production |
| `SEED_DEFAULT_CATEGORIES` | product | `false` để không seed 7 danh mục mặc định |
| `ADMIN_PORT` (8081), `DRAIN_SECONDS` (10), `READY_CACHE_SECONDS` (3), `READY_CHECK_TIMEOUT_MS` (1000), `PPROF_ENABLED` | mọi service Go | chuẩn vận hành; `SERVICE_VERSION/GIT_COMMIT/BUILD_TIME` do build-arg |
Địa chỉ gRPC giữa service (`auth-service:50051`, `order-service:50052`, `product-service:50053`, `user-service:50054`, `analytics-service:50055`, `payment-service:50057`) **cứng trong code** (`internal/config/grpc_config.go`), không có biến `*_SERVICE_ADDR`.

## 4. Chạy cục bộ không cần Swarm
Khởi động hạ tầng bằng `docker compose --env-file deploy/.env -f deploy/infra.yml up -d` (publish cổng khi cần; ClickHouse `8123`), copy `services/<svc>/cmd/.env.example` thành `.env`, rồi `go run ./cmd` ở mỗi service (auth/product trước order/user). Listener quảng bá của Kafka là `broker1:9092`: thêm `127.0.0.1 broker1` vào `/etc/hosts`. Với payment cần `PAYMENT_WEBHOOK_SECRET`; với analytics cần `CLICKHOUSE_URL`, `KAFKA_BROKERS_ADDR`.

## 5. Ghi chú từ lần deploy thật (Swarm)
- Traefik phải ≥ v3.6 trên Docker ≥ 29 (Traefik cũ dùng Docker API 1.24 bị daemon từ chối).
- `docker stack deploy` cần compose đã render, không có `name:` cấp cao nhất và cổng dạng số (`scripts/deploy.sh` lo).
- Frontend phải lắng nghe `0.0.0.0` (`HOSTNAME=0.0.0.0`) nếu không healthcheck `localhost` hỏng và Swarm giết nó.
- Docker Hub giới hạn tốc độ (429) có thể làm gián đoạn pull/build; thử lại hoặc dùng mirror.

## 6. Giám sát (Prometheus + Grafana)
`scripts/deploy.sh observability` triển khai `marketplace-obs`: Prometheus (scrape `tasks.<service>:8081/metrics`, cAdvisor, node-exporter, exporters, Traefik `:8082`), Grafana ở `https://grafana.<PUBLIC_HOST>` (đăng nhập `GRAFANA_ADMIN_*`, không anonymous), **chỉ xem – không có cảnh báo**. 5 dashboard provision bằng file (`deploy/grafana/dashboards`, không sửa trong UI): **Overview, Services (RED), Containers & hosts, Kafka·Postgres·Redis·Traefik, Analytics pipeline**. Cách đọc: [runbook.md](runbook.md). Chi tiết: [platform/05-analytics-monitoring.md](platform/05-analytics-monitoring.md).

## 7. Endpoint vận hành
Mọi service Go mở cổng HTTP **nội bộ** `ADMIN_PORT` (8081; không publish, không qua Traefik): `/health`=`/healthz` (liveness), `/ready`=`/readyz` (readiness), `/metrics`, `/version`; payment-service gắn thêm `POST /internal/payments/webhook` ở đó. Dịch vụ gRPC đăng ký `grpc.health.v1`. SIGTERM ⇒ not-ready `DRAIN_SECONDS` rồi dừng êm. Healthcheck Docker dùng `/healthz`; Traefik chỉ route tới replica gateway khi `/ready` (cổng 8081) = 200. Frontend có `/api/health|healthz|ready|readyz`. Chi tiết ma trận readiness: [platform/02](platform/02-health-ready-metrics.md).

## 8. Vận hành
- Scale: `docker service scale marketplace_api-gateway=3`. Service có consumer Kafka chạy nhiều replica được (consumer group chia partition; worker outbox dùng `SKIP LOCKED`; trùng sự kiện an toàn vì consumer idempotent). Riêng payment-service và order-service có worker theo thời gian (webhook/hết hạn, hết hạn đơn/ tự giao) – an toàn khi nhiều replica vì mọi chuyển trạng thái có khoá hàng & điều kiện.
- Postgres, Kafka và ClickHouse đều **một node** (single point of failure). Sao lưu: [runbook.md](runbook.md) §Sao lưu. HA thật: Postgres managed, Kafka 3 broker (RF 3, min ISR 2), ClickHouse replicated.
- Tắt `KAFKA_AUTO_CREATE_TOPICS_ENABLE` khi topic đã có (service tự tạo cái cần).
- CI chỉ build image, không push; thêm bước push registry trước khi deploy lên Swarm nhiều node.
