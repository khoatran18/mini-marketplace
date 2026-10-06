# Platform – thiết kế nền tảng và trạng thái triển khai

> **Trạng thái: backend ĐÃ TRIỂN KHAI phần lớn; hạ tầng triển khai CHƯA chạy thật.** Các tài liệu này ban đầu là thiết kế "chuẩn bị hạ tầng"; nay đã được đối chiếu với code. Mỗi tài liệu có banner trạng thái; **bảng "Trạng thái triển khai" tập trung ở [09-roadmap.md](09-roadmap.md)** (xong / một phần / chưa làm) cùng danh sách "Giới hạn đã biết". Khi tài liệu và code khác nhau, **code đúng**.
> **Không có gì liên quan tới AI ở đây.** Ý tưởng AI nằm ở một file nháp riêng: [../AI-DRAFT.md](../AI-DRAFT.md) – *nháp, không được dùng, không được code theo*.

## Mục lục
| # | Tài liệu | Nội dung | Trạng thái |
|---|---|---|---|
| 01 | [architecture.md](01-architecture.md) | Kiến trúc, 7 service, cổng, luồng dữ liệu | ✅ |
| 02 | [health-ready-metrics.md](02-health-ready-metrics.md) | **Chuẩn `/health` `/ready` `/metrics` `/version`**, ma trận readiness thực tế, metric đang có | ✅ (chưa chạy thật trên Swarm) |
| 03 | [data-model.md](03-data-model.md) | Trường mới, tracking event, ClickHouse (đối chiếu thiết kế ↔ thực tế) | ✅ trừ các mục "chưa làm" |
| 04 | [orders-payments.md](04-orders-payments.md) | Thanh toán **mô phỏng**, vòng đời đơn, tồn kho/reservation, thẻ test | ✅ |
| 05 | [analytics-monitoring.md](05-analytics-monitoring.md) | Analytics (ClickHouse), Grafana (5 dashboard), cảnh báo = chưa làm | ✅ analytics · 🟡 Grafana · ❌ cảnh báo |
| 06 | [api-contracts.md](06-api-contracts.md) | Bản đồ thiết kế → REST/gRPC/Kafka thực tế (tham chiếu đầy đủ: [../api.md](../api.md), [../events-kafka.md](../events-kafka.md)) | ✅ |
| 07 | [ui-design.md](07-ui-design.md) | Sitemap, wireframe, component, tracking SDK (không AI) | do nhánh UI |
| 08 | [infra-plan.md](08-infra-plan.md) | Container, env var, healthcheck, tài nguyên, CI | 🟡 (MinIO, update_config, e2e ❌) |
| 09 | [roadmap.md](09-roadmap.md) | **Trạng thái triển khai**, giới hạn đã biết, ADR, rủi ro | – |

## Quyết định đã chốt (từ chủ dự án) – và đã làm
- Đơn hàng **tách theo shop** khi checkout (`checkout_id` cha) ✅ (ADR-15).
- Thanh toán: **mô phỏng** (không cổng thật) ✅ – xem 04 (ADR-16).
- Analytics: **ClickHouse** ✅; tracking đi qua gRPC `IngestEvents` (không Kafka – ADR-13); báo cáo gom bảng fact, chưa có materialized view (ADR-14).
- Mọi service Go có **health + ready + metrics** ✅ – xem 02. Tên endpoint: hỗ trợ cả `/health` & `/healthz`, `/ready` & `/readyz`.
- Múi giờ cố định `Asia/Ho_Chi_Minh` ✅ (ADR-17); dark mode (nhánh UI); **không mã giảm giá**; **chưa có cảnh báo** (chỉ Grafana theo dõi metric) ✅ (ADR-12).
- AI: chưa làm gì, chưa tạo `ai-service`; chỉ có file nháp.

## Chưa làm (tóm tắt – chi tiết ở 09)
Consents DB + `DELETE /users/me/data` · reviews/wishlist · upload ảnh (MinIO) · khoá đăng nhập + `audit_log` · `/admin/system/metrics` (proxy Prometheus) · đối soát analytics (`DataHealth`) · cảnh báo · `update_config: start-first`/`wait-ready.sh` · e2e cho API mới.

## Tóm tắt kiến trúc
```
Browser ─▶ Traefik ─▶ frontend (Next.js)         ─▶ api-gateway ──gRPC──▶ auth · user · product · order · payment · analytics
   └ tracking SDK ─ POST /events ─▶ gateway ──gRPC IngestEvents──▶ analytics-service ─▶ ClickHouse
order/product/payment ─ outbox ─▶ Kafka ─▶ order · product · payment · analytics (consumer)
Mọi service Go: :PORT (gRPC/HTTP) + :8081 admin (health, ready, metrics, version) – chỉ trong mạng nội bộ
Postgres · Redis · Kafka · ClickHouse · Prometheus · Grafana · cAdvisor · node-exporter   (MinIO: chưa có)
```
