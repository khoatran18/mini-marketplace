# 09. Phân bổ tài nguyên cho thử tải CCU (cấu hình `.perf`)

Hai file `deploy/services.perf.yml` và `deploy/infra.perf.yml` chỉ dùng để thử tải. **Trạng thái: ĐÃ ÁP DỤNG lên Swarm ngày 2026-11-23** (infra + services, kiểm: mọi service đủ replica, gateway giới hạn 100000/phút, mỗi `X-Forwarded-For` có bộ đếm riêng, Postgres `max_connections=200`, ClickHouse đã bỏ 6 bảng log).

Áp dụng: `./scripts/deploy-perf.sh all` (hoặc `infra` / `services`). Trở lại bình thường: `./scripts/deploy.sh infra && ./scripts/deploy.sh services`. Cần script riêng vì `docker stack deploy` đòi `cpus` là chuỗi. Gọi API từ ngoài phải có header `Host: api.<PUBLIC_HOST>`. IP giả bằng `X-Forwarded-For` xem [08_NETWORK_CLIENT_IP.md](08_NETWORK_CLIENT_IP.md).

## 1. Ngân sách (máy 16 core, 31 GB; trừ 3 GB container ngoài dự án, 3 GB OS + k6)
Cho stack: khoảng 25 GB RAM, 13 core (k6 chiếm khoảng 3 core). Giới hạn CPU là trần, không phải mức dùng, nên tổng giới hạn có thể vượt 13.

| Service | Replicas | CPU | RAM |
|---|---|---|---|
| traefik | 1 | 2 | 512 MB |
| api-gateway | 3 | 1.5 mỗi bản | 512 MB |
| ai-agent | 3 (đo với 1 trước) | 1 mỗi bản | 768 MB |
| auth, product, order | 1 | 1 | 384 MB |
| user, payment, analytics, ops | 1 | 0.5 | 256 MB (ops 96 MB) |
| frontend | 1 | 1 | 512 MB |
| postgres | 1 | 3 | 4 GB, `shared_buffers=1GB`, `max_connections=200` |
| redis | 1 | 1 | 512 MB, `maxmemory 400mb`, `noeviction` |
| kafka x3 | 1 mỗi broker | 1 | 1,5 GB, heap 1 GB (gốc 512 MB) |
| clickhouse | 1 | 2 | 2 GB |
| obs | giữ nguyên | không đặt | giữ nguyên |

Giới hạn tốc độ của gateway được nâng lên 100000 mỗi phút (nếu không test chỉ đo ra 429).

## 2. ai-agent và song song
- 1 tiến trình uvicorn = 1 event loop = tối đa 1 core (GIL). Chờ model không tốn CPU, nhưng LangGraph, JSON, SSE và gọi tool thì có.
- Trạng thái nằm ngoài tiến trình (lịch sử chat và checkpoint RCA ở Postgres, slot ở Redis) nên nhân bản được.
- Chỉ nhân bản khi `AI_RCA_ENABLED=false`: watcher RCA không có khoá chọn người dẫn đầu, nhiều bản sẽ điều tra trùng.
- Điểm nghẽn mềm chưa nâng: `GatewayClient` giới hạn 50 kết nối mỗi tiến trình, pool asyncpg tối đa 5 mỗi bản (3 bản = 15 trên 100 kết nối Postgres), cap token mỗi ngày mỗi user (`limits.daily_tokens`, nằm trong yaml, chưa có biến môi trường).
- Số replica cần = `ceil(CCU_mục_tiêu x CPU_mỗi_luồng / 0.7)`. Đo CPU mỗi luồng với 1 replica rồi tính.

## 3. Nguyên nhân các chỗ ăn tài nguyên lúc rảnh
- **clickhouse (CPU 3–39%, nhấp nháy)**: không phải do tải dự án. Truy vấn của analytics rất nhẹ (vài ms, đọc vài nghìn dòng). Nguyên nhân là các bảng log nội bộ của chính ClickHouse: `asynchronous_metric_log` 172 triệu dòng, `trace_log` 9,6 triệu, `text_log` 5,2 triệu, cộng merge nền và luồng AsynchronousMetrics. Đã sửa: `deploy/clickhouse/system-logs.xml` (gắn trong `infra.yml`) tắt `asynchronous_metric_log`, `metric_log`, `trace_log`, `text_log`, `processors_profile_log`, `error_log` và đặt TTL 7 ngày cho `query_log`. Bảng cũ không tự xoá, phải `DROP TABLE system.<tên>` sau khi áp dụng.
- **kafka (khoảng 57% giới hạn 768 MB lúc rảnh)**: phần lớn là heap JVM và page cache, thường không giảm khi rảnh. Heap nâng lên 1 GB và giới hạn lên 1,5 GB để có dư khi tải lên.
- **cadvisor (206/384 MB)**: bộ nhớ tăng theo số container và metric; chấp nhận được.

## 4. Kịch bản CCU và mô hình giả
- `scripts/load/ccu.mjs` (vòng kín, mỗi VU = một người dùng thật với tài khoản, JWT và IP giả riêng). Mỗi lần lặp, VU chọn NGẪU NHIÊN (có trọng số) một hành trình con, nên tải đa dạng. Loại VU theo `MIX` (mặc định buyer 50%, seller 12%, admin 8%, operator 8%, ai_buyer 12%, ai_seller/ai_admin/ai_operator 4% mỗi loại):
  - **buyer**: duyệt (+ trang frontend, `/events`, tìm kiếm có lọc/sắp xếp); mua COD / MOCK_CARD / ví / chuyển khoản / thẻ bị từ chối / 3-D Secure (OTP) / huỷ thanh toán, đặt lại cùng `Idempotency-Key`; quản lý giỏ (thêm, merge, sửa, xoá); hồ sơ buyer, địa chỉ (thêm/sửa/xoá), đổi mật khẩu; theo dõi đơn (lọc, tóm tắt, xác nhận đã nhận, trả hàng, huỷ).
  - **seller** (tài khoản chủ shop dùng chung): giao, hoàn tất, huỷ đơn; duyệt/từ chối trả hàng (kéo theo hoàn tiền ở payment-service); tạo/sửa/ẩn sản phẩm, chỉnh tồn, sổ cái tồn; 5 báo cáo analytics; tạo nhân viên rồi đăng nhập bằng nhân viên.
  - **admin** (tài khoản bootstrap, đọc từ `ADMIN_USERNAME`/`ADMIN_PASSWORD`, mặc định `ADMIN_BOOTSTRAP_*`): đơn và thanh toán toàn nền tảng, hoàn tiền, 9 báo cáo analytics, tài khoản, danh mục, đổi trạng thái sản phẩm, và các đọc ops.
  - **operator** (script tạo bằng tài khoản admin lúc setup và xoá lúc kết thúc): sức khoẻ hệ thống, sự cố, audit, sổ hành động, truy vấn Prometheus (lấy tên và tham số từ danh mục thật), playbook, outbox, đề xuất, công tắc AI, phiên RCA.
  - **ai_***: hỏi trợ lý AI qua `/ai/chat` (SSE) với từng persona, tới `event: done`.
  Id sản phẩm, checkout, payment, đơn đều lấy từ phản hồi thật. Mỗi lời gọi có mã mong đợi; mã ngoài dự kiến, 429, VU không vào được, hoặc service nào không được gọi thì phép đo bị báo chưa hợp lệ / thiếu độ phủ. Cuối bài in bảng độ phủ theo container (đếm từ đường dẫn; đã đối chiếu `grpc_request_rate` của Prometheus: gateway gọi đủ Order, Payment, Auth, Product, Analytics, User).
- Mô hình giả có độ trễ: provider `fake` nhận `ttft_ms` và `tokens_per_sec` (chờ bằng `asyncio.sleep`, không chặn event loop); profile `fake_perf_buyer/seller/admin/operator/text` trong `config.prod.yaml` (800 ms, 60 token/s: GIẢ ĐỊNH cho DeepSeek, chưa đo). `deploy/services.perf.yml` chọn chúng bằng `AI_LLM_ROLE_BUYER/SELLER/ADMIN/OPERATOR`; mặc định (không đặt biến) vẫn là mô hình thật. Mô hình giả vẫn gọi tool thật (`list_my_orders`, `count_store_orders`, `admin_list_orders`, `ops_system_health`), nên đo được đường ai-agent → gateway → service.
- Hạn chế: hai tính năng chưa có trong kịch bản (tạo/duyệt đề xuất ops cần hai người và dữ liệu sự cố thật; RCA tắt). Dữ liệu thử tải tích luỹ trong DB (tài khoản `lb...`, `lo...`, `ls...`, shop "LOAD shop ...", đơn, danh mục `CCU ...`); chưa có script dọn.

## 5. Kết quả thử tải 25 / 50 / 100 / 200 CCU (2026-11-23)
**Đính chính quan trọng.** Các số AI trong lần chạy 25 CCU trước đó và lần chạy 50 CCU đầu tiên KHÔNG hợp lệ: ai-agent trên Swarm chạy `APP_ENV=dev` (`AI_AGENT_ENV=dev` trong `deploy/.env`) nên đọc `config.dev.yaml`, trong khi mô hình giả và biến `AI_LLM_ROLE_*` lúc đó chỉ có ở `config.prod.yaml`. Hậu quả: các lượt chat AI trong những lần chạy đó gọi MODEL THẬT (`opencode_deepseek_flash`, hơn 5000 token vào mỗi lượt, có cả 429 và thử lại 18-30 s). Đã sửa: profile `fake_perf_*` và `AI_LLM_ROLE_*` có ở cả hai file cấu hình (test `test_roles_can_switch_to_perf_fakes_by_env` kiểm cả dev và prod), `scripts/deploy-perf.sh` sau khi deploy kiểm mọi replica ai-agent phân giải 4 role sang provider `fake` và thoát 1 nếu không. Lưu ý: image cùng tag thì `docker stack deploy` không tạo lại task, cần `docker service update --force marketplace_ai-agent`. Từ lúc sửa, nhật ký ai-agent chỉ có các profile `fake_perf_*`. Các số bên dưới (trừ dòng 25 CCU) đo sau khi sửa.

Cấu hình: bộ kịch bản đa dạng (mục 4), ramp + 15 s vào + 90 s ổn định, ai-agent 3 replica, mô hình giả 800 ms ttft và 60 token/s. Một lần chạy mỗi mức (chưa có phương sai giữa các lần).

| | 50 | 100 | 200 |
|---|---|---|---|
| Lời gọi/s (tổng lời gọi) | 72 (7464) | 143 (14816) | 288 (30039) |
| Lỗi ngoài dự kiến, 429 | 0, 0 | 0, 0 | 14 (0,05%), 0 |
| Đơn tạo / giao đi / hoàn tiền | 175 / 77 / 22 | 415 / 126 / 66 | 860 / 219 / 143 |
| Lượt chat AI xong, lỗi | 131, 0 | 256, 0 | 529, 0 |
| AI cả lượt p50 / p95 | 2,57 / 2,59 s | 2,41 / 2,59 s | 2,43 / 2,64 s |
| Đặt đơn COD p50 / p95 | ~9 / ~20 ms | ~9 / ~25 ms | 9 / 24 ms |
| CPU đỉnh: auth-service | 39% | 45% | 88% |
| CPU đỉnh: postgres | 19% | 25% | 62% |
| CPU đỉnh: traefik | 9% | 21% | 44% |
| CPU đỉnh: order-service | | | 41% |
| CPU đỉnh: ai-agent (mỗi replica) | 12% | 30% | 38% |
| CPU đỉnh: kafka (mỗi broker) | | 26% | 36% (bản chạy đầu) |

- **API đồng bộ chưa nghẽn tới 200 CCU**: p95 phần lớn ≤ 30 ms; chậm nhất là đổi mật khẩu (p95 ~210 ms) và đăng nhập (p95 ~120 ms), đều là băm mật khẩu. auth-service chạm 88% của 1 core ở 200 CCU: **sắp nghẽn đầu tiên nếu có nhiều đăng nhập/đăng ký đồng thời** (script đăng ký mọi VU lúc vào, nên phần này bị phóng đại so với thực tế).
- **AI**: độ trễ cả lượt không đổi (≈ 2,4 s, toàn bộ là độ trễ giả) từ 50 tới 200 CCU, ai-agent mỗi replica ≤ 38% core, RAM ~100 MB: 3 replica dư sức cho 200 CCU với mô hình giả. Mô hình thật sẽ khác (ttft và token/s thật, 429 của nhà cung cấp).
- **ĐIỂM NGHẼN THẬT: outbox của order-service và product-service** (xem `docs/planning/01_OPEN_ISSUES.md` R11). Thanh toán online được tạo bất đồng bộ qua Kafka; ở 200 CCU từ lúc đặt đơn tới lúc có payment p50 15 s, p95 50 s, max 61 s, trong khi frontend chỉ chờ ~8 s. Độ trễ outbox (`published_at - created_at`): order-service p50 7 s / p95 29 s, product-service p50 10 s / p95 24 s, payment-service ~1 s. CPU các service chỉ vài % nên nghẽn do thiết kế worker (mỗi 2 s tối đa 100 dòng, gửi tuần tự), không do thiếu tài nguyên.
- Hệ quả khác: ở tải cao, đơn "chờ thanh toán" (543 trong lần chạy đầu 200 CCU) lấn át hộp thư seller nên seller gần như không thấy đơn đã sẵn sàng giao nếu chỉ đọc 20 đơn mới nhất (script đã đổi sang lọc theo trạng thái).
- Chưa điều tra: khoảng 0,05% lời gọi buyer trả 401 ở 200 CCU (nghi quanh đổi mật khẩu thu hồi token) và 2 lần 400 ở `checkout/preview`.
- Bước tiếp: sửa outbox rồi đo lại 100/200 CCU và độ trễ payment; thử 300-400 CCU; đo ai-agent 1 replica; đo với ttft và token/s thật (cần duyệt riêng vì tốn tiền).

### 5b. Sau khi sửa outbox (2026-11-23, cùng kịch bản, mô hình giả đã kiểm)
| | 100 CCU trước → sau | 200 CCU trước → sau |
|---|---|---|
| Lời gọi/s | 143 → 144 | 288 → 286 |
| Đơn tạo | 415 → 473 | 860 → 1081 |
| Đơn giao đi / hoàn tiền | 126 / 66 → 174 / 68 | 219 / 143 → 324 / 151 |
| Đặt đơn → payment xuất hiện (p50 / p95) | quá ~8 s (đa số) → 0,5 / 1,0 s | p50 15 s, p95 50 s → 0,5 / 1,0 s |
| Đơn online quá 8 s chưa có payment | gần hết → 0/188 | 294/294 → 0/395 |
| Độ trễ outbox order-service p50 / p95 | 7 / 29 s → 0,10 / 0,23 s (200 CCU) | |
| Độ trễ outbox product-service p50 / p95 | 10 / 24 s → 0,11-0,28 / 0,21-0,43 s (200 CCU) | |
| CPU đỉnh postgres (giới hạn 3 core) | 25% → | 62% → 148% (của 1 core, tức ~49% giới hạn) |
- Sửa: `events.RunMany` + `KafkaProducer.PublishMany` (một lần ghi Kafka mỗi topic cho cả lô, tick 200 ms, lô 500, rút liền cả hàng đợi) và worker trả lời kiểm kho của product-service (16 luồng, tick 200 ms). Chi tiết: `docs/planning/01_OPEN_ISSUES.md` R11, `docs/decisions/01_ADR_LOG.md`.
- Vì luồng mua hàng không còn kẹt, cùng 200 CCU làm được nhiều việc hơn (đơn +26%, giao đi +48%), nên tải dồn xuống postgres (CPU gấp 2,4 lần, nay là thành phần nặng nhất) và auth-service (~80%). **Điểm nghẽn tiếp theo nhiều khả năng là postgres, rồi băm mật khẩu ở auth-service**: nên đo 300-400 CCU và xem truy vấn chậm, kết nối, khoá.
- Vẫn còn ~0,07% lỗi 401 phía buyer chưa giải thích (R12).
