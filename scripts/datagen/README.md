# datagen (D5): sinh / nạp dữ liệu lịch sử

Công cụ chạy một lần, nằm ngoài các service. Ghi **Postgres bằng SQL trực tiếp** (đúng bảng, cột của từng service, thời gian
trong quá khứ) và **`events_raw` của ClickHouse bằng HTTP** (JSONEachRow). Thiết kế: [docs/design/07_DATA_PIPELINE.md](../../docs/design/07_DATA_PIPELINE.md) §4.
Không gọi API thật, không cần stack chạy; chỉ cần Postgres (đã áp migration của 5 service) + ClickHouse.

- Chỉ dùng thư viện chuẩn Python 3.9+ (xem `requirements.txt`). Không cần driver: ghi Postgres bằng cách đưa SQL vào stdin của `psql`.
- Mật khẩu mọi tài khoản sinh ra là `password` (bcrypt cost 10 cố định, để chạy lại cho dữ liệu y hệt).

## Chạy nhanh

```bash
# 1) Postgres 16 + ClickHouse 24.8 (ví dụ, cổng tuỳ ý) và áp migration (bỏ phần sau '-- +goose Down')
docker run -d --name mm-pg -e POSTGRES_PASSWORD=postgres -p 5432:5432 postgres:16
docker run -d --name mm-ch -p 8123:8123 clickhouse/clickhouse-server:24.8
docker exec mm-pg psql -U postgres -c "CREATE DATABASE marketplace"
for s in auth user product order payment; do
  sed '/+goose Down/,$d' services/$s-service/migrations/*.sql | docker exec -i mm-pg psql -U postgres -d marketplace -q
done

# 2) Nạp dữ liệu nhân tạo 3 tháng (--ch-ensure-schema tạo events_raw nếu analytics-service chưa chạy lần nào)
python3 scripts/datagen/datagen.py synthetic --seed 1 --start 2026-07-01 --end 2026-09-30 \
  --psql "docker exec -i mm-pg psql -U postgres -d marketplace" \
  --ch-url http://127.0.0.1:8123 --ch-db marketplace --ch-ensure-schema

# 3) Xem trước, không ghi gì:   ... --dry-run   (thêm --sql-out f.sql / --events-out f.ndjson để lấy file)
# 4) Xoá làm lại:               ... --reset     (hoặc lệnh con `reset`)
python3 scripts/datagen/datagen.py reset --psql "..." --ch-url ... --ch-db marketplace
```

`--psql` cũng đọc biến `DATAGEN_PSQL`; ClickHouse đọc `CLICKHOUSE_URL`, `CLICKHOUSE_DB` (mặc định `marketplace`),
`CLICKHOUSE_USER`, `CLICKHOUSE_PASSWORD`. Dùng `--skip-pg` / `--skip-ch` để chỉ ghi một phía.
Sau khi nạp, chạy D4 backfill để dựng `fact_*` / `dim_products` từ Postgres (datagen chỉ ghi `events_raw` ở ClickHouse).

## Tham số chính

| Tham số | Ý nghĩa |
|---|---|
| `--seed N` | cùng seed + cùng tham số ⇒ dữ liệu y hệt từng byte (SQL và events). |
| `--start/--end YYYY-MM-DD` | khoảng lịch sử theo **giờ Việt Nam**, gồm cả hai đầu. Mặc định `end` = hôm qua, `start` = end−59 ngày (mặc định phụ thuộc ngày chạy; muốn tái lập hãy truyền cả hai). Đơn gần cuối khoảng còn ở trạng thái chưa xong (AWAITING_PAYMENT, CONFIRMED, PAID, SHIPPED), tồn `reserved` phản ánh đúng. |
| `--id-base` | khoá chính tường minh bắt đầu từ `id-base + 1` (mặc định 5.000.000) để không đụng dữ liệu thật; **sequence không bị chỉnh**. |
| `--dry-run` | sinh, in số dòng mỗi bảng, không ghi DB. |
| `--reset` | xoá dữ liệu datagen cũ rồi mới ghi. Không có `--reset` mà đã có dữ liệu datagen ⇒ từ chối (tránh ghi đôi). |
| synthetic: `--buyers --sellers --products --sessions-per-day --visitors --consent-rate` | quy mô (mặc định 200/6/120/150/400/0.9). |
| retailrocket: `--rr-dir DIR --sellers --buyer-share` | thư mục chứa `events.csv` (bắt buộc), `category_tree.csv`, `item_properties_part*.csv` (tuỳ chọn). |

## Chế độ retailrocket

1. Tải bộ **RetailRocket recommender system dataset** từ Kaggle (`retailrocket/ecommerce-dataset`): trang
   https://www.kaggle.com/datasets/retailrocket/ecommerce-dataset , hoặc `kaggle datasets download -d retailrocket/ecommerce-dataset`
   (cần tài khoản + API token Kaggle).
2. Giải nén vào một thư mục **ngoài git**, ví dụ `scripts/datagen/data/retailrocket/` (thư mục `data/` nên thêm vào `.gitignore` nếu bạn đặt ở đây;
   `out/` đã được bỏ qua). Cần `events.csv`; nên có `category_tree.csv`, `item_properties_part1.csv`, `item_properties_part2.csv`.
3. `python3 scripts/datagen/datagen.py retailrocket --rr-dir scripts/datagen/data/retailrocket --start ... --end ... --psql ... --ch-url ...`

Ánh xạ: `view`→`product_view`; `addtocart`→`add_to_cart`; `transaction`→đơn (cùng `transactionid` = một checkout, mỗi store một đơn, dòng lặp lại
của cùng `itemid` = số lượng); `visitorid`→`anonymous_id = dg-rr-<visitorid>` và, với người có giao dịch + `--buyer-share` (10%) số còn lại, tài khoản
`dg_buyer_rr_<visitorid>`; `itemid`→`products` (sku `dg-rr-<itemid>`); `categoryid` mới nhất theo `timestamp` trong `item_properties` quyết định danh mục.
Thời gian 2015 được co giãn tuyến tính vào `[start, end]`. RetailRocket không có giá / tên / tồn / người bán nên sinh tất định từ `itemid`
(giá 20.000–5.000.000 VND, 6 store giả). Phiên = cách nhau > 30 phút.
**Bảng ánh xạ** (`kind,source_id,target,note` với kind ∈ item | visitor | category) ghi ra `--map-file` (mặc định `scripts/datagen/out/retailrocket_map.csv`).

Bộ mẫu tí hon cùng định dạng ở `fixtures/` (61 dòng sự kiện, 4 giao dịch) dùng cho test: `--rr-dir scripts/datagen/fixtures`.

## Bảng được ghi

| Dịch vụ | Bảng | Nội dung |
|---|---|---|
| auth | `accounts` | `dg_buyer_NNNN` (buyer), `dg_seller_NNN` (seller_admin, `store_id` = `sellers.id`); mật khẩu bcrypt của `password` |
| user | `buyers`, `sellers`, `addresses` | hồ sơ, địa chỉ mặc định (+ địa chỉ phụ) |
| product | `categories` (slug `dg-…`), `products` (sku `dg-…`; `inventory/reserved/sold` cuối cùng), `inventory_ledgers` | sổ cái `initial`, `restock`, `reserve`, `release`, `sale` khớp từng thời điểm |
| order | `checkouts` (id `co_dg…`), `orders`, `order_items`, `order_status_histories` | mỗi store một đơn; timeline đầy đủ |
| payment | `payments`, `attempts`, `refunds` | chỉ đơn online (COD không có payment) |
| analytics | `events_raw` (ClickHouse; `event_id` `dg-…`) | consent, session, `position`/`surface` cho impression |

**Không ghi**: `domain_events` / các bảng outbox (nếu ghi `PENDING` thì worker sẽ đẩy lên Kafka và nhân đôi dữ liệu), `cart_items`, `webhook_deliveries`,
`processed_events`, `pwd_version_events`, `validate_order_events`, các bảng `fact_*` / `dim_products` (việc của D4 backfill).

## Ngữ nghĩa dữ liệu

- Đơn **online** (MOCK_CARD / MOCK_WALLET): `PENDING → AWAITING_PAYMENT → PAID` (actor `payment`, có `payments.status=SUCCEEDED`, `paid_at` trùng `orders.paid_at`)
  `→ SHIPPED → DELIVERED`; nhánh: hết hạn 15 phút `EXPIRED` (`payment_timeout`, payment `EXPIRED`), thanh toán lỗi 5 lần (payment `FAILED`, đơn hết hạn),
  buyer huỷ khi chưa trả (payment `CANCELED`), huỷ sau khi đã trả (hoàn tiền đủ), yêu cầu trả hàng `REFUND_REQUESTED → REFUNDED | DELIVERED (từ chối)`.
  Có 0–2 lần thử thất bại trước lần thành công. Một payment cho cả checkout (tổng các đơn, kể cả phí ship); hoàn theo từng đơn ⇒ `PARTIALLY_REFUNDED`/`REFUNDED`.
- Đơn **COD**: `PENDING → CONFIRMED → SHIPPED → DELIVERED` (`payment_status` = PAID khi DELIVERED), hoặc `CANCELED`. Không có dòng `payments`.
- Doanh thu theo ADR-22 (PAID online / DELIVERED COD, trừ hoàn tiền) tính được từ các dòng `order_status_histories` / `refunds`.
- Tiền: `NUMERIC(18,2)` VND (giá nguyên nghìn), `payments.amount_minor` = VND×100. Phí ship 30.000 VND, miễn phí từ 500.000 VND (giả định; đổi trong `commerce.py` nếu cấu hình order-service khác).
- Tồn kho: `inventory + reserved + sold` = tổng nhập; `reserved` = đơn chưa xuất kho (chưa SHIPPED, chưa huỷ); `sold` = đơn đã SHIPPED; hết hàng ⇒ ledger `restock`.
- `events_raw`: `product_click`/`product_view`/`add_to_cart`/`search`/`impression`(position ≥ 1, surface hợp lệ)/`checkout_*`/`payment_page_view`/`dwell`/`page_view`;
  người từ chối consent (~10%) chỉ còn `page_view` ẩn danh (mất `anonymous_id`/`session_id`/`user_id`/`ip_hash`), đúng luật `ingest/tracking.go`.
  Sự kiện `order_placed`, `payment_succeeded`… (đến từ Kafka) **không** sinh ở đây. Mọi sự kiện của phiên đăng nhập mang `user_id` (không mô phỏng `identify`).
- `--reset` nhận diện bằng tiền tố: `accounts.username LIKE 'dg\_%'` (và buyers/addresses/sellers theo id đó), `products.sku LIKE 'dg-%'`, `categories.slug LIKE 'dg-%'`,
  `checkouts.id LIKE 'co\_dg%'` (kéo theo orders/items/histories/payments/attempts/refunds), `events_raw.event_id` bắt đầu `dg-`.

## Test

```bash
python3 -m unittest discover -s scripts/datagen/tests -t scripts/datagen -v    # phần không cần hạ tầng luôn chạy
# thêm phần tích hợp (tự bỏ qua nếu thiếu env):
export DATAGEN_TEST_PSQL="docker exec -i dg-pg psql -U postgres"     # server Postgres 16, không kèm -d
export DATAGEN_TEST_CH_URL=http://127.0.0.1:18123                      # ClickHouse 24.8
```
Test tạo DB tạm cho mỗi ca, áp **đúng** `services/{auth,user,product,order,payment}-service/migrations/*.sql` (phần goose Up) và lấy DDL `events_raw` thật từ
`services/analytics-service/internal/store/schema.go`, nên đổi schema ở service sẽ làm test hỏng. Kiểm: hai lần cùng seed cho hash bảng y hệt, ràng buộc nghiệp vụ
(tổng tiền, payment ↔ đơn PAID, COD, hoàn tiền, tồn kho không âm và khớp sổ cái, FK logic), đếm `events_raw`, `--reset` không đụng dữ liệu thật, fixture RetailRocket.
