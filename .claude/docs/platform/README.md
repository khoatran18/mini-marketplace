# Platform – thiết kế nền tảng (giai đoạn "chuẩn bị hạ tầng")

> **Trạng thái: THIẾT KẾ – chưa có code.** Tài liệu này mô tả những gì sẽ **bổ sung vào marketplace hiện tại** để nó vận hành được, quan sát được và có đủ dữ liệu: health/ready/metrics, thanh toán mô phỏng, vòng đời đơn, tồn kho, tracking + ClickHouse, analytics, cảnh báo, giao diện quản trị.
> **Không có gì liên quan tới AI ở đây.** Ý tưởng AI nằm ở một file nháp riêng: [../AI-DRAFT.md](../AI-DRAFT.md) – *nháp, không được dùng, không được code theo*.

## Mục lục
| # | Tài liệu | Nội dung |
|---|---|---|
| 01 | [architecture.md](01-architecture.md) | Kiến trúc đích, service mới, cổng/port, luồng dữ liệu |
| 02 | [health-ready-metrics.md](02-health-ready-metrics.md) | **Chuẩn `/healthz` `/readyz` `/metrics` `/version`** cho mọi service + docker/Traefik/Swarm + checklist từng dependency |
| 03 | [data-model.md](03-data-model.md) | Trường mới (product, order, payment, inventory, user…), tracking event, ClickHouse |
| 04 | [orders-payments.md](04-orders-payments.md) | Thanh toán **mô phỏng** (`payment-service`), vòng đời đơn, tồn kho/reservation, kịch bản test |
| 05 | [analytics-alerting.md](05-analytics-alerting.md) | Analytics (doanh thu, traffic), metrics hệ thống CPU/RAM, **rule cảnh báo**, vòng đời alert |
| 06 | [api-contracts.md](06-api-contracts.md) | Toàn bộ REST mới/đổi, gRPC nội bộ, Kafka topic mới |
| 07 | [ui-design.md](07-ui-design.md) | Sitemap, wireframe, component, tracking SDK (không AI) |
| 08 | [infra-plan.md](08-infra-plan.md) | Container mới, env var, healthcheck, tài nguyên, CI/E2E |
| 09 | [roadmap.md](09-roadmap.md) | Giai đoạn + nghiệm thu, ADR, rủi ro, câu hỏi còn mở |

## Quyết định đã chốt (từ chủ dự án)
- Thanh toán: **mô phỏng** (không cổng thật) – xem 04.
- Analytics: **ClickHouse**.
- Mọi service/container có **health + ready + metrics** – xem 02.
- AI: chưa làm gì, chưa tạo `ai-service`; chỉ có file nháp. Khi làm sẽ dùng Python (ghi trong nháp).

## Tóm tắt kiến trúc
```
Browser ─▶ Traefik ─▶ frontend (Next.js)         ─▶ api-gateway ──gRPC──▶ auth · user · product · order
   └ tracking SDK ─ POST /events ─▶ gateway ─▶ Kafka `tracking.events`            payment* · analytics* · alert*
Mọi service: :PORT (gRPC/HTTP) + :8081 admin (healthz, readyz, metrics, version) – chỉ trong mạng nội bộ
Postgres · Redis · Kafka · ClickHouse* · MinIO* · Prometheus* · Alertmanager* · Grafana* · cAdvisor* · node-exporter*
(*) = mới
```
