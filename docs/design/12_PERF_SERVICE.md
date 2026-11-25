# 12. Chịu tải từng service (đợt 11)

Các đợt trước (`09`, `10`) đo CẢ HỆ THỐNG bằng hành trình người dùng hỗn hợp (`ccu.mjs`); nút thắt là suy ra từ CPU và lỗi. Đợt này đo RIÊNG từng loại lời gọi để có
con số năng lực của từng service và tìm nút thắt chính xác hơn. Hỏng/gãy chỉ là chế độ hỏng của CẤU HÌNH NÀY (số bản và trần CPU của đợt 9-10), không phải trần của phần mềm.

## 1. Phương pháp

- Script `scripts/load/svc.mjs`, chạy qua `LOAD_SCRIPT=svc.mjs scripts/load/perf-one.sh NHÃN --target=a,b,c`. `perf-one.sh` vẫn dọn + dựng stack sạch (`deploy-perf.sh all`: infra + obs + services, mô hình giả), lưu capture/series/diag/log, rồi in thêm bảng CPU theo bậc (`scripts/load/perf-svc-report.py`).
- Tải MỞ: n yêu cầu/s CỐ ĐỊNH (không chờ hệ thống), tăng theo bậc 30 s cho tới khi gãy. Dừng target khi một bậc có lỗi > 30% hoặc xong < 50% số gửi.
- Token và tài khoản được dựng SẴN trước khi tăng tải (đăng ký + đăng nhập + địa chỉ, 8 luồng), nên auth không che các service khác; riêng auth thì chính là đối tượng đo.
- Cấu hình: y nguyên đợt 9-10 (gateway 2 bản, auth 5 bản x 1 nhân, ai-agent 2, còn lại 1; CPU/RAM theo `*.perf.yml`). Không đổi cấu hình nào.
- Sau mỗi bậc: gửi, xong, ok/s (thông lượng thật), % lỗi, 429, bỏ (vượt MAX_INFLIGHT), p50/p95/p99/max, CPU toàn máy, trễ vòng lặp node (máy tải). Bảng CPU: nhân dùng (và % trần), % chu kỳ bị throttle của từng container, đối chiếu theo mốc epoch của bậc.
- Máy tải gắn 2 nhân (14,15) như các đợt trước. Mọi lượt dưới đây có CPU toàn máy < 75% và trễ vòng lặp node p99 ~11-12 ms (máy tải không nghẽn).
- Smoke test script trước khi đo (3 yêu cầu/s x 4 s cho cả 16 target, rồi `perf-one.sh` với `DEPLOY=0`): cả 16 target chạy, không lỗi; kết quả smoke không dùng.

Dữ liệu thô: `docs/perf-results/2026-11-25-dot11/` (git-ignored).

## 2. Nhóm A: auth (`svc-A-auth`)

### 2.1 `POST /auth/login` (200 tài khoản có sẵn, mật khẩu đúng)

| tải/s | ok/s | lỗi | p50 ms | p95 ms | p99 ms | CPU auth (5 bản, trần 5,0) | throttle auth | CPU toàn máy |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 10 | 10,0 | 0 | 61 | 76 | 80 | 0,63 (13%) | 0% | 12% |
| 20 | 20,0 | 0 | 59 | 61 | 67 | 1,17 (23%) | 0% | 19% |
| 40 | 40,0 | 0 | 59 | 78 | 92 | 2,32 (46%) | 7% | 28% |
| 60 | 60,0 | 0 | 59 | 91 | 96 | 3,47 (69%) | 15% | 38% |
| 80 | 80,0 | 0 | 60 | 94 | 105 | 4,63 (93%) | 41% | 44% |
| **100** | 97,4 | **2,6%** | **3 062** | 7 779 | 10 003 | 5,00 (100%) | 100% | 45% |
| 130 | 84,2 | 35,2% | 6 817 | 10 003 | 10 004 | 5,00 (100%) | 100% | 47% |

- CPU của auth tăng TUYẾN TÍNH theo tải: ~0,058 nhân cho mỗi đăng nhập/s (≈58 ms CPU mỗi lần, khớp bcrypt ~56 ms đã đo ở 10_PERF_SPIKE). Ngoài auth không service nghiệp vụ hay hạ tầng nào dùng >= 0,1 nhân (Postgres, gateway, Redis đều dưới ngưỡng; chỉ vector 0,13 ở bậc cuối): nút thắt là CPU bcrypt của auth, KHÔNG phải Postgres hay gateway.
- Năng lực: **không lỗi tới 80 đăng nhập/s** (auth 93% trần, p95 94 ms, thêm 41% throttle); **gãy ở 100/s**. Từ 80 lên 100 chưa đo bậc giữa (90/s), nên trần chính xác nằm trong khoảng 80-100/s; ước lượng tuyến tính 5,0 / 0,058 ≈ 86/s (suy ra, chưa đo).
- Gãy kiểu "vách đá", không xuống từ từ: ở 100/s hàng đợi tăng (tải đến 100/s > năng lực ~86/s nên ~14 yêu cầu/s dồn lại), p50 vọt từ 60 ms lên 3 s.
- Nguồn mã lỗi (đối chiếu log gateway/auth, không suy đoán):
  - **504** (độ trễ đúng 10,0 s trong log gateway): gRPC deadline `GRPC_TIMEOUT_WRITE_MS=10000` (đăng nhập tính là ghi); `AuthClient: Login error ... DeadlineExceeded`.
  - **503**: `circuit open: AuthClient` (circuit breaker của gateway mở sau 5 lỗi hạ tầng liên tiếp, `CIRCUIT_FAILURE_THRESHOLD=5`); 282 lần ở bậc 130/s.
  - Log auth-service ghi `AuthService: DB error ... context canceled` 669 lần, `context deadline exceeded` 62 lần (`auth_service.go:174`). Nhãn "DB error" gây hiểu nhầm: đây là context bị gateway hủy khi quá deadline lúc yêu cầu còn chờ CPU (Postgres không bận: dưới 0,1 nhân), không phải lỗi truy vấn. (Nguyên nhân hủy suy ra từ trùng khớp 10 s deadline và CPU auth 100%; chưa soi từng truy vấn.)
- Không có 429 (rate limit đã nâng) và không có yêu cầu bị bỏ (MAX_INFLIGHT).

### 2.2 `POST /auth/register` (tên mới mỗi lần)

| tải/s | ok/s | lỗi | p50 ms | p95 ms | p99 ms | CPU auth | throttle auth |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 5 | 5,0 | 0 | 64 | 68 | 70 | 0,31 (6%) | 0% |
| 10 | 10,0 | 0 | 63 | 67 | 72 | 0,60 (12%) | 0% |
| 20 | 20,0 | 0 | 66 | 78 | 84 | 1,18 (24%) | 4% |
| 30 | 30,0 | 0 | 63 | 77 | 81 | 1,75 (35%) | 4% |
| 40 | 40,0 | 0 | 63 | 83 | 91 | 2,32 (46%) | 7% |
| 60 | 60,0 | 0 | 60 | 94 | 104 | 3,46 (69%) | 17% |

- Cũng ~0,058 nhân cho mỗi đăng ký/s (bcrypt khi băm mật khẩu). **Bậc cao nhất (60/s) vẫn chưa gãy** (auth 69% trần): KHÔNG đo được trần đăng ký trong bậc đã chọn. Theo hệ số tuyến tính, trần ước lượng cũng ~86/s (suy ra, chưa đo); muốn xác nhận cần thêm bậc 80 và 100/s.
- Target này chạy ngay sau khi login gãy (cách 20 s nghỉ + chuẩn bị): bậc 5/s đầu sạch, nên hệ thống tự hồi phục trong thời gian đó (không đo thời gian hồi phục chính xác).

### 2.3 Nhận định nhóm A

- Auth là service có năng lực THẤP nhất tính theo yêu cầu/s (~86/s trên 5 nhân) vì bcrypt; mọi service còn lại (đo ở các nhóm sau) sẽ so sánh với con số này. Phù hợp phát hiện ở `10_PERF_SPIKE.md`: spike mở 10→60 phiên/s gãy vì mỗi phiên mới đăng ký/đăng nhập.
- Chưa sửa gì (auth là giới hạn tài nguyên theo quy ước "cấu hình đã chốt").
- Việc tồn đọng: chưa đo bậc 90/s cho login và bậc 80-100/s cho register.

## 3. Nhóm B: gateway (`svc-B-gateway-frontend`, `svc-B2-gateway-high`)

Route đo: `GET /health` (gateway tự trả lời, KHÔNG gọi gRPC tới service nào). Nên đây là trần của đường Traefik → api-gateway (HTTP, middleware), chưa gồm chi phí gọi service; route có gRPC (product, user, order ...) sẽ thấp hơn và đo ở các nhóm sau. Mỗi yêu cầu có IP giả riêng (X-Forwarded-For ngẫu nhiên). Bậc 200-2000/s ở lượt B, bậc 3000-5000/s chạy tiếp trên cùng stack (B2, không dựng lại).

| tải/s | ok/s | lỗi | bỏ | p50 ms | p95 ms | p99 ms | Traefik (trần 0,7) | api-gateway (2 bản x 0,6) | vector | CPU toàn máy | trễ vòng lặp node p99 |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 200 | 200 | 0 | 0 | 3 | 4 | 26 | 0,18 (26%) | 0,13 (11%) | 0,22 | 15% | 11 ms |
| 500 | 500 | 0 | 0 | 3 | 5 | 52 | 0,34 (48%) | 0,27 (23%) | 0,37 | 20% | 12 ms |
| 1000 | 1000 | 0 | 0 | 1 | 3 | 22 | 0,26 (38%) | 0,21 (18%) | 0,31 | 19% | 16 ms |
| 1500 | 1500 | 0 | 0 | 1 | 3 | 19 | 0,30 (43%) | 0,24 (20%) | 0,37 | 22% | 20 ms |
| 2000 | 2000 | 0 | 0 | 2 | 6 | 46 | 0,37 (54%) | 0,30 (25%) | 0,46 | 24% | 22 ms |
| 3000 | 2999 | 0 | 0 | 2 | 21 | 67 | 0,50 (72%) | 0,40 (33%) | 0,65 | 29% | 26 ms |
| 4000 | 4000 | 0 | 0 | 3 | 35 | 65 | 0,64 (92%), throttle 34% | 0,51 (42%) | 0,82 | 34% | 27 ms |
| **5000** | 2 424 | 0 | **77 291** | **1 052** | 2 997 | 3 418 | **0,69 (99%), throttle 97%** | 0,37 (31%) | 0,49 | 28% | 59 ms |

- **Gateway không gãy tới 4000 yêu cầu/s trên route nhẹ**; ở 5000/s thông lượng thật chỉ 2 424/s, p50 vọt lên 1 s, và máy tải phải bỏ 77 291 yêu cầu (vượt MAX_INFLIGHT=3000: yêu cầu không còn trả về kịp). Không có lỗi HTTP (mọi yêu cầu xong đều 200).
- **Nút thắt là Traefik** (1 bản, trần CPU 0,7): 99% trần và 97% chu kỳ bị throttle ở bậc gãy, trong khi api-gateway (2 bản, trần tổng 1,2) chỉ dùng 0,37-0,51 nhân (31-42%). Mỗi yêu cầu tốn ~0,16 ms CPU ở Traefik và ~0,13 ms ở gateway. Trần Traefik ≈ 4000/s trên route nhẹ (0,7 / 0,16 ms ≈ 4400/s, ước lượng; bậc đo thực: 4000 ổn, 5000 gãy; chưa đo 4500).
- `vector` (gom log của observability) dùng 0,65-0,82 nhân ở 3000-4000/s, NHIỀU HƠN api-gateway; nó gom log container (suy ra: log truy cập mỗi yêu cầu của Traefik/gateway là phần chính, chưa kiểm). Không phải nút thắt (82% trần, throttle ≤ 4%) nhưng chia CPU máy với hệ thống; chỉ đo được chi phí, chưa thử tắt.
- Giới hạn của phép đo: máy tải là một tiến trình node (một luồng), trễ vòng lặp p99 tăng 11 → 27 ms khi tải 200 → 4000/s và 59 ms ở bậc 5000 (lúc đó có 3000 yêu cầu treo). Từ ~3000/s trở lên, độ trễ p95/p99 báo cáo có thể cộng thêm vài chục ms từ máy tải; thông lượng/lỗi ở bậc 5000 thì do hệ thống (CPU Traefik chạm trần là số đo độc lập).
- Cảnh báo CPU toàn máy: tối đa 34% (< 75%), phép đo hợp lệ.

## 4. Nhóm B: frontend (`svc-B-gateway-frontend`, `svc-B3-frontend-high`)

Tải: trang SSR qua Traefik, 4 loại ngẫu nhiên (`/`, `/products`, `/products/:id`, `/login`). Frontend 1 bản, trần 0,7 nhân.

| tải/s | ok/s | lỗi | p50 ms | p95 ms | p99 ms | CPU frontend (trần 0,7) | throttle |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 5 | 5 | 0 | 13 | 17 | 26 | dưới 0,1 | 0% |
| 10 | 10 | 0 | 12 | 15 | 16 | 0,11 (16%) | 0% |
| 20 | 20 | 0 | 12 | 14 | 15 | 0,20 (29%) | 0% |
| 40 | 40 | 0 | 12 | 14 | 15 | 0,37 (53%) | 1% |
| 60 | 60 | 0 | 11 | 14 | 41 | 0,54 (77%) | 16% |
| 80 | 80 | 0 | 12 | 56 | 156 | 0,65 (93%) | 51% |
| **100** | 82 | **17,9%** | 3 953 | 30 001 | 30 001 | 0,70 (100%) | 100% |
| 130 | 118,7 | 8,7% | 18 346 | 30 000 | 30 001 | 0,70 (100%) | 100% |

- CPU frontend tăng tuyến tính: ~8 ms CPU cho mỗi trang SSR (0,65 nhân / 80 trang/s). Trần ≈ 0,7 / 8 ms ≈ **85-90 trang/s**; đo thực: 80/s ổn (p95 56 ms, đã throttle 51%), 100/s gãy.
- Gãy kiểu hàng đợi: ở 100/s p50 lên 3,9 s, 538 yêu cầu quá 30 s (mã 0 = timeout phía máy tải); ở 130/s thêm 4 lần 502. Frontend CPU 100% trần, throttle 100%. Lượt B3 dừng ở bậc 2 theo quy tắc (xong < 50% số gửi); các bậc 160 và 200/s không chạy.
- **Frontend là service có trần thấp nhất sau auth khi tính theo yêu cầu trang**, và 1 bản. Nhất quán với `10_PERF_SPIKE.md` §4d: ở 1500 CCU frontend bị Swarm kill do healthcheck trễ.
- Lần này Swarm KHÔNG kill frontend dù CPU kịch trần ~60 s: task vẫn là bản cũ (không có bản thay thế trong `docker service ps`), nên healthcheck (`/api/healthz`, timeout 5 s) vẫn trả lời trong hạn khi chỉ có ~60 s quá tải. Điều kiện kill ở 1500 CCU (quá tải kéo dài hơn, tải nền khác) không tái hiện ở đây (chưa đo quá tải dài hơn).
- Hồi phục: ngay sau khi tải dừng, trang `/products` trả lời 12-29 ms (đo bằng curl), không cần can thiệp.
- Vì mọi người dùng thật đều đi qua frontend nên 85-90 trang/s là một trần thực tế của hệ thống ở 1 bản (xem tổng kết).

## 5. Nhóm C: product-service (`svc-C-product`, `svc-C2-product-list-high`, `svc-C3-search-high`, `svc-C4-product-detail-high`)

Ba route công khai (không token): `GET /products?page=&page_size=10` (danh sách), `GET /products/:id` (chi tiết), `GET /search?...&page_size=20` (tìm + lọc + sắp xếp). Lượt C đo 50-1200/s (search 20-600/s), không target nào gãy; các lượt C2-C4 chạy tiếp trên cùng stack để tìm trần. product-service 1 bản, trần 0,7 nhân; api-gateway 2 bản (trần 1,2); Traefik 0,7.

### 5.1 Danh sách sản phẩm `GET /products`

| tải/s | ok/s | lỗi | p50 | p95 | p99 ms | product-service (trần 0,7) | throttle PS | api-gateway (trần 1,2) | Traefik (0,7) | Postgres |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 200 | 200 | 0 | 2 | 3 | 9 | 0,18 (25%) | 0% | 0,17 | 0,12 | dưới 0,1 |
| 400 | 400 | 0 | 2 | 3 | 7 | 0,27 (38%) | 0% | 0,30 | 0,18 | 0,13 |
| 800 | 800 | 0 | 3 | 3 | 10 | 0,43 (61%) | 0% | 0,48 | 0,28 | 0,19 |
| 1200 | 1200 | 0 | 3 | 5 | 24 | 0,55 (79%) | 2% | 0,62 | 0,38 | 0,24 |
| 1500 | 1500 | 0 | 3 | 9 | 35 | 0,65 (93%) | 27% | 0,74 | 0,46 | 0,28 |
| **1800** | 1800 | 0 (1 lần 504) | 25 | 101 | 189 | 0,70 (**100%**) | **100%** | 0,79 | 0,53 | 0,33 |
| 2200 | 1623 | 10,4% (503: 3183, 504: 2461) | 1245 | 3250 | 3893 | 0,68 (97%) | 92% | 0,81 | 0,69 (98%), throttle 87% | 0,32 |
| 2600 | 1372 | (bỏ 36 842) | 2035 | 3357 | 4160 | 0,55 | 63% | 0,68 | 0,70 (100%), throttle 100% | 0,27 |

- **Trần ~1500-1800 req/s**, do **product-service** (CPU 100% và throttle 100% từ 1800/s; trước đó đã 93% ở 1500/s): ~0,43 ms CPU mỗi yêu cầu. 1500/s ổn (p95 9 ms); 1800/s chưa lỗi nhưng đã không còn dư (p95 100 ms, max 3 s); 2200/s gãy.
- Ở 2200-2600/s thêm Traefik chạm trần (98-100%) nên hai nút thắt chồng nhau; thông lượng thật chỉ 1370-1620/s, phần còn lại bị bỏ ở máy tải (MAX_INFLIGHT). Bậc 2600 hiện "lỗi 0%" vì chỉ tính yêu cầu ĐÃ xong, 36 842 yêu cầu bị bỏ không tính là lỗi: đọc kèm cột thông lượng thật.
- Postgres chỉ 0,13-0,33 nhân (≤ 11% trần): đọc danh sách không đè DB (suy ra từ CPU Postgres thấp; chưa soi truy vấn).

### 5.2 Tìm kiếm `GET /search`

| tải/s | ok/s | lỗi | p50 | p95 | p99 ms | product-service | throttle PS | api-gateway | Postgres |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 100 | 100 | 0 | 3 | 3 | 4 | 0,11 (15%) | 0% | 0,11 | dưới 0,1 |
| 200 | 200 | 0 | 3 | 3 | 6 | 0,20 (28%) | 0% | 0,21 | 0,13 |
| 400 | 400 | 0 | 3 | 3 | 8 | 0,30 (44%) | 0% | 0,37 | 0,20 |
| 600 | 600 | 0 | 3 | 4 | 11 | 0,40 (58%) | 0% | 0,49 | 0,26 |
| 800 | 800 | 0 | 3 | 5 | 31 | 0,50 (71%) | 2% | 0,64 | 0,32 |
| 1000 | 1000 | 0 | 3 | 5 | 16 | 0,58 (83%) | 2% | 0,75 | 0,35 |
| **1300** | 1014 | **18,1%** (503: 2366, 504: 4348) | 864 | 3916 | 5372 | 0,70 (**100%**) | 100% | 0,94 (79%) | 0,49 |
| 1700 | 738 | 23,0% | 2614 | 6276 | 7153 | 0,68 (97%) | 94% | 0,87 | 0,53 |

- **Trần ~1000-1300 req/s**, do **product-service** (~0,58 ms CPU mỗi yêu cầu, hơn danh sách ~35%). 1000/s ổn (p95 5 ms).
- Nguồn mã lỗi, đối chiếu log gateway: 504 = `DeadlineExceeded` đúng 3,0 s (`GRPC_TIMEOUT_READ_MS=3000`, 7 596 dòng log); 503 = `circuit open: ProductClient` (4 136 dòng); 20 dòng `RST_STREAM ... CANCEL`.

### 5.3 Chi tiết sản phẩm `GET /products/:id`

| tải/s | ok/s | lỗi | p50 | p95 | p99 ms | product-service | throttle PS | api-gateway | Traefik | vector |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 400 | 400 | 0 | 2 | 2 | 4 | 0,18 (26%) | 0% | 0,25 | 0,15 | 0,16 |
| 1200 | 1200 | 0 | 2 | 3 | 18 | 0,31 (45%) | 0% | 0,44 | 0,28 | 0,33 |
| 2000 | 2000 | 0 | 3 | 7 | 35 | 0,43 (62%) | 1% | 0,61 | 0,41 | 0,50 |
| 2500 | 2500 | 0 | 3 | 30 | 77 | 0,51 (72%) | 0% | 0,71 | 0,50 | 0,60 |
| 3000 | 3000 | 0 | 3 | 28 | 63 | 0,59 (84%) | 8% | 0,82 | 0,58 (83%), throttle 14% | 0,70 |

- **Không gãy tới 3000/s**; chưa tìm được trần. Chi phí mỗi yêu cầu thấp hơn danh sách/tìm kiếm (~0,20 ms CPU ở product-service). Ở 3000/s, product-service 84% và Traefik 83% đều gần trần (ước lượng: trần ~3500/s; chưa đo). Redis xuất hiện 0,11 nhân ở 3000/s (suy ra: chi tiết sản phẩm có dùng cache Redis; chưa kiểm).
- Trễ vòng lặp node của máy tải tăng tới 18-25 ms ở các bậc trên 1600/s; p95 28-30 ms ở 2500-3000/s có thể gồm một phần từ máy tải.

### 5.4 Nhận định nhóm C

- product-service (1 bản, 0,7 nhân) là nút thắt của hai route đọc nặng: danh sách ~1500-1800/s, tìm kiếm ~1000-1300/s; chi tiết cao hơn nhiều (> 3000/s). Gateway (2 bản) chỉ dùng 0,74-0,94 nhân ở các điểm gãy (còn dư), nên NÂNG số bản product-service là cách hợp lý nhất để tăng trần đọc (chưa thử, không đổi cấu hình theo quy ước).
- Cách gãy giống auth: vách đá, hàng đợi → deadline 3 s → 504, sau 5 lỗi liên tiếp circuit breaker mở → 503. Không có 429, không mất dữ liệu (chỉ đọc).
- So với tải thật của ccu (đợt 9, 1200 CCU: tổng ~1000-1270 lời gọi/s, trong đó `GET /products*` + `/search` ~145-190/s, tính từ số liệu `hold-closed-200-1200-300s-200.json` và `ramp-closed-1200-r2.json`): tải đọc sản phẩm thật thấp hơn trần đo được ở đây (danh sách ~1500/s, tìm ~1000/s) khoảng 5-10 lần. Suy ra product-service không phải điểm yếu của kịch bản người dùng đã đo; điểm yếu đã quan sát vẫn là auth và frontend.

## 6. Nhóm D: user-service (`svc-D-user-cart`, `svc-D2-user-mid`)

Route: `GET /users/me/addresses` (token buyer dựng sẵn, 200 tài khoản; gateway kiểm token rồi gọi gRPC tới user-service). **user-service chỉ có trần 0,2 nhân (1 bản, 256 MiB)**, thấp nhất trong các service nghiệp vụ.

| tải/s | ok/s | lỗi | p50 | p95 | p99 ms | user-service (trần 0,2) | throttle | api-gateway | Postgres |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 200 | 200 | 0 | 2 | 3 | 27 | 0,11 (55%) | 1% | 0,16 | dưới 0,1 |
| 400 | 400 | 0 | 2 | 2 | 37 | 0,16 (78%) | 4% | 0,27 | dưới 0,1 |
| 450 | 450 | 0 | 2 | 3 | 43 | 0,17 (84%) | 8% | 0,29 | 0,11 |
| 500 | 500 | 0 | 2 | 3 | 35 | 0,18 (90%) | 23% | 0,31 | 0,12 |
| **550** | 520 | **5,5%** (503: 245, 504: 657) | 500 | 2 775 | 3 006 | 0,20 (**100%**) | **100%** | 0,51 | 0,20 |
| 600 | 594 | 0,9% (503: 40, 504: 128) | 41 | 1 538 | 2 700 | 0,20 (99%) | 81% | 0,45 | 0,16 |

- **Trần ~500-550 yêu cầu/s**: ổn tới 500/s (p95 3 ms dù user-service đã 90% trần, throttle 23%), gãy ở 550/s. Chi phí ~0,36 ms CPU mỗi yêu cầu. Bậc 600/s (lượt D) nhẹ hơn 550/s (lượt D2) vì chạy ở hai lượt khác nhau trên cùng stack; mức gãy thật nằm quanh 550-600/s, chưa đo lặp lại.
- Gãy kiểu vách đá như các service khác: 504 = `DeadlineExceeded` (log gateway: 336 dòng), 503 = `circuit open: UserClient` (155 dòng). Postgres và gateway còn dư (gateway 42%, Postgres 7%): nút thắt là **trần CPU 0,2 của chính user-service**.
- So với tải thật: một người dùng chỉ ghé địa chỉ/hồ sơ thỉnh thoảng (đợt 9 ở 1200 CCU: tổng ~1000-1270 lời gọi/s toàn hệ thống), nên trần 500/s của riêng user-service khó là điểm nghẽn trong kịch bản đã đo (chưa đếm riêng số lời gọi `/users/*` của ccu).

## 7. Nhóm D: giỏ hàng (`svc-D-user-cart`, `svc-D3-cart-high`)

Route: `PUT /cart/items/:id` (số lượng 1-2; token buyer; mỗi lượt thuê một buyer trong kho 200 người để không hai yêu cầu cùng giỏ). Đi qua order-service (trần 0,8) và, theo số liệu CPU, cả product-service.

| tải/s | ok/s | lỗi | bỏ | p50 | p95 | p99 ms | order-service (0,8) | product-service (0,7) | Postgres | api-gateway |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 100 | 100 | 0 | 0 | 6 | 7 | 10 | 0,17 (21%) | 0,16 (22%) | 0,13 | 0,11 |
| 200 | 200 | 0 | 0 | 6 | 7 | 22 | 0,29 (36%) | 0,27 (39%) | 0,21 | 0,20 |
| 400 | 400 | 0 | 0 | 6 | 8 | 34 | 0,45 (56%) | 0,45 (64%) | 0,35 | 0,35 |
| 600 | 600 | 0 | 0 | 6 | 11 | 43 | 0,60 (75%) | 0,60 (86%), throttle 4% | 0,56 | 0,47 |
| **800** | 800 | 0 | 0 | 12 | 62 | 84 | 0,69 (86%), 8% | 0,70 (**100%**), **100%** | 0,74 | 0,61 |
| 1000 | 997 | 0 | 92 | 109 | 198 | 260 | 0,73 (91%), 17% | 0,70 (100%), 100% | 0,96 | 0,68 |
| 1300 | 966 | 0 | 10 019 | 182 | 287 | 364 | 0,70 (88%) | 0,70 (100%), 100% | 0,94 | 0,66 |

- **Trần ~800-1000 PUT/s**: từ 800/s product-service (100%, throttle 100%) và order-service (86-91%) cùng gần trần, độ trễ tăng dần (p95 11 → 62 → 198 → 287 ms) chứ KHÔNG gãy bằng 504/503 như các service trước; thông lượng đứng yên ~970-1000/s.
- Ghi chú phép đo: cột "bỏ" của cart là yêu cầu không còn người dùng rảnh trong kho 200 người (nhiều yêu cầu cùng bay hơn 200), không phải lỗi hệ thống. Ở bậc 1300/s phép đo vì thế bị chặn thêm bởi kho người dùng (thông lượng thật thấp hơn tải đặt), nhưng product-service đã 100% từ 800/s nên kết luận trần không đổi.
- product-service dùng CPU ngang order-service cho một thao tác giỏ (0,60 nhân ở 600/s cả hai): suy ra mỗi lần thêm vào giỏ order-service gọi product-service (kiểm giá/tồn); chưa đọc code để xác nhận. Nút thắt đầu tiên của giỏ hàng là **product-service** (1 bản, trần 0,7), kế đó order-service.
- Postgres lên 0,56-0,96 nhân (khoảng 19-32% trần) do ghi giỏ; chưa phải nút thắt.

## 8. Nhóm E: chuỗi đặt hàng (order + product + payment) và route admin (`svc-E-order-chain`, `svc-E2-chain-high`, `svc-E4-chain-higher`, `svc-E3-admin-high`, `svc-E5-admin-higher`)

### 8.1 Chuỗi đặt hàng `checkout-chain`

Một "đơn vị" = một chuỗi của một buyer có sẵn (kho 400 người, mỗi chuỗi thuê một người): thêm 1-3 món vào giỏ → `POST /checkout/preview` → `POST /orders` (40% COD, 60% MOCK_CARD) → nếu thẻ: chờ payment xuất hiện (poll 500 ms, tối đa 8 s) → xác nhận thẻ. Mỗi chuỗi tạo ĐÚNG MỘT đơn nên chuỗi/s = đơn/s. Đi qua gateway, order-service, product-service (xác nhận tồn kho qua Kafka), payment-service (tạo payment qua Kafka), Postgres, Kafka.

| chuỗi (đơn)/s | ok/s | lỗi | bỏ | p50 chuỗi ms | p95 chuỗi ms | payment trễ p50/p95 ms | order-service (trần 0,8) | throttle OS | product-service | Postgres | CPU toàn máy |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 2 | 2 | 0 | 0 | 538 | 1 048 | 1006/1010 | dưới 0,1 | 0% | dưới 0,1 | dưới 0,1 | 13% |
| 8 | 8 | 0 | 0 | 524 | 540 | 504/505 | dưới 0,1 | 0% | dưới 0,1 | 0,14 | 18% |
| 16 | 16 | 0 | 0 | 526 | 548 | 503/505 | 0,15 (19%) | 0% | 0,11 | 0,21 | 18% |
| 32 | 32 | 0 | 0 | 526 | 582 | 504/512 | 0,42 (52%) | 3% | 0,28 | 0,53 | 28% |
| 64 | 64 | 0 | 0 | 522 | 535 | 503/505 | 0,48 (60%) | 0% | 0,31 | 0,55 | 32% |
| 96 | 96 | 0 | 0 | 523 | 539 | 503/505 | 0,66 (82%) | 3% | 0,41 | 0,80 | 41% |
| 128 | 128 | 0 | 0 | 528 | 576 | 504/528 | 0,80 (**100%**) | **96%** | 0,51 | 1,05 (35%) | 49% |
| **160** | **108** | 0 | 1 564 | 4 497 | 7 430 | **6 794/7 330** | 0,80 (100%) | 99% | 0,41 | 1,19 | 53% |
| 200 | 92 | 0 | 3 240 | 5 879 | 7 556 | 6 882/7 731 | 0,80 (100%) | 100% | 0,38 | 1,19 | 52% |

- **Trần chuỗi đặt hàng ≈ 128 đơn/s** (CPU 100% của order-service, trần 0,8 nhân, 1 bản). Dưới đó ổn định: chuỗi p50 ~525 ms, không lỗi. Mỗi chuỗi tốn ~6 ms CPU order-service (0,80 / 128). Từ 160/s thông lượng thật rơi xuống ~100/s và payment tạo muộn ~7 s.
- Độ trễ chuỗi ~525 ms và "payment trễ ~503 ms" ở mọi bậc ổn định KHÔNG phải độ trễ thật: poll cách nhau 500 ms nên số đo có độ phân giải 500 ms (lần poll đầu đã thấy payment); độ trễ thật nhỏ hơn 500 ms. Giá trị 1006 ms ở bậc 2/s là vài chuỗi cần poll lần hai. Chỉ khi quá tải (6,8 s) mới là độ trễ thật.
- Kiểu gãy: **hàng đợi bất đồng bộ**, không có lỗi HTTP. Lag consumer `order-service-validate-result` (topic `product.validate_order`) lên tới 608 (partition 3 phân vùng) khi tải 160-200/s; các nhóm khác (`payment-service-create`, `product-service-validate-order`, `order-service-payment-succeeded`) lag tối đa 17, 1 và 4. Sau khi dừng tải lag tiêu hết trong vòng ~20-60 s (Prometheus: đỉnh 608 lúc +50 s, 0 lúc +120 s; mẫu 10 s). Bậc 160/200 "bỏ" là chuỗi không còn người dùng rảnh trong kho 400 (chuỗi kéo dài 4-6 s nên số chuỗi bay cùng lúc > 400), một giới hạn của phép đo; nhưng order-service đã 100% từ 128/s nên trần không đổi.
- Toàn vẹn dữ liệu sau lượt quá tải (`fault-check.sh`, `svc-E-datacheck.txt`): 19 002 đơn = 11 502 PAID (đúng bằng 11 502 payment) + 7 500 CONFIRMED (COD); không có đơn trả trước thiếu payment, không có đơn PENDING kẹt, outbox order/payment/product toàn SUCCESS.
- Chưa có chuỗi nào quá 8 s (ngưỡng chờ payment của ccu) trong lượt này nhưng bậc 200/s đã sát (payment p95 7,7 s, max 7,9 s); tăng thêm sẽ có timeout.
- **Liên hệ với lượt dot8 (closed 1200, `10_PERF_SPIKE.md`)**: triệu chứng giống (lag đúng nhóm consumer `order-service-validate-result`, payment chậm) nhưng KHÔNG cùng nguyên nhân đã chứng minh. Đối chiếu dữ liệu dot8: lag lên tới 1022 và kéo dài ~150 s, nhưng order-service lúc đó chỉ ~0,48 nhân, throttle 0% (không chạm trần CPU) và lag về 0 cuối lượt. Nghĩa là ở E4 lag do order-service kịch trần CPU, còn ở dot8 thì không; nguyên nhân dot8 vẫn CHƯA GIẢI THÍCH (không đổi so với ghi chép cũ).
- So với tải thật: lượt hold 1200 CCU của đợt 9 tạo 13 371 đơn trong 433 s ≈ **31 đơn/s** (`hold-closed-200-1200-300s-200.json`, `counters.orders` / `elapsed`; hơi cao vì bộ đếm gồm cả pha vào), tức khoảng 4 lần thấp hơn trần 128 đơn/s. Chỉ dùng số của lượt hold: ở các lượt ramp, `counters.orders` đếm cả pha vào nên chia cho thời gian đo ổn định sẽ ra số quá lớn.

### 8.2 Route admin `GET /admin/orders` và `GET /admin/payments` (admin token, `page_size=10`, lọc ngẫu nhiên)

| tải/s | admin-orders: ok/s, lỗi, p95 ms | order-service (0,8) / throttle | Postgres | admin-payments: ok/s, lỗi, p95 ms | payment-service (trần 0,4) / throttle |
|---:|---|---|---:|---|---|
| 80 | 80, 0, 13 | 0,20 (25%) / 0% | 0,30 | 80, 0, 5 | dưới 0,1 |
| 300 | 300, 0, 6 | 0,29 (37%) / 0% | 0,44 | 300, 0, 4 | 0,20 (49%) / 1% |
| 600 | 600, 0, 6 | 0,52 (65%) / 0% | 0,80 | 600, 0, 4 | 0,31 (78%) / 2% |
| 1000 | 1000, 0, 12 | 0,77 (96%) / 39% | 1,31 (44%) | 1000, 0, 52 | 0,40 (**100%**) / **100%** |
| **1300** | 908, **30,2%** (503: 5 629, 504: 6 141), 3 068 | 0,80 (100%) / 100% | 2,03 (68%) | 702, **40,1%** (503: 8 307, 504: 5 801), 3 636 | 0,40 (100%) / 100% |

- **admin-orders trần ~1000-1300/s** (order-service); **admin-payments trần ~1000/s** (payment-service có trần CPU chỉ 0,4 nhân, ~0,4 ms CPU mỗi yêu cầu). Ở 1300/s cả hai gãy theo kiểu quen thuộc (504 hết deadline đọc, 503 circuit breaker mở).
- Ở 1300/s admin-payments, Traefik cũng 96% (throttle 79%); ở admin-orders, `vector` (gom log) 0,94 nhân = 94% trần, throttle 74%: observability bắt đầu tranh CPU khi số yêu cầu rất cao (đo được, chưa tắt thử).
- Ghi nhận phép đo: lượt `svc-E-order-chain` lần đầu có admin-payments chạy lúc CPU toàn máy lên 58-80% (> 75%), do tiến trình NGOÀI hệ thống (`run_agent_search.py` của một dự án khác, ~750% CPU, và `chatter` của PyCharm, tìm bằng `ps` và cột `top` của capture). Số đo admin-payments của lượt đó KHÔNG dùng; đã đo lại ở `svc-E3-admin-high` (CPU toàn máy 11-39%). Đã thêm `scripts/load/wait-idle.sh` (chờ CPU máy < 25% ba mẫu liên tiếp trước mỗi lượt) để tránh lặp lại.

## 9. Nhóm F: analytics-service (`svc-F-analytics`, `svc-F2-ingest-high`, `svc-F3-report-high`)

Chạy trên stack đã có ~19 000 đơn từ nhóm E (không dựng lại), nên báo cáo của shop có dữ liệu thật để truy vấn.

### 9.1 Ghi sự kiện `POST /events` (không token, 3 sự kiện mỗi lần, trả 202)

| tải/s | ok/s | lỗi | bỏ | p50 | p95 | p99 ms | analytics-service (trần 0,3) | throttle AS | Traefik | Redis (trần 0,2) |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 400 | 400 | 0 | 0 | 2 | 2 | 22 | 0,12 (39%) | 1% | 0,16 | dưới 0,1 |
| 800 | 800 | 0 | 0 | 2 | 65 | 157 | 0,19 (65%) | 10% | 0,30 | dưới 0,1 |
| 1200 | 1200 | 0 | 0 | 3 | 30 | 97 | 0,22 (74%) | 15% | 0,37 | dưới 0,1 |
| 1600 | 1600 | 0 | 0 | 3 | 376 | 619 | 0,27 (91%) | 48% | 0,49 | 0,13 (64%) |
| 2200 | 2200 | 0 | 0 | 11 | 154 | 217 | 0,30 (98%) | 86% | 0,57 (81%) | 0,15 (74%) |
| 3000 | 2656 | 0 | 10 309 | 112 | 1 800 | 2 753 | 0,27 (88%) | 74% | 0,67 (95%), throttle 67% | 0,17 (86%), throttle 45% |

- **Không bao giờ trả lỗi** (mọi yêu cầu xong đều 202), nhưng độ trễ tăng dần: ổn định tới ~1200/s (p95 30 ms); từ 1600/s analytics-service ≥ 91% trần và độ trễ đuôi bắt đầu dài (p95 376 ms); ở 3000/s thông lượng thật 2656/s, 10 309 yêu cầu bị bỏ ở máy tải. Trần tiếp nhận ≈ 2000-2600/s, chia giữa analytics-service (0,3 nhân), Traefik (0,7) và Redis (0,2, một luồng).
- p95 không đơn điệu (376 ms ở 1600/s rồi 154 ms ở 2200/s): **ĐÃ KIỂM BẰNG LẶP LẠI (2026-11-25, đợt 12, `docs/perf-results/2026-11-25-dot12/`, hai lượt `svc-I1/I2-ingest`, mỗi lượt 1600/s rồi 2200/s, 30 s mỗi bậc).** 1600/s: p95 20 và 16 ms (lượt đầu 376); 2200/s: p95 32 và 55 ms (lượt đầu 154). analytics-service ở 1600/s dùng 0,23 nhân, throttle 13% (lượt đầu 0,27 nhân, 48%); ở 2200/s 0,29 nhân (96%), throttle 69%. Kết luận: 376 ms không tái hiện, là một cú giật nhất thời của lượt đầu (giây 5-6 của bậc 2: Traefik, gateway và analytics cùng tăng CPU), không phải đặc tính của 1600/s. Nguyên nhân cú giật không xác định được (không có log lỗi). Lưu ý lượt đầu chạy 4 bậc liên tiếp (1200→3000), hai lượt sau chỉ 2 bậc, nên chưa loại trừ ảnh hưởng của bậc trước. Còn lại: ở 2200/s p95 dao động 32-154 ms giữa 3 lượt.
- **Kiểm dữ liệu thật sự được lưu (đợt 12):** hai lượt gửi 227.974 yêu cầu × 3 sự kiện = 683.922; `marketplace.events_raw` có đúng 683.922 dòng, 683.922 `event_id` khác nhau (không mất, không trùng). Mã 202 của gateway cũng trả 202 khi analytics lỗi (`accepted:0, dropped:N`), nên số đếm mã HTTP một mình không chứng minh được điều này; ở các bậc ≤ 2200/s đã chứng minh, ở 3000/s (lượt dot11) chưa kiểm.
- So với tải thật của ccu: một người xem trang gửi vài sự kiện mỗi lần, tải ccu 1200 CCU thấp hơn trần này nhiều (không đếm riêng số `POST /events` trong đợt 9).
- Chưa đo độ trễ từ lúc gửi sự kiện tới lúc xuất hiện trong ClickHouse (đường Kafka → ClickHouse); chỉ đo phần nhận yêu cầu.

### 9.2 Báo cáo người bán `GET /seller/analytics/{summary|timeseries|top-products|funnel}` (token chủ shop, shop có ~19 000 đơn)

| tải/s | ok/s | lỗi | p95 ms | ClickHouse CPU (trần 1,5; trung bình bậc) | throttle CH | RAM ClickHouse (trần 2 GiB) |
|---:|---:|---:|---:|---:|---:|---|
| 5-80 | = tải | 0 | 3-28 | dưới 0,1 | 0% | không đo ở bậc này |
| **150** | 116 | **22,9%** (503: 879, 504: 151) | 10 (p99 3 004) | 0,90 (nhưng **1,50 = 100% trần** trong từng đợt vài giây, xem dưới) | 60% | 408 → ~1 900 MiB trong 10 s đầu |
| 300 | 147 | 51,1% (503: 3 404, 504: 1 196) | 3 003 | 1,5 rồi **bị kill** | - | chạm **2 030-2 046 MiB**, **OOMKilled lúc 02:29:01 UTC** |

- **Trần ~80-115 báo cáo/s**, nút thắt là **ClickHouse**: cả CPU (kịch trần 1,5 nhân, 1,50 trong các mẫu 5 s) lẫn **RAM** (trần 2 GiB). Số 0,90 nhân ở bảng là trung bình cả bậc 30 s (có khoảng 10 s gần như rảnh vì mọi truy vấn đang lỗi), che mất việc nó chạm trần; chi tiết theo mẫu 5 s từ `svc-F3-report-high.capture.jsonl`: 02:28:09 0,79 nhân/1 272 MiB → 02:28:14-24 1,50 nhân/1 428-1 908 MiB → 02:28:29-39 gần như rảnh/~650 MiB → 02:28:44-59 1,50 nhân/2 000+ MiB.
- **ClickHouse bị OOM-kill (exit 137, `OOMKilled=true`, `docker inspect`) ở bậc 300/s**; Swarm tự dựng lại (task mới Running). Đây là lần đầu trong các đợt đo một container hạ tầng chết vì hết RAM do tải API (các lần chết trước là kill chủ ý hoặc healthcheck frontend). Trước đó, RAM giảm đột ngột 1,9 GiB → ~650 MiB lúc 02:28:29 trong khi CPU rảnh: suy ra ClickHouse tự hủy các truy vấn vượt giới hạn bộ nhớ truy vấn (chưa đọc log ClickHouse để xác nhận).
- Nguồn lỗi phía gateway (log): `DeadlineExceeded` (1 044 dòng ở `funnel`), `circuit open: AnalyticsClient` (~1 000+ dòng mỗi loại báo cáo). Vì ClickHouse chết/đang quá tải nên breaker mở và phần lớn yêu cầu bị chặn nhanh.
- Sau khi ClickHouse tự khởi động lại (đo ~19 phút sau, `clickhouse-client`): dữ liệu còn nguyên (volume còn): `fact_payment_events` 11 502 dòng = đúng số payment đã tạo, `fact_order_status` 49 506, `fact_order_items` 36 995, `events_raw` 828 441. Chưa đo thời gian ClickHouse sẵn sàng sau khi bị kill và bơm Kafka → ClickHouse có bắt kịp trong lúc chết hay không (không có lượt đo nào ghi lại).
- Báo cáo nằm trong luồng "xem báo cáo" của người bán/admin (ccu: vài lần mỗi hành trình), khối lượng thật thấp hơn nhiều trần này; nhưng đây là điểm yếu nếu nhiều người mở báo cáo nặng cùng lúc: **một đợt báo cáo ~150/s có thể làm ClickHouse hết RAM và chết**, kéo theo mọi báo cáo analytics.

## 10. Nhóm G: ops-service (`svc-G-ops`, `svc-G2-ops-high`)

Route admin: `/ops/system/health`, `/ops/incidents`, `/ops/queries`, `/ops/audit`, `/ops/actions` (chọn ngẫu nhiên). ops-service 1 bản, trần **0,2 nhân**.

| tải/s | ok/s | lỗi | p50 | p95 | p99 ms | ops-service (0,2) | throttle | api-gateway |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 80 | 80 | 0 | 4 | 6 | 37 | dưới 0,1 | 0% | 0,16 |
| 150 | 150 | 0 | 4 | 21 | 73 | dưới 0,1 | 0% | 0,27 |
| 600 | 600 | 0 | 2 | 3 | 39 | 0,12 (60%) | 2% | 0,38 |
| **1000** | 1000 | 0 | 2 | 39 | 78 | 0,20 (**98%**) | **63%** | 0,62 |

- Tới 1000/s không lỗi; ops-service ở 98% trần (~0,2 ms CPU/yêu cầu), p95 bắt đầu tăng (39 ms). Trần ≈ 1000/s, chưa tìm điểm gãy. Các route chỉ đọc ở mức nhẹ (health, danh sách): các truy vấn nặng hơn của `/ops/queries/:name` (Prometheus/Loki) KHÔNG nằm trong target này.
- Tải thật của ops rất thấp (vài operator/admin), nên không phải điểm yếu.

## 11. Nhóm H: ai-agent qua `POST /ai/chat` (mô hình GIẢ; `svc-H-ai`, `svc-H2-ai-high`, `svc-H3-ai-fifo`)

Mỗi "đơn vị" = một cuộc chat buyer (SSE tới `event: done`); mỗi user chỉ một luồng chat cùng lúc (khóa `ai_slot`) nên máy tải thuê user từ kho. Mô hình giả trả lời sau ~2,6 s (độ trễ cố định của mô hình giả, nên p50 ~2,6 s là mức đáy). ai-agent 2 bản x 0,5 nhân (tổng 1,0). Đã kiểm `deploy-perf.sh` xác nhận mô hình giả khi dựng stack; KHÔNG gọi mô hình thật.

**Lỗi phép đo đã sửa trong quá trình đo (kết quả cũ không dùng)**: lượt `svc-H-ai` (kho 150 user) và `svc-H2-ai-high` (kho 1200 user) dính 429 `daily_token_limit` (trần 150 000 token/user/ngày) từ ~8-24 chuỗi/s. Nguyên nhân phía máy tải: hàm thuê user lấy kiểu ngăn xếp (user vừa trả được dùng lại ngay) nên vài user bị dùng lặp và chạm trần, trong khi phần lớn kho không được dùng. Đã đổi thành xoay vòng (FIFO) riêng cho `ai` và đo lại bằng `svc-H3-ai-fifo` (kho 1500 user): không còn 429.

| cuộc chat/s | ok/s | lỗi | 429 | p50 ms | p95 ms | byte đầu p50/p95 ms | ai-agent (tổng 1,0) | throttle | api-gateway |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 1-4 | = tải | 0 | 0 | 2 580 | 2 610-2 710 | 8-10 / 13-17 | 0,11-0,17 | 3% | dưới 0,1 |
| 8 | 8 | 0 | 0 | 2 589 | 2 793 | 8 / 16 | 0,27 (27%) | 4% | dưới 0,1 |
| 16 | 16 | 0 | 0 | 2 616 | 2 855 | 8 / 14 | 0,46 (46%) | 5% | dưới 0,1 |
| 24 | 24 | 0 | 0 | 2 649 | 3 043 | 8 / 27 | 0,64 (64%) | 8% | 0,10 |
| 32 | 32 | 0 | 0 | 2 707 | 3 220 | 8 / 34 | 0,80 (80%) | 20% | 0,12 |
| **40** | 40 | 0 | 0 | 3 027 | 4 351 | 11 / 70 | 0,95 (95%) | **66%** | 0,15 |
| 50 | 50 | 0 | 0 | **11 544** | **15 906** | **717 / 7 239** | 1,00 (**100%**) | **100%** | 0,15 |

- **Trần ≈ 40-45 cuộc chat/s** (cho đường mã ai-agent + mô hình giả): CPU ai-agent tăng tuyến tính ~0,024 nhân mỗi cuộc chat/s và chạm trần 1,0 ở ~42/s (ước lượng; đo thực: 40 ổn dù đã 95%, 50 gãy về độ trễ). Gãy kiểu hàng đợi (p50 từ 2,7 s lên 11,5 s, byte đầu từ 11 ms lên 717 ms), không có lỗi vì giới hạn thời gian chờ chat là 130 s và lượt chạy chỉ 30 s mỗi bậc.
- Độ trễ này KHÔNG phản ánh mô hình thật (độ trễ và chi phí của LLM thật lớn hơn nhiều, chưa đo; không được phép gọi). Con số hữu ích: chi phí CPU phần ai-agent (~24 ms mỗi cuộc chat), gateway gần như không tốn (≤ 0,15 nhân tới 50/s), Postgres ≤ 0,16.
- Trần token/ngày của mỗi user (`limits.daily_tokens`, 429) là giới hạn nghiệp vụ nên khi đo AI phải có đủ user; lượt đo sai trước là bài học ghi ở trên.
- Ở tải thật, chat AI chỉ chiếm 19% VU của mix (12+4+2+1 trên 100) và mỗi VU hỏi cách ~8 s: ~1200 VU ≈ vài chục cuộc chat/s ở mức tối đa; không đo riêng ở đây.

## 12. Tổng kết: năng lực từng service (cấu hình đợt 9-10, mô hình giả, một lượt mỗi điểm)

Bảng xếp từ thấp tới cao theo trần đo được. "Trần" = mức chịu được mà không lỗi và p95 chưa tăng vọt; điểm gãy nằm giữa trần và bậc kế đã đo (khoảng cách ghi ở cột "gãy ở"). Đơn vị yêu cầu/s trừ khi ghi khác. Nút thắt là service/container chạm trần CPU đầu tiên (đã đối chiếu CPU từng bậc); với chuỗi nhiều service thì chỉ ra cả hai.

| # | target | trần (ổn) | gãy ở | nút thắt (cấu hình) | cách gãy |
|---:|---|---:|---:|---|---|
| 1 | frontend (trang SSR) | ~80 trang/s | 100 | frontend 1 bản, 0,7 nhân (~8 ms CPU/trang) | hàng đợi, timeout 30 s phía khách, 502 |
| 2 | auth đăng nhập | ~80/s | 100 | auth 5 bản x 1 nhân (~58 ms CPU bcrypt/lần) | 504 (deadline 10 s) + 503 (breaker) |
| 3 | auth đăng ký | >= 60/s (chưa gãy) | chưa đo | như trên (ước 86/s) | chưa quan sát |
| 4 | analytics: báo cáo người bán | ~80-115/s | 150 | ClickHouse: CPU 1,5 nhân kịch trần VÀ RAM 2 GiB (OOM-kill ở 300/s) | 504 + 503 (breaker); ở 300/s container ClickHouse bị OOM-kill |
| 5 | user: danh sách địa chỉ | ~500/s | 550 | user-service 1 bản, 0,2 nhân | 504 + 503 |
| 6 | giỏ hàng PUT | ~800/s | > 1000 (không lỗi, độ trễ tăng) | product-service (0,7) rồi order-service (0,8) | độ trễ tăng dần, thông lượng đứng ~1000/s |
| 7 | admin: danh sách thanh toán | ~1000/s | 1300 | payment-service 1 bản, 0,4 nhân | 504 + 503 |
| 8 | admin: danh sách đơn | ~1000/s | 1300 | order-service 1 bản, 0,8 nhân (Postgres 1,3-2,0) | 504 + 503 |
| 9 | ops (route nhẹ) | ~1000/s | chưa gãy | ops-service 1 bản, 0,2 nhân (98%) | chưa quan sát |
| 10 | tìm kiếm `/search` | ~1000/s | 1300 | product-service 1 bản, 0,7 nhân | 504 + 503 |
| 11 | danh sách sản phẩm | ~1500/s | 1800-2200 | product-service, rồi Traefik | 504 + 503 |
| 12 | analytics: ghi sự kiện | ~1200/s (p95 30 ms); chấp nhận tới ~2600/s | độ trễ đuôi từ 1600 | analytics-service 0,3, Traefik, Redis 0,2 | không lỗi (202), độ trễ tăng, máy tải bỏ yêu cầu |
| 13 | gateway + Traefik (`/health`) | ~4000/s | 5000 | Traefik 1 bản, 0,7 nhân | hàng đợi, thông lượng 2400/s |
| 14 | chi tiết sản phẩm | >= 3000/s (chưa gãy) | chưa đo | product-service 84%, Traefik 83% | chưa quan sát |
| - | chuỗi đặt hàng (đơn/s) | ~128 đơn/s | 160 | order-service 1 bản, 0,8 nhân (~6 ms CPU/chuỗi) | lag Kafka `validate-result`, payment trễ ~7 s, không lỗi HTTP |
| - | ai-chat (mô hình giả, chat/s) | ~40/s | 50 | ai-agent 2 bản x 0,5 (~24 ms CPU/chat) | hàng đợi, độ trễ 11 s, không lỗi |

Nhận định:

1. **Điểm yếu thật của hệ thống là frontend (80 trang/s) và auth (80 đăng nhập/s)**: hai số thấp nhất, cùng một lúc đều là nút thắt đã quan sát trong ccu/spike (đợt 9-10). Đo riêng xác nhận chúng, chứ không phải suy ra từ CPU.
2. Các route API đọc (sản phẩm, tìm kiếm, người dùng, admin) có trần từ 500 tới > 3000/s, cao hơn tải thật đã đo 5-10 lần (tải ccu 1200 CCU: tổng ~1000-1270 lời gọi/s, sản phẩm+tìm ~145-190/s, đặt đơn ~31 đơn/s). Riêng báo cáo analytics (~100/s, ClickHouse) và địa chỉ user (~500/s, trần 0,2 nhân) là những chỗ gần nhất với tải thật theo thứ tự lớn, nhưng chưa đếm tải thật của hai route này.
3. Kiểu gãy giống nhau ở mọi service đồng bộ qua gRPC: CPU chạm trần (throttle ≈ 100%) → hàng đợi → deadline gRPC (3 s đọc, 10 s ghi) → 504 → sau 5 lỗi circuit breaker mở → 503 → phục hồi sau khi dừng tải. Ngoại lệ DUY NHẤT thấy được: ClickHouse bị OOM-kill ở bậc 300 báo cáo/s (§9.2); các service Go không chết/không restart khi quá tải (kiểm `docker service ps`: chỉ có auth x2 exit 1 và ai-agent exit 3 lúc deploy, là lỗi khởi động đồng thời đã ghi ở `11_PERF_FAULT.md` §2b/§10, và ClickHouse). Không có dữ liệu bị mất trong các kiểm tra toàn vẹn đã chạy (chỉ kiểm sau lượt chuỗi đặt hàng, §8.1). Riêng đường bất đồng bộ (đặt hàng → payment) không trả lỗi HTTP mà tích lag Kafka.
4. Trần phụ thuộc cấu hình: nhiều service chỉ có 1 bản và trần CPU nhỏ (user 0,2, ops 0,2, payment 0,4, analytics 0,3). Nâng số bản hoặc trần CPU của service là nút thắt sẽ nâng trần tương ứng (chưa thử, không đổi cấu hình theo quy ước).
5. Observability tranh CPU khi số yêu cầu cao: `vector` dùng 0,3-0,9 nhân ở 1000-4000 yêu cầu/s (đôi khi nhiều hơn api-gateway); gần trần ở 1300/s admin-orders (94%, throttle 74%).

### Giới hạn và việc chưa làm

- Mỗi điểm chỉ một lượt (n=1), bậc cách nhau 25-100%: điểm gãy chính xác nằm giữa "trần" và "gãy ở"; chưa lặp lại để có độ biến thiên.
- Chưa đo riêng: payment-service qua `POST /payments/:id/confirm` (chỉ qua chuỗi đặt hàng), product-service ghi (tạo/sửa sản phẩm, tồn kho), seller routes (đơn người bán, trả hàng), auth refresh-token, ops-service truy vấn nặng (Prometheus/Loki), ai-agent với mô hình thật (không được phép), đường Kafka → ClickHouse (độ trễ analytics), soak, rolling update. Đăng ký (`auth-register`) chưa tới điểm gãy; chi tiết sản phẩm chưa tới điểm gãy; ops chưa tới điểm gãy.
- Lượt lệch vì tiến trình ngoài (admin-payments trong `svc-E-order-chain`) đã loại và đo lại; lượt ai-chat sai kho user đã loại; nhóm A-D đo khi CPU toàn máy luôn < 50%.
- Dữ liệu thô và log từng lượt: `docs/perf-results/2026-11-25-dot11/` (git-ignored); script `scripts/load/svc.mjs`, `perf-svc-report.py`, `wait-idle.sh`.
