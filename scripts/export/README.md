# D3 export: ClickHouse -> file huấn luyện có phiên bản

Thiết kế: [docs/design/07_DATA_PIPELINE.md](../../docs/design/07_DATA_PIPELINE.md) §6. Công cụ chạy một lần, Python 3, chỉ dùng thư viện chuẩn (gọi ClickHouse qua HTTP bằng `urllib`). Parquet cần thêm `pyarrow` (tuỳ chọn).

## Dùng
```bash
python3 scripts/export/export.py --from 2026-10-01 --to 2026-10-11 --profile otto --format csv \
  --out out/otto-oct --split-by-time train=0.8,valid=0.1,test=0.1 \
  --clickhouse-url http://127.0.0.1:8123 --user default --password '***' --db marketplace
```
- `--from` (gồm) / `--to` (không gồm): `YYYY-MM-DD` hoặc `YYYY-MM-DDTHH:MM:SS`, hiểu theo múi giờ nền tảng `Asia/Ho_Chi_Minh`.
- Mặc định kết nối lấy từ biến môi trường `CLICKHOUSE_URL`, `CLICKHOUSE_USER`, `CLICKHOUSE_PASSWORD`, `CLICKHOUSE_DB` (mặc định `http://127.0.0.1:8123`, `default`, rỗng, `marketplace`).
- `--format parquet` cần `pip install pyarrow` (nên dùng venv); thiếu thì báo lỗi rõ ràng, không xuất gì.
- `--version` đặt tên phiên bản (mặc định `snapshot-<from>_<to>`). `--order-session-gap-hours` (mặc định 24) xem mục Profile.

## Profile
| Profile | Cột | Nguồn |
|---|---|---|
| `otto` | `session, aid, ts(ms UTC), type` với type = `clicks` (`product_click`), `carts` (`add_to_cart`), `orders` | `events_raw` + `fact_order_items`/`fact_order_status` |
| `rees46` | `event_time, event_type(view/cart/remove_from_cart/purchase), product_id, category_id, brand, price, user_id, user_session` | `events_raw` + `dim_products FINAL` (+ đơn) |
| `orders` | `order_ts, order_date, order_id, item_id, buyer_id, store_id, product_id, category_id, qty, unit_price, line_amount, payment_method` | đơn đã ghi nhận doanh thu |
| `raw` | `events_raw` đã lọc cột | `events_raw FINAL` |

Quy ước:
- Đơn được tính theo định nghĩa doanh thu ADR-22: `PAID` (online) hoặc `DELIVERED` (COD); thời điểm = lần đầu đạt trạng thái đó. Chưa trừ hoàn tiền.
- `session`/`user_session` là số 63 bit băm ổn định (sha256) từ `session_id` (hoặc `anonymous_id` nếu thiếu), cùng chuỗi cho cùng số ở mọi lần chạy. `user_id` (rees46) là id tài khoản, hoặc băm của `anonymous_id` nếu chưa đăng nhập.
- Đơn hàng không có session trong bảng fact, nên `otto`/`rees46` gắn đơn vào phiên cuối của chính buyer (chỉ event `consent=1`, trước thời điểm ghi nhận, cách tối đa 24 giờ). Đơn không gắn được thì **bị bỏ** khỏi hai profile này (vẫn có trong `orders`).
- `price` là đơn vị chính (VND, 2 số lẻ). Event dùng giá hiện tại của `dim_products`; dòng purchase dùng giá lúc đặt.
- `orders`: `buyer_id = 0` nếu buyer không có event nào `analytics_consent=1`.

## Quyền riêng tư
Chỉ event `analytics_consent = 1`. Không bao giờ xuất `ip_hash`, `ua_family` (và email, SĐT không nằm trong các bảng này). Bảng `ReplacingMergeTree` đều đọc bằng `FINAL`. Manifest che mật khẩu ClickHouse.

## Chia train/valid/test theo thời gian
`--split-by-time` có hai dạng (không trộn):
- Tỉ lệ theo độ dài khoảng thời gian: `train=0.8,valid=0.1,test=0.1` (tổng <= 1). Mốc cắt làm tròn xuống giây.
- Khoảng ngày nửa mở, không chồng lấn: `train=2026-10-01..2026-10-08,valid=2026-10-08..2026-10-10,test=2026-10-10..2026-10-11`.

Kết quả: `max(ts train) < min(ts valid)`, `max(ts valid) < min(ts test)`. Cắt theo thời điểm event, nên một phiên đi qua mốc cắt sẽ bị tách ở hai file.

## Đầu ra và tính lặp lại
Trong `--out`: `<profile>.csv` (hoặc `<profile>_train.csv`...), `manifest.json`. Manifest có: `version`, `created_at`, khoảng thời gian, `profile`, `format`, URL ClickHouse (đã che mật khẩu), `columns` (tên + kiểu), `split`, mỗi file: `rows`, `bytes`, `sha256`, ts min/max, và `filters`. Dữ liệu sắp xếp xác định (`ts` rồi khoá nguồn), nên cùng tham số + cùng dữ liệu ⇒ file giống hệt từng byte (chỉ `created_at` trong manifest khác).

## Test
```bash
scripts/export/tests/run_tests.sh [python]   # dựng ClickHouse 24.8 trong docker (cổng 18125), chạy unittest, dọn
```
Test tạo bảng bằng đúng DDL đọc từ `services/analytics-service/internal/store/schema.go`, chèn dữ liệu tự sinh, so kết quả với bản tham chiếu độc lập. Nhánh parquet chỉ chạy khi `pyarrow` import được (truyền python của venv làm tham số). Không có Docker: đặt `TEST_CLICKHOUSE_URL` (+ `TEST_CLICKHOUSE_USER`, `TEST_CLICKHOUSE_PASSWORD`) rồi `python3 -m unittest discover -s tests` trong `scripts/export`.

## Giới hạn
- Chưa có `request_id`/`model_version`, chưa nối ẩn danh <-> tài khoản (S5); `product_view` không vào profile `otto` (click = `product_click` theo thiết kế).
- Parquet: `price` là float64, chưa kiểm hash giống hệt giữa các phiên bản pyarrow khác nhau.
- Không có scheduler hay kho đối tượng (D6, D1): ghi ra thư mục trên đĩa.
- `props`/`path`/`referrer` của profile `raw` xuất nguyên văn (không quét PII trong nội dung).
