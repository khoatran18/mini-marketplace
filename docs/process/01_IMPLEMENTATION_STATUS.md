# Đã làm được gì (trạng thái triển khai)

> Bản tóm tắt để đọc nhanh. Bảng chi tiết từng hạng mục (có ghi chú code) nằm ở [../design/platform/09_ROADMAP_STATUS.md](../design/platform/09_ROADMAP_STATUS.md). Khi tài liệu và code khác nhau, **code đúng**.
> Chú thích: ✅ xong · 🟡 một phần · ❌ chưa làm.

## 1. Dòng thời gian (theo lịch sử git)

| Giai đoạn | Nội dung | Commit tiêu biểu |
|---|---|---|
| 0. Bản đầu | Microservices Go + gateway + frontend Next.js, Docker Swarm + Traefik, Postgres HA (sau đó bỏ Patroni), Swagger | `2a4dd77` … `56a0da2` |
| 1. Dọn repo & làm cứng | Bỏ secrets/binaries, layout deploy mới, sửa logic order/auth/product, test + CI, khép các lỗ hổng phân quyền, e2e trên Swarm | `c01ace5`, `67de823`, `d92bfd8`, `21fb766` |
| 2. Nền vận hành | Cổng admin `:8081` (`/health /ready /metrics /version`) ở mọi service; role `admin`; Prometheus + Grafana | `17197a0`, `48c5ce0` |
| 3. Catalog & tồn kho | Danh mục, trường sản phẩm mới, tìm kiếm không dấu, sổ cái tồn kho, outbox `product.changed` | `36e166e` |
| 4. Đơn hàng | Giỏ hàng server, checkout tách theo store, máy trạng thái mở rộng, địa chỉ, hết hạn giữ hàng | `fdc4234` |
| 5. Thanh toán mô phỏng | `payment-service` (mockpay), webhook ký HMAC, hoàn tiền, dữ liệu demo | `cf89e52` |
| 6. Analytics | `analytics-service` + ClickHouse, `POST /events`, báo cáo seller / admin | `f72866a`, `934245d` |
| 7. Giao diện | Frontend trên API mới (catalog, giỏ, checkout, console seller / admin, dark mode, tracking, dashboard analytics) | `f73facf`, `6a389cb` |
| 8. Tài liệu | Cập nhật docs theo code; gom về thư mục `docs/` gốc (lần này) | `075b489`, `09104fb` |

## 2. Tính năng theo vai trò

### Buyer
✅ đăng ký / đăng nhập / refresh / đổi mật khẩu · ✅ xem & tìm kiếm catalog (không dấu), lọc danh mục · ✅ giỏ hàng (gộp khi đăng nhập), xem trước checkout · ✅ địa chỉ giao hàng · ✅ đặt hàng idempotent, tách đơn theo store · ✅ thanh toán mô phỏng (thẻ / ví / chuyển khoản / COD) · ✅ hủy đơn, xác nhận đã nhận, yêu cầu trả hàng, theo dõi timeline · ❌ review, wishlist · ❌ upload ảnh (ảnh chỉ là URL)

### Seller
✅ tạo store, CRUD sản phẩm, trạng thái sản phẩm, điều chỉnh tồn kho + sổ cái · ✅ hộp thư đơn: ship / deliver / duyệt-từ chối trả hàng · ✅ analytics của store: summary, timeseries, top products, funnel, low-stock

### Admin
✅ bootstrap qua biến môi trường · ✅ analytics toàn sàn (+ traffic, payments, search terms, data-health) · ✅ quản lý danh mục · ✅ xem thanh toán, hoàn tiền thủ công · ✅ `/admin/system/health` · ❌ `/admin/users`, `/admin/stores` (duyệt / khóa), audit log, proxy metrics

### Nền tảng
✅ 7 service Go + gateway 2 replica · ✅ outbox giao dịch + consumer idempotent · ✅ health / ready / metrics mọi service · ✅ Prometheus + 5 dashboard Grafana · ✅ tracking clickstream có consent, che PII, hash IP xoay ngày · ✅ CI (build, vet, test 7 service, ClickHouse 24.8 thật) · ❌ cảnh báo (cố ý, ADR-12) · ❌ đối soát Postgres ↔ ClickHouse · ❌ backfill ClickHouse · ❌ materialized view (cố ý, ADR-14)

## 3. Kiểm thử
Mỗi service là module Go riêng, có test đơn vị / tích hợp (tích hợp DB bị skip nếu không đặt `TEST_POSTGRES_DSN` / `TEST_CLICKHOUSE_URL`). Phạm vi test từng khu vực: [../instruction/04_TESTING.md](../instruction/04_TESTING.md) §2.

## 4. Chưa từng được kiểm chứng
Docker, Swarm, Traefik, ClickHouse server, Prometheus, Grafana **chưa từng chạy trong môi trường soạn code / tài liệu** (chỉ kiểm cú pháp; SQL ClickHouse được thử trên chDB, ClickHouse 24.8 thật chỉ chạy ở CI). E2E `scripts/e2e/*.mjs` viết cho API cũ nên sẽ hỏng. Danh sách đầy đủ: [../planning/01_OPEN_ISSUES.md](../planning/01_OPEN_ISSUES.md).

## 5. AI
Chưa có `ai-service` hay endpoint AI nào. Mới có tài liệu nghiên cứu: [../ai/README.md](../ai/README.md) và [../dataset/README.md](../dataset/README.md).
