# 09 · Kế hoạch hạ tầng

Mở rộng `deploy/` theo cách hiện có (compose cho Swarm). Không đổi quy ước secrets: mọi giá trị thật nằm ở `deploy/.env` (ignored), thêm biến mới vào `.env.example`.

## 1. Stack
| File | Thay đổi |
|---|---|
| `deploy/infra.yml` | Postgres → image **pgvector** (hoặc cài extension) · thêm `clickhouse` · (tuỳ chọn) `minio` · giữ redis/kafka |
| `deploy/observability.yml` **(mới)** | prometheus, alertmanager, grafana, cadvisor (global), node-exporter (global), postgres/redis/kafka-exporter |
| `deploy/services.yml` | thêm `analytics-service`, `alert-service`, `ai-service`; label Traefik cho grafana; cấu hình SSE cho `/ai` |
| `scripts/deploy.sh` | thứ tự: infra → observability → services |
| `deploy/prometheus/` `deploy/grafana/provisioning/` `deploy/alertmanager/` **(mới)** | cấu hình scrape, dashboard dạng file (provisioning), rule |

## 2. Container mới & tài nguyên (khởi điểm, máy dev ~ 8 GB trở lên)
| Service | Image / build | Port nội bộ | RAM giới hạn gợi ý | Phụ thuộc |
|---|---|---|---|---|
| clickhouse | `clickhouse/clickhouse-server:24.x` | 8123 HTTP, 9000 native | 1–2 GB | volume `clickhouse-data` |
| analytics-service | `services/analytics-service` (Go) | 50055 | 256 MB | Kafka, ClickHouse, Redis |
| alert-service | `services/alert-service` (Go) | 50056 | 256 MB | Postgres(ai), ClickHouse, Prometheus, Redis |
| ai-service | `services/ai-service` (Python) | 50057 gRPC, 8000 HTTP (health/metrics) | 512 MB – 2 GB (tuỳ model local) | Postgres(ai), Redis, Kafka, gateway |
| prometheus | `prom/prometheus` | 9090 | 512 MB | scrape targets |
| alertmanager | `prom/alertmanager` | 9093 | 64 MB | → alert-service |
| grafana | `grafana/grafana` | 3000 | 256 MB | prometheus, clickhouse plugin (tuỳ chọn) |
| cadvisor / node-exporter | mode `global` | 8080 / 9100 | 128 MB mỗi cái | host mounts (chỉ đọc) |
| minio (tuỳ chọn) | `minio/minio` | 9000/9001 | 256 MB | volume |
Tổng thêm ≈ 4–6 GB RAM. Máy yếu: bỏ Grafana/exporter không cần, hoặc chạy compose "lite" (không ClickHouse → analytics tạm dùng Postgres; xem ADR-A2 phương án lùi).

## 3. Biến môi trường mới (thêm vào `.env.example`)
| Biến | Dùng bởi | Ghi chú |
|---|---|---|
| `CLICKHOUSE_URL/USER/PASSWORD/DB` | analytics, alert | password bắt buộc |
| `AI_POSTGRES_DSN` | ai, alert | DB `ai` riêng |
| `AI_INTERNAL_TOKEN` | gateway, ai, alert, analytics | ≥ 32 ký tự |
| `AI_SERVICE_ADDR`, `ANALYTICS_SERVICE_ADDR`, `ALERT_SERVICE_ADDR` | gateway | |
| `LLM_PROVIDER`, `LLM_API_KEY`, `LLM_MODEL`, `EMBEDDING_MODEL`, `EMBEDDING_DIM` | ai-service | **bạn đặt**; key không bao giờ commit; qua Docker secret nếu có thể |
| `AI_DAILY_BUDGET_USD`, `AI_RATE_LIMIT_PER_MINUTE`, `AI_MAX_TOOL_STEPS`, `AI_TURN_TIMEOUT_S` | gateway/ai | |
| `PROMETHEUS_URL` | gateway | proxy metrics admin |
| `ALERTMANAGER_WEBHOOK_SECRET` | alert, alertmanager | |
| `SMTP_*`, `ALERT_WEBHOOK_URL` | alert | tuỳ chọn |
| `MINIO_*` / `UPLOAD_DIR`, `MAX_UPLOAD_MB` | gateway, ai | |
| `NEXT_PUBLIC_TRACKING_ENABLED`, `NEXT_PUBLIC_AI_ENABLED` | frontend (build-time, như `API_BASE_URL`) | |
| `ADMIN_BOOTSTRAP_USERNAME/PASSWORD` | auth | tạo admin lần đầu nếu chưa có; **không** dùng cùng `SEED_DEMO_DATA` |
| `TRACKING_IP_SALT_ROTATE` | gateway | xoay muối băm IP |
| `SEED_DEMO_DATA` | giữ nguyên; mở rộng seed: danh mục, mô tả, ảnh mẫu | |

## 4. Mạng & định tuyến
- Mạng `marketplace-net` dùng chung. **Không** publish port của ClickHouse/ai-service/Prometheus ra host (chỉ qua Traefik khi cần: `grafana.<host>` có basic-auth).
- Traefik: route `/ai/*` (gateway) – tắt compression/buffering, tăng `respondingTimeouts.readTimeout` cho SSE; `/events` giới hạn body ở gateway.
- `TRUSTED_PROXIES` phải đúng (nếu không, rate-limit `/events` và `ip_hash` đều sai – xem security.md).

## 5. Dữ liệu & sao lưu
Volume mới: `clickhouse-data`, `prometheus-data`, `grafana-data`, `minio-data`. Sao lưu: `pg_dump` cả DB `postgres` và `ai`; ClickHouse có thể **dựng lại** từ Kafka (retention 7 ngày) + Postgres cho fact đơn hàng ⇒ viết script `backfill` (đọc đơn từ order-service) — không coi ClickHouse là nguồn sự thật cho tiền. Kafka: tăng retention topic tracking lên 7 ngày để replay.

## 6. CI/CD
- Thêm job cho `analytics-service`, `alert-service` (build/vet/test như service Go khác), `ai-service` (ruff + pytest + kiểm tra proto đồng bộ), frontend (`tsc`, build, test SDK tracking).
- Kiểm tra *proto drift*: sinh lại và `git diff --exit-code`.
- E2E (`scripts/e2e`): thêm kịch bản: gửi events → có trong ClickHouse; đặt hàng → xuất hiện trong doanh thu; hạ tồn → alert; tắt `ai-service` → UI vẫn chạy (fallback); shop A không đọc được số liệu shop B qua mọi tool endpoint.
- Resilience (`resilience.sh`): ClickHouse down ⇒ `/events` vẫn 202 (Kafka đệm), dashboard báo "dữ liệu chậm"; ai-service down ⇒ chat báo lỗi thân thiện, gợi ý rơi về trending.

## 7. Bảo mật hạ tầng (bổ sung security.md)
- Prometheus/Alertmanager/ClickHouse không có auth mặc định ⇒ chỉ trong overlay; Grafana đặt sau basic-auth + tài khoản mạnh.
- cAdvisor/node-exporter cần mount host (đọc) ⇒ chỉ chạy với quyền tối thiểu, không publish.
- Key LLM chỉ có trong `ai-service` (và Docker secret); không đưa vào log/ audit.
- Giới hạn egress của `ai-service` (chỉ tới nhà cung cấp LLM) nếu môi trường cho phép.
- Upload: kiểm tra MIME/size, quét ký tự điều khiển, không thực thi; lưu ngoài web root.
- `admin` bootstrap: đổi mật khẩu ngay lần đầu, bật rate-limit chặt cho đăng nhập admin.
