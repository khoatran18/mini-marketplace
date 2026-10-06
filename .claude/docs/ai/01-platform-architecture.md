# 01 · Kiến trúc nền tảng

## 1. Kiến trúc đích

```
                          ┌──────────────────────────── Traefik (TLS) ────────────────────────────┐
Browser ─────────────────▶│ frontend (Next.js)                         api-gateway (gin, 2+ replicas)│
  • UI mua/bán/admin      └──────────────┬───────────────────────────────────┬───────────────────────┘
  • tracking SDK  ───── POST /events ────┘                                   │
  • chat (SSE)    ───── POST /ai/chat ───────────────────────────────────────┤
                                                                             │ gRPC
        ┌──────────────┬──────────────┬───────────────┬──────────────┬──────┴───────┬───────────────┐
        ▼              ▼              ▼               ▼              ▼              ▼               ▼
   auth-service   user-service   product-service  order-service  analytics-svc*  alert-svc*      ai-service*
   (+role admin)                 (+mô tả, danh mục (+lifecycle,   (consume Kafka,  (rule engine,    (BẠN CODE:
                                  ảnh, product.     cart server)    query ClickHouse) scheduler)     agent, RAG,
                                  changed event)                                                     recsys serving)
        │              │              │               │              │              │               │
        └──────────────┴──────┬───────┴───────────────┴──────┬───────┴──────┬───────┴───────┬───────┘
                              ▼                              ▼              ▼               ▼
                        Postgres (pgvector ext.)          Kafka          ClickHouse*      Redis
                        db `postgres` (nghiệp vụ)         + topic mới    (events, rollups) (cache, feature
                        db `ai` (conv, doc, embed, alert)                                    online, flags)

   Observability*: Prometheus ◀── /metrics của mọi service, cAdvisor, node-exporter, kafka-exporter, postgres-exporter
                   Grafana (dashboard nội bộ) · Alertmanager → alert-service webhook · (tuỳ chọn) Loki/Tempo/OTel
   (*) = thành phần mới
```

## 2. Thành phần mới

| Thành phần | Công nghệ (đề xuất) | Vai trò | Ai code |
|---|---|---|---|
| `analytics-service` | Go (cùng khuôn với service hiện có), gRPC `:50055` | Consume `tracking.events` + sự kiện nghiệp vụ → ghi ClickHouse; cung cấp query doanh thu / traffic / funnel theo vai trò | Nền tảng (mình dựng) |
| `alert-service` | Go, gRPC `:50056` | Rule engine (SQL/PromQL/ngưỡng), scheduler, vòng đời alert, kênh thông báo, nhận webhook Alertmanager | Nền tảng |
| `ai-service` | **Python (FastAPI + gRPC)**, `:50057` / HTTP `:8000` | Agent, RAG, embedding, serving gợi ý. Ban đầu là **stub** trả dữ liệu mock đúng contract | **Bạn** (nền tảng chỉ dựng khung) |
| ClickHouse | `clickhouse/clickhouse-server` 1 node | OLAP: events, rollup doanh thu/traffic, training data | Hạ tầng |
| pgvector | Image `pgvector/pgvector:pg16` thay `postgres:16-alpine` (hoặc extension trên cùng instance) | Vector store cho RAG & tìm sản phẩm bằng mô tả | Hạ tầng |
| Prometheus + Grafana + Alertmanager + exporters | upstream images | CPU/RAM/network, lỗi, lag | Hạ tầng |
| (tuỳ chọn) MinIO | S3-compatible | Lưu file văn bản pháp lý gốc (PDF), ảnh sản phẩm | Hạ tầng |

Vì sao Python cho `ai-service`: hệ sinh thái LLM/ML (embedding, LightGBM, PyTorch, LangGraph…) nằm ở Python; services Go giữ nguyên. Giao tiếp qua gRPC/HTTP nên có thể đổi ngôn ngữ mà không đụng phần còn lại.

## 3. Luồng dữ liệu chính

### 3.1 Tracking (clickstream)
```
Browser SDK ─batch 5s/20 events─▶ POST /events (gateway: JWT *tuỳ chọn*, rate-limit, size-limit, validate)
   ─▶ Kafka `tracking.events` (key = anonymous_id|user_id)
   ─▶ analytics-service ─▶ ClickHouse `events_raw` ─▶ materialized views (rollups)
                       └▶ (sau) feature job ghi Redis cho gợi ý online
```
Gateway **gắn** `user_id`/`role`/`store_id` từ JWT (nếu có) vào event; client không được tự khai.

### 3.2 Sự kiện nghiệp vụ → analytics
Order/product/user service đã có outbox. Thêm các topic `order.status_changed`, `product.changed`, `inventory.changed` (xem [08](08-api-contracts.md#3-kafka-topic-mới)). `analytics-service` consume và ghi bảng fact (`fact_orders`, `fact_order_items`, `dim_products` snapshot). Doanh thu **tính từ fact đã `PAID/DELIVERED`**, không đếm đơn PENDING/FAILED/CANCELED.

### 3.3 Chat với agent
```
UI ─POST /ai/chat {conversation_id?, message, page_context}─▶ gateway (JWT, rate-limit AI riêng)
   ─gRPC stream Chat(ctx{user_id, role, store_id, jwt}, message)─▶ ai-service
        ai-service: agent loop → gọi *tool* → mỗi tool = HTTP tới gateway kèm đúng JWT của user
        ◀── stream token + tool events + citations + UI cards
   ◀── SSE: event: token | tool_call | card | citation | done | error
```
Gateway ghi `ai_audit` (không phải ai-service tự khai) để có nhật ký độc lập.

### 3.4 Gợi ý
```
UI (surface) ─GET /recommendations?surface=home&context=…─▶ gateway ─▶ ai-service.Recommend()
        ◀─ {request_id, model_version, items:[{product_id, score, reason}]}   (mock = bán chạy)
UI: impression (IntersectionObserver) / click / add_to_cart đều mang request_id + position
```
`request_id` là "chìa khoá" nối *gợi ý → hành vi → mua hàng* để tính CTR/CVR và huấn luyện.

### 3.5 Quét & cảnh báo
```
Prometheus ─rule firing─▶ Alertmanager ─webhook─▶ alert-service ─┐
scheduler (cron trong alert-service) ─chạy rule nghiệp vụ trên ClickHouse/Postgres─┤
                                                                  ▼
                      bảng `alerts` (dedupe theo fingerprint) ─▶ kênh: in-app (SSE), email, webhook
                                                                  ▼ (tuỳ chọn, BẠN CODE)
                                  ai-service.ExplainAlert() → thêm `ai_summary`, `suggested_actions`
```

### 3.6 Nạp văn bản pháp lý (RAG)
```
Admin upload PDF/MD ─▶ gateway /policies ─▶ (MinIO) + bảng policy_documents/versions (status=pending)
   ─▶ Kafka `policy.document_uploaded` ─▶ ai-service.Ingest(): parse → chunk → embed → pgvector (status=indexed)
```

## 4. Phân tầng quyền & danh tính cho AI
- `ai-service` **chỉ** chấp nhận request từ gateway (mTLS hoặc shared secret nội bộ `AI_INTERNAL_TOKEN`, cùng overlay network, không publish port).
- Gateway truyền `AgentContext{user_id, role, store_id, jwt, locale, page_context}`.
- Tool gọi ngược gateway bằng `Authorization: Bearer <jwt của user>` → mọi kiểm tra hiện có (RBAC, ownership, rate limit) áp dụng. **Không** có "service account vạn năng" cho agent của người dùng.
- Job nền (quét, huấn luyện) dùng *service token* riêng, scope hẹp, chỉ đọc (`scope=analytics:read`).

## 5. Điểm cắm AI (AI seams)
Đây là danh sách *chính xác* những chỗ bạn sẽ viết code AI. Mỗi seam có contract (08) và bản mock.

| Seam | Interface (trong ai-service) | Gọi từ | Mock mặc định |
|---|---|---|---|
| `ChatAgent` | `Chat(ctx, message, history) → stream[Event]` | gateway `/ai/chat` | Echo + trả 1 card sản phẩm giả |
| `ProductRetriever` | `Search(query, filters, k) → [product_id, score]` | tool `search_products` | Postgres `ILIKE` |
| `PolicyRetriever` | `Retrieve(question, scope, k) → [chunk+citation]` | tool `ask_policy` | Trả "chưa có dữ liệu" |
| `Embedder` | `Embed(texts) → vectors` | indexer, retriever | Vector ngẫu nhiên cố định seed (đủ để test pipeline) |
| `Recommender` | `Recommend(user, surface, context, k) → [item]`; `Similar(item,k)` | gateway `/recommendations` | Bán chạy 7 ngày (từ ClickHouse) |
| `AlertExplainer` | `Explain(alert, evidence) → {summary, actions}` | alert-service | Không làm gì (field rỗng) |
| `Evaluator` | `RunSuite(name) → report` | CI/cron | Không có |

Mỗi seam có flag riêng (`ai.chat.enabled`, `ai.recs.enabled`, …) lưu Redis/DB, đọc qua `GET /config/features`.

## 6. Vì sao chọn như vậy (tóm tắt)
- **Gateway làm cổng duy nhất** (ADR-3 hiện có) → AI cũng đi qua nó. Hưởng RBAC miễn phí, nhưng gateway phải chịu thêm SSE/stream (cần tăng timeout, tắt buffering ở Traefik cho route `/ai`).
- **ClickHouse thay vì Postgres cho events**: clickstream tăng nhanh, truy vấn cột/aggregation rất hợp; Postgres vẫn là nguồn sự thật cho giao dịch.
- **pgvector thay vì vector DB riêng** (giai đoạn đầu): đã có Postgres, đủ cho ≤ vài triệu vector, ít thứ phải vận hành; sau có thể thay sau interface `Retriever`.
- **Không text-to-SQL cho agent ở giai đoạn đầu**: dùng *semantic layer* (tool số liệu có tham số, đã scope theo vai trò). Tránh rò dữ liệu chéo shop và SQL sai.

## 7. Ràng buộc từ hệ thống hiện tại cần lưu ý
- Gateway hiện **bắt JWT ở mọi route trừ auth/health** → cần route tracking/search công khai có rate-limit riêng (xem [02 §6](02-data-foundation.md#6-tracking-ẩn-danh)).
- Gateway giới hạn body chưa có (security.md "known gaps") → bắt buộc thêm giới hạn cho `/events` và upload.
- Token access sống 5 phút → phiên chat dài cần cơ chế refresh trong SSE (UI refresh trước, gửi header mới ở lần gọi kế tiếp; không đưa token vào body stream).
- `AutoMigrate`, chưa có migration có version → với bảng mới nên dùng migration có version ngay (goose) cho `ai`/`analytics`; không cần sửa dịch vụ cũ.
- Cart đang ở localStorage → cần cart phía server để gợi ý "giỏ hàng" và tín hiệu `add_to_cart` đáng tin (xem 02).
