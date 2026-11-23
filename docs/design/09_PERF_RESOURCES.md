# 09. Phân bổ tài nguyên cho thử tải CCU (cấu hình `.perf`)

Hai file `deploy/services.perf.yml` và `deploy/infra.perf.yml` chỉ dùng để thử tải. **Trạng thái: ĐÃ ÁP DỤNG lên Swarm ngày 2026-11-23** (infra + services, kiểm: mọi service đủ replica, gateway giới hạn 100000/phút, mỗi `X-Forwarded-For` có bộ đếm riêng, Postgres `max_connections=200`, ClickHouse đã bỏ 6 bảng log).

Áp dụng: `./scripts/deploy-perf.sh all` (hoặc `infra` / `services`). Trở lại bình thường: `./scripts/deploy.sh infra && ./scripts/deploy.sh services`. Cần script riêng vì `docker stack deploy` đòi `cpus` là chuỗi. Gọi API từ ngoài phải có header `Host: api.<PUBLIC_HOST>`. IP giả bằng `X-Forwarded-For` xem [08_NETWORK_CLIENT_IP.md](08_NETWORK_CLIENT_IP.md).

## 1. Ngân sách (máy 16 luồng = 10 nhân vật lý i5-13450HX, 31 GB; trừ 3 GB container ngoài dự án, 3 GB OS + k6)
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

### 5c. 300 và 500 CCU (2026-11-23, sau sửa R12, mô hình giả)

| | 200 CCU | 300 CCU | 500 CCU |
|---|---|---|---|
| Lời gọi/s (ổn định) | 286 | 427 | 575 |
| Lỗi ngoài dự kiến | 0,07% | 0,00% | 0,79% (đa số lúc vào) |
| Đơn tạo / VU hỏng | 1081 / 0 | (0 lỗi) | 1677 / 107 |
| Payment tạo bất đồng bộ p50/p95 | 506/1014 ms | 504/1007 ms | 504/515 ms, 0 quá 8 s |
| AI cả lượt p50/p95 (giả) | ~2,4/2,9 s | 2408/2607 ms | 2392/2618 ms |

Tài nguyên đỉnh (CPU % của 1 core mỗi container; % so với giới hạn): 300 CCU: auth-service 105 (105%), postgres 93 (31% của 3 core), order 32, ai-agent 38, traefik 31; 500 CCU: postgres 193 (64%), auth-service 107 (107%), clickhouse 100, order 49, ai-agent 45, product 43, traefik 41. RAM đều thấp (broker ~800MB/1,5GB, mọi service Go < 120MB, tempo chạm 512MB = 100% ở 500 CCU).

**Giới hạn:** auth-service (bcrypt trên 1 CPU) chạm trần từ 300 CCU. Ở 500 CCU, 500 người cùng đăng ký/đăng nhập trong pha vào làm register/login p50 3,6-5,9 s và 504 sau 10 s (107 VU hỏng); sau khi vào xong, login p50 62 ms và không còn 504. Các 401 ở 500 CCU nghi do `change-password` bị 504 ở phía client nhưng auth-service vẫn áp dụng, nên token của VU bị thu hồi (chưa xác nhận bằng log). Cách nới: tăng replica/CPU của auth-service (chạy 2-3 replica), hoặc giảm cost bcrypt khi thử tải.

### 5d. Một cấu hình cố định, 100-800 CCU (2026-11-23, mô hình giả)

Cấu hình cố định cho mọi mức: gateway 3 bản, ai-agent 3 bản, auth-service 2 bản, postgres 3 core, tempo 1 GB; VU vào 5/giây (`ENTRY_RATE`), 15 s ổn định rồi đo 90 s, lượt vào thử lại tối đa 3 lần khi 5xx/timeout.

| CCU | 100 | 200 | 300 | 500 | 800 |
|---|---|---|---|---|---|
| Lời gọi/s | 142.2 | 286.3 | 424.7 | 497.0 | 500.0 |
| Lỗi (%) | 0.00 | 0.00 | 0.00 | 0.00 | 0.94 |
| Payment p50 (ms) | 509 | 504 | 505 | 6656 | 7608 |
| Đơn quá 8 s chưa có payment | 0 | 0 | 0 | 503 | 530 |
| AI cả lượt p50 (ms) | 2431 | 2425 | 2453 | 2945 | 3765 |
| AI p95 (ms) | 2643 | 2600 | 2650 | 3474 | 4722 |
| Đơn tạo | 433 | 971 | 1571 | 2170 | 2411 |
| VU vào hỏng | 0 | 0 | 0 | 0 | 2 |

**CPU đỉnh (% của 1 core, một container; trong ngoặc: % so với giới hạn CPU) / RAM đỉnh MiB (% giới hạn RAM)**

| Service | 100 CCU | 200 CCU | 300 CCU | 500 CCU | 800 CCU |
|---|---|---|---|---|---|
| ai-agent | 34 (34%) / 110M (14%) | 32 (32%) / 119M (15%) | 38 (38%) / 117M (15%) | 43 (43%) / 119M (15%) | 57 (57%) / 121M (16%) |
| analytics-service | 7 (15%) / 28M (11%) | 7 (15%) / 26M (10%) | 10 (21%) / 26M (10%) | 13 (26%) / 26M (10%) | 11 (22%) / 26M (10%) |
| api-gateway | 15 (10%) / 43M (8%) | 14 (9%) / 44M (9%) | 24 (16%) / 44M (9%) | 26 (17%) / 54M (11%) | 24 (16%) / 63M (12%) |
| auth-service | 77 (77%) / 38M (10%) | 73 (73%) / 38M (10%) | 101 (101%) / 38M (10%) | 105 (105%) / 36M (9%) | 106 (106%) / 37M (10%) |
| broker1 | 13 (13%) / 816M (53%) | 15 (15%) / 829M (54%) | 26 (26%) / 848M (55%) | 71 (71%) / 862M (56%) | 71 (71%) / 885M (58%) |
| broker2 | 13 (13%) / 858M (56%) | 14 (14%) / 868M (57%) | 30 (30%) / 886M (58%) | 62 (62%) / 903M (59%) | 66 (66%) / 921M (60%) |
| broker3 | 11 (11%) / 828M (54%) | 12 (12%) / 838M (55%) | 21 (21%) / 853M (56%) | 55 (55%) / 875M (57%) | 38 (38%) / 900M (59%) |
| clickhouse | 42 (21%) / 425M (21%) | 42 (21%) / 458M (22%) | 43 (22%) / 515M (25%) | 59 (29%) / 569M (28%) | 129 (64%) / 561M (27%) |
| frontend | 14 (14%) / 52M (10%) | 19 (19%) / 53M (10%) | 34 (34%) / 51M (10%) | 32 (32%) / 51M (10%) | 37 (37%) / 52M (10%) |
| ops-service | 7 (13%) / 14M (14%) | 5 (10%) / 14M (14%) | 13 (27%) / 13M (14%) | 16 (31%) / 14M (15%) | 15 (30%) / 19M (19%) |
| order-service | 26 (26%) / 34M (9%) | 24 (24%) / 36M (9%) | 49 (49%) / 35M (9%) | 53 (53%) / 41M (11%) | 51 (51%) / 57M (15%) |
| payment-service | 7 (13%) / 28M (11%) | 6 (11%) / 28M (11%) | 12 (25%) / 29M (12%) | 18 (36%) / 25M (10%) | 19 (37%) / 27M (11%) |
| postgres | 60 (20%) / 310M (8%) | 75 (25%) / 324M (8%) | 179 (60%) / 368M (9%) | 318 (106%) / 610M (15%) | 317 (106%) / 672M (16%) |
| product-service | 20 (20%) / 31M (8%) | 19 (19%) / 30M (8%) | 39 (39%) / 39M (10%) | 63 (63%) / 46M (12%) | 63 (63%) / 55M (14%) |
| redis | 10 (10%) / 8M (2%) | 5 (5%) / 8M (2%) | 9 (9%) / 7M (1%) | 10 (10%) / 7M (1%) | 9 (9%) / 12M (2%) |
| traefik | 20 (10%) / 48M (9%) | 26 (13%) / 55M (11%) | 41 (21%) / 88M (17%) | 42 (21%) / 150M (29%) | 35 (18%) / 257M (50%) |
| user-service | 3 (7%) / 26M (10%) | 4 (8%) / 27M (11%) | 9 (17%) / 25M (10%) | 8 (16%) / 23M (9%) | 12 (23%) / 24M (9%) |
| cadvisor | 14 / 253M (66%) | 12 / 254M (66%) | 12 / 254M (66%) | 14 / 253M (66%) | 13 / 253M (66%) |
| grafana | 5 / 66M (17%) | 2 / 66M (17%) | 4 / 67M (17%) | 2 / 68M (18%) | 3 / 69M (18%) |
| kafka-exporter | 4 / 18M (28%) | 3 / 18M (28%) | 3 / 18M (28%) | 4 / 18M (28%) | 3 / 19M (29%) |
| loki | 6 / 206M (40%) | 4 / 246M (48%) | 6 / 250M (49%) | 6 / 272M (53%) | 5 / 317M (62%) |
| node-exporter | 9 / 17M (13%) | 4 / 16M (13%) | 3 / 17M (13%) | 2 / 17M (13%) | 2 / 16M (13%) |
| postgres-exporter | 5 / 17M (27%) | 3 / 19M (30%) | 4 / 17M (27%) | 3 / 18M (28%) | 3 / 17M (27%) |
| prometheus | 10 / 206M (27%) | 11 / 194M (25%) | 18 / 192M (25%) | 16 / 200M (26%) | 13 / 210M (27%) |
| redis-exporter | 1 / 13M (21%) | 1 / 14M (22%) | 3 / 14M (22%) | 1 / 15M (24%) | 1 / 15M (24%) |
| tempo | 7 / 339M (33%) | 5 / 438M (43%) | 5 / 438M (43%) | 4 / 140M (14%) | 4 / 162M (16%) |
| vector | 37 / 60M (47%) | 40 / 62M (48%) | 66 / 63M (50%) | 69 / 70M (54%) | 61 / 75M (58%) |

Nhận xét: Postgres chạm trần 3 core từ 500 CCU (106%): lời gọi/s dừng ở ~500, outbox chậm theo (payment p50 6,6-7,6 s, ~500 đơn hết hạn chờ), từ 800 CCU lỗi ngoài dự kiến 0,94%. Sức chịu thực tế giữa 300 và 500 CCU. auth-service chạm 100% CPU từ 300 CCU dù 2 bản: gateway gọi `auth-service:50051` qua VIP Swarm, gRPC giữ kết nối lâu nên mỗi kết nối dính một bản (nghi một bản chạy trần, bản kia nhàn; chưa đo từng bản). gateway tổng ~71% của một core ở 300-500 CCU nên thừa bản; ai-agent tổng ~85% đỉnh ở 500 CCU. RAM không phải nút thắt (cao nhất broker ~60%, traefik 50% ở 800).

**Đã chỉnh sau lượt đo này, CHƯA đo lại:** postgres 3 -> 5 core, gateway 3 -> 2 bản, ai-agent 3 -> 2 bản (`deploy/*.perf.yml`); auth-service `endpoint_mode: dnsrr` trong `deploy/services.yml` để gRPC round_robin chia đều các bản; cảnh báo `OutboxPublishLag` (`mm_outbox_oldest_pending_seconds > 5` trong 2 phút) trong `deploy/prometheus/alerts.yml`, gắn vào playbook `outbox-stuck` (cần nạp lại Prometheus để có hiệu lực).

### 5e. Dải đo cố định cấu hình, vòng 1: đóng 100-1000 CCU và mở 5-40 phiên/giây (2026-11-23)

Máy: i5-13450HX (10 nhân vật lý, 16 luồng; trước đây ghi "16 core" là 16 luồng), Linux, chế độ nền tảng `balanced`, cắm điện, mô hình AI giả (đã kiểm). Máy tải (node) gắn `taskset -c 14,15`, cùng một máy với hệ thống. Mỗi lượt: khởi động (đóng: vào 5 VU/giây rồi +15 s; mở: 30 s), đo 90 s. Dữ liệu thô: `docs/perf-results/2026-11-23/<nhãn>.{json,resources.json,config.json,txt,stats.txt}` (JSON giữ số trước khi làm tròn: đủ để tính lại %). Mới có vòng 1 (và thêm một lần lặp `closed-100-r2`); vòng 2-3 để dành vì dải đo bị dừng theo yêu cầu. Trước vòng này, DB được xoá sạch (`teardown.sh --volumes`) và build lại toàn bộ image.

| Lượt | Lời gọi/s | Lỗi ngoài dự kiến (số/tổng) | Mã lỗi chính (số lượng) | Payment p50/p95 ms | Đơn quá 8 s | VU/khách vào hỏng | CPU máy tb/đỉnh % | Node p99 ms | Đỉnh người cùng lúc |
|---|---|---|---|---|---|---|---|---|---|
| closed-100-r1 | 142.6 | 0% (0/14490) | - | 508/1014 (n=192) | 0 | 0 | 23.6/37.3 | 11.3 | - |
| closed-100-r2 | 143.5 | 0% (0/14931) | - | 505/1015 (n=176) | 0 | 0 | 19.6/31.8 | 11.1 | - |
| closed-200-r1 | 287.6 | 0% (0/29347) | - | 505/1010 (n=357) | 0 | 0 | 27.6/36 | 11.1 | - |
| closed-300-r1 | 419.2 | 0% (0/43306) | - | 505/1008 (n=568) | 0 | 0 | 39.9/51.1 | 11.8 | - |
| closed-500-r1 | 694.1 | 0% (0/72258) | - | 505/531 (n=911) | 0 | 0 | 54/84 | 15.3 | - |
| closed-800-r1 | 792.5 | 1.597% (1365/85489) | {'400': 22, '500': 1105, '502': 39, '503': 199} | 8070/8094 (n=10) | 842 | 0 | 78.9/94.1 | 26.4 | - |
| closed-1000-r1 | 807.3 | 3.712% (3183/85739) | {'400': 38, '401': 79, '500': 2509, '502': 98, '503': 451, '504': 8} | 0/0 (n=0) | 754 | 2 | 80.6/96.9 | 27.6 | - |
| open-5-r1 | 98.6 | 0.395% (35/8850) | {'403': 15, '404': 20} | 505/1010 (n=133) | 0 | 0 | 21.4/63.3 | 11.1 | 100 |
| open-10-r1 | 199.5 | 0.675% (121/17939) | {'403': 21, '404': 100} | 505/1015 (n=188) | 0 | 0 | 30.9/69.6 | 11.4 | 181 |
| open-20-r1 | 410.8 | 0.414% (153/36953) | {'403': 21, '404': 132} | 504/1012 (n=437) | 0 | 0 | 43.3/49.5 | 12.3 | 351 |
| open-30-r1 | 683.9 | 5.521% (3397/61529) | {'400': 26, '401': 90, '403': 135, '404': 168, '500': 1552, '502': 23, '503': 385, '504': 1022} | 16501/29546 (n=14) | 441 | 27 | 99/100 | 32.7 | 1265 |
| open-40-r1 | 713.8 | 11.563% (7426/64222) | {'401': 3, '403': 37, '404': 156, '503': 3002, '504': 4192} | 507/1011 (n=716) | 0 | 740 | 67.3/78.2 | 20.4 | 1179 |

CPU đỉnh của một container (% của 1 core; trong ngoặc: % giới hạn CPU) / RAM đỉnh (% giới hạn RAM):

| Lượt | postgres | auth | order | product | gateway | traefik | ai-agent | clickhouse | broker1 | Tổng container (core) tb/đỉnh |
|---|---|---|---|---|---|---|---|---|---|---|
| closed-100-r1 | 42 (8%) / 144M (4%) | 15 (15%) / 23M (6%) | 19 (13%) / 24M (6%) | 16 (11%) / 24M (6%) | 20 (20%) / 36M (7%) | 18 (18%) / 31M (6%) | 36 (36%) / 89M (12%) | 29 (14%) / 193M (9%) | 18 (18%) / 649M (42%) | 1.63/2.91 |
| closed-100-r2 | 78 (16%) / 411M (10%) | 13 (13%) / 36M (10%) | 22 (14%) / 31M (8%) | 16 (10%) / 34M (9%) | 16 (16%) / 68M (13%) | 19 (19%) / 50M (10%) | 26 (26%) / 106M (14%) | 68 (34%) / 399M (20%) | 15 (15%) / 799M (52%) | 1.44/2.56 |
| closed-200-r1 | 68 (14%) / 162M (4%) | 38 (38%) / 25M (6%) | 23 (16%) / 25M (6%) | 18 (12%) / 25M (7%) | 20 (20%) / 40M (8%) | 19 (19%) / 37M (7%) | 16 (16%) / 90M (12%) | 28 (14%) / 236M (12%) | 37 (37%) / 662M (43%) | 2.48/3.42 |
| closed-300-r1 | 103 (20%) / 190M (5%) | 29 (29%) / 26M (7%) | 35 (24%) / 25M (7%) | 30 (20%) / 26M (7%) | 25 (25%) / 42M (8%) | 34 (34%) / 42M (8%) | 42 (42%) / 100M (13%) | 18 (9%) / 239M (12%) | 18 (18%) / 667M (43%) | 3.61/4.83 |
| closed-500-r1 | 254 (51%) / 244M (6%) | 45 (45%) / 26M (7%) | 58 (39%) / 27M (7%) | 49 (33%) / 27M (7%) | 39 (39%) / 47M (9%) | 47 (47%) / 55M (11%) | 60 (60%) / 92M (12%) | 84 (42%) / 291M (14%) | 24 (24%) / 688M (45%) | 6.31/9.12 |
| closed-800-r1 | 515 (103%) / 558M (14%) | 40 (40%) / 25M (7%) | 79 (53%) / 45M (12%) | 73 (48%) / 35M (9%) | 44 (44%) / 71M (14%) | 48 (48%) / 147M (29%) | 68 (68%) / 94M (12%) | 44 (22%) / 367M (18%) | 52 (52%) / 696M (45%) | 9.8/11.78 |
| closed-1000-r1 | 517 (103%) / 636M (16%) | 50 (50%) / 26M (7%) | 76 (51%) / 53M (14%) | 72 (48%) / 43M (11%) | 43 (43%) / 81M (16%) | 48 (48%) / 228M (44%) | 67 (67%) / 105M (14%) | 63 (32%) / 377M (18%) | 46 (46%) / 721M (47%) | 10.55/11.63 |
| open-5-r1 | 37 (7%) / 350M (9%) | 33 (33%) / 27M (7%) | 15 (10%) / 30M (8%) | 13 (9%) / 29M (8%) | 10 (10%) / 68M (13%) | 13 (13%) / 132M (26%) | 37 (37%) / 97M (13%) | 58 (29%) / 373M (18%) | 9 (9%) / 725M (47%) | 1.61/2.37 |
| open-10-r1 | 91 (18%) / 352M (9%) | 49 (49%) / 28M (7%) | 16 (11%) / 30M (8%) | 14 (9%) / 29M (8%) | 14 (14%) / 61M (12%) | 18 (18%) / 51M (10%) | 22 (22%) / 98M (13%) | 27 (13%) / 374M (18%) | 12 (12%) / 732M (48%) | 2.71/4.04 |
| open-20-r1 | 125 (25%) / 384M (9%) | 86 (86%) / 28M (7%) | 33 (22%) / 30M (8%) | 26 (17%) / 29M (8%) | 24 (24%) / 62M (12%) | 32 (32%) / 61M (12%) | 50 (50%) / 95M (12%) | 22 (11%) / 361M (18%) | 17 (17%) / 756M (49%) | 5.06/5.74 |
| open-30-r1 | 511 (102%) / 647M (16%) | 103 (103%) / 34M (9%) | 77 (52%) / 54M (14%) | 59 (39%) / 43M (11%) | 41 (41%) / 100M (20%) | 46 (46%) / 316M (62%) | 80 (80%) / 108M (14%) | 68 (34%) / 409M (20%) | 12 (12%) / 759M (49%) | 10.75/13.33 |
| open-40-r1 | 203 (40%) / 476M (12%) | 107 (107%) / 40M (10%) | 52 (35%) / 31M (8%) | 42 (28%) / 30M (8%) | 39 (39%) / 91M (18%) | 50 (50%) / 278M (54%) | 73 (73%) / 98M (13%) | 34 (17%) / 393M (19%) | 16 (16%) / 776M (50%) | 8.96/10.27 |

**Kết luận.**
- Đóng: sạch tới 500 CCU (694 lời gọi/s, 0 lỗi, payment p50 0,5 s). So với cấu hình cũ (§5d: 497 lời gọi/s, payment 6,6 s, 503 đơn quá hạn ở 500 CCU) là cải thiện rõ nhờ Postgres 3 -> 5 core và các chỉnh cấu hình ở §6. Gãy từ 800 CCU: Postgres 103% giới hạn (5 core), lỗi 500/503, payment 8 s. Ở 1000 CCU thông lượng không tăng nữa (807 lời gọi/s).
- Mở: sạch tới 20 phiên/giây (351 người cùng lúc, 411 lời gọi/s); gãy từ 30. Một phiên kéo dài khoảng 20 s khi khoẻ (5 phiên/s cho đỉnh 100 người).
- **Giới hạn gần đúng:** 500-800 CCU kiểu đóng, 20-30 phiên/giây kiểu mở. CPU toàn máy 94-100% ở đóng 800/1000 và mở 30 (vượt ngưỡng 75%): điểm gãy ở các mức này một phần do chính máy (dùng chung với máy tải) chứ không hoàn toàn do ứng dụng. Độ trễ vòng lặp node thấp (p99 11-33 ms) nên máy tải không phải nút thắt.
- **Điểm nghẽn thật của chuỗi bất đồng bộ là consumer Kafka tuần tự của product-service**, không phải outbox: độ trễ phát outbox tối đa 3,1 s (product-service, đóng 1000), nhưng lag consumer lúc đóng 800/1000: `product-service-validate-order` 452/594 tin, `product-service-ship-order` 932/1153 tin (đóng 500: 0). `shared/messagequeue/kafkaimpl/kafka_consumer.go` đọc một tin, xử lý, commit rồi mới đọc tiếp, mỗi nhóm một goroutine; product-service chỉ 1 bản (CPU 73% của 1,5 core, chưa trần), nên thông lượng bị giới hạn bởi độ trễ mỗi giao dịch Postgres. Chuỗi: order phát `create_order` -> product giữ kho -> `validate_order` -> order -> payment. Lag dồn lên thì payment trễ hàng chục giây. Đó là lý do payment p50 ở đóng 1000 "không có mẫu" (n=0): cả 754 đơn không có payment trong 8 s, script chỉ ghi thời gian khi payment xuất hiện. Outbox đã rỗng sau khi dừng tải (mọi dòng `SUCCESS`).
- **Vì sao mở 30 nhiều vấn đề hơn mở 40:** gãy ở hai chỗ khác nhau. Mở 30 gãy ở đường dữ liệu: khách vào được, dồn vào Postgres và consumer, lỗi 500 (1588) và 504 (1030) tập trung ở `GET /checkouts/:id` và giỏ hàng, payment p50 16,5 s, 441 đơn quá hạn. Mở 40 gãy ở cửa vào: auth-service chạm trần (107%) nên `POST /auth/login` (2216 lỗi 503/504) và `/auth/register` (1212) bị từ chối, 740 khách không vào được; khách không vào thì không tạo tải xuống Postgres (chỉ 40% giới hạn) nên payment vẫn 0,5 s và 0 đơn quá hạn. Tức tỉ lệ lỗi ở 40 cao hơn (11,6% so với 5,5%) nhưng phần bên trong lại khoẻ hơn 30; auth đóng vai van tự đóng ở cửa (và khách vào lỗi không trả tài khoản về kho nên phải đăng ký thêm, càng tốn bcrypt).
- Còn lại chưa điều tra: `GET /ops/outbox/:svc/dead` trả 502 khi tải nặng (39 lần ở đóng 800, 23 ở mở 30).

## 6. Nhật ký thay đổi cấu hình và phương pháp (2026-11-23)

Mọi lần chỉnh phải ghi lại, vì kết quả chỉ so sánh được khi biết cấu hình và cách đo lúc đó (mỗi lượt đo cũng chụp `<nhãn>.config.json`: số bản, giới hạn, image, git sha, chế độ nền tảng).

| Thay đổi | Từ -> sang | Lý do / bằng chứng | Ảnh hưởng đo được |
|---|---|---|---|
| Giới hạn tốc độ, tin X-Forwarded-For | mặc định -> 100000/phút, Traefik và gateway tin proxy | giả lập IP riêng cho từng VU (`deploy/services.perf.yml`) | không bị 429 |
| ClickHouse log hệ thống | bật -> tắt 6 bảng, TTL 7 ngày | CPU/RAM lúc rảnh (172M dòng `asynchronous_metric_log`) | hết ngốn CPU lúc rảnh |
| Kafka heap | mặc định -> 1 GB | tránh OOM | – |
| Outbox | `events.Run` -> `RunMany` + `PublishMany` (R11) | payment trễ 15 s ở 200 CCU | payment p50 15 s -> 0,5 s |
| Token sau đổi mật khẩu (R12) | Redis lệch phiên bản -> hỏi DB | token mới bị 401 oan 1,5-2 s | hết 401 oan |
| Image build | `kafkaimpl` import `events` -> kiểu `Message` chuyển sang `kafkaimpl` | commit trước làm gateway/auth/user/analytics không build được (thiếu go.sum gorm) | build lại được |
| tempo | 512 MB -> 1 GB | chạm 100% RAM ở 500 CCU | – |
| auth-service | 1 -> 2 -> 3 bản, `endpoint_mode: dnsrr` | chạm 100% CPU từ 300 CCU; gRPC qua VIP dính một bản | hết nghẽn ở cửa tới mở 20 |
| api-gateway | 3 bản x 1,5 core -> 2 bản x 1 core | đỉnh 26% của một core | không ảnh hưởng |
| ai-agent | 3 -> 2 bản (RCA tắt vì S9) | đỉnh ~85% tổng của một core | – |
| postgres | 3 -> 5 core | chạm 106% giới hạn ở 500 CCU | đóng 500: 497 -> 694 lời gọi/s |
| order-service | 1 -> 1,5 core | còn dự phòng cho đường mua hàng | – |
| product-service (R13, đợt 2, CHƯA ĐO) | 1 reader Kafka mỗi nhóm -> 3 reader cùng nhóm trong một tiến trình (`KAFKA_CONSUMER_WORKERS=3`), vẫn 1 bản x 1,5 core | consumer tuần tự: lag 452/594 tin ở đóng 800/1000 dù CPU product-service chỉ 73%; 3 partition chia đều nên mỗi reader nhận một partition, thứ tự theo đơn giữ nguyên (khóa = mã đơn, Hash) | cần đo lại lag và payment ở đóng 800/1000, mở 30 |
| Mọi service dùng Kafka consumer (R13, đợt 2, CHƯA ĐO) | commit offset từng tin (đồng bộ) -> theo lô mỗi 100 ms (`KAFKA_COMMIT_INTERVAL_MS`, 0 = như cũ) | mỗi tin tốn thêm một vòng gọi broker chỉ để commit; handler idempotent nên giao lại tối đa 100 ms khi sập là chấp nhận được | cần đo lại cùng lag |
| Pool Postgres mỗi service (R15, đợt 2, CHƯA ĐO) | không giới hạn (trừ auth 25, analytics 2) -> order 30, product 30, payment 15, auth 10 (x3 bản), user 10, gateway 5 (x2 bản), ops 5, analytics 2: tổng tối đa ~135 + exporter/quản trị, `max_connections` giữ 200 | Postgres từ chối kết nối ở đóng 800/1000 và mở 30 (~2500+ dòng FATAL mỗi lượt) | cần đo lại: kỳ vọng hết FATAL, yêu cầu thừa xếp hàng trong ứng dụng (tăng độ trễ thay vì 500) |
| traefik | 2 -> 1 core | đỉnh 42% | – |
| analytics flush ClickHouse | 2 s -> 20 s (`ANALYTICS_FLUSH_INTERVAL_MS`) | ghi lẻ tẻ: 8960 lần ghi cho 422 nghìn dòng (~47 dòng/lần), CPU tốn vào gộp part; báo cáo trễ tối đa 20 s | chưa tách riêng được |
| Cảnh báo | thêm `OutboxPublishLag` (`mm_outbox_oldest_pending_seconds > 5` trong 2 phút) | tín hiệu sớm của nghẽn Postgres/Kafka; gắn playbook `outbox-stuck`; cần nạp lại Prometheus | – |
| Dữ liệu | xoá sạch volume Postgres/Kafka/ClickHouse/Redis, dựng lại, build lại mọi image | dữ liệu thử tải cũ (2820/2953 tài khoản) làm lệch | DB bắt đầu 10 MB |

Phương pháp (`scripts/load/ccu.mjs`, `perf-run.sh`, `perf-matrix.sh`, `perf_summary.py`):
- Cách vào của VU cố định cho mọi mức đóng: `ENTRY_RATE` 5 VU/giây (ramp = VUS/5), +15 s; lượt vào bị 5xx/timeout thử lại tối đa 3 lần.
- **Chế độ đóng** (`--vus=N`): số người cố định, mỗi người gửi, chờ trả lời, nghĩ rồi gửi tiếp. Hệ thống chậm thì mỗi người gửi ít hơn nên tải tự giảm (800 người chỉ cho 793 lời gọi/s). **Chế độ mở/nửa mở** (`--arrival-rate=R`): R khách mới mỗi giây đều đặn, mỗi khách chọn loại theo tỉ lệ `MIX` (buyer 50, seller 12, admin 8, operator 8, AI 24) rồi làm khoảng 3 hành trình (`SESSION_JOURNEYS`) và rời đi; hệ thống chậm thì người dồn lại, tải không giảm. CCU ở chế độ mở là kết quả (`đỉnh người cùng lúc` = nhịp x độ dài phiên, ~20 s khi khoẻ), không phải tham số. Khách đến cách đều nhau (chưa Poisson).
- Chế độ mở dùng lại tài khoản cũ (chỉ đăng nhập, không đăng ký mỗi phiên, vì bcrypt của auth-service sẽ thành thứ bị đo); kho tài khoản riêng cho buyer, khách AI (tài khoản AI không có địa chỉ mặc định, dùng chung sẽ gây `POST /orders` 404 "address not found"), seller AI, staff admin, staff operator.
- Sonde máy tải trong pha đo: độ trễ vòng lặp sự kiện node (cảnh báo p99 > 100 ms), CPU toàn máy từ `/proc/stat` (cảnh báo > 75%), RAM trống thấp nhất; node gắn 2 core riêng.
- Báo cáo ghi số thô: từng nhãn lời gọi (n, ok, dự kiến, 429, lỗi, p50/p95/p99/max, từng mã lỗi kèm số lượng), tổng, độ phủ, bộ đếm nghiệp vụ, AI, payment, tài nguyên từng service trong đúng cửa sổ đo (trung bình/đỉnh một container, cộng các bản, % giới hạn, RAM).
- Việc cần nhớ khi so sánh: máy tải và hệ thống dùng chung một máy, DB bắt đầu rất nhỏ, dải đo chỉ mới 1 vòng (độ dao động mới có một cặp: đóng 100, 142,6 và 143,5 lời gọi/s, nhưng CPU Postgres 42% rồi 78%).

**Truy vấn chậm và hết kết nối (điều tra 2026-11-23 trên Loki/Prometheus cũ của vòng 1):**
- Không có truy vấn nào vốn chậm: ở đóng 100/200/300 không có câu nào >= 200 ms. Ở đóng 800 và mở 30 các câu chậm là tra cứu theo khóa chính (`products WHERE id IN`, `checkouts WHERE id = ? AND buyer_id = ?`, `payments WHERE checkout_id = ?`, `accounts WHERE id = ?`), trung bình 400-1000 ms, tức DB bão hòa chứ không thiếu chỉ mục. Postgres lúc đó có 43-59 phiên `active` trên 5 core, 17-30 phiên `idle in transaction`, không có deadlock; bảng khóa (`pg_locks`) lên 450+.
- Nguyên nhân gốc của nhiều lỗi 500/502/503 là R15: Postgres từ chối kết nối (`too many clients already`) ở đóng 800/1000 và mở 30 (0 ở các mức thấp hơn). Chỉ auth-service có giới hạn pool; các service khác mở kết nối không giới hạn. Prometheus quét 15 s chỉ thấy đỉnh 159/200 nên không báo.
- Lỗi handler consumer vì hết kết nối chỉ vài chục lần (15 ở product-service đóng 800), nên lag R13 vẫn chủ yếu do consumer tuần tự, hết kết nối chỉ góp thêm.

**Điều tra sau vòng 1 (2026-11-23, chưa đo lại):**
- R13 xác nhận từ mã và số liệu: `kafkaimpl.Consume` là một goroutine fetch -> xử lý -> commit cho cả nhóm, product-service chỉ 1 bản nên một goroutine phục vụ cả 3 partition. Partition chia đều (order.create_order 6697/6672/6684, order.status_changed 22548/22303/22488) nên 3 bản sẽ chia việc đều. Đã làm: `KAFKA_COMMIT_INTERVAL_MS=100` (commit gộp) và `KAFKA_CONSUMER_WORKERS` (shared/messagequeue/kafkaimpl) chạy N reader cùng nhóm trong một tiến trình, product-service đặt 3 trong `services.perf.yml`; nếu chưa đủ mới tính tăng replica. Chưa làm consumer song song theo khóa trong cùng partition (commit offset phải đúng thứ tự).
- R14: gateway chỉ chuyển tiếp mã của ops-service, nên 502 nằm ở lời gọi ops-service -> cổng admin `:8081` của service đích (khác 503 của gateway khi ops-service không trả lời trong 5 s). Truy vấn không chậm (có chỉ mục riêng cho hàng DEAD) và p50 ~230 ms, max 1,8 s nên không phải timeout 15 s. Nhật ký cũ đã quay vòng, không còn nguyên nhân thật. Đã thêm log nguyên nhân ở ops-service (`outbox dead read failed`) và ở cổng admin dùng chung (`list dead failed`); lần chạy tải sau sẽ thấy.
- `POST /checkout/preview` 400 (22 ở đóng 800, 26 ở mở 30, 0 ở đóng 100/500): không phải lỗi. Chỉ xuất hiện khi `PUT /cart/items/:id` cùng lượt bị 500/503 (66+164 ở đóng 800) nên giỏ rỗng và `from_cart` bị từ chối; hậu quả của quá tải, không phải lỗi riêng.

**Ngân sách kết nối Postgres (R15, 2026-11-23, mã `shared/dbpool`, chưa đo lại):**

| Service | Bản | Trần mỗi bản | Tổng tối đa | Lý do |
|---|---|---|---|---|
| order-service | 1 | 30 | 30 | nhiều việc nhất: giỏ, checkout, đơn, outbox |
| product-service | 1 | 30 | 30 | catalog, giữ tồn, 3 worker Kafka, outbox |
| payment-service | 1 | 15 | 15 | thanh toán giả lập, outbox |
| auth-service | 3 | 10 | 30 | nghẽn ở bcrypt (CPU), không phải DB; trước đây 25 mỗi bản (75) |
| user-service | 1 | 10 | 10 | hồ sơ, địa chỉ |
| api-gateway | 2 | 5 | 10 | dùng DB rất ít |
| ops-service | 1 | 5 | 5 | sự cố, sổ kiểm toán |
| analytics-service | 1 | 2 | 2 | chỉ đối soát |
| **Tổng** | | | **~132** | |

- `max_connections=200` là đủ, không nâng: còn ~68 kết nối cho postgres-exporter, backup, phiên quản trị và cập nhật cuốn chiếu (Swarm bật bản mới trước khi tắt bản cũ, tạm thời cộng thêm pool của đúng một service). Với 5 core, số truy vấn chạy song song hữu ích chỉ khoảng 10-20 (đã thấy 43-59 phiên `active` lúc nghẽn); thêm kết nối chỉ thêm chuyển ngữ cảnh và RAM (vài MB mỗi kết nối).
- Quy tắc: tổng trần của mọi bản mọi service phải nhỏ hơn `max_connections` với dư địa nói trên. Thêm replica hoặc service mới thì tính lại bảng này.
- Hệ quả mong đợi khi đo lại: hết `FATAL: sorry, too many clients already`; yêu cầu thừa xếp hàng trong ứng dụng, bị chặn bởi deadline gRPC của gateway, nên lỗi 500 vì hết kết nối chuyển thành độ trễ cao hơn (hoặc 504 nếu quá hạn). Cần xem lại ngưỡng "chạm giới hạn" sau đo.
- Chỉnh: mặc định theo service nằm trong mã; `DB_MAX_OPEN_CONNS`/`DB_MAX_IDLE_CONNS` (`deploy/.env`) ghi đè cho mọi service; chỉnh riêng một service thì thêm biến vào `environment:` của nó trong `deploy/services.perf.yml`.

**Cách điều tra sau khi tắt Swarm:** volume `deploy_prometheus-data` và `deploy_loki-data` còn nguyên; sao chép sang thư mục tạm rồi chạy Prometheus/Loki tạm trên bản sao (không cần bật lại Swarm) để truy vấn các lượt đo cũ theo cửa sổ `steadyStartMs`/`steadyEndMs` trong JSON. Lưu ý Prometheus quét mỗi 15 s nên bỏ sót đỉnh ngắn (đỉnh kết nối chỉ thấy 159/200 trong khi log Postgres đã báo hết chỗ); log Loki mới là nguồn đáng tin cho lỗi tức thời.

**Việc tiếp theo đề xuất (chưa làm, chờ lệnh đo):** triển khai lại services (`./scripts/deploy-perf.sh services`) rồi đo lại đóng 500/800/1000 và mở 20/30/40, thêm đóng 600/700 và mở 25/35; chỉ thêm 1200/1500 và 50/60 nếu vẫn sạch.
