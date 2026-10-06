# 08 · Kế hoạch hạ tầng

Mở rộng `deploy/` theo cách hiện có. Secrets như cũ: giá trị thật ở `deploy/.env` (ignored), biến mới thêm vào `.env.example`.

> **Đã làm**: `deploy/observability.yml`, `deploy/prometheus/`, `deploy/grafana/`, healthcheck + limits cho service Go, Traefik `--ping` + metrics, `scripts/deploy.sh observability`, biến `GRAFANA_*`, `PROMETHEUS_RETENTION`, `ADMIN_BOOTSTRAP_*`, `MAX_BODY_BYTES`, `SYSTEM_HEALTH_TARGETS`. Chưa làm: ClickHouse, MinIO, payment/analytics.

## 1. Stack
| File | Thay đổi |
|---|---|
| `deploy/infra.yml` | thêm `clickhouse`, `minio`; healthcheck đủ cho mọi container (02 §7); giữ postgres/redis/kafka |
| `deploy/observability.yml` **(mới)** | prometheus, grafana, cadvisor (global), node-exporter (global), postgres/redis/kafka-exporter |
| `deploy/services.yml` | thêm `payment-service`, `analytics-service`; `HEALTHCHECK` + `stop_grace_period: 30s` + `update_config: start-first` + `resources.limits` cho **mọi** service; label Traefik healthcheck `/ready`; grafana sau basic-auth |
| `deploy/prometheus/prometheus.yml`, `deploy/grafana/provisioning/**` (datasource + dashboard JSON) | cấu hình dưới dạng file (không tạo tay trong UI) |
| `scripts/deploy.sh` | thứ tự infra → observability → services → `scripts/wait-ready.sh` (poll `/ready` mọi service, timeout) |

## 2. Container mới & tài nguyên khởi điểm
| Service | Image | Cổng nội bộ | Admin :8081 | RAM limit gợi ý |
|---|---|---|---|---|
| clickhouse | `clickhouse/clickhouse-server:24.x` | 8123, 9000, 9363(metrics) | – | 1–2 GB |
| minio | `minio/minio` | 9000, 9001 | – | 256 MB |
| payment-service | build Go | 50057 | ✔ | 192 MB |
| analytics-service | build Go | 50055 | ✔ | 256 MB |
| prometheus | `prom/prometheus` | 9090 | – | 512 MB |
| grafana | `grafana/grafana` | 3000 | – | 256 MB |
| cadvisor, node-exporter | global | 8080, 9100 | – | 128 MB mỗi cái |
| postgres/redis/kafka-exporter | | 9187, 9121, 9308 | – | 64 MB mỗi cái |
Tổng thêm ≈ 3–4,5 GB RAM. Cho máy yếu: `docker compose` profile `lite` (bỏ grafana & exporters; giữ prometheus, clickhouse).

## 3. Biến môi trường mới
| Biến | Dùng bởi | Ghi chú |
|---|---|---|
| `ADMIN_PORT` (mặc định 8081), `PPROF_ENABLED`, `DRAIN_SECONDS` (10), `READY_CACHE_SECONDS` (3), `READY_CHECK_TIMEOUT_MS` (1000) | mọi service Go | chuẩn 02 |
| `SERVICE_VERSION`, `GIT_COMMIT`, `BUILD_TIME` | mọi service | truyền qua `--build-arg`/ldflags trong `build-images.sh` |
| `ENV` (`dev|staging|prod`) | mọi service | bật/tắt công cụ dev (force payment, pprof) |
| `ADMIN_BOOTSTRAP_USERNAME/PASSWORD` | auth | tạo admin lần đầu nếu chưa có; không dùng cùng `SEED_DEMO_DATA` |
| `LOGIN_MAX_FAILURES`, `LOGIN_LOCK_MINUTES` | auth | |
| `CLICKHOUSE_URL/USER/PASSWORD/DB` | analytics | password bắt buộc |
| `ANALYTICS_SERVICE_ADDR`, `PAYMENT_SERVICE_ADDR` | gateway, order | |
| `PAYMENTS_MODE=mock`, `PAYMENT_WEBHOOK_SECRET`, `PAYMENT_CHAOS_RATE`, `MOCK_WEBHOOK_DELAY_MS`, `ORDER_PAYMENT_TTL_MIN` (15), `AUTO_DELIVER_DAYS`, `RETURN_WINDOW_DAYS` (7) | payment, order | |
| `SHIPPING_FLAT_FEE`, `FREE_SHIP_OVER` | order | |
| `PROMETHEUS_URL` | gateway | proxy metrics admin |
| `MINIO_ENDPOINT/ACCESS_KEY/SECRET_KEY/BUCKET`, `MAX_UPLOAD_MB` | product, gateway | |
| `EVENTS_RATE_LIMIT_PER_MINUTE`, `EVENTS_MAX_BATCH`, `MAX_BODY_BYTES` | gateway | |
| `IP_HASH_SALT` (+ xoay hằng ngày) | gateway | |
| `NEXT_PUBLIC_TRACKING_ENABLED` | frontend (build-time) | |
| `SEED_DEMO_DATA` | auth, product | mở rộng seed: danh mục, ≥ 100 sản phẩm có mô tả/ảnh mẫu, đơn lịch sử giả |

## 4. Mạng & định tuyến
- `marketplace-net` dùng chung. **Không publish** cổng admin :8081, ClickHouse, Prometheus, MinIO; Grafana qua Traefik có basic-auth.
- Traefik: gateway healthcheck `/ready` (cổng 8081); `--ping`, `--metrics.prometheus`.
- `TRUSTED_PROXIES` đúng (nếu sai, rate-limit và `ip_hash` sai).

## 5. Dữ liệu & sao lưu
Volume mới: `clickhouse-data`, `prometheus-data`, `grafana-data`, `minio-data`. `pg_dump` DB `postgres`. ClickHouse dựng lại được: Kafka retention 7 ngày + script `backfill` từ order/product/payment-service. Ảnh sản phẩm trong MinIO cần sao lưu (mc mirror).

## 6. CI/CD & kiểm thử
- Job Go cho 3 service mới (build, vet, test -race), kiểm tra **mọi service có đủ 4 endpoint** (test chung `healthtest`).
- `proto drift`: generate lại + `git diff --exit-code`.
- Frontend: `tsc`, build, test SDK tracking, test định dạng tiền/ múi giờ.
- E2E mới (`scripts/e2e`): events → ClickHouse; đặt hàng + thanh toán theo từng thẻ test → doanh thu đúng; hết hạn giữ chỗ → tồn trả lại; seller A không đọc dữ liệu seller B ở **mọi** endpoint analytics/orders; `/ready` 503 khi dừng Postgres mà container không restart; Prometheus target `up==1`; hạ tồn → xuất hiện trong danh sách "tồn thấp" của seller.
- `resilience.sh`: thêm ClickHouse down (`/events` vẫn 202, dashboard báo chậm), payment-service down, Redis down (gateway degraded).

## 7. Bảo mật hạ tầng
Prometheus/ClickHouse/MinIO không auth mặc định ⇒ chỉ trong overlay; Grafana basic-auth + mật khẩu mạnh; cAdvisor/node-exporter mount host chỉ đọc; upload kiểm MIME/size, lưu ngoài web root; secret webhook/ HMAC không log; admin bootstrap đổi mật khẩu ngay + rate-limit chặt; `audit_log` mọi thao tác admin.
