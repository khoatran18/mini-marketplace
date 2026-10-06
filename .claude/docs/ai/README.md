# AI Platform – bộ tài liệu thiết kế

> **Trạng thái: THIẾT KẾ (chưa có code).** Mục tiêu: biến Mini Marketplace thành *nền tảng* sẵn sàng để cắm các ứng dụng AI vào.
> Phần "hạ tầng + giao diện + hợp đồng (contract)" sẽ được dựng trước; **phần AI do chủ dự án tự code sau**.

## Đọc theo thứ tự

| # | Tài liệu | Nội dung |
|---|---|---|
| 00 | [overview.md](00-overview.md) | Mục tiêu, persona, use case, nguyên tắc, phạm vi (làm gì / không làm gì) |
| 01 | [platform-architecture.md](01-platform-architecture.md) | Kiến trúc đích, thành phần mới, luồng dữ liệu, "điểm cắm AI" |
| 02 | [data-foundation.md](02-data-foundation.md) | Khoảng trống dữ liệu hiện tại, tracking event, bảng mới, ClickHouse, privacy |
| 03 | [agents-and-tools.md](03-agents-and-tools.md) | Agent theo vai trò, danh mục tool, phân quyền, guardrail, hội thoại |
| 04 | [recsys.md](04-recsys.md) | Tín hiệu, pipeline gợi ý nhiều tầng, đánh giá offline/online, lộ trình mô hình |
| 05 | [rag-policy.md](05-rag-policy.md) | Hỏi đáp điều luật / chính sách sản phẩm (RAG), quản lý văn bản, trích dẫn |
| 06 | [monitoring-alerting.md](06-monitoring-alerting.md) | CPU/RAM/traffic, rule engine, quét tự động, cảnh báo, AI giải thích |
| 07 | [ui-design.md](07-ui-design.md) | Sitemap, wireframe từng màn hình, component, tracking SDK, design token |
| 08 | [api-contracts.md](08-api-contracts.md) | REST mới ở gateway, gRPC của AI service, Kafka topic mới, JSON schema |
| 09 | [infra-plan.md](09-infra-plan.md) | Container mới, env var, port, tài nguyên, thay đổi deploy |
| 10 | [roadmap-phases.md](10-roadmap-phases.md) | Các giai đoạn, checklist, tiêu chí nghiệm thu, ADR, câu hỏi còn mở |

## Tóm tắt 1 phút

```
        ┌────────────────────────── Next.js (UI mới: widget chat, gợi ý, dashboard, alert) ───────────────┐
        │                                   │  tracking SDK (click, view, search, cart…)                   │
        ▼                                   ▼                                                              │
   api-gateway (JWT, RBAC, rate-limit) ── /events ──▶ Kafka `tracking.events` ──▶ analytics-service ─▶ ClickHouse
        │  /ai/* (SSE)        /analytics/*  /alerts/*  /policies/*                                        │
        ▼                                                                                                 │
   ai-service  (← BẠN CODE)  ──tools──▶ gateway REST (dùng JWT của user)    pgvector (embedding, RAG)      │
        ▲                                                                                                 │
   Prometheus + Grafana + Alertmanager + cAdvisor/node-exporter  ──▶  alert-service (rule engine)  ───────┘
```

Ba nguyên tắc xuyên suốt:
1. **AI không có quyền riêng.** Agent gọi lại API của gateway bằng *chính JWT của người dùng* → RBAC và ownership hiện có tự áp dụng.
2. **Quy tắc phát hiện, AI giải thích.** Cảnh báo do rule engine xác định (tin cậy, rẻ); AI chỉ bổ sung ngữ cảnh/đề xuất.
3. **Mọi thứ AI đều tắt được.** Feature flag + fallback không-AI (tìm theo từ khoá, hàng bán chạy, bảng số liệu thuần).

Tài liệu nền tảng hiện có (đọc trước nếu chưa quen): [../README.md](../README.md).
