# 09 · Lộ trình, nghiệm thu, ADR, câu hỏi mở

Thứ tự: nền vận hành trước (health/ready/metrics) → dữ liệu & thanh toán → tracking/analytics → cảnh báo → giao diện. Mỗi giai đoạn merge được độc lập, CI xanh.

## Giai đoạn
### P0 – Thiết kế *(đang ở đây)*
Tài liệu này; chủ dự án duyệt. **Chưa code.**

### P1 – Nền vận hành & danh tính
| Việc | Nghiệm thu |
|---|---|
| Thư viện chung nhỏ (`pkg/ops` copy vào từng module, theo ADR-8): admin HTTP :8081 với `/healthz /readyz /metrics /version`, grpc health, interceptor metrics, graceful shutdown | test health (fake dep) · mọi service hiện có trả đúng · SIGTERM ⇒ readyz 503 rồi thoát sạch |
| Docker `HEALTHCHECK` + Traefik `/readyz` + `update_config start-first` + `wait-ready.sh` | rolling update không rớt request (e2e) |
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
| (P2.5) coupons | |

### P3 – Tracking & analytics
ClickHouse + `analytics-service` + `POST /events` + SDK + rollups + analytics API seller/admin + đối soát + `scripts/datagen` (dữ liệu giả có seed) + `/admin/data-health`. Nghiệm thu: 1000 event giả khớp số; doanh thu CH ≡ Postgres (< 0,5%); shop A không đọc shop B (test); `/events` p95 < 50 ms; ClickHouse down ⇒ `/events` vẫn 202.

### P4 – Cảnh báo & quản trị hệ thống
`alert-service` (rule engine, Alertmanager webhook, vòng đời, SSE, notifications) + bộ rule 05 §3.3 + `/admin/system/*` + runbook cho từng rule. Nghiệm thu: gây lỗi có chủ đích (ngắt consumer, hạ tồn, ép CPU, payment fail) ⇒ alert đúng, dedupe, tự resolve, shop chỉ thấy alert của mình.

### P5 – Giao diện
Console seller/admin, checkout + `/pay`, timeline đơn, alert center, system page, product form mới, tracking trong UI. Nghiệm thu theo wireframe 07; trạng thái loading/empty/error/stale đủ; trang hoạt động khi analytics/alert-service down (thông báo nhẹ).

## ADR đề xuất (ghi vào `../decisions.md` sau khi duyệt)
- **P-1 Tách liveness/readiness; readiness có critical vs non-critical.** Tránh cascade restart; `degraded` vẫn nhận traffic.
- **P-2 Cổng admin :8081 riêng, không publish.** `/metrics`/`/readyz` không lộ ra Internet; gateway giữ `/health` công khai tối thiểu.
- **P-3 Thanh toán mô phỏng nhưng cùng hình dạng với thật** (idempotency, webhook ký, bất đồng bộ, hết hạn, hoàn tiền); provider là interface.
- **P-4 `SUCCESS` → `AWAITING_PAYMENT`/`CONFIRMED`; doanh thu chỉ tính khi PAID hoặc COD đã giao.** Định nghĩa duy nhất, hiển thị trên UI.
- **P-5 Reservation có hạn + sổ cái tồn kho bất biến.** Giải quyết đơn treo (roadmap #2), cho phép phân tích tồn.
- **P-6 ClickHouse cho analytics; không là nguồn sự thật tiền.** Dựng lại từ Kafka/Postgres.
- **P-7 Tracking ẩn danh có consent, ip băm, whitelist event, event nghiệp vụ chỉ từ server.**
- **P-8 Rule cảnh báo: hạ tầng ở file Prometheus (GitOps), nghiệp vụ ở DB chỉnh được qua UI + audit.**
- **P-9 Cart ở `order-service`; payment tách `payment-service`** (ranh giới an toàn/ vận hành khác).
- **P-10 Tách đơn theo shop khi checkout (`checkout_id` cha).**
- **P-11 Admin role không đăng ký công khai.**

## Rủi ro
| Rủi ro | Giảm thiểu |
|---|---|
| Phạm vi lớn | P1→P5 mỗi cái nghiệm thu riêng; làm tuần tự |
| Máy dev thiếu RAM | profile `lite`, limits rõ, bỏ Grafana/exporters |
| Healthcheck gây tải/ restart loop | cache 3 s, timeout 1 s, liveness không đụng dependency |
| Metrics bùng nổ cardinality | không nhãn id; route mẫu; review trong CI |
| Số liệu doanh thu lệch | định nghĩa duy nhất + job đối soát + alert |
| Thanh toán mô phỏng bị nhầm là thật | nhãn MÔ PHỎNG khắp UI, `PAYMENTS_MODE` bắt buộc, không lưu thẻ |
| Alert fatigue | dedupe, cooldown, inhibit, mức nghiêm trọng, chỉnh ngưỡng qua UI |
| Rò dữ liệu chéo shop | scope ép ở gateway, test chéo shop cho mọi endpoint `/seller/*` |

## Câu hỏi còn mở
1. Mã giảm giá (coupons) có làm ở P2 không, hay để sau?
2. Tách đơn theo shop ngay P2 (khuyến nghị) – đồng ý?
3. Email/ webhook thông báo cảnh báo: để sau hay làm trong P4?
4. Có cần dark mode / song ngữ không?
5. Múi giờ báo cáo cố định `Asia/Ho_Chi_Minh` – ổn chứ?
