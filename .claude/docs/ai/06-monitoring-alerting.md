# 06 · Giám sát, quét tự động và cảnh báo

Hai việc khác nhau:
1. **Giám sát hạ tầng** (CPU, RAM, mạng, lỗi, lag) → Prometheus/Grafana/Alertmanager.
2. **Quét nghiệp vụ** (tồn thấp, doanh thu bất thường, đơn kẹt, listing vi phạm) → rule engine trong `alert-service`.
Cả hai đổ vào **một bảng `alerts`** và một trung tâm cảnh báo trên UI. AI chỉ *giải thích/đề xuất* (tuỳ chọn).

## 1. Metrics hạ tầng (Prometheus)
| Nguồn | Exporter | Metric chính |
|---|---|---|
| Container (CPU/RAM/net/IO theo service & replica) | **cAdvisor** (global service trên mỗi node) | `container_cpu_usage_seconds_total`, `container_memory_working_set_bytes`, `container_network_*` |
| Node | **node-exporter** | CPU, RAM, disk, load, filesystem |
| Go services | `/metrics` (client_golang) | `http_requests_total{route,status}`, `http_request_duration_seconds`, `grpc_server_handled_total`, `go_*`, `outbox_pending{table}`, `kafka_consumer_lag` |
| Gateway | middleware | RPS, p50/p95/p99, 4xx/5xx, rate-limit hits, request theo role |
| Postgres | postgres-exporter | connections, locks, replication, cache hit, slow queries |
| Redis | redis-exporter | memory, hit rate, ops |
| Kafka | kafka-exporter | lag theo group/topic, offset |
| ClickHouse | built-in `/metrics` | insert lag, merges, disk |
| ai-service | `/metrics` | xem 03 §8 |
Quy ước: mọi service Go thêm middleware metrics + endpoint `/metrics` **không publish ra ngoài** (chỉ trong overlay network; Traefik không route).

### Admin xem RAM/CPU trong app như thế nào
Không cho trình duyệt gọi Prometheus. Gateway có `GET /admin/system/metrics` (role `admin`) → proxy tới Prometheus với **whitelist truy vấn có tên** (`cpu_by_service`, `mem_by_service`, `p95_latency`, `error_rate`, `kafka_lag`, `node_cpu`, `node_mem`…) + tham số `range/step` đã kiểm tra. Agent `admin` dùng chính endpoint này (tool `system_metrics`) ⇒ không có PromQL tự do (tránh truy vấn nặng/rò thông tin).
Grafana vẫn dùng cho SRE (`grafana.<host>` sau basic-auth như dashboard Traefik), UI app chỉ hiện các biểu đồ chọn lọc.

## 2. Số liệu kinh doanh & lưu lượng
`analytics-service` (ClickHouse) trả: doanh thu/đơn/AOV/tỉ lệ huỷ theo thời gian, theo shop, theo danh mục; lượt xem trang, phiên, người dùng duy nhất, mới/quay lại, nguồn, funnel; top sản phẩm theo xem/mua/chuyển đổi. Cache Redis 30–60 s. Mỗi phản hồi có `as_of`.

## 3. Rule engine (`alert-service`)
### 3.1 Loại rule
| type | Định nghĩa | Ví dụ |
|---|---|---|
| `promql` | biểu thức Prometheus + `for` | CPU service > 85% trong 5 phút (thường để Prometheus tự đánh giá rồi gửi Alertmanager) |
| `threshold` | truy vấn đơn giản + so sánh | tồn kho < ngưỡng; đơn `PENDING` > 10 phút |
| `sql` | truy vấn Postgres/ClickHouse (chỉ đọc, whitelist) trả các dòng vi phạm | sản phẩm xem nhiều nhưng 0 đơn; shop có tỉ lệ huỷ > 30% |
| `anomaly` | so với baseline cùng giờ/ngày tuần (median ± k·MAD hoặc z-score trên rollup) | doanh thu giờ hiện tại thấp bất thường; traffic tăng đột biến (bot?) |
| `ai_scan` | job gọi `ai-service` quét nội dung/hành vi, trả danh sách nghi vấn | listing nghi vi phạm, review spam |
Rule lưu ở `alert_rules` (JSON `definition`), có `owner`, `scope` (`platform|store`), `schedule` (cron), `severity`, `enabled`, `cooldown`.

### 3.2 Bộ rule ban đầu (đề xuất)
**Hệ thống (admin)**: CPU container > 85% 5 phút · RAM > 90% working-set 5 phút · RAM tăng đơn điệu 30 phút (rò bộ nhớ) · node disk > 85% · p95 gateway > 1 s 5 phút · 5xx > 2% 5 phút · service replica down > 1 phút · Kafka lag > 1000 trong 5 phút · outbox `PENDING` > 500 hoặc cũ nhất > 2 phút · Postgres connections > 80% · ClickHouse insert lag > 2 phút · cert TLS < 14 ngày · chi phí AI vượt 80% ngân sách ngày.
**Nghiệp vụ (admin)**: doanh thu giờ < 50% baseline · đơn `PENDING` kẹt > 10 phút (đã có runbook) · tỉ lệ huỷ > 15% · đối soát doanh thu lệch > 0.5% · số đăng ký tăng đột biến/ lặp IP (nghi bot).
**Shop (seller)**: tồn < ngưỡng hoặc còn < N ngày bán · sản phẩm bán chạy sắp hết · doanh thu tuần giảm > 30% · sản phẩm có >X lượt xem, 0 đơn (giá/mô tả có vấn đề?) · đơn chờ xử lý lâu · đánh giá thấp tăng.
**Nội dung (admin duyệt)**: listing nghi hàng cấm / tuyên bố sai sự thật (`ai_scan`) · review nghi giả.

### 3.3 Vòng đời alert
```
firing ─▶ open ──ack──▶ ack ──resolve──▶ resolved
              └─ tự resolved khi điều kiện hết đúng ≥ N chu kỳ ─┘
              └─ suppress/silence (thời hạn, lý do)
```
- **Dedupe** theo `fingerprint = hash(rule_id, scope, labels chính)`: đang open thì chỉ tăng `count`, cập nhật `last_seen`.
- **Chống bão cảnh báo**: `cooldown`, gom nhóm (một node chết ≠ 20 alert), mức nghiêm trọng `info|warning|critical`.
- **Định tuyến**: `scope=store` ⇒ chỉ chủ/nhân viên shop đó thấy; `scope=platform` ⇒ admin.
- **Kênh**: in-app (bảng `notifications` + SSE `/alerts/stream`), email (SMTP, sau), webhook (Slack/Telegram, sau). Tin quan trọng không phụ thuộc AI.

### 3.4 Lớp AI (tuỳ chọn, bạn code)
- `AlertExplainer.Explain(alert, evidence)` gọi ngay sau khi tạo alert: `evidence` là gói dữ liệu **chuẩn bị sẵn** (chuỗi thời gian 24 h, deploy gần đây, alert liên quan, top service theo CPU, log lỗi đã cắt). Kết quả `ai_summary` + `suggested_actions[]` lưu cạnh alert, UI gắn nhãn "AI gợi ý – cần kiểm chứng".
- Quét định kỳ `ai_scan` do scheduler gọi; kết quả phải là **alert chờ duyệt**, không tự hành động (không tự khoá shop/gỡ hàng).
- Hành động tự động hoá (self-healing, scale) **ngoài phạm vi** giai đoạn này; nếu làm sau, chỉ qua allowlist + xác nhận.

## 4. Luồng triển khai
```
Prometheus rules ─▶ Alertmanager ─ webhook /internal/alerts/prometheus ─▶ alert-service
alert-service scheduler (leader-elect bằng Redis lock để 2 replica không chạy trùng)
      ├─ chạy rule threshold/sql/anomaly ─▶ upsert `alerts` ─▶ notify
      └─ gọi Explain / ai_scan (nếu bật flag)
UI: /alerts (shop), /admin/alerts (sàn), chuông trên NavBar (SSE)
```

## 5. Dashboard
- **Grafana** (SRE): Overview, Services (RED: rate/errors/duration), Containers, Nodes, Kafka/Outbox, Postgres, Redis, ClickHouse, AI.
- **App** `/admin` & `/seller`: số liệu chọn lọc (xem 07); đây là thứ người dùng nghiệp vụ thấy.

## 6. Runbook liên kết
Mỗi rule có trường `runbook_url` trỏ tới mục trong [../runbook.md](../runbook.md) (bổ sung mục CPU/RAM, Kafka lag, ClickHouse, AI budget khi làm). Alert hiển thị link này.
