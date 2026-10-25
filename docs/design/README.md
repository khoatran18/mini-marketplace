# Thiết kế chi tiết

| File | Nội dung |
|---|---|
| [01_ARCHITECTURE.md](01_ARCHITECTURE.md) | 7 service Go, luồng đặt hàng, tracking, nơi dữ liệu nằm |
| [02_DATA_MODEL.md](02_DATA_MODEL.md) | Bảng Postgres, bảng ClickHouse, ngữ nghĩa tồn kho / trạng thái / tiền |
| [03_EVENTS_KAFKA.md](03_EVENTS_KAFKA.md) | Topic, producer / consumer, payload, outbox |
| [04_API_REFERENCE.md](04_API_REFERENCE.md) | Toàn bộ REST gateway |
| [05_SECURITY.md](05_SECURITY.md) | Xác thực, phân quyền, riêng tư, khoảng trống |
| [06_PROTOBUF.md](06_PROTOBUF.md) | Sinh lại `.pb.go` offline |
| [07_DATA_PIPELINE.md](07_DATA_PIPELINE.md) | Đường ống dữ liệu: datagen (D5), backfill / đối soát (D4), export (D3) |
| [platform/](platform/README.md) | Thiết kế nền tảng + trạng thái triển khai (01–09) |
