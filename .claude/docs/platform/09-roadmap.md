# 09 · Lộ trình, nghiệm thu, ADR, câu hỏi mở

Thứ tự: nền vận hành trước (health/ready/metrics) → dữ liệu & thanh toán → tracking/analytics → cảnh báo → giao diện. Mỗi giai đoạn merge được độc lập, CI xanh.

## Giai đoạn
### P0 – Thiết kế *(đang ở đây)*
Tài liệu này; chủ dự án duyệt. **Chưa code.**

### P1 – Nền vận hành & danh tính *(đang làm: phần health/ready/metrics đã code – xem 02)*
| Việc | Nghiệm thu |
|---|---|
| ✅ *đã làm* Thư viện chung nhỏ (`pkg/ops` copy vào từng module, theo ADR-8): admin HTTP :8081 với `/health /ready /metrics /version`, grpc health, interceptor metrics, graceful shutdown | test health (fake dep) · mọi service hiện có trả đúng · SIGTERM ⇒ ready 503 rồi thoát sạch |
| ✅ *đã làm (chưa chạy thử trên Swarm thật)* Docker `HEALTHCHECK` + Traefik `/ready` + `update_config start-first` + `wait-ready.sh` | rolling update không rớt request (e2e) |
| Healthcheck cho mọi container hạ tầng | `docker service ls` toàn healthy |
| Prometheus + exporters + cAdvisor + node-exporter + Grafana (dashboard Overview/Services/Containers/Nodes) | thấy CPU/RAM từng service; target `up==1` |
| Role `admin` + bootstrap + `Role` ở frontend + khoá tạm đăng nhập + `audit_log` | `/auth/register` từ chối admin; test RBAC; test khoá |
| Gateway: giới hạn body, nhóm route công khai, rate-limit riêng | test |

### P2 – Catalog, giỏ hàng, thanh toán mô phỏng, vòng đời đơn
| Việc | Nghiệm thu |
|---|---|
| Trường mới product + `categories` + MinIO ảnh + FTS `/search` | CRUD/ validate; seed ≥ 100 SP; tìm không dấu |
| `inventory_ledger` + `reserved` + outbox `product.changed/inventory.changed` | ledger khớp tồn; test đồng thời |
| Cart server + merge + `checkout/preview` + địa chỉ + idempotency key | test trùng key; giá luôn tính server |
| State machine mới + `order_status_history` + hết hạn giữ chỗ + seller ship/deliver + return | bảng chuyển trạng thái đủ test; hết hạn giải phóng đúng 1 lần |
| `payment-service` mockpay (thẻ test, webhook ký HMAC, trễ/ trùng/ sau-hết-hạn, refund) | mỗi dòng bảng kịch bản 04 §3.2 có e2e |
| Reviews/ wishlist (tối thiểu) | chỉ người đã nhận hàng review |

### P3 – Tracking & analytics
ClickHouse + `analytics-service` + `POST /events` + SDK + rollups + analytics API seller/admin + đối soát + `scripts/datagen` (dữ liệu giả có seed) + `/admin/data-health`. Nghiệm thu: 1000 event giả khớp số; doanh thu CH ≡ Postgres (< 0,5%); shop A không đọc shop B (test); `/events` p95 < 50 ms; ClickHouse down ⇒ `/events` vẫn 202.

### P4 – Giám sát hệ thống (Grafana) – *không có cảnh báo*
Dashboard Grafana provisioning bằng file (Overview, Services/RED, Containers, Nodes, Kafka & Outbox, Postgres/Redis, ClickHouse, Payments, Business) theo 05 §3.1; `GET /admin/system/health|metrics|outbox` (proxy whitelist) cho trang `/admin/system`; runbook đọc dashboard. Nghiệm thu: ngắt consumer / ép CPU / payment fail có chủ đích ⇒ **nhìn thấy** trên Grafana & `/admin/system`; không có thành phần cảnh báo nào được dựng.

### P5 – Giao diện
Console seller/admin, checkout + `/pay`, timeline đơn, system page, product form mới, tracking trong UI, **dark mode (sáng/tối/theo hệ thống)**. Nghiệm thu theo wireframe 07; trạng thái loading/empty/error/stale đủ; trang hoạt động khi analytics/alert-service down (thông báo nhẹ).

## ADR đề xuất (ghi vào `../decisions.md` sau khi duyệt)
- **P-1 Tách liveness/readiness; readiness có critical vs non-critical.** Tránh cascade restart; `degraded` vẫn nhận traffic.
- **P-2 Cổng admin :8081 riêng, không publish.** `/metrics`/`/ready` không lộ ra Internet; gateway giữ `/health` công khai tối thiểu.
- **P-3 Thanh toán mô phỏng nhưng cùng hình dạng với thật** (idempotency, webhook ký, bất đồng bộ, hết hạn, hoàn tiền); provider là interface.
- **P-4 `SUCCESS` → `AWAITING_PAYMENT`/`CONFIRMED`; doanh thu chỉ tính khi PAID hoặc COD đã giao.** Định nghĩa duy nhất, hiển thị trên UI.
- **P-5 Reservation có hạn + sổ cái tồn kho bất biến.** Giải quyết đơn treo (roadmap #2), cho phép phân tích tồn.
- **P-6 ClickHouse cho analytics; không là nguồn sự thật tiền.** Dựng lại từ Kafka/Postgres.
- **P-7 Tracking ẩn danh có consent, ip băm, whitelist event, event nghiệp vụ chỉ từ server.**
- **P-8 Chưa có cảnh báo; chỉ Grafana.** Không alert-service/Alertmanager/notification. Ngưỡng trong 05 §3.1 chỉ tô màu panel. Thêm sau không cần đổi code service.
- **P-9 Cart ở `order-service`; payment tách `payment-service`** (ranh giới an toàn/ vận hành khác).
- **P-10 Tách đơn theo shop khi checkout (`checkout_id` cha) – ĐÃ CHỐT.** Mỗi shop một đơn, thanh toán gộp theo `checkout_id`.
- **P-11 Admin role không đăng ký công khai.**

## Rủi ro
| Rủi ro | Giảm thiểu |
|---|---|
| Phạm vi lớn | P1→P5 mỗi cái nghiệm thu riêng; làm tuần tự |
| Máy dev thiếu RAM | profile `lite`, limits rõ, bỏ Grafana/exporters |
| Healthcheck gây tải/ restart loop | cache 3 s, timeout 1 s, liveness không đụng dependency |
| Metrics bùng nổ cardinality | không nhãn id; route mẫu; review trong CI |
| Số liệu doanh thu lệch | định nghĩa duy nhất + job đối soát hiển thị ở `/admin/data-health` |
| Thanh toán mô phỏng bị nhầm là thật | nhãn MÔ PHỎNG khắp UI, `PAYMENTS_MODE` bắt buộc, không lưu thẻ |
| Rò dữ liệu chéo shop | scope ép ở gateway, test chéo shop cho mọi endpoint `/seller/*` |

## Đã chốt
- Thanh toán mô phỏng · ClickHouse · health/ready/metrics mọi service · **múi giờ cố định `Asia/Ho_Chi_Minh`** · **dark mode** · **không cảnh báo (chỉ Grafana)** · **không mã giảm giá** · endpoint hỗ trợ cả `/health` & `/healthz`, `/ready` & `/readyz` · **tách đơn theo shop (A)** · mọi service Go mở cổng admin HTTP `:8081`.

## Câu hỏi còn mở
Không còn.
