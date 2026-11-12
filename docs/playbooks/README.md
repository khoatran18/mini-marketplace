# Playbook sự cố (dạng máy đọc được)

Viết lại các mục của [RUNBOOK](../instruction/03_RUNBOOK.md) thành YAML có cấu trúc, để một agent (hoặc script) biết: cảnh báo nào kích hoạt, chẩn đoán bằng truy vấn nào, nguyên nhân nào ứng với kết quả nào, hành động nào an toàn, hành động nào cần người duyệt. **Không có kiến thức ngoài runbook**; nếu hai nơi mâu thuẫn thì runbook đúng và playbook phải sửa. Phần LLM/agent do người dùng tự xây, thư mục này chỉ là dữ liệu.

| File | Cảnh báo kích hoạt | Runbook |
|---|---|---|
| [service-unavailable.yml](service-unavailable.yml) | ServiceDown, ServiceMissing, ServiceNotReady, CircuitBreakerOpen, GatewayServerErrors | §1, §10 |
| [outbox-stuck.yml](outbox-stuck.yml) | OutboxDeadRows, OutboxBacklogOld | §5 |
| [kafka-consumer-dlq.yml](kafka-consumer-dlq.yml) | ConsumerDLQGrowing (và lag consumer) | §6 |
| [analytics-data.yml](analytics-data.yml) | AnalyticsRowsDropped, ReconcileDrift, ReconcileStale | §8, §12 |
| [resource-pressure.yml](resource-pressure.yml) | ContainerNearMemoryLimit | §10 |
| [ops-service.yml](ops-service.yml) | OpsPollerStale, OpsProposalsWaiting | §14 |

## Lược đồ
```yaml
id: tên-playbook
title: ...
triggers: {alerts: [TênCảnhBáo, ...]}     # phải khớp deploy/prometheus/alerts.yml (có kiểm tra)
runbook: "§..."                           # mục trong 03_RUNBOOK.md
diagnose:                                 # chỉ đọc, chạy tự do
  - id: ...
    tool: prometheus | loki | http | shell | sql | trace
    query: ...                            # PromQL / LogQL / URL / lệnh
    read_only: true
    interpret: "kết quả nghĩa là gì"
causes:                                   # nguyên nhân thường gặp, theo thứ tự nên thử
  - id: ...
    when: "điều kiện thấy trong kết quả chẩn đoán"
    fix: [action-id, ...]
actions:
  - id: ...
    risk: observe | low | high            # cùng nghĩa với annotation `action` của cảnh báo (RUNBOOK §13)
    approval: none | confirm | required   # low = confirm, high = required (có lý do)
    command: ...
    precondition: ...
    verify: "cách biết đã xong"
    rollback: ...
escalate: "khi nào dừng và gọi người"
ground_truth: [A, D]                      # kịch bản resilience.sh tạo ra lỗi này (scripts/e2e/resilience.sh)
```

Kiểm tính nhất quán: `scripts/check-playbooks.sh` (cảnh báo tồn tại trong `alerts.yml`, mục runbook tồn tại, mỗi hành động có `risk` hợp lệ, `high` luôn `approval: required`).

## Quy tắc chung cho mọi playbook
- Chẩn đoán trước, hành động sau; **không restart service chỉ vì `/ready` 503** (`/healthz` 200 nghĩa là còn sống, hỏng là do phụ thuộc).
- Mở mạch (breaker) là triệu chứng: tìm nguyên nhân ở service đích.
- `risk: high` (requeue outbox, backfill, restore, hoàn tiền) luôn cần người duyệt và ghi lý do; không tự chạy.
- Lệnh chạy trong mạng swarm: dùng container tạm `docker run --rm --network marketplace-net alpine ...`; token `METRICS_TOKEN` lấy từ `deploy/.env`, không in ra.
- Mẹo môi trường (chỉ máy local có DNS nhà mạng chuyển hướng NXDOMAIN): lỗi `dial tcp <IP công khai lạ>:<port>: i/o timeout` giữa service nghĩa là service đích đang ở 0 replica; xem `service-unavailable.yml`.

## Ground truth (đáp án đúng để đo agent)
`scripts/e2e/resilience.sh` ghi mỗi lần tiêm lỗi vào `~/mm-ground-truth/resilience.jsonl` (`fault_start` / `fault_end`: kịch bản, thành phần, loại lỗi, playbook đúng, cảnh báo mong đợi, tín hiệu mong đợi). Nhãn do script biết chắc vì chính nó gây lỗi. `GT_HOLD=<giây>` giữ lỗi lâu hơn để cảnh báo (có `for:`) kịp bắn. `scripts/e2e/ground_truth_check.sh` so cảnh báo mong đợi với cảnh báo Prometheus thực sự đã bắn. Đã kiểm: kịch bản C (payment-service về 0, giữ 100 s) bắn đúng `ServiceDown`.

**V7 đã xử lý:** `ServiceMissing` (`absent(up{...})`, test bằng `promtool test rules deploy/prometheus/alerts_test.yml`) bắn khi target biến mất (DNS trả NXDOMAIN); `ServiceDown` bắn khi target còn nhưng `up == 0` (máy local có DNS nhà mạng trả IP giả). Đáp án ground truth ghi `ServiceDown/ServiceMissing` (một trong hai).
