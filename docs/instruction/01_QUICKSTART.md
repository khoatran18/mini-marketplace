# Hướng dẫn bắt đầu: cần gì, chạy lệnh gì, xem ở đâu

> Đường chạy được hỗ trợ là **Docker Swarm trên một máy** (3 stack). Các bước Docker / Swarm / Traefik / ClickHouse / Grafana **chưa từng được chạy trong môi trường soạn tài liệu** (xem [../planning/01_OPEN_ISSUES.md](../planning/01_OPEN_ISSUES.md)); lần đầu hãy theo dõi `docker service logs` từng service.

## 1. Cần cài gì

| Thứ | Dùng cho | Ghi chú |
|---|---|---|
| Docker (Engine ≥ 24) + Docker Compose v2 | chạy cả hệ thống | `scripts/deploy.sh` dùng `docker compose config` và `docker stack deploy` |
| Go 1.25 | build / test từng service | mỗi service là một module riêng (`services/<tên>/go.mod`) |
| Node.js + npm | frontend (Next.js 14) | chỉ cần khi chạy frontend ngoài Docker |
| `openssl` | `scripts/gen-dev-certs.sh` | chỉ khi bật HTTPS (`PUBLIC_SCHEME=https`) |
| Python 3 (`pip install chdb`) | tùy chọn: test analytics khi không có Docker | `scripts/dev/chdb_server.py` |

## 2. Chạy cả hệ thống (Swarm)

```bash
cp deploy/.env.example deploy/.env      # điền các biến bắt buộc (bảng dưới)
# (mặc định chạy HTTP, không cần cert. Muốn HTTPS: PUBLIC_SCHEME=https trong deploy/.env + ./scripts/gen-dev-certs.sh)
./scripts/build-images.sh   # build 7 service Go + frontend
docker swarm init
./scripts/deploy.sh                     # = infra → observability → services (hoặc: deploy.sh infra|observability|services)
```

Thêm vào `/etc/hosts`:

```
127.0.0.1 marketplace.swarm.localhost api.marketplace.swarm.localhost dashboard.marketplace.swarm.localhost grafana.marketplace.swarm.localhost
```

Biến bắt buộc trong `deploy/.env` (thiếu là stack không lên). `deploy/.env.example` đã điền sẵn giá trị local, copy là chạy được; bảng dưới chỉ để biết cần đổi gì khi lên prod:

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
| **Website** (buyer / seller / admin) | `https://marketplace.swarm.localhost` | tài khoản bên dưới | mặc định HTTP (`PUBLIC_SCHEME=http`). HTTPS tự ký: đặt `PUBLIC_SCHEME=https`, rồi phải import `deploy/traefik/certs/local.crt` vào trình duyệt (lệnh in ra cuối `gen-dev-certs.sh`), nếu không web báo "Failed to fetch" |
| **REST API** (gateway) | `http://api.marketplace.swarm.localhost` | JWT | tài liệu route: [../design/04_API_REFERENCE.md](../design/04_API_REFERENCE.md); Swagger ở `services/api-gateway/docs` **đã cũ** |
| **Grafana** (11 dashboard, gồm Logs, chỉ xem) | `http://grafana.marketplace.swarm.localhost` | `admin` / `admin` (`GRAFANA_ADMIN_USER` / `GRAFANA_ADMIN_PASSWORD`) | thư mục *Marketplace*; cách đọc: [03_RUNBOOK.md](03_RUNBOOK.md) §2 |
| **Traefik dashboard** | `http://dashboard.marketplace.swarm.localhost` | basic auth `admin` / `admin` (`TRAEFIK_DASHBOARD_USERS`) | xem router / service đang có |
| Trạng thái service (admin) | `https://marketplace.swarm.localhost/admin/system` | admin | gom `/ready` của 7 service |
| Prometheus, ClickHouse, Postgres, Redis, Kafka, cổng `:8081` | **không publish** | – | chỉ trong mạng overlay `marketplace-net`; vào bằng `docker exec` (xem dưới) |

Hostname lấy từ `PUBLIC_HOST` (mặc định `marketplace.swarm.localhost`). Mọi địa chỉ đều đi qua Traefik cổng 80 (HTTP; khi `PUBLIC_SCHEME=https` thì cổng 443 và cổng 80 tự chuyển sang HTTPS) nên **không cần ghi port**. Mật khẩu ở bảng trên là mặc định của `deploy/.env.example` (chỉ cho local; lên prod phải đổi).

### Tài khoản

| Tài khoản | Cách có | Ghi chú |
|---|---|---|
| `buyer1` / `password` | `SEED_DEMO_DATA=true` | đặt hàng, thanh toán thử |
| `seller1` / `password` | `SEED_DEMO_DATA=true` | seed **không tạo store**: đăng nhập seller rồi tạo store trước khi quản lý sản phẩm |
| `admin` / `Admin12345` | `ADMIN_BOOTSTRAP_*` (đã điền sẵn trong `.env.example`) | không đăng ký công khai được; đổi mật khẩu rồi bỏ biến sau lần đăng nhập đầu |
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
Rồi: đăng nhập admin → `/admin/system` toàn `ready`; Grafana → *Marketplace* → Overview có số liệu. Sinh traffic giả cho dashboard: `python3 scripts/dev/gen_traffic.py --api http://api.marketplace.swarm.localhost --visitors 200 --products 1-60`.

## 5. Phát triển từng phần (sửa và test một service, không cần dựng cả Swarm)

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

## 6. Restart và triển khai lại nhanh

Không cần gỡ cả stack mỗi khi sửa gì. Các lệnh dưới chạy từ thư mục gốc repo; tên service Swarm có dạng `marketplace_<tên>` (app), `marketplace-infra_<tên>`, `marketplace-obs_<tên>`.

| Tình huống | Lệnh | Mất dữ liệu? |
|---|---|---|
| Sửa code **một** service Go, chạy lại ngay | `./scripts/build-images.sh order-service && docker service update --force marketplace_order-service` | không |
| Sửa frontend (hoặc đổi `PUBLIC_SCHEME`/`PUBLIC_HOST`: URL API được nướng vào frontend lúc build) | `./scripts/build-images.sh frontend && docker service update --force marketplace_frontend` | không |
| Sửa nhiều service | `./scripts/build-images.sh api-gateway auth-service` (không tham số = build tất cả) rồi `docker service update --force` từng service | không |
| Đổi `deploy/.env` hoặc file `deploy/*.yml` | `./scripts/deploy.sh services` (hoặc `infra`, `observability`); không tham số = cả ba | không |
| Một service treo / hành xử lạ, không đổi code | `docker service update --force marketplace_<tên>` | không |
| Restart **toàn bộ** service ứng dụng (không đụng DB/Kafka) | `for s in $(docker service ls --format '{{.Name}}' \| grep '^marketplace_'); do docker service update --force --detach=true $s; done` | không |
| Vừa restart Postgres / Redis / Kafka (`marketplace-infra_*`) | restart lại service ứng dụng bằng lệnh ở trên (chúng có thể đã lỗi kết nối lúc đó) | không |
| Tắt / bật tạm một service (mô phỏng sự cố) | `docker service scale marketplace_payment-service=0` rồi `=1` | không |
| Chờ mọi thứ sẵn sàng | `./scripts/wait-ready.sh` | không |
| Làm lại từ đầu, DB rỗng, seed demo chạy lại | `scripts/teardown.sh --volumes -y && ./scripts/deploy.sh` | **có** |

Image dùng tag `latest` trên một node nên `docker service update --force` sẽ chạy lại với image vừa build. Nếu có nhiều node phải đẩy image lên registry (`scripts/push-images.sh`).

Kiểm tra nhanh sau khi restart:
```bash
docker service ls                                      # cột REPLICAS phải là 1/1 (gateway 2/2)
docker service ps marketplace_<tên> --no-trunc         # lý do task bị tắt / khởi động lại
docker service logs --tail 50 marketplace_<tên>
```
Nếu **đăng ký seller xong mà vẫn báo "Create a store first"** (hoặc e2e `setup` báo `ok:false`): event Kafka tạo store không tới auth-service. Kiểm tra consumer có partition không:
```bash
docker exec $(docker ps -q -f name=marketplace-infra_broker1) /opt/kafka/bin/kafka-consumer-groups.sh \
  --bootstrap-server broker1:9092,broker2:9092,broker3:9092 --describe --group auth-service     # phải có dòng PARTITION; trống = chưa được chia partition
docker service update --force marketplace_auth-service                  # cách gỡ tạm
```

## 7. Tắt và dọn dẹp

```bash
scripts/teardown.sh                  # gỡ 3 stack (marketplace, marketplace-obs, marketplace-infra); GIỮ dữ liệu (volume deploy_*)
scripts/teardown.sh --volumes        # + XÓA dữ liệu: Postgres, Kafka, ClickHouse, Redis, Grafana, Loki, Prometheus, spool analytics
scripts/teardown.sh --images         # + xóa image marketplace/* (build lại bằng scripts/build-images.sh)
scripts/teardown.sh --leave-swarm    # + docker swarm leave --force
scripts/teardown.sh --volumes --images --leave-swarm -y   # dọn sạch hoàn toàn, không hỏi
```
Chạy lại từ đầu sạch (DB rỗng, seed demo chạy lại): `scripts/teardown.sh --volumes -y && ./scripts/deploy.sh`. Gỡ stack mà giữ volume thì lần `deploy.sh` sau vẫn thấy dữ liệu cũ.

## 8. Gặp sự cố

Đơn kẹt, outbox, Kafka lag, ClickHouse, thanh toán: [03_RUNBOOK.md](03_RUNBOOK.md). Quy ước viết code (identity chỉ từ JWT, tiền, outbox, middleware theo route…): [../../CLAUDE.md](../../CLAUDE.md).

## 9. Bảng tra cứu script (copy dùng luôn)

Chạy từ thư mục gốc repo. Mặc định HTTP, địa chỉ `http://*.marketplace.swarm.localhost`. Chi tiết tham số nằm ở phần đầu mỗi script.

### Dựng và chạy
```bash
cp deploy/.env.example deploy/.env                     # giá trị local đã điền sẵn
./scripts/build-images.sh                              # build 7 service Go + frontend
./scripts/build-images.sh order-service frontend       # chỉ build vài cái
docker swarm init                                      # một lần
./scripts/deploy.sh                                    # infra + observability + services, rồi chờ sẵn sàng
./scripts/deploy.sh services                           # chỉ một stack (infra | observability | services)
./scripts/deploy.sh --no-wait                          # không chờ sẵn sàng
LITE=true ./scripts/deploy.sh                          # máy yếu (gateway 1 replica, memory thấp hơn, Kafka 1 broker thay vì 3)
DEPLOY_ENV=dev ./scripts/deploy.sh                     # dùng deploy/env/.env.dev (mẫu: deploy/env/dev.env.example)
./scripts/wait-ready.sh                                # chờ N/N replica và /ready của 7 service (WAIT_TIMEOUT=300)
```

### HTTPS (tùy chọn)
```bash
# đặt PUBLIC_SCHEME=https trong deploy/.env, rồi:
./scripts/gen-dev-certs.sh                             # cert tự ký; in cách import vào Chrome
./scripts/build-images.sh frontend && ./scripts/deploy.sh   # frontend phải build lại vì URL API được nướng vào
```

### Restart nhanh
```bash
docker service update --force marketplace_auth-service          # restart một service (đổi tên cho hợp)
for s in $(docker service ls --format '{{.Name}}' | grep '^marketplace_'); do docker service update --force --detach=true $s; done   # restart toàn bộ service ứng dụng
docker service scale marketplace_payment-service=0              # tắt tạm (=1 để bật lại)
docker service ls                                               # trạng thái (cột REPLICAS)
docker service logs --tail 100 -f marketplace_order-service     # log
```

### Dọn dẹp
```bash
scripts/teardown.sh                                    # gỡ stack, giữ dữ liệu
scripts/teardown.sh --volumes -y                       # + xóa dữ liệu (làm lại từ đầu)
scripts/teardown.sh --volumes --images --leave-swarm -y  # dọn sạch hoàn toàn
```

### Kiểm thử trên stack đang chạy
```bash
scripts/e2e/run-in-swarm.sh                            # bộ e2e chính (11 kịch bản) từ container tạm trong mạng swarm
scripts/e2e/run-in-swarm.sh --only 03,04               # chọn kịch bản (hoặc --skip 09)
RATE_LIMIT_TEST=1 scripts/e2e/run-in-swarm.sh          # thêm kịch bản rate limit 429
scripts/e2e/resilience.sh                              # tiêm lỗi, cả bộ A..I (dừng / bật từng thành phần)
scripts/e2e/resilience.sh A F                          # chọn từng chữ
API_URL=http://api.marketplace.swarm.localhost node scripts/load/load.mjs --rps=20 --duration=30   # tải nhẹ; thêm P95_MS=800 để đặt ngưỡng
python3 scripts/dev/gen_traffic.py --api http://api.marketplace.swarm.localhost --visitors 200 --products 1-60   # traffic giả cho dashboard
```
`resilience.sh` tự đọc `ADMIN_BOOTSTRAP_*` trong `deploy/.env` (kiểm `/admin/system/health`); đặt `ADMIN_USERNAME` / `ADMIN_PASSWORD` để ghi đè. Mô tả từng kịch bản: [04_TESTING.md](04_TESTING.md) §5.

### Test từng service (không cần Swarm; máy không có Go thì dùng container)
```bash
(cd services/order-service && go build ./... && go vet ./... && go test ./...)
# không cài Go trên máy:
docker run --rm -v "$PWD/services/order-service":/src -w /src -v gomod:/go/pkg/mod golang:1.25-alpine sh -c 'gofmt -l . ; go build ./... && go vet ./... && go test ./...'
scripts/protogen/gen.sh payment-service                # sinh lại .pb.go sau khi sửa .proto
```

### Dữ liệu giả, backfill, xuất dữ liệu
```bash
python3 scripts/datagen/datagen.py synthetic --seed 1 --start 2026-07-01 --end 2026-09-30 --psql "docker exec -i <pg> psql -U postgres -d postgres" --ch-url http://127.0.0.1:8123 --ch-db marketplace --ch-ensure-schema   # lịch sử giả (cần ClickHouse :8123 truy cập được)
(cd services/analytics-service && go run ./cmd/backfill backfill --from 2026-07-01 --to 2026-09-30 && go run ./cmd/backfill reconcile --from 2026-07-01 --to 2026-09-30)   # Postgres -> ClickHouse + đối soát
python3 scripts/export/export.py --profile otto ...    # xuất snapshot CSV / Parquet (profile: otto | rees46 | orders | raw); xem scripts/export/README.md
REGISTRY=ghcr.io/<owner> ./scripts/build-images.sh && REGISTRY=ghcr.io/<owner> ./scripts/push-images.sh   # đẩy image lên registry (cần docker login)
```
Datagen / backfill cần Postgres và ClickHouse; datagen ghi Postgres qua `docker exec` vào container Postgres (đặt tên container ở `--psql`), còn ClickHouse qua `--ch-url`. `infra.yml` **không publish cổng**, nên `http://127.0.0.1:8123` chỉ dùng được khi bạn tự publish cổng ClickHouse hoặc chạy chDB giả (`scripts/dev/chdb_server.py`). Chi tiết: [../design/07_DATA_PIPELINE.md](../design/07_DATA_PIPELINE.md). Lệnh `load.mjs` ở trên gọi qua Traefik cổng 80 nên cần dòng `/etc/hosts` ở mục 2.
