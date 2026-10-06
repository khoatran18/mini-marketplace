# 08 · Hợp đồng API (REST, gRPC, Kafka)

Mọi thứ ở đây là **thiết kế đề xuất**; khi code sẽ sinh OpenAPI (swag) và proto (buf) tương ứng. Quy ước giữ nguyên hiện tại: `Authorization: Bearer`, lỗi `{"error":"..."}`, ánh xạ mã gRPC→HTTP ở `handler/helpers.go`.

## 1. REST mới ở gateway

Ký hiệu quyền: 🌐 công khai (rate-limit riêng) · 🔑 đăng nhập · B buyer · S seller_admin/employee · SA seller_admin · A admin.

### 1.1 Tracking & cấu hình
| Method & path | Quyền | Mô tả |
|---|---|---|
| POST `/events` | 🌐 (JWT tuỳ chọn) | Batch ≤ 50 event, body ≤ 64 KB; trả `202 {accepted, rejected:[{index,reason}]}` |
| POST `/events/identify` | 🔑 | Nối `anonymous_id` ↔ user (sau login) |
| GET `/config/features` | 🌐 | `{ "ai.chat": true, "ai.search.semantic": false, … }` theo role/rollout |
| DELETE `/users/me/data` | 🔑 | Yêu cầu xoá dữ liệu cá nhân (hội thoại, identity links, ẩn danh events) |
| PUT `/users/me/consent` | 🔑 | `{analytics, personalization}` |

### 1.2 Catalog & tìm kiếm
| Method & path | Quyền | Mô tả |
|---|---|---|
| GET `/search` | 🌐 | `q, mode=keyword|semantic|hybrid, category_id, price_min, price_max, in_stock, sort, page, page_size≤50` → `{request_id, items[], inferred_filters?, total}` |
| GET `/categories` | 🌐 | cây danh mục |
| GET `/catalog/overview` | 🌐 | thống kê "trong kho có gì" (cache) |
| POST/PUT `/products` | S | **mở rộng** body: `description, category_id, brand, tags[], image_urls[], status` |
| POST `/products/:id/images` | S | upload ảnh (MinIO), kiểm tra loại/size |
| GET `/products/:id/stock-level` | 🌐 | `{level: none|low|ok}` (người mua); số chính xác chỉ `S` của shop |
| POST/GET/DELETE `/reviews` | B | (tuỳ chọn Phase sau) |

### 1.3 Giỏ hàng
| Method & path | Quyền | Mô tả |
|---|---|---|
| GET `/cart` | B | giỏ của caller |
| PUT `/cart/items/:product_id` | B | `{quantity}` (0 = xoá) |
| DELETE `/cart` | B | |
| POST `/cart/merge` | B | gộp giỏ ẩn danh (localStorage) khi đăng nhập |
Checkout vẫn là `POST /orders` (server định giá lại như ADR-4).

### 1.4 Gợi ý
| Method & path | Quyền | Mô tả |
|---|---|---|
| GET `/recommendations` | 🌐 | `surface, seed_product_id?, product_ids?, k≤30` → `{request_id, model_version, experiment, items:[{product, score?, reason_code}]}`; gateway hydrate `product`, lọc `active`/còn hàng |
| POST `/recommendations/feedback` | 🌐/🔑 | `{request_id, product_id, type: not_interested|hide}` |

### 1.5 AI
| Method & path | Quyền | Mô tả |
|---|---|---|
| POST `/ai/chat` | 🔑 (buyer cho phép 🌐 giới hạn) | Body `{conversation_id?, message, page_context?}` → **SSE** (xem 03 §3) |
| GET `/ai/conversations` · GET `/ai/conversations/:id` · DELETE `/ai/conversations/:id` | 🔑 | chủ sở hữu |
| POST `/ai/feedback` | 🔑 | `{message_id, thumb, reason?}` |
| POST `/ai/actions/:id/confirm` | 🔑 | xác nhận `action_proposal` (gateway kiểm tra proposal thuộc về user, chưa hết hạn, còn hợp lệ) |
| POST `/policies/ask` | 🌐 (giới hạn) | `{question, scope?}` → `{answer, citations[]}` (không cần hội thoại) |

### 1.5.1 Tool endpoints cho agent (cũng dùng cho dashboard)
Mọi tool bên dưới là REST bình thường, dùng JWT của người gọi; **không** có endpoint riêng "cho AI".
| Path | Quyền | Tool tương ứng |
|---|---|---|
| GET `/seller/analytics/summary` `…/timeseries` `…/top-products` `…/low-stock` `…/products/:id/funnel` | S (ép `store_id` từ JWT; tham số `store_id` của client bị bỏ) | `store_*` |
| GET `/admin/analytics/summary` `…/traffic` `…/funnel` `…/top-stores` | A | `platform_*` |
| GET `/admin/system/metrics?q=<tên_truy_vấn>&service=&range=&step=` | A | `system_metrics` |
| GET `/admin/system/health` | A | `service_health` |
Phản hồi chung: `{ "as_of": "...", "source": "clickhouse", "data": … }`; số tiền `{amount, currency}`; khoảng thời gian tối đa 400 ngày.

### 1.6 Cảnh báo
| Method & path | Quyền | Mô tả |
|---|---|---|
| GET `/alerts` | S (chỉ store của mình) / A (tất cả) | lọc `status, severity, since` |
| GET `/alerts/:id` | như trên | + `evidence`, `ai_summary` |
| POST `/alerts/:id/ack` · `/resolve` · `/silence` | S/A | |
| GET `/alerts/stream` | S/A | SSE realtime cho chuông |
| GET/POST/PUT `/admin/alert-rules[/:id]` · POST `/admin/alert-rules/:id/test` | A | quản lý rule, chạy thử |
| POST `/internal/alerts/prometheus` | **nội bộ** (secret) | webhook Alertmanager — không route qua Traefik public |

### 1.7 Văn bản chính sách
| Method & path | Quyền | Mô tả |
|---|---|---|
| GET `/policies` · `/policies/:id` | 🌐 | thư viện (chỉ văn bản xuất bản, `kind` công khai) |
| GET `/policies/chunks/:id` | 🌐 | đoạn gốc để mở trích dẫn |
| POST `/admin/policies` (multipart) | A | upload |
| POST `/admin/policies/:id/publish` · `/retire` · `/reindex` | A | |
| POST `/admin/policies/debug-retrieve` | A | thử truy hồi |

### 1.8 Quản trị AI
| Path | Quyền | |
|---|---|---|
| GET/PUT `/admin/ai/flags` | A | cờ + % rollout |
| GET `/admin/ai/usage` · `/admin/ai/audit` | A | chi phí, token, tool audit |
| POST `/admin/ai/eval/run` · GET `/admin/ai/eval/runs/:id` | A | |
| GET `/admin/recs/stats` | A | CTR/CVR theo surface/model/experiment |
| GET `/admin/data-health` | A | |

## 2. Giao tiếp nội bộ

### 2.1 Service mới
- `analytics-service` gRPC `:50055`: `Summary`, `TimeSeries`, `TopN`, `Funnel`, `Traffic`, `ProductStats` — tất cả nhận `Scope{role, store_id}` do **gateway** điền (không nhận từ client).
- `alert-service` gRPC `:50056`: `ListAlerts`, `GetAlert`, `Ack`, `Resolve`, `Silence`, `UpsertRule`, `TestRule`, `IngestExternal`.
- Cart: đặt trong `order-service` (cùng DB, ít service hơn) hoặc service riêng — chốt ở ADR-A6.

### 2.2 `ai-service` (gRPC) – đây là **hợp đồng bạn triển khai**
```proto
service AiService {
  // Agent
  rpc Chat(ChatRequest) returns (stream ChatEvent);
  // Retrieval
  rpc SearchProducts(SearchRequest) returns (SearchResponse);      // semantic/hybrid
  rpc Retrieve(PolicyQuery) returns (PolicyResult);                // RAG
  // Recsys
  rpc Recommend(RecommendRequest) returns (RecommendResponse);
  rpc Similar(SimilarRequest) returns (RecommendResponse);
  rpc BoughtTogether(SimilarRequest) returns (RecommendResponse);
  rpc CartAddons(CartRequest) returns (RecommendResponse);
  // Indexing (được gọi bởi consumer/ admin)
  rpc IndexProduct(IndexProductRequest) returns (Ack);
  rpc IngestDocument(IngestRequest) returns (IngestResult);
  // Alerts
  rpc ExplainAlert(ExplainRequest) returns (ExplainResponse);
  rpc ScanContent(ScanRequest) returns (ScanResponse);
  // Eval
  rpc RunEval(EvalRequest) returns (EvalReport);
  rpc Health(Empty) returns (HealthInfo);          // model_version, index_version, flags hiểu được
}
message AgentContext { uint64 user_id=1; string role=2; uint64 store_id=3; string jwt=4; string locale=5; string request_id=6; PageContext page=7; }
message ChatRequest  { AgentContext ctx=1; string conversation_id=2; string message=3; repeated HistoryMsg history=4; }
message ChatEvent    { oneof e { Token token=1; ToolCall tool_call=2; ToolResult tool_result=3; Card card=4; Citation citation=5; Suggestions suggestions=6; Done done=7; Error error=8; } }
message RecommendRequest { AgentContext ctx=1; string surface=2; string anonymous_id=3; repeated uint64 seed_product_ids=4; map<string,string> context=5; uint32 k=6; repeated uint64 exclude=7; }
message RecommendResponse{ string request_id=1; string model_version=2; string experiment=3; repeated RecItem items=4; }
message RecItem { uint64 product_id=1; float score=2; string reason_code=3; }
```
Quy tắc: proto đặt ở `services/ai-service/proto/ai.proto`, sinh code bằng `buf` cho Go (gateway) và Python (ai-service). Phiên bản hoá: `ai.v1`; thêm trường tuỳ chọn thay vì đổi trường cũ.
**Bản stub** (do nền tảng dựng): mọi RPC trả dữ liệu mock hợp lệ + header `x-mock: true`.

### 2.3 Bảo mật nội bộ
`AI_INTERNAL_TOKEN` (metadata gRPC), không publish port, chỉ gateway/alert-service/analytics được gọi. ai-service **gọi ngược** gateway bằng JWT trong `AgentContext` (cho tool) hoặc service token scope hẹp (job nền).

## 3. Kafka topic mới
| Topic | Producer → Consumer | Key | Payload |
|---|---|---|---|
| `tracking.events` | gateway → analytics-service (`analytics-ingest`) | `anonymous_id` hoặc `user_id` | envelope (02 §3.1) + server fields |
| `product.changed` | product-service (outbox) → ai-service (`ai-indexer`), analytics (dim) | product id | `{product_id, op: upsert|delete, name, description, category_id, price, status, updated_at, text_hash}` |
| `inventory.changed` | product-service → analytics, alert-service | product id | `{product_id, seller_id, delta, new_level, reason, ref_id}` |
| `order.status_changed` | order-service (outbox) → analytics, alert-service | order id | `{order_id, buyer_id, status, prev, at, items:[{item_id,product_id,seller_id,category_id,qty,unit_price}]}` |
| `cart.updated` | order/cart-service → analytics | user id | `{user_id, items, at}` |
| `policy.document_uploaded` | gateway → ai-service (`ai-ingest`) | doc id | `{document_id, file_key, kind, sha256}` |
| `policy.document_indexed` | ai-service → gateway/alert | doc id | `{document_id, version, chunks, status}` |
| `alert.created` / `alert.updated` | alert-service → notifier (+ai-service `ai-explain`) | alert id | alert JSON |
| `reco.requests` | gateway/ai-service → analytics | request id | `{request_id, surface, user/anon, model_version, experiment, items:[{id,score,pos}]}` |
Nguyên tắc cũ giữ nguyên: outbox cho producer nghiệp vụ, consumer idempotent (dedupe theo `event_id`/khoá tự nhiên), version hoá payload bằng trường `v`. Dead-letter topic (`*.dlq`) cho các topic mới (đã nằm trong nợ kỹ thuật – làm luôn cho topic mới).
