# Mục lục tài liệu

> Tài liệu phản ánh **code hiện tại** (backend đã triển khai tới payment-service + analytics-service). Khi code và tài liệu khác nhau, code đúng. Phần "chưa làm" được ghi rõ.

| Tài liệu | Nội dung |
|---|---|
| [architecture.md](architecture.md) | 7 service Go, luồng đặt hàng → thanh toán → giao hàng, tracking/analytics, cổng, bố cục repo |
| [data-model.md](data-model.md) | Bảng Postgres từng service, bảng ClickHouse, ngữ nghĩa tồn kho/ trạng thái đơn/ payment, tiền |
| [api.md](api.md) | **Toàn bộ REST gateway**: route, role, request/response, lỗi, idempotency, phân trang, thẻ test, analytics, tracking |
| [events-kafka.md](events-kafka.md) | Topic, producer/ consumer + group, payload, outbox `domain_events`, idempotency |
| [security.md](security.md) | Xác thực/ phân quyền, bí mật, privacy tracking, khoảng trống đã biết |
| [deployment.md](deployment.md) | 3 stack compose, checklist deploy đầu, ClickHouse + analytics, biến môi trường, vận hành |
| [testing.md](testing.md) | Chạy test, `TEST_POSTGRES_DSN`/`TEST_CLICKHOUSE_URL`, chDB shim, phạm vi, bảng truy cập route, thẻ test |
| [runbook.md](runbook.md) | Đọc Grafana, readiness, sự cố thường gặp (đơn kẹt, outbox, Kafka lag, payment, ClickHouse), sao lưu, múi giờ |
| [protobuf.md](protobuf.md) | Sinh lại `.pb.go` offline (`scripts/protogen/gen.sh`), lịch sử đổi proto |
| [decisions.md](decisions.md) | ADR-1…25 |
| [roadmap.md](roadmap.md) | Nợ kỹ thuật + tính năng chưa làm |
| [platform/README.md](platform/README.md) | **Thiết kế nền tảng + trạng thái triển khai** (health/ready/metrics, thanh toán mô phỏng, vòng đời đơn, analytics, API, hạ tầng, lộ trình). `platform/07-ui-design.md` do nhánh UI quản lý |
| [AI-DRAFT.md](AI-DRAFT.md) | ⚠️ **Nháp ý tưởng AI – không dùng, không code theo** |

OpenAPI sinh vào `services/api-gateway/docs/` (`swag init -g cmd/main.go -o docs --parseInternal`, chạy từ `services/api-gateway`) hiện **đã cũ**; [api.md](api.md) là tài liệu đầy đủ.
