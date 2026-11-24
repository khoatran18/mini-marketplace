# Perf: spike, bậc thang tải và sửa R25 (từ đợt 9, 2026-11-24)

Tiếp theo [`09_PERF_RESOURCES.md`](09_PERF_RESOURCES.md) (đo ổn định, đóng/mở, soak, spike cũ ở §5q-§5s; file đó đã quá dài nên các lượt từ đợt 9 ghi ở đây). Dữ liệu thô: `docs/perf-results/2026-11-24-dot9/`.

Quy ước: mỗi lượt đo xong thì phân tích khi stack còn sống, lưu log, ghi vào file này, rồi mới dọn (xem CLAUDE.md). Lượt có vấn đề chưa rõ nguyên nhân thì KHÔNG dọn.

## 1. Sửa R25 (khóa `safe_after`) và spike THẬT (2026-11-24, đợt 9, `docs/perf-results/2026-11-24-dot9/`)

**Sửa logic `safe_after` (code + test, chưa commit):**
- Trước: Redis mất dữ liệu => khóa `pwd_version:safe_after` = bây giờ + 6 phút => 6 phút đó MỌI request có đăng nhập hỏi auth-service -> Postgres.
- Sau: khi khóa này không có, request đầu tiên giữ khóa ngắn `pwd_version:rebuild_lock` (15 s), gọi auth-service MỘT lần `ListRecentPwdChanges(6 phút)` (bảng `pwd_version_events`, cột mới `changed_at` = lần đổi mật khẩu GẦN NHẤT; migration 00005; trước đó chỉ có `created_at` là lần đầu và dòng không bao giờ bị xóa), ghi lại `<userID>:pwd_version` bằng SetNX (không đè giá trị mới hơn Kafka vừa ghi) với TTL = cửa sổ, rồi đặt `safe_after` = BÂY GIỜ => Redis được tin ngay. Request khác trong lúc đó (khóa đang bị giữ) hỏi DB cho riêng nó và KHÔNG tạo sentinel (nếu tạo sẽ chặn sentinel của lần dựng lại). Lần hỏi lỗi => quay về hành vi cũ (sentinel = bây giờ + cửa sổ, DB quyết định mọi request). Auth sập hẳn => vẫn 503 (fail closed).
- Khóa `safe_after` vẫn được đọc ở MỌI request (cùng MGet với khóa user, không thêm vòng Redis); chỉ khi nó thiếu/ở tương lai mới liên quan DB, nên "không phải lúc nào cũng dùng DB".
- File: `services/api-gateway/internal/middleware/pwdversion.go` (+ `authclient.ListRecentPwdChanges`), `auth-service` (proto `ListRecentPwdChanges`, sinh lại `.pb.go` cho gateway/order/product/user, `outbox_repo.go`, `auth_service.go`, `auth_server.go`, migration `00005_pwd_version_events_changed_at.sql`). Test: 5 test gateway mới (dựng lại một lần; không ai đổi thì chỉ 1 lần hỏi DB; không đè khóa mới hơn; lỗi danh sách => cửa sổ; khóa bận => không tạo sentinel), 1 test auth với Postgres thật (chỉ trả dòng trong cửa sổ, version mới nhất sau hai lần đổi).
- KIỂM CHỨNG THẬT trên stack đang chạy: đăng ký, đổi mật khẩu, `FLUSHALL` Redis => vé CŨ 401, vé MỚI 200, `safe_after` = bây giờ (lệch 0 s), 146 khóa deny được dựng lại với TTL ~360 s.

**`ccu.mjs`: spike thật:** `--vu-stages=200:60,600:90,1000:90,200:90` (đóng: nền, NHẢY trong `--spike-jump`=10 s, giữ, hạ xuống), `--rate-stages=10:60,20:60,30:60` (mở: nhịp phiên/s đổi đột ngột), in BẢNG THEO CỬA SỔ 10 s (calls/s, lỗi + mã, p50/p95/p99/max, VU hỏng, payment p95, VU đang chạy) và ghi `timeline` vào JSON. VU xen kẽ theo MIX nên bậc nền cũng đủ loại. LƯU Ý: VU mới tự ĐĂNG KÝ tài khoản lúc vào (bcrypt), nên bước nhảy là "người mới đổ vào", không phải "người cũ quay lại".

**Spike đóng (n=1, stack MỚI, mặc định, Redis mới => đi đường dựng lại; `spike-closed-200-600-1000`):** nền 200 VU 60 s -> 600 (vào trong 10 s) 90 s -> 1000 90 s -> về 200 90 s.
- Lỗi ngoài dự kiến 0 trong MỌI cửa sổ, 0 VU hỏng, 0 lỗi 503 (không còn dòng "cannot verify pwd version"), 0 lần 429. 31 "lỗi" là `GET /admin/accounts` 401 ở cửa sổ cuối: do bước dọn tài khoản của ccu sau khi đo xong, không phải hệ thống.
- Cú nhảy làm p95 cửa sổ đầu tăng rồi tụt lại: 200->600: 13 -> 259 ms (cửa sổ nhảy) -> 15-23 ms; 600->1000: 16 -> 527 ms -> 55-225 ms; hạ về 200: 12-13 ms trong cửa sổ kế tiếp. p50 3 -> 7 ms. Thông lượng 285 -> 860 -> 1.300-1.400 calls/s rồi 285. Payment p95 505-540 ms (có cửa sổ 1007 do chu kỳ hỏi lại 500 ms) và KHÔNG xấu đi khi tải tăng; 0 đơn quá 8 s chưa có payment. p99 ~2,4 s là `POST /ai/chat` (mô hình giả), không đổi theo tải.
- Hệ thống: lag outbox tối đa 0,35 s, lag consumer tối đa 9, chờ pool DB đỉnh 4,4 s/s (trung bình 0,2), Postgres 3.554 commit/s đỉnh, không có chờ kéo dài.
- Giới hạn: toàn máy đỉnh 86% CPU (máy tải + hệ thống dùng chung, ngưỡng tin cậy 75%), n=1.

**Bậc thang mở 10 -> 20 -> 30 -> 10 phiên/s (n=1, stack mới, `spike-open-10-20-30`, 60 s mỗi bậc; ĐÂY LÀ BẬC THANG, KHÔNG PHẢI SPIKE QUÁ TẢI: 30 phiên/s còn dưới trần đã biết 40-45):**
- 0 lỗi, 0 VU hỏng, 0 lần 429 trong mọi cửa sổ; 4.501 phiên, đỉnh 506 phiên cùng lúc, 83.199 lời gọi (347/s).
- Thông lượng tăng gần tuyến tính theo nhịp: ~195 calls/s (10) -> ~400 (20) -> ~590 (30) -> về ~200 sau khi hạ nhịp (cửa sổ chuyển 404 -> 226 -> 200). p50 3 ms; p95 ~60 ms không đổi theo bậc (là bcrypt của đăng nhập, ~56 ms), chỉ nhích 62-71 ms ở bậc 30. Payment p50 503 ms, p95 1007 ms (độ phân giải 500 ms của vòng hỏi lại), 0 đơn quá 8 s chưa có payment.
- Hệ thống: CPU toàn máy trung bình 36%, đỉnh 55% (dưới ngưỡng tin cậy 75%); lag outbox tối đa 0,49 s; Postgres tối đa 1.584 commit/s; throttle auth 4,3%, không service nghiệp vụ nào >5%.
- Kết luận hẹp: trong 10-30 phiên/s cú tăng gấp đôi/gấp ba không gây lỗi, hệ thống co giãn gọn và hạ tải xong về mức nền ngay. Chưa nói gì về vùng 40-60 (trần auth). Cột "đang chạy" của bảng cửa sổ in 0 cho chế độ mở (đã sửa trong `ccu.mjs`, lượt sau mới có).
- Quy ước đo kế tiếp (đề xuất, chờ xác nhận): spike quá tải đóng 200 -> 1200 -> 200 và mở 10 -> 50 -> 10 (cộng 10 -> 40 nếu muốn sát biên).

## 2. Spike đóng 200 -> 800 -> 200 (`spike-closed-200-800-200`, n=1, stack mới, mặc định)

Nền 200 VU 60 s, nhảy lên 800 (VU mới vào trong 10 s) giữ 90 s, về 200 giữ 90 s. Lỗi ngoài dự kiến 56 (0,04%), phép đo hợp lệ; 0 VU hỏng, 0 lần 429.
- **Chỉ cú nhảy có lỗi:** cửa sổ giây 60 (2 lỗi) và 70 (29 lỗi), đều là 504 (quá hạn 3 s ở gateway); sau đó 0 lỗi suốt phần còn lại. p95 cửa sổ nhảy 1.786 và 1.615 ms (nền 13-14 ms), max 5,4 và 9,6 s; từ giây 80 p95 về 24-67 ms, hạ về 200 VU thì 12-14 ms ngay (hồi phục tức thì). 25 lỗi 401 + 1 lỗi 504 ở `GET /admin/accounts` là bước dọn của ccu sau khi đo xong, không tính.
- **Ở 800 VU ổn định** (giây 80-140): ~1.100-1.160 calls/s, p50 4-5 ms, p95 24-67 ms, payment p95 516-610 ms, lag outbox tối đa 0,35 s, lag consumer tối đa 3: hệ thống không có vấn đề khi đã vào xong.

**Nguyên nhân 31 lỗi 504 (đã xác định, log + số đo của chính lượt này):**
1. Cả 31 lỗi là `AuthClient: GetStoreIDRoleById error ... DeadlineExceeded` (khớp 31/31 với 504 của gateway); 27/31 là userID 7 = chủ shop của bài đo, ở các route `/seller/*`, `POST/PUT/PATCH /products*`. Mọi route cần quyền seller hỏi auth-service `GetStoreIDRoleById` MỖI request (19.317 lần cả lượt, so với Login 1.435, Register 607) và gateway chỉ chờ 3 s.
2. Thời điểm khớp: lỗi 20:37:45-51 (10-16 s sau cú nhảy lúc 20:37:35). Trong 20:37:37-49 bản auth được capture chạy 0,53-1,10 core trên trần 1 core, bị throttle 250-1.000 ms mỗi giây (từ 0,04 core trước đó); postgres thấy `idle in transaction` tăng 2 -> 20 (giao dịch mở chờ CPU). Chỉ auth-service có chờ pool DB (tổng 1.250 s cộng dồn; mọi service khác ~0; chờ pool đỉnh 24,6 s/s).
3. Vì sao CPU: 600 VU mới đăng ký + đăng nhập trong 10 s = ~1.200 lần bcrypt (~56 ms) = ~67 core-giây trong ~12 s, vượt 5 core của 5 bản auth. Request `GetStoreIDRoleById` (một lần đọc DB) phải xếp hàng cùng bcrypt trên cùng CPU/pool nên quá 3 s.
4. Tải CÂN giữa 5 bản (4.454-4.719 lời gọi mỗi bản), nên không phải một bản bị dồn. Hạn chế của chứng cứ: `perf-capture.py` chỉ ghi MỘT bản mỗi tên service (nhiều bản trùng tên, bản cuối thắng) và Prometheus/cAdvisor mẫu thưa, nên mức CPU 5 bản cùng lúc là SUY RA (tải đều + một bản đo được chạm trần), không đo trực tiếp.
- **Phần là do bài đo:** VU mới tự đăng ký (bcrypt) lúc vào; nếu tài khoản có sẵn thì chỉ còn đăng nhập (một bcrypt thay vì hai). **Phần là thiết kế thật:** `GetStoreIDRoleById` gọi auth mỗi request seller (có thể đưa store/role vào JWT hoặc cache ngắn ở gateway), và bcrypt không có giới hạn song song nên chiếm hết CPU của auth làm các RPC nhẹ chết đói. Chưa sửa (cần quyết định).
- Logic `safe_after` mới không liên quan lỗi này: 0 lỗi "cannot verify pwd version" (so với 313 lỗi 503 của spike cũ cùng cú nhảy 800); R25 giải quyết được phần 503, phần 504 của auth còn lại.

## 3. Spike đóng 200 -> 1000 -> 200 (`spike-closed-200-1000-200`, n=1, stack mới, mặc định) - CHƯA HỢP LỆ

Lỗi ngoài dự kiến 226 (0,15%, ngưỡng 1% nhưng) VU hỏng 11 nên bị đánh CHƯA HỢP LỆ; 0 lần 429. Chỉ cú nhảy có lỗi: giây 60 (22 lỗi 504), giây 70 (168 lỗi: 504 x155, 503 x7, 409 x6), giây 80 (7 lỗi); từ giây 90 là 0 lỗi, hạ về 200 VU thì p95 12-13 ms ngay (hồi phục tức thì). p95 cửa sổ nhảy 2.477 / 3.277 ms, max 7,4 / 10 s. 29 lỗi 401 + 5 lỗi 504 ở `GET /admin/accounts` là bước dọn của ccu sau khi đo xong, không tính.
- **Ở 1000 VU ổn định** (giây 90-140): ~1.300-1.370 calls/s, p50 5-6 ms, p95 62-134 ms, payment p95 535-1.021 ms, lag outbox tối đa 0,39 s. Không có lỗi khi đã vào xong. 989/1000 VU đang chạy (11 VU không vào được, xem dưới).
- Toàn máy đỉnh CPU 91% (máy tải + hệ thống dùng chung, ngưỡng tin cậy 75%): độ trễ của lượt này có thể bị cộng thêm.

**Nguyên nhân (đã xác định, cùng cơ chế với spike 800 nhưng nặng hơn; số đo CPU lần này cộng đủ 5 bản nhờ sửa `perf-capture.py`):**
1. Auth-service hết CPU: từ 20:50:24 (đúng lúc nhảy) tổng CPU 5 bản = 5,0 core trên trần 5 core trong ~18 s (20:50:26-42), throttle ~4.500-5.100 ms mỗi giây (cả 5 bản bị chặn gần như liên tục); trước đó 0,3 core. 800 VU mới đăng ký + đăng nhập trong 10 s = ~1.600 lần bcrypt = ~90 core-giây > 5 core x ~12 s.
2. Chỉ auth có chờ pool DB (tổng 4.384 s cộng dồn, đỉnh 201 s/s; mọi service khác ~0; lượt 800 chỉ 1.250 s).
3. Hậu quả theo chuỗi: các lời gọi nhẹ tới auth quá hạn 3 s. 75 lần `GetStoreIDRoleById` DeadlineExceeded (lượt 800: 31) làm 504 trên mọi route seller và ghi sản phẩm; `register` 36 lần 504 (+9 lần 409 do script đăng ký trùng tên), `login` 7, `change-password` 3. Auth báo `publish to Kafka failure ... context deadline exceeded` x7 (outbox sẽ thử lại).
4. Mạch ngắt (circuit breaker) của gateway cho AuthClient mở lúc 13:50:37 UTC (8 lần `circuit open: AuthClient`): 7 lỗi 503 trả sau 0,4-1,6 ms, không còn chờ 3 s. Đây là cơ chế bảo vệ chạy đúng (nhanh gọn thay vì treo), KHÔNG phải lỗi `pwd_version`: 0 dòng "cannot verify pwd version".
5. 11 VU không vào được = đăng ký/đăng nhập của VU mới bị 504/503 trong lúc auth hết CPU.
- Kết luận như spike 800: giới hạn tài nguyên của auth (5 core, bcrypt) bị vượt bởi đợt đăng ký dồn; route seller là nạn nhân vì mỗi request hỏi auth. Mức tải gây lỗi lần này (800 người mới trong 10 s) cao hơn lần trước (600), lỗi tăng từ 31 lên ~190 lỗi 5xx trong cú nhảy. Không sửa (auth là giới hạn tài nguyên đã chốt); hướng giảm đã ghi ở mục 2.

## 4. Spike đóng 200 -> 1200 -> 200 (`spike-closed-200-1200-200`, n=1, stack mới, mặc định) - CHƯA HỢP LỆ

Lỗi ngoài dự kiến 713 (0,44%), VU hỏng 25 (1.175/1.200 VU đang chạy), 0 lần 429. Gồm 582 lỗi 504, 50 lỗi 503, 64 lỗi 401 (29 là bước dọn của ccu sau khi đo xong; xem dưới), còn lại 409 (script đăng ký trùng tên). Cú nhảy: giây 60: 70 lỗi; giây 70: 496 lỗi (5,2%; 450 504 + 39 503), p95 7.044 ms, max 10 s, 15 VU hỏng; giây 80: 87 lỗi, 10 VU hỏng; từ giây 90 hệ thống ổn định ở 1.175 VU.
- **Sau khi vào xong, 1200 VU CHẠY ỔN ĐỊNH, KHÔNG gãy:** giây 90-140 ~1.420-1.500 calls/s, p50 7-9 ms, p95 76-437 ms (hầu hết ~100-180 ms), payment p95 540-655 ms, lag outbox tối đa 0,46 s, lag consumer tối đa 3. KHÁC lượt gãy ở 1200 trước đó (đợt 8: payment p95 10,5 s, lag `validate-result` không hồi): lượt này không có dấu hiệu đó. Khác biệt chính: lượt trước vào bằng ramp 160 s theo cách cũ; lượt này nền 200 rồi nhảy 10 s. n=1 mỗi cách nên CHƯA kết luận được hai kết quả mâu thuẫn hay phụ thuộc cách vào (đáng lặp ramp 1200 để biết). Hạ về 200 VU: hồi phục ngay (p95 13 ms từ giây 160).
- **Độ tin cậy:** toàn máy đỉnh CPU 96% (máy tải dùng chung máy, ngưỡng tin cậy 75%), nên một phần độ trễ có thể do máy tải.

**Nguyên nhân lỗi ở cú nhảy (cùng cơ chế với 800 và 1000, nặng hơn; số đo từ capture đã cộng đủ 5 bản + Prometheus):**
1. Auth hết CPU: tổng 5 bản đạt 5,28 core (trần 5), >4,5 core trong 24 s liền từ 21:01:29 (đúng lúc nhảy). 1.000 VU mới đăng ký + đăng nhập trong 10 s = ~2.000 lần bcrypt.
2. Chỉ auth có chờ pool DB: 8.961 s cộng dồn (lượt 1000: 4.384; lượt 800: 1.250), đỉnh 371 s/s; order-service 17,8 s (không đáng kể).
3. gateway -> auth quá hạn (Prometheus, `mm_grpc_client_requests_total{service="AuthClient"}`): DeadlineExceeded 391 = `GetStoreIDRoleById` 184 (route seller), `Register` 142, `Login` 53, `ChangePassword` 3, `CreateStaffAccount` 2, `RegisterSellerRoles` 2, `ListAccounts` 5. 0 lỗi "cannot verify pwd version": R25 không còn gây lỗi.
4. 503 = mạch ngắt AuthClient mở (nhanh, không đợi 3 s), như lượt 1000; log gateway của lượt này KHÔNG còn phần cú nhảy (file bắt đầu 14:01:55, sau khi nhảy: log của Swarm bị xoay vòng theo kích thước vì gateway ghi ~40 MB), nên mạch ngắt suy ra từ số liệu Prometheus và lượt 1000, không đọc trực tiếp lần này. Cần nâng giới hạn kích thước log hoặc dump log trong lúc đo nếu muốn đọc cú nhảy của các lượt nặng.

**401 giữa lượt ổn định (32 lỗi): hành vi đúng, do script + auth quá tải, không phải lỗi hệ thống.** Cả 32 lỗi thuộc MỘT người dùng (userID 243, mọi request của nó từ 14:01:56 tới 14:02:54 đều 401 `invalid or expired pwd version`). DB: `accounts.pwd_version = 1` và `pwd_version_events` có dòng của 243 lúc 14:01:46 (đổi mật khẩu thành công), Redis `243:pwd_version = 1`. Hành trình `buyerProfile` của ccu đổi mật khẩu rồi đăng nhập lại để lấy token mới; lần đăng nhập lại gặp 504 do auth quá tải nên VU giữ token cũ và hệ thống đúng khi từ chối token cũ. Tức đường R25 (khóa deny trong Redis) hoạt động đúng ngay trong lúc quá tải. 3 lỗi 401 còn lại và 29 lỗi `GET /admin/accounts` cuối là bước dọn.

## 4b. Giải thích cửa sổ p95 1.847 ms ở giây 110 của spike đóng 1200 (kiểm chứng bằng log Traefik)

Log gateway của lượt 1200 mất phần giữa, nhưng log Traefik (có cả router `api@swarm` và `frontend@swarm`, kèm thời gian xử lý từng request) còn đủ từ 14:01:47, phủ các cửa sổ giây 80-140. Tách theo router, mỗi cửa sổ 10 s ở bậc 1200 VU:

| cửa sổ | api (gateway): số lời gọi, p95 | frontend (trang Next.js): số lời gọi, p95, số lời gọi >1 s |
|---|---|---|
| 90 | 14.540, 33 ms | 600, 1.717 ms, 426 |
| 100 | 13.860, 29 ms | 624, 2.664 ms, 475 |
| 110 | 13.900, 39 ms | 557, 2.845 ms, 461 |
| 120 | 14.366, 38 ms | 621, 2.986 ms, 461 |
| 130 | 13.760, 39 ms | 570, 3.415 ms, 439 |
| 140 | 13.619, 36 ms | 548, 2.841 ms, 382 |

- **API khỏe, FRONTEND là thứ chạm trần ở 1200 VU:** gateway p95 29-39 ms suốt bậc 1200; các trang frontend (`/`, `/products`, `/products/:id`, `/cart`, `/login`) mất 1,7-3,4 s ở p95, ~75% số lời gọi trang chậm hơn 1 s. Khớp với số đo tài nguyên: frontend chạm trần CPU (throttle 25,6% cả lượt, cao nhất trong các service nghiệp vụ, như ở CCU đóng 1000: 106% trần).
- p95 gộp của ccu (`calls/s`, p95 cột "p95") là p95 của TẤT CẢ lời gọi (api + trang frontend + AI chat). Lời gọi chậm (trang frontend ~4% + `/ai/chat` ~1,4%) xấp xỉ ngưỡng 5%, nên p95 gộp dao động giữa ~100 ms và ~1,8 s tuỳ cửa sổ có vượt 5% hay không (giây 110: 1.847 ms; giây 90, 100: 62-127 ms). Cửa sổ giây 110 vì vậy KHÔNG phải sự cố riêng, là hệ quả của frontend quá tải kéo dài + cách tính p95 gộp.
- Hàm ý cho báo cáo: khi nói "p95 của hệ thống", tách API và frontend; ở 1000-1200 VU điểm yếu về độ trễ người dùng thấy là trang frontend (Next.js, giới hạn CPU), không phải gateway/backend.

## 5. Spike mở 10 -> 30 -> 10 phiên/s (`spike-open-10-30-10`, n=1, stack mới, mặc định) - HỢP LỆ, SẠCH

Nhịp phiên MỚI đổi đột ngột ngay ranh giới bậc (không tăng dần): 10 phiên/s 60 s -> 30 phiên/s 60 s -> 10 phiên/s 60 s (sau 30 s khởi động ở 10 phiên/s). 3.301 phiên, đỉnh 505 phiên cùng lúc, 59.885 lời gọi.
- 0 lỗi, 0 VU hỏng, 0 lần 429 trong mọi cửa sổ 10 s, kể cả cửa sổ nhảy. Chờ pool DB không có. p50 3 ms; p95 60 ms ở bậc 10 (là bcrypt đăng nhập ~56 ms), 62-70 ms ở bậc 30 (cửa sổ nhảy 70 ms). Số phiên cùng lúc theo nhịp: 174 -> ~500 sau ~40 s ở bậc 30 -> về ~170 sau ~40 s khi hạ (độ trễ tự nhiên: mỗi phiên kéo dài vài hành trình).
- Thông lượng tuyến tính: ~195 -> ~590 calls/s -> ~190. Payment p95 ~507-520 ms (cửa sổ nhảy 1.007 do độ phân giải 500 ms của vòng hỏi lại), 0 đơn quá 8 s chưa có payment.
- Hệ thống: CPU toàn máy trung bình 38%, đỉnh 63% (dưới ngưỡng tin cậy 75%); lag outbox tối đa 0,24 s; Postgres tối đa 1.534 commit/s; throttle auth 5,1%.
- Kết luận: 3 lần tăng nhịp phiên mới (10 -> 30) hoàn toàn trong sức chịu của auth (trần mở đã biết 40-45 phiên/s); mức này nhảy đột ngột cũng sạch và hồi phục ngay. Chưa nói gì về 40-50 (trần auth): lượt kế tiếp.

## 5b. Spike mở 10 -> 40 -> 10 phiên/s (`spike-open-10-40-10`, n=1, stack mới, mặc định) - SẠCH (cờ "chưa hợp lệ" chỉ do 1 lỗi script)

Nhịp phiên mới: 10 phiên/s 60 s -> 40 phiên/s 60 s -> 10 phiên/s 60 s. 3.900 phiên, đỉnh 692 phiên cùng lúc, 72.321 lời gọi.
- Chỉ 1 lỗi cả lượt: `POST /auth/register 409` (script tạo trùng tên người dùng) làm 1 VU hỏng, nên ccu in "CHƯA HỢP LỆ"; không có 5xx, không có 429, không có chờ pool DB. Không phải lỗi hệ thống.
- Thông lượng ~200 -> ~800 -> ~200 calls/s, p50 3-4 ms, p95 60-157 ms (cửa sổ nhảy 157 ms; bậc 40: 72-91 ms), payment p95 ~507-520 ms, lag outbox tối đa 0,42 s, Postgres tối đa 2.043 commit/s. Toàn máy CPU trung bình 40%, đỉnh 73% (dưới ngưỡng tin cậy 75% sát).
- Throttle auth 11,1% (lượt 30 phiên/s: 5,1%): auth bắt đầu sát trần nhưng ở 40 phiên/s chưa gây lỗi (ở CCU mở ổn định 40 phiên/s cũng hợp lệ 2/2 nhưng auth trung bình 3,4/5 core).
- **Cửa sổ giây 130 có p95 2.395 ms: ĐÃ KIỂM CHỨNG, không phải lỗi.** Dựng lại từ log gateway (cùng cửa sổ): p95 = 2.393 ms (khớp). 143/143 yêu cầu chậm >1 s trong cửa sổ đó đều là `POST /ai/chat` (mô hình giả trả sau ~2,4-2,6 s). Tỉ lệ `/ai/chat` trong tổng lời gọi mỗi cửa sổ: 2,0-3,2% (giây 60-110), 3,9% (120), **5,9% (130)**, 4,3% (140), 3,7% (150). Khi nhịp phiên mới hạ, lời gọi nhanh (đăng nhập, duyệt, giỏ) giảm nhanh hơn các cuộc AI chat đang dở (mỗi cuộc kéo dài), nên tỉ lệ AI vượt 5% và p95 rơi vào dải độ trễ của AI. Chỉ là hiệu ứng thống kê của p95 + độ trễ cố định của mô hình giả.
- Cửa sổ p95 1.847 ms ở giây 110 của spike đóng 1200 là chuyện KHÁC (đã kiểm chứng bằng log Traefik, xem mục 4b): do trang frontend chậm, không phải do tỉ lệ AI.

## 5c. Spike mở 10 -> 50 -> 10 phiên/s (`spike-open-10-50-10`, n=1, stack mới, mặc định) - HỢP LỆ (0 lỗi) nhưng độ trễ tăng rõ

Nhịp phiên mới: 10 phiên/s 60 s -> 50 phiên/s 60 s -> 10 phiên/s 60 s. 4.500 phiên, đỉnh 895 phiên cùng lúc, 85.992 lời gọi. 0 lỗi, 0 VU hỏng, 0 lần 429, 0 lỗi 5xx; 0 đơn quá 8 s chưa có payment (payment p95 505-606 ms).
- **Độ trễ trong cú nhảy tăng rõ rồi hồi phục:** p95 (gateway, đối chiếu log) giây 60: 808 ms, giây 70: 1.448 ms, giây 80: 599 ms (nền 60 ms), rồi 141-203 ms từ giây 90 khi đang ở 50 phiên/s; p50 3 -> 5-10 ms; max 5,7 s. Hạ về 10 phiên/s thì p95 về 59 ms từ giây 150.
- **Auth sát trần nhưng chưa vỡ:** CPU 5 bản đỉnh 5,54 core (trần 5), >4,0 core trong 48 s liền (suốt bậc 50), throttle auth 24,1% (lượt 40 phiên/s: 11,1%; 30: 5,1%), chờ pool DB của auth 262 s cộng dồn (đỉnh toàn hệ thống 12 s/s; các service khác ~0). Không lỗi vì mỗi bậc chỉ giữ 60 s: số phiên cùng lúc mới lên 895 (trạng thái ổn định ở 50 phiên/s cần ~2.500 phiên cùng lúc) nên hàng đợi chưa đầy.
- **So với CCU mở ổn định 50 phiên/s đã gãy 2/2 (đợt 7):** hai kết quả KHÔNG mâu thuẫn. Ở đây chỉ giữ 50 phiên/s 60 s và bắt đầu từ nền 10; CCU ổn định giữ 50 phiên/s liền từ đầu (30 s khởi động + 90 s đo, auth trung bình 4,65-4,68 core, lỗi 1,9-3,6%). Spike 50 chịu được 60 s với độ trễ tăng, nhưng 50 phiên/s kéo dài thì auth không theo kịp. (Giả thuyết đối chiếu, n=1 mỗi cách, chưa kiểm chứng bằng lượt giữ 50 phiên/s lâu hơn sau nền 10.)
- Cửa sổ giây 130-140 có p95 gộp 2,4-2,5 s: cùng hiệu ứng thống kê như mục 5b (tỉ lệ `/ai/chat` trong tổng lời gọi 6,6% và 6,1% > 5%, các cửa sổ khác 1,7-4,3%), không phải lỗi.
- Toàn máy CPU đỉnh 91,5% (trung bình 47%), trên ngưỡng tin cậy 75% (máy tải dùng chung máy): độ trễ có thể bị cộng thêm.
- Chưa chạy 10 -> 60 -> 10 (không nằm trong chuỗi đã chốt): 60 phiên/s sẽ vượt hẳn trần auth (50 đã chạm 5,5/5 core).

## 6. Cách phát biểu kết quả (QUY ƯỚC KHI VIẾT BÁO CÁO)

- **Spike đóng 1200 chỉ được nói là "giữ 80 giây sau cú nhảy thì không gãy"**, KHÔNG nói "chịu được 1200 VU bền vững". Lý do: (1) lần đo trước ở 1200 (đợt 8, ramp 160 s) đã gãy: payment p95 10,5 s, lag consumer `validate-result` tăng dần từ ~210 s lên 1.022 s và không hồi phục; lỗi đó tích lũy theo thời gian nên 80 s có thể chưa đủ để thấy; (2) mỗi cách chỉ n=1; (3) toàn máy đỉnh CPU 96% trong lượt spike (máy tải dùng chung máy); (4) chỉ vào được 1.175/1.200 VU; (5) cửa sổ giây 110 có p95 1.847 ms (đã giải thích ở mục 4b: frontend Next.js chạm trần CPU, trang chậm 1,7-3,4 s, API vẫn p95 ~35 ms).
- **Giả thuyết chưa kiểm chứng** vì sao spike 1200 không gãy như ramp 1200: thời gian giữ ngắn hơn, nền chỉ 200 VU (ít dữ liệu tồn) và cách vào khác. Cách kiểm: spike 200 -> 1200 giữ ~5 phút (xem lag consumer có tích lũy không) và lặp ramp 1200; CHƯA làm.
- **"Chạm trần" ở 1000 VU = nhiều service chạm trần CPU nhưng độ trễ còn tốt** (CCU đóng 1000 sạch, 9/9 vòng ở 700-1000); 1100 bắt đầu yếu, 1200 (ramp) gãy. Spike 1000 giữ 90 s sạch sau cú nhảy.
- **Tóm tắt chuẩn cho spike đóng:** không sập, hồi phục tức thì khi hạ tải; sau cú nhảy lên 800-1200 VU có lỗi tạm thời ~10-20 s đầu (31 -> ~190 -> ~600 lỗi 5xx cho 800 -> 1000 -> 1200), gần như toàn bộ ở auth-service (CPU 5/5 core, bcrypt đăng ký/đăng nhập dồn, route seller phụ thuộc auth); ở 600 thì sạch.
- **Điều kiện kịch bản:** VU mới tự ĐĂNG KÝ lúc vào (bcrypt), nên lỗi cú nhảy nặng hơn spike chỉ gồm người đã có tài khoản.
- **Spike mở:** 10 -> 30 -> 10 và 10 -> 40 -> 10 sạch; 10 -> 50 -> 10 không lỗi nhưng auth chạm trần (5,5/5 core) và p95 tăng lên ~0,6-1,5 s trong cú nhảy. Nói "chịu được cú nhảy 10 -> 50 phiên/s giữ 60 s", không nói "chịu được 50 phiên/s": CCU mở ổn định 50 phiên/s đã gãy 2/2.

## 7. Tổng hợp chuỗi 6 lượt đợt 9 (2026-11-24, n=1 mỗi lượt, stack mới dựng cho từng lượt, cấu hình mặc định)

| lượt | kết quả | lỗi ngoài dự kiến | cửa sổ tệ nhất | điểm yếu |
|---|---|---|---|---|
| đóng 200 -> 600 -> 1000 -> 200 | sạch | 0 | p95 527 ms | - |
| đóng 200 -> 800 -> 200 | hợp lệ | 56 (31 là 5xx trong cú nhảy) | 29 lỗi | auth CPU/bcrypt, route seller |
| đóng 200 -> 1000 -> 200 | CHƯA hợp lệ (11 VU hỏng) | 226 | 168 lỗi (1,6%) | auth CPU, circuit breaker mở |
| đóng 200 -> 1200 -> 200 | CHƯA hợp lệ (25 VU hỏng) | 713 | 496 lỗi (5,2%) | auth CPU; frontend chạm trần |
| mở 10 -> 30 -> 10 | sạch | 0 | p95 70 ms | - |
| mở 10 -> 40 -> 10 | sạch (1 lỗi script 409) | 1 | p95 157 ms | auth throttle 11% |
| mở 10 -> 50 -> 10 | hợp lệ (0 lỗi) | 0 | p95 1,45 s | auth 5,5/5 core, throttle 24% |

Nguyên nhân chung của mọi lỗi: auth-service (5 bản x 1 core) hết CPU khi đợt đăng ký/đăng nhập dồn (bcrypt); các route seller phụ thuộc auth (`GetStoreIDRoleById` mỗi request) và circuit breaker của gateway phản ứng theo. Frontend (Next.js) là điểm chậm thứ hai ở 1000-1200 VU (mục 4b). Không còn lỗi `pwd_version`/Redis (R25 đã sửa, 0 lỗi 503 loại đó trong cả chuỗi).

## 8. Trạng thái cuối đợt 9, việc còn lại và cách commit (2026-11-24)

**Đã làm trong đợt 9:** sửa R25 (mục 1), công cụ spike (`ccu.mjs --vu-stages/--rate-stages`, bảng cửa sổ 10 s; `perf-capture.py` cộng các bản cùng tên; `perf-one.sh`, `perf-series.py`), 6 lượt spike (mục 2-5c, tổng hợp mục 7), quy ước phát biểu (mục 6). Stack đã dọn (Swarm trống, không còn volume `deploy_*`).

**Việc còn lại (không bắt buộc, chờ quyết định):**
- Kiểm chứng giả thuyết: giữ 1200 VU khoảng 5 phút sau nền 200; giữ 50 phiên/s lâu hơn sau nền 10; lặp ramp 1200 (đợt 8 gãy n=1, nguyên nhân chưa biết).
- Lặp mỗi lượt spike thêm vòng (n=1 hiện tại); spike 10 -> 60 -> 10 (vượt trần auth) nếu muốn xem quá tải sâu.
- Giảm lỗi auth (đã bàn, quyết định KHÔNG làm vì auth là giới hạn tài nguyên đã chốt): đưa `store_id`/role vào JWT hoặc cache ở gateway; semaphore giới hạn bcrypt song song (bỏ bcrypt cho request đã bị gateway hủy); rate limit đăng nhập theo tài khoản.
- Log Swarm của gateway bị xoay vòng ở lượt nặng (mất phần cú nhảy của lượt 1200): nâng giới hạn log của gateway khi đo, hoặc dump log giữa lượt.
- Chịu lỗi (kill một bản, restart Redis dưới tải), dữ liệu lớn, soak dài: bàn xong là không cần cho dự án mô phỏng (chỉ làm nếu muốn).
- Trả lại 6 file sửa tạm của `marketplace-net2` sau khi Docker khởi động lại: `git checkout -- deploy/infra.yml deploy/observability.yml deploy/services.yml deploy/services.perf.yml scripts/wait-ready.sh scripts/teardown.sh` (xem 09 §5s).

**Commit (người dùng chạy; KHÔNG đưa 6 file sửa tạm net2; log không vào commit vì `.gitignore` đã chặn `*.log`, và `*.capture.jsonl` (~94 MB dữ liệu thô 1 Hz) cũng loại ra):**
```
git add CLAUDE.md deploy/.env.example docs/design/09_PERF_RESOURCES.md docs/design/10_PERF_SPIKE.md docs/planning/01_OPEN_ISSUES.md scripts/load shared/dbpool services/api-gateway services/auth-service services/order-service services/product-service services/user-service services/payment-service services/ops-service docs/perf-results ':(exclude)docs/perf-results/*/*.capture.jsonl'
git commit -m "perf: real spike tests (ccu --vu-stages/--rate-stages), fix R25 by rebuilding pwd_version deny keys from the DB (ListRecentPwdChanges), dbpool idle=open (R23), round 6-9 results and 10_PERF_SPIKE.md"
```
Dữ liệu `docs/perf-results/2026-11-24-dot9/` nặng 597 MB nhưng 574 MB là `logs-*` (không vào commit); phần vào commit khoảng 24 MB (đợt 9) + phần nhỏ của đợt 6-8. Nếu muốn giữ cả `*.capture.jsonl`, bỏ phần `':(exclude)...'` ở cuối lệnh `git add`.
