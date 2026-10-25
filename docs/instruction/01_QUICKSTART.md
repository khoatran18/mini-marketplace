# Hướng dẫn bắt đầu: cần gì, chạy lệnh gì, xem ở đâu

> Đường chạy được hỗ trợ là **Docker Swarm trên một máy** (3 stack). Các bước Docker / Swarm / Traefik / ClickHouse / Grafana **chưa từng được chạy trong môi trường soạn tài liệu** (xem [../planning/01_OPEN_ISSUES.md](../planning/01_OPEN_ISSUES.md)); lần đầu hãy theo dõi `docker service logs` từng service.

## 1. Cần cài gì

| Thứ | Dùng cho | Ghi chú |
|---|---|---|
| Docker (Engine ≥ 24) + Docker Compose v2 | chạy cả hệ thống | `scripts/deploy.sh` dùng `docker compose config` và `docker stack deploy` |
| Go 1.25 | build / test từng service | mỗi service là một module riêng (`services/<tên>/go.mod`) |
| Node.js + npm | frontend (Next.js 14) | chỉ cần khi chạy frontend ngoài Docker |
| `openssl` | `scripts/gen-dev-certs.sh` | tạo chứng chỉ TLS tự ký |
| Python 3 (`pip install chdb`) | tùy chọn: test analytics khi không có Docker | `scripts/dev/chdb_server.py` |

## 2. Chạy cả hệ thống (Swarm)

```bash
cp deploy/.env.example deploy/.env      # điền các biến bắt buộc (bảng dưới)
./scripts/gen-dev-certs.sh              # TLS tự ký cho *.marketplace.swarm.localhost
API_BASE_URL=https://api.marketplace.swarm.localhost ./scripts/build-images.sh   # build 7 service Go + frontend
docker swarm init
./scripts/deploy.sh                     # = infra → observability → services (hoặc: deploy.sh infra|observability|services)
```

Thêm vào `/etc/hosts`:

```
127.0.0.1 marketplace.swarm.localhost api.marketplace.swarm.localhost dashboard.marketplace.swarm.localhost grafana.marketplace.swarm.localhost
```

Biến bắt buộc trong `deploy/.env` (thiếu là stack không lên):

| Biến | Gợi ý |
|---|---|
| `POSTGRES_PASSWORD` | tùy ý |
| `JWT_SECRET` | ≥ 16 ký tự |
| `TRAEFIK_DASHBOARD_USERS` | htpasswd, nhớ đổi mỗi `$` thành `$$` |
| `GRAFANA_ADMIN_PASSWORD` | tùy ý |
| `CLICKHOUSE_PASSWORD` | tùy ý |
| `PAYMENT_WEBHOOK_SECRET` | ≥ 16 ký tự (thiếu thì payment-service không khởi động) |
| `ADMIN_BOOTSTRAP_USERNAME` / `ADMIN_BOOTSTRAP_PASSWORD` | muốn có admin thì đặt: 3–16 ký tự `[a-zA-Z0-9_]`, mật khẩu ≥ 8 |
| `SEED_DEMO_DATA=true` | muốn có tài khoản và ~100 sản phẩm demo (đừng bật ở production) |

Danh sách đầy đủ biến môi trường: [02_DEPLOYMENT.md](02_DEPLOYMENT.md) §3.

## 3. Giao diện và địa chỉ

| Cái gì | Địa chỉ | Đăng nhập | Ghi chú |
|---|---|---|---|
| **Website** (buyer / seller / admin) | `https://marketplace.swarm.localhost` | tài khoản bên dưới | trình duyệt sẽ cảnh báo cert tự ký |
| **REST API** (gateway) | `https://api.marketplace.swarm.localhost` | JWT | tài liệu route: [../design/04_API_REFERENCE.md](../design/04_API_REFERENCE.md); Swagger ở `services/api-gateway/docs` **đã cũ** |
| **Grafana** (5 dashboard, chỉ xem) | `https://grafana.marketplace.swarm.localhost` | `GRAFANA_ADMIN_USER` / `GRAFANA_ADMIN_PASSWORD` | thư mục *Marketplace*; cách đọc: [03_RUNBOOK.md](03_RUNBOOK.md) §2 |
| **Traefik dashboard** | `https://dashboard.marketplace.swarm.localhost` | basic auth (`TRAEFIK_DASHBOARD_USERS`) | xem router / service đang có |
| Trạng thái service (admin) | `https://marketplace.swarm.localhost/admin/system` | admin | gom `/ready` của 7 service |
| Prometheus, ClickHouse, Postgres, Redis, Kafka, cổng `:8081` | **không publish** | – | chỉ trong mạng overlay `marketplace-net`; vào bằng `docker exec` (xem dưới) |

Hostname lấy từ `PUBLIC_HOST` (mặc định `marketplace.swarm.localhost`).

### Tài khoản

| Tài khoản | Cách có | Ghi chú |
|---|---|---|
| `buyer1` / `password` | `SEED_DEMO_DATA=true` | đặt hàng, thanh toán thử |
| `seller1` / `password` | `SEED_DEMO_DATA=true` | seed **không tạo store**: đăng nhập seller rồi tạo store trước khi quản lý sản phẩm |
| admin | `ADMIN_BOOTSTRAP_*` | không đăng ký công khai được; đổi mật khẩu rồi bỏ biến sau lần đăng nhập đầu |
| tài khoản mới | `/register` | chọn role buyer hoặc seller |

### Các trang chính của frontend

| Vai trò | Trang |
|---|---|
| Mọi người | `/products`, `/products/[id]`, `/categories/[slug]`, `/login`, `/register` |
| Buyer | `/cart`, `/checkout`, `/pay/[id]`, `/orders`, `/orders/[id]`, `/profile` |
| Seller | `/seller` (tổng quan), `/seller/products`, `/seller/inventory`, `/seller/orders`, `/seller/analytics` |
| Admin | `/admin`, `/admin/analytics`, `/admin/orders`, `/admin/payments`, `/admin/categories`, `/admin/system` |
| Dev | `/dev/tracking` (chỉ hiện khi `NEXT_PUBLIC_APP_ENV=dev`) |

### Thẻ test thanh toán (mô phỏng)

| Số thẻ | Kết quả |
|---|---|
| `4242 4242 4242 4242` | thành công ngay |
| `4000 0000 0000 0002` | bị từ chối |
| `4000 0000 0000 9995` | không đủ tiền |
| `4000 0000 0000 3220` | 3-D Secure: nhập OTP `123456` |
| `4000 0000 0000 0341` | "timeout": thành công sau ~30 s qua webhook |
Đủ bảng thẻ và luồng: [../design/platform/04_ORDERS_PAYMENTS.md](../design/platform/04_ORDERS_PAYMENTS.md) §3.2.

## 4. Kiểm tra hệ thống đã lên

```bash
docker stack services marketplace-infra     # mọi dòng N/N
docker stack services marketplace-obs
docker stack services marketplace
docker run --rm --network marketplace-net alpine wget -qO- http://api-gateway:8081/ready     # "ready"
docker service logs --tail 100 -f marketplace_<service>   # ví dụ marketplace_order-service
```
Rồi: đăng nhập admin → `/admin/system` toàn `ready`; Grafana → *Marketplace* → Overview có số liệu. Sinh traffic giả cho dashboard: `python3 scripts/dev/gen_traffic.py --api https://api.marketplace.swarm.localhost --visitors 200 --products 1-60`.

## 5. Phát triển từng phần

```bash
# một service Go (chạy từ services/<tên>)
go build ./... && go vet ./... && go test ./...

# test cần Postgres / ClickHouse (không có thì bị skip)
export TEST_POSTGRES_DSN="host=localhost port=5432 user=postgres password=postgres sslmode=disable"
export TEST_CLICKHOUSE_URL="http://127.0.0.1:8123"
# không có Docker: pip install chdb && python3 scripts/dev/chdb_server.py --port 8123

# sinh lại .pb.go offline sau khi sửa .proto
scripts/protogen/gen.sh payment-service

# frontend (cần gateway chạy ở NEXT_PUBLIC_API_BASE_URL, mặc định http://localhost:8080)
cd apps/frontend && npm install && npm run dev      # http://localhost:3000
```
Chi tiết test: [04_TESTING.md](04_TESTING.md). Chạy từng service Go ngoài Docker cần file `.env` (mẫu `services/<tên>/cmd/.env.example`; payment và analytics chưa có file mẫu) và Postgres / Redis / Kafka tự công bố cổng — `infra.yml` hiện **không publish cổng nào**, nên cách đơn giản nhất vẫn là chạy trong Swarm.

## 6. Gặp sự cố

Đơn kẹt, outbox, Kafka lag, ClickHouse, thanh toán: [03_RUNBOOK.md](03_RUNBOOK.md). Quy ước viết code (identity chỉ từ JWT, tiền, outbox, middleware theo route…): [../../CLAUDE.md](../../CLAUDE.md).
