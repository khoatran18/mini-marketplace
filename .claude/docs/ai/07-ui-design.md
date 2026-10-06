# 07 · Thiết kế giao diện

Giữ stack hiện tại: **Next.js 14 (app router) + Tailwind**, gọi **chỉ qua gateway**. Giao diện được dựng *trước* và chạy với **mock/fallback**; khi bạn cắm AI vào, UI không phải sửa.

## 1. Nguyên tắc UI
1. **AI là lớp tăng cường, không phải điều kiện**: mọi màn hình dùng được khi `ai.*.enabled=false` (tìm kiếm từ khoá, trending, bảng số liệu).
2. **Minh bạch**: nội dung do AI sinh có nhãn "AI"; số liệu có `as_of`; câu trả lời luật có trích dẫn + miễn trừ.
3. **Không hành động ngầm**: AI đề xuất, người dùng bấm xác nhận.
4. **Mỗi thẻ gợi ý/ kết quả mang `request_id` + `position`** để tracking (SDK lo).
5. **Streaming**: chat hiện token dần, có chip tool-call, nút dừng, thử lại.
6. Tiếng Việt là mặc định (UI hiện tại đã tiếng Việt); chuẩn bị `i18n` dạng từ điển đơn giản để thêm EN sau.
7. Truy cập được: tương phản, focus ring, điều hướng bàn phím, `aria-live` cho luồng chat, tôn trọng `prefers-reduced-motion`.

## 2. Sitemap

```
Công khai / buyer
  /                         Trang chủ: hero tìm kiếm AI · "Dành cho bạn" · "Đang thịnh hành" · danh mục
  /products                 Danh sách + lọc + [Từ khoá | Mô tả (AI)] + sắp xếp
  /products/[id]            Chi tiết + "Tương tự" + "Thường mua cùng" + hỏi AI về sản phẩm này
  /categories/[slug]        (mới) trang danh mục
  /cart                     Giỏ (đồng bộ server) + "Có thể bạn cần"
  /orders, /orders/[id]     Đơn hàng + trạng thái + hỏi AI về đơn
  /policies                 (mới) Thư viện chính sách/luật + hỏi đáp có trích dẫn
  /profile                  (có) + quyền riêng tư: consent, tắt cá nhân hoá, xoá hội thoại/dữ liệu
  /login, /register         (có)
  [Trợ lý AI]               widget nổi toàn site (không phải route)

Người bán (seller_admin, seller_employee)
  /seller                   Tổng quan: doanh thu, đơn, sắp hết hàng, cảnh báo
  /seller/products          (thay /products/mine) + trạng thái + tồn + hiệu quả
  /seller/inventory         Tồn kho, ngưỡng cảnh báo, lịch sử, dự báo hết hàng
  /seller/orders            Hộp thư đơn
  /seller/insights          Phân tích: doanh thu, funnel sản phẩm, trợ lý "Hỏi số liệu shop"
  /seller/alerts            Cảnh báo của shop
  /seller/team              (seller_admin) nhân viên

Quản trị (admin)
  /admin                    Tổng quan sàn: GMV, đơn, traffic, người dùng, sức khoẻ hệ thống
  /admin/system             CPU/RAM theo service/node, latency, lỗi, Kafka lag, outbox
  /admin/analytics          Doanh thu/ traffic/ funnel/ top shop/ danh mục
  /admin/alerts             Trung tâm cảnh báo + quy tắc
  /admin/policies           Văn bản pháp lý: upload, xem chunk, xuất bản, retire, thử truy vấn
  /admin/ai                 Cờ tính năng, chi phí/token, hội thoại (metadata), tool audit, eval
  /admin/recs               Gợi ý: model_version, A/B, CTR theo surface, coverage
  /admin/data-health        Chất lượng dữ liệu tracking
```
Điều hướng: `NavBar` hiện tại sinh theo `role`; mở rộng bằng một bảng cấu hình route→role (một nơi duy nhất) và thêm role `admin` vào `type Role` (`lib/types.ts`). `/dashboard` cũ chuyển hướng theo role.

## 3. Layout & bố cục chung
- **Buyer**: giữ khung `max-w-5xl`, header + nội dung. Thêm ô tìm kiếm trung tâm trong header (có phím tắt `/`).
- **Seller/Admin**: layout console — **sidebar trái** (thu gọn trên mobile thành drawer) + thanh trên có bộ chọn khoảng thời gian toàn cục (Hôm nay / 7 ngày / 30 ngày / Tuỳ chọn) + chuông cảnh báo.
- Route group: `app/(shop)/…`, `app/(seller)/seller/…`, `app/(admin)/admin/…`; mỗi group một `layout.tsx` kiểm tra role phía client (UX) — **bảo vệ thật nằm ở gateway**.

## 4. Wireframe các màn hình chính

### 4.1 Trang chủ / tìm kiếm AI (buyer)
```
┌──────────────────────────────────────────────────────────────────────────────┐
│ Mini Marketplace   [ 🔍 Tìm: "áo khoác chống nước dưới 500k"        ] 🛒2 👤  │
│                    ( • Từ khoá  ○ Mô tả bằng lời (AI) )                      │
├──────────────────────────────────────────────────────────────────────────────┤
│ Danh mục:  Thời trang · Điện tử · Gia dụng · Sách · …                         │
│ ── Dành cho bạn ────────────────────────────────── (vì bạn đã xem “Tai nghe”)│
│ [ảnh]   [ảnh]   [ảnh]   [ảnh]   [ảnh]   →      mỗi thẻ: tên, giá, ★, “Còn ít”│
│ ── Đang thịnh hành trong Điện tử ───────────────────────────────────────────  │
│ [ảnh]   [ảnh]   [ảnh]   [ảnh]   [ảnh]   →                                    │
│                                                                      ┌────┐   │
│                                                                      │ 💬 │◀ widget trợ lý
└──────────────────────────────────────────────────────────────────────────────┘
```
- Carousel "Dành cho bạn": skeleton khi tải; lỗi/flag tắt ⇒ tự rơi về "Đang thịnh hành"; có menu `⋯` mỗi thẻ: *Không quan tâm* (gửi `reco_feedback`).
- Chế độ **Mô tả (AI)**: gọi `/search?mode=semantic`; hiển thị chip "AI hiểu: áo khoác · chống nước · ≤ 500.000₫" (bộ lọc suy ra – người dùng sửa/xoá được).

### 4.2 Trợ lý AI (widget, mọi trang) – buyer
```
┌─ Trợ lý Mini ───────────────────── ⤢ ✕ ┐
│ Xin chào! Bạn muốn tìm gì?              │
│ [Có gì mới trong kho?] [Đơn của tôi] [Chính sách đổi trả]  ← gợi ý nhanh
│─────────────────────────────────────────│
│ Bạn: Có tai nghe chống ồn dưới 1 triệu? │
│ AI : Mình tìm thấy 3 sản phẩm phù hợp:  │
│  ┌──────┐ Tai nghe A  890.000₫  Còn hàng│
│  │ ảnh  │ [Xem] [Thêm vào giỏ]          │   ← card product_list
│  └──────┘                                │
│  ⏳ đang tra cứu tồn kho… ✓ (320ms)     │   ← chip tool_call
│ 👍 👎  · Nguồn dữ liệu: kho hiện tại     │
│─────────────────────────────────────────│
│ [ Nhập câu hỏi…                   ] [Gửi]│
│ AI có thể sai. Kiểm tra thông tin quan trọng. │
└──────────────────────────────────────────┘
```
- `page_context` tự gắn (đang xem sản phẩm nào ⇒ "sản phẩm này" hiểu được). Lịch sử hội thoại (danh sách bên trái khi phóng to ⤢). Nút "Thêm vào giỏ" từ `action_proposal` → bấm mới gọi API.
- Trạng thái: `idle → streaming → done|error`; nút **Dừng**, **Thử lại**; hết hạn token ⇒ refresh im lặng.
- Hỏi luật: câu trả lời có footnote `[1]`; bấm mở **panel Nguồn** (tên văn bản, điều/khoản, hiệu lực, đoạn trích) + dòng miễn trừ.

### 4.3 Chi tiết sản phẩm
```
┌───────────────────────────────┬──────────────────────────────────────────┐
│  [ảnh lớn + thumbnails]       │ Tên sản phẩm                  ★4.6 (120) │
│                               │ 890.000₫          Shop: ABC ›  Còn hàng  │
│                               │ [ − 1 + ]  [Thêm vào giỏ] [♡]            │
├───────────────────────────────┴──────────────────────────────────────────┤
│ Mô tả · Thuộc tính · Đánh giá · Hỏi đáp                [💬 Hỏi AI về sản phẩm này]
│ ── Sản phẩm tương tự ──  [..][..][..][..]   (pdp_similar)                │
│ ── Thường được mua cùng ──  [..][..][..]    (pdp_bought_together)        │
└──────────────────────────────────────────────────────────────────────────┘
```
Sự kiện: `product_view`, `dwell`, `impression` từng thẻ, `add_to_cart`, `wishlist_add`.

### 4.4 Giỏ hàng
Danh sách + tổng; cột phải "Có thể bạn cần" (`cart_addon`); đồng bộ server khi đăng nhập (gộp giỏ localStorage → server).

### 4.5 Trang chính sách `/policies`
```
┌ Tìm trong chính sách & luật: [ đổi trả hàng điện tử trong bao lâu?   ] [Hỏi] ┐
│ Bộ lọc: ☑ Luật  ☑ Chính sách sàn  ☐ Quy định ngành hàng  | Hiệu lực: Hiện hành▾
├─ Trả lời (AI) ────────────────────────────────────────────────────────────────┤
│ … theo Điều 5 Chính sách đổi trả [1] … ; Luật … Điều … [2]                    │
│ ⚠ Thông tin tham khảo, không thay thế tư vấn pháp lý.                        │
├─ Nguồn ────────────────────────────────────────────────────────────────────────┤
│ [1] Chính sách đổi trả v3 (hiệu lực 01/09/2026) › Điều 5 › Khoản 2  [Xem toàn văn]
├─ Thư viện văn bản: danh sách theo loại, ngày hiệu lực, trạng thái ───────────┤
└────────────────────────────────────────────────────────────────────────────────┘
```

### 4.6 Seller – Tổng quan `/seller`
```
┌ Sidebar ─┬ [Hôm nay | 7 ngày | 30 ngày | Tuỳ chọn]                🔔3  ──────┐
│ Tổng quan│ ┌Doanh thu┐ ┌Đơn hàng┐ ┌Giá trị TB┐ ┌Tỉ lệ huỷ┐   (so kỳ trước ▲▼) │
│ Sản phẩm │ │ 12,4tr  │ │  38    │ │ 326k     │ │ 4,2%    │                    │
│ Tồn kho  │ └─────────┘ └────────┘ └──────────┘ └─────────┘                    │
│ Đơn hàng │ ┌─ Doanh thu theo ngày (line + kỳ trước) ─────────────┐ ┌ Cần chú ý ┐
│ Phân tích│ │                                                     │ │ ⚠ 3 SP sắp hết│
│ Cảnh báo │ └─────────────────────────────────────────────────────┘ │ ⚠ 2 đơn chờ │
│ Nhân viên│ ┌ Top sản phẩm ┐ ┌ Funnel: xem→giỏ→mua ┐ ┌ Tồn thấp ┐ │ >24h        │
│          │ Cập nhật lúc 08:15 (as_of)             [💬 Hỏi số liệu shop]         │
└──────────┴───────────────────────────────────────────────────────────────────────┘
```
`/seller/insights` có khung chat với agent `seller`; câu trả lời chèn `card` (metric/chart/table) cùng component với dashboard (dùng chung thư viện biểu đồ).

### 4.7 Admin – Tổng quan & Hệ thống
```
/admin:  KPI: GMV | Đơn | Người dùng hoạt động | Phiên | Lỗi 5xx | Alert đang mở (critical/warn)
         Biểu đồ: GMV theo giờ · Traffic (PV/Session/UV) · Funnel · Top shop · Top danh mục
/admin/system:
 ┌ Services ─────────────────────────────────────────────────────────────────────┐
 │ service           replicas  CPU%   RAM (MB / limit)  p95    5xx   Kafka lag   │
 │ api-gateway        2/2      34 ▇▃  310/512  ▇▇▃     120ms  0.1%   –          │
 │ order-service      1/1      12     180/512           45ms  0%    12          │
 │ …                                                                              │
 ├ Nodes: CPU / RAM / Disk (gauge) ── Biểu đồ chọn (service, metric, 1h/6h/24h/7d) ─
 ├ Hàng đợi: outbox pending/oldest, consumer lag theo group ──────────────────────┤
 └ [💬 Hỏi hệ thống: "service nào ngốn RAM nhất 6 giờ qua?"] ────────────────────┘
```

### 4.8 Trung tâm cảnh báo `/admin/alerts` (và `/seller/alerts` rút gọn)
```
Bộ lọc: Trạng thái ▾ Mức ▾ Phạm vi ▾ Rule ▾ Thời gian ▾            [Tạo quy tắc]
┌────┬──────────────────────────────────────┬────────┬───────┬──────────────┬────────┐
│ ●  │ CPU order-service 91% (5 phút)       │CRITICAL│ ×3    │ 08:12→nay    │ [Ack]  │
│ ●  │ Doanh thu giờ thấp 62% so baseline   │WARNING │ ×1    │ 08:00        │ [Ack]  │
├────┴──────────────────────────────────────┴────────┴───────┴──────────────┴────────┤
│ Chi tiết: biểu đồ bằng chứng · rule · runbook ›                                      │
│ 🤖 AI gợi ý (cần kiểm chứng): "CPU tăng sau lần deploy 08:05 …"  [👍][👎]            │
│ [Ack] [Resolve] [Tắt tiếng 1h/1 ngày…] [Mở trong Grafana]                            │
└──────────────────────────────────────────────────────────────────────────────────────┘
```
Quy tắc: danh sách quy tắc (bật/tắt, ngưỡng, kênh), nút "Chạy thử" trả các dòng vi phạm hiện tại.

### 4.9 Admin – Văn bản `/admin/policies`
Bảng văn bản (trạng thái `pending/indexed/retired`, hiệu lực) · Upload (kéo thả) · Xem chunk & sửa metadata · Xuất bản/Retire · **"Thử truy vấn"** (nhập câu hỏi → xem các chunk truy hồi + điểm; công cụ debug RAG của bạn).

### 4.10 Admin – AI `/admin/ai`
Cờ tính năng (bật/tắt + % rollout) · chi phí/ token theo ngày, theo agent · tỉ lệ lỗi/tool-call · độ trễ · thumbs down gần đây · tool audit (lọc theo user/tool) · chạy eval suite & xem báo cáo. `/admin/recs`: bảng surface × model_version × experiment → CTR/ATC/CVR, coverage, tỉ lệ fallback.

## 5. Thành phần (component) mới
```
components/
  ai/            AssistantWidget · ChatPanel · MessageList · Composer · ToolChip · CitationPanel
                 ActionProposal · FeedbackButtons · AiBadge · useChatStream (SSE hook)
  cards/         ProductCard (có sẵn, thêm impression/click) · ProductCarousel · MetricCard
                 ChartCard · DataTable · AlertCard · StatusPill
  reco/          RecoCarousel({surface, seed?}) · NotInterestedMenu
  search/        SearchBar · SearchModeToggle · InferredFilters · ResultGrid
  analytics/     DateRangePicker · KpiRow · TimeSeries · FunnelChart · Heatmap(tuỳ chọn)
  system/        ServiceTable · ResourceGauge · LagBadge
  alerts/        AlertList · AlertDetail · RuleEditor · BellMenu
  policy/        PolicySearch · SourceCard · DocumentTable · ChunkViewer · UploadDropzone
  layout/        ConsoleLayout (sidebar) · RoleGuard · FeatureGate({flag, fallback})
lib/
  tracking.ts    SDK (mục 7)       api.ts  (mở rộng: ai, analytics, alerts, policies, config)
  sse.ts         client SSE có tự ngắt/reconnect, parse event
  flags.ts       useFeature('ai.chat')  ← đọc /config/features, cache
  mockAi.ts      bộ mock theo contract để dev UI khi chưa có AI
```
Biểu đồ: **Recharts** (nhẹ, React-native). Số lượng KPI/biểu đồ nhiều ⇒ nên lazy-load (`next/dynamic`). Phong cách biểu đồ: 1 màu nhấn chính, kỳ trước màu xám nét đứt, trục có đơn vị, tooltip rõ, trạng thái rỗng/lỗi/đang tải thống nhất.

## 6. Hợp đồng `card` (UI ↔ agent) – cấu trúc payload
```ts
type Card =
 | { type:'product_list'; items: {product_id:number; name:string; price:number; image?:string; stock_level?:'none'|'low'|'ok'; reason?:string}[] }
 | { type:'metric'; label:string; value:number; unit?:'VND'|'count'|'percent'; delta_pct?:number; as_of:string }
 | { type:'chart'; kind:'line'|'bar'|'area'; series:{name:string; points:{t:string; v:number}[]}[]; unit?:string; as_of:string }
 | { type:'table'; columns:{key:string;label:string;format?:string}[]; rows:Record<string,unknown>[] }
 | { type:'alert'; alert_id:number }
 | { type:'action_proposal'; id:string; action:'add_to_cart'|'ack_alert'|…; label:string; params:Record<string,unknown>; requires_confirm:true }
```
UI chỉ render `type` đã biết; loại lạ ⇒ bỏ qua và hiển thị văn bản. Tất cả giá trị là dữ liệu, **không** HTML.

## 7. Tracking SDK (`lib/tracking.ts`)
- API: `track(type, props)`, `identify(userId)`, `setConsent(...)`, và hook/HOC: `<TrackImpression item surface position reco>` (IntersectionObserver ≥ 50% trong ≥ 1 s, 1 lần/ phiên/ vị trí), `<TrackClick …>`.
- Hàng đợi bộ nhớ + localStorage fallback; gửi batch mỗi 5 s hoặc 20 event; `navigator.sendBeacon` khi `visibilitychange=hidden`; retry mũ, bỏ khi > 3 lần; **không bao giờ chặn UI**.
- Sinh `event_id`, `anonymous_id` (cookie `mm_aid`), `session_id` (hết hạn 30 phút); gắn `surface`, `reco{request_id,model_version}`, `position`, `viewport`, `app_version`.
- Tôn trọng consent (xem 02 §7); tắt hoàn toàn bằng `NEXT_PUBLIC_TRACKING_ENABLED=false` (dev).
- Test: bộ test đơn vị cho batching/dedupe + trang `/dev/tracking` (chỉ môi trường dev) hiển thị event realtime.

## 8. Trạng thái giao diện & lỗi
Mỗi vùng dữ liệu có đủ 4 trạng thái: **loading (skeleton) · empty (giải thích + hành động) · error (thử lại, mã lỗi) · stale (hiện `as_of` cũ + cảnh báo)**. Flag AI tắt ⇒ `FeatureGate` hiển thị fallback, không hiện lỗi. Nhãn thống nhất: `AiBadge` (AI sinh), `Mock` badge khi backend trả `X-Mock: true` (giúp phân biệt khi bạn chưa cắm AI).

## 9. Bảo mật phía UI
- Không render HTML do AI/ người bán sinh (chỉ markdown giới hạn đã sanitize); mô tả sản phẩm cũng vậy.
- Không đặt token vào URL/SSE query. Chuyển token sang cookie httpOnly là nợ kỹ thuật đã ghi (security.md) – ưu tiên làm trước khi mở rộng bề mặt tấn công bằng chat.
- Ẩn UI theo role chỉ là tiện lợi; quyền thật ở gateway.

## 10. Tokens thiết kế (khởi đầu)
Tailwind `extend`: màu `brand` (một màu nhấn), `surface`, `muted`; trạng thái `success/warning/danger/info` (dùng cho alert, stock level); bán kính `xl` cho card; bóng nhẹ; font hệ thống (giữ). Dark mode: để sau (dùng biến CSS để dễ thêm). Breakpoints mặc định Tailwind; console ưu tiên desktop nhưng không vỡ ở mobile.
