# Load / soak cơ bản

`load.mjs` (Node 18+, không cần cài gói) phát request theo RPS cố định qua gateway ("open loop") với 4 loại tải:
duyệt catalog (`browse`), tìm kiếm (`search`), đặt hàng COD có `Idempotency-Key` (`order`), tracking `POST /events` (`events`).
Tự tạo 1 shop + sản phẩm (tồn rất lớn) + vài buyer, in tiến độ mỗi 10 s và bảng p50/p95/p99 theo endpoint khi kết thúc.

```bash
# thử nhanh
API_URL=http://localhost:8080 node scripts/load/load.mjs --rps=20 --duration=30
# tải vừa, đặt ngưỡng p95 = 800 ms và lỗi <= 1%
RPS=100 DURATION=300 P95_MS=800 node scripts/load/load.mjs
# soak 1 giờ, thiên về đọc
RPS=30 DURATION=3600 MIX=browse:70,search:20,order:2,events:8 node scripts/load/load.mjs
```

| Tham số | Mặc định | Ý nghĩa |
|---|---|---|
| `API_URL` | `http://localhost:8080` | gateway |
| `RPS` / `DURATION` / `RAMP` | 20 / 30 s / 5 s | tải mục tiêu, thời lượng, thời gian tăng dần |
| `MIX` | `browse:50,search:25,order:5,events:20` | tỉ trọng các loại request |
| `BUYERS` / `PRODUCTS` | 5 / 20 | số buyer và sản phẩm tạo sẵn |
| `MAX_INFLIGHT` | 500 | trần request đồng thời; vượt thì bỏ và đếm `dropped` |
| `MAX_ERR_PCT` | 1 | vượt ngưỡng lỗi (5xx, lỗi mạng, 4xx bất ngờ) thì exit 1 |
| `P95_MS` | (tắt) | đặt để exit 1 khi p95 của bất kỳ endpoint nào vượt ngưỡng |

Mã thoát: `0` đạt, `1` vượt ngưỡng hoặc setup lỗi, `2` tham số sai. Truyền tham số bằng env hoặc `--ten-tham-so=giatri`.

Lưu ý:
- Rate limit theo IP: stack test nên nâng `AUTH_RATE_LIMIT_PER_MINUTE` (setup gọi ~`2 + 2*BUYERS` lần `/auth/*`),
  `RATE_LIMIT_PER_MINUTE` và `EVENTS_RATE_LIMIT_PER_MINUTE`. Các phản hồi 429 được báo riêng (cột `429`), không tính vào tỉ lệ lỗi.
- Chạy bằng nhiều IP/máy nếu muốn đo mà không cần nâng giới hạn; không chạy trên môi trường production.
- Token access hết hạn sau ~5 phút; script tự `refresh-token` nên soak dài vẫn chạy.
- Đơn COD sinh ra nhiều dữ liệu (mỗi lần = 1 đơn + giữ hàng + event outbox). Dọn bằng cách tạo lại DB test sau khi chạy.
- Quan sát song song Grafana (`deploy/observability.yml`) để biết service nào nghẽn; đơn vẫn xử lý bất đồng bộ qua Kafka
  nên sau tải nên kiểm tra không còn đơn `PENDING`.

## `ccu.mjs`: thử tải theo số người dùng đồng thời (vòng kín)
Mỗi VU là một người dùng với tài khoản và IP giả riêng; mỗi lần lặp chọn ngẫu nhiên một hành trình con (buyer, seller, admin, operator, hỏi AI theo từng persona) nên chạm mọi container nghiệp vụ; cuối bài in bảng độ phủ. Cần `ADMIN_USERNAME`/`ADMIN_PASSWORD` (mặc định `ADMIN_BOOTSTRAP_*`) cho admin và operator. Cần cấu hình `.perf` đã áp dụng (`scripts/deploy-perf.sh all`: nâng rate limit, tin `X-Forwarded-For`, ai-agent dùng mô hình giả). Chi tiết: `docs/design/09_PERF_RESOURCES.md` §4-5.
```bash
node scripts/load/ccu.mjs --vus=25 --duration=90 --ramp=20 --out=/tmp/ccu25.json
```
Tham số xem đầu file (`VUS`, `DURATION`, `RAMP`, `MIX`, `THINK_MS`, `AI_THINK_MS`, `PRODUCTS`, `OUT`, `MAX_ERR_PCT`). Phép đo chỉ "HỢP LỆ" khi lỗi ngoài dự kiến ≤ ngưỡng, không có 429 và mọi VU vào được.

## ccu.mjs: đóng và mở, chạy cả dải

- **Đóng** `--vus=N`: N người cố định (vào 5 VU/giây, `ENTRY_RATE`), hệ thống chậm thì tải tự giảm.
- **Mở (nửa mở)** `--arrival-rate=R`: R khách mới mỗi giây, mỗi khách chọn loại theo `MIX` rồi làm ~3 hành trình (`SESSION_JOURNEYS`) và rời đi; dùng lại tài khoản cũ (chỉ đăng nhập). CCU là kết quả (`đỉnh người cùng lúc`), không phải tham số.
- `scripts/load/perf-run.sh NHÃN [tham số]`: một lượt (gắn node vào 2 core `LOADGEN_CPUS=14,15`, ghi `docker stats`, chụp cấu hình Swarm, tổng hợp) ra `docs/perf-results/<ngày>/` (`OUT_DIR` đổi được).
- `scripts/load/perf-matrix.sh`: đóng `100 200 300 500 800 1000` rồi mở `5 10 20 30 40`, lặp `REPEAT` (3) lần (ghi đè bằng `CLOSED`, `OPEN`, `REPEAT`, `DURATION`). Cần `scripts/deploy-perf.sh` trước (mô hình AI giả).
- Sonde máy tải trong báo cáo: độ trễ vòng lặp node, CPU toàn máy (cảnh báo > 75%), RAM trống. Kết quả và giải thích: `docs/design/09_PERF_RESOURCES.md` §5e, §6.
