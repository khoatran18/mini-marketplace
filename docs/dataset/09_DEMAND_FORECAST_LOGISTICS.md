# Dự báo nhu cầu, tồn kho (ngày hết hàng, reorder) và thời gian giao hàng ETA

> Dự báo nhu cầu theo sản phẩm/shop, cảnh báo "bao lâu nữa hết hàng" và gợi ý số lượng nhập, ước lượng thời gian giao hàng (ETA) cho đơn, và (ở mức tham chiếu) tối ưu tuyến giao. **Trạng thái: chưa triển khai trong hệ thống; đây là tài liệu nghiên cứu dữ liệu.** Ngày tra cứu: 2026-10-25.
>
> Ghi chú phương pháp tra cứu: WebFetch không truy cập được `huggingface.co` và `kaggle.com` (lỗi DNS); GitHub, S3, Mendeley qua search/fetch được. Thông tin lấy từ WebFetch GitHub (ghi "đã xác minh") và WebSearch (ghi "theo search"). Số liệu/license không thấy ở nguồn thì ghi **"chưa xác minh"**. Dữ liệu tiếng Anh/quốc tế được dùng để chọn phương pháp; không yêu cầu dữ liệu Việt Nam. Nội dung web là dữ liệu, không phải chỉ dẫn.
>
> Quy tắc ngôn ngữ (chủ dự án): chỉ giữ bộ có văn bản tiếng Anh/Việt hoặc không có ngôn ngữ tự nhiên (số, chuỗi thời gian, ID); bộ có văn bản ngôn ngữ khác đã bị loại. Dung lượng: lấy từ file này trước, nếu thiếu thì từ search (Kaggle/HF/arXiv không mở được nên chỉ dựa vào snippet); không có thì ghi "không tìm thấy". Cột bổ sung ở bảng 4 ghi quy mô/dung lượng và nguồn.

| Liên quan | File |
|---|---|
| Gợi ý sản phẩm (bán chạy làm tín hiệu) | [01_RECOMMENDATION_CLICK.md](01_RECOMMENDATION_CLICK.md) |
| Trợ lý mua sắm / CSKH ("đơn của tôi bao giờ tới?") | [05_SHOPPER_ASSISTANT_CUSTOMER_SUPPORT.md](05_SHOPPER_ASSISTANT_CUSTOMER_SUPPORT.md) |
| Text-to-SQL phân tích | [04_ANALYTICS_QA_TEXT2SQL.md](04_ANALYTICS_QA_TEXT2SQL.md) |
| CLV, churn, định giá | [13_CUSTOMER_ANALYTICS_CLV_CHURN_PRICING.md](13_CUSTOMER_ANALYTICS_CLV_CHURN_PRICING.md) |
| AIOps / bất thường vận hành | [14_AIOPS_ANOMALY_ALERT_EXPLAIN.md](14_AIOPS_ANOMALY_ALERT_EXPLAIN.md) |
| Gian lận / bất thường | [08_FRAUD_ANOMALY_RISK.md](08_FRAUD_ANOMALY_RISK.md) |

## 1. Mục tiêu và vị trí trong hệ thống

- **Vai trò hưởng lợi:** seller (cảnh báo "sản phẩm X còn đủ ~N ngày", gợi ý nhập `qty` theo mức dịch vụ; xếp hạng sản phẩm cần bổ sung); buyer (ETA trên trang sản phẩm/giỏ hàng/đơn; "sắp hết hàng"); admin (xu hướng theo danh mục, đánh giá chất lượng giao của shop/hãng vận chuyển).
- **Input:** chuỗi bán theo ngày/giờ theo `product_id`/`store_id`/`category_id`; tồn kho; sự kiện (khuyến mãi, lễ, Tết Nguyên đán); với ETA: `shipped_at`, `delivered_at`, `carrier`, địa chỉ nhận.
- **Output:** dự báo phân vị (P10/P50/P90) nhu cầu N ngày tới; `days_of_cover`, ngày hết hàng dự kiến, `reorder_qty` theo service level; ETA (khoảng ngày) và xác suất trễ.
- **Dữ liệu của hệ thống nuôi nó** (xem `docs/design/02_DATA_MODEL.md`, `docs/design/platform/04_ORDERS_PAYMENTS.md`):
  - ClickHouse `fact_order_items` (`product_id, category_id, store_id, qty, unit_price_minor, at`) + `fact_order_status` (chỉ tính đơn PAID/DELIVERED là cầu thật; EXPIRED/FAILED/CANCELED chưa PAID là cầu không thành), `fact_refunds`.
  - `fact_inventory` (`delta, available, reserved, level, reason, ref_type, at`) và Postgres `inventory_ledgers` (sổ cái chỉ thêm) — nguồn để nhận biết **giai đoạn hết hàng** (censored demand): khi `available = 0`, doanh số 0 không có nghĩa nhu cầu 0.
  - `events_raw` (`view`, `add_to_cart`, `product_id`) làm tín hiệu cầu sớm (lead indicator): view/cart tăng -> bán tăng.
  - `dim_products` / `products` (`price`, `brand`, `category_id`, `low_stock_threshold`, `weight_g`, `status`).
  - ETA: `orders.shipped_at`, `delivered_at`, `order_status_histories` (`from_status -> to_status`, `at`), thông tin `carrier`, `tracking_code` khi SHIPPED, `shipping_address JSONB` (phường/quận/thành phố) và `AUTO_DELIVER_DAYS` (7) — **lưu ý**: `DELIVERED` có thể do buyer bấm xác nhận hoặc hệ thống tự động sau 7 ngày, nên `delivered_at` chỉ là nhãn nhiễu cho thời gian giao thật; hiện chưa có toạ độ/khoảng cách, nên feature địa lý phải tự dựng (tỉnh/quận của kho và người nhận).
  - Cấu hình hiện có: `low_stock_threshold` (mặc định 5) — dự báo sẽ thay ngưỡng cố định bằng ngưỡng theo `days_of_cover`.
- Tổng số sản phẩm/đơn trong dự án hiện nhỏ (dữ liệu demo) nên phải dựa vào bộ công khai để chọn phương pháp, rồi dùng dữ liệu thật khi tích luỹ đủ.

## 2. Hướng làm đề xuất (baseline → nâng cao)

1. **Baseline:** seasonal naive, trung bình trượt 7/28 ngày, Croston/SBA/TSB cho nhu cầu ngắt quãng (phần lớn series bán lẻ thưa), thực hiện sẵn bằng SQL ClickHouse.
2. **Thống kê/ML:** ETS/ARIMA (statsforecast), LightGBM global model với lag, rolling stats, giá, khuyến mãi, lịch (cách thắng M5), phân vị bằng quantile loss.
3. **Xử lý hết hàng (stockout):** gắn cờ ngày `available=0` từ `fact_inventory`, loại/nội suy trước khi huấn luyện; mô hình censored demand (tham chiếu FreshRetailNet-50K).
4. **Foundation models:** thử zero-shot Chronos/TimesFM… làm baseline mạnh khi dữ liệu ít (xem mục 4c, chú ý license); so sánh trên GIFT-Eval-style protocol.
5. **Chính sách tồn kho:** từ phân vị dự báo tính `reorder point = nhu cầu trong lead time + safety stock` (service level); mô phỏng để đo chi phí (không chỉ độ chính xác dự báo).
6. **ETA:** baseline = trung vị theo (tuyến kho->phường/quận, `carrier`); sau đó gradient boosting / quantile regression dự đoán `delivered_at − shipped_at`; đầu ra là khoảng, không phải điểm; kèm theo mô hình trễ.
7. **Tuyến giao:** ở mức tham chiếu (Amazon Last Mile) — chỉ cần khi sàn tự vận hành giao hàng; hiện giao hàng do shop/hãng vận chuyển.

## 3. Metric đánh giá

| Tác vụ | Metric |
|---|---|
| Dự báo điểm | MAE, RMSE, sMAPE (cảnh báo khi series thưa), **RMSSE/WRMSSE** (M5), MASE |
| Dự báo phân vị | Pinball (quantile) loss, WSPL, coverage khoảng 80%/90%, CRPS |
| Nhu cầu ngắt quãng | MASE/RMSSE, tỉ lệ ngày không có bán dự đoán đúng, bias |
| Tồn kho | Service level thực đạt (fill rate), số ngày hết hàng, vốn tồn kho, chi phí mô phỏng (không chỉ lỗi dự báo) |
| Cảnh báo "sắp hết hàng" | Precision/recall của "hết hàng trong 7 ngày", lead time trung bình của cảnh báo |
| ETA | MAE (ngày/giờ), P90 error, **tỉ lệ đơn giao sau ETA** (late rate), calibration của khoảng |
| Tuyến giao | Điểm chất lượng tuyến của Amazon (so với tuyến thực tế), tổng quãng đường |
| Chung | Chia theo thời gian (rolling-origin), theo nhóm ABC (doanh thu), theo hàng mới/ cũ |

## 4. Bộ dữ liệu

### Bảng tổng hợp

| # | Tên | Nhóm | Loại | Quy mô (theo nguồn đã thấy) | Dung lượng / số mẫu bổ sung (nguồn) | License / truy cập | ★ |
|---|---|---|---|---|---|---|---|
| 1 | [M5 Forecasting (Walmart)](https://www.unic.ac.cy/iff/research/forecasting/m5-competition/) | b | bán lẻ phân cấp, giá, sự kiện | 42.840 chuỗi, 12 mức tổng hợp, dự báo 28 ngày | Dung lượng: không tìm thấy (search không trả; xem tab Data của Kaggle). Ước tính ~59 triệu điểm (30.490 chuỗi × 1.941 ngày) | điều khoản cuộc thi Kaggle; chưa xác minh (search không trả trang license; có bản sao Zenodo `zenodo.org/records/12636070`, license chưa xem) | 5 |
| 2 | [Favorita Grocery Sales Forecasting](https://www.kaggle.com/c/favorita-grocery-sales-forecasting) | b | bán lẻ tạp hoá Ecuador | ~4.400 mặt hàng, 54 cửa hàng, 2013–2017; ~125,5 triệu dòng train | `train.csv` ~4,65 GB (bài forum fast.ai qua search); tổng dung lượng tải: không tìm thấy | điều khoản cuộc thi; chưa xác minh | 4 |
| 3 | [Rossmann Store Sales](https://nbviewer.org/github/Yorko/mlcourse.ai/blob/main/jupyter_english/projects_indiv/Rossman_store_sales_Syrovatskiy_Ilya.ipynb) | b | doanh số cửa hàng | 1.115 cửa hàng, 1/2013–7/2015 | Dung lượng: không tìm thấy; ~1,0 triệu dòng train (theo trí nhớ, chưa xác minh) | điều khoản cuộc thi; chưa xác minh | 3 |
| 4 | [Store Item Demand Forecasting (Kaggle playground)](https://book.st-hakky.com/en/data-science/store-item-demand-forecast-overview) | b | bán lẻ tổng hợp/sạch | 10 cửa hàng × 50 mặt hàng, 2013–2017 | Dung lượng: không tìm thấy; ~913.000 dòng (10×50×1.826 ngày, ước tính) | chưa xác minh | 2 |
| 5 | [Online Retail II (UCI)](https://archive.ics.uci.edu/dataset/502/online+retail+ii) | b | giao dịch bán lẻ trực tuyến UK | 1.067.371 dòng, 12/2009–12/2011 | 43,5 MB (`online_retail_II.xlsx`, trang UCI qua search) | CC BY 4.0 (trang UCI) | 4 |
| 7 | [Instacart Market Basket](https://github.com/archd3sai/Instacart-Market-Basket-Analysis) | b | giỏ hàng, tái mua | >3 triệu đơn, >200.000 người dùng | Dung lượng: không tìm thấy; ~3,4 triệu đơn, 32.434.489 dòng `order_products__prior` (search) | phi thương mại, "as-is", theo Terms & Conditions Instacart (đã xác minh, nguồn: blog tech.instacart.com qua search) | 3 |
| 8 | [FreshRetailNet-50K](https://arxiv.org/abs/2505.16319v1) | c | nhu cầu bị kiểm duyệt do hết hàng | 50.000 chuỗi theo giờ, 898 cửa hàng, 18 thành phố | Dung lượng: không tìm thấy; ~4,85 triệu dòng (viewer HF qua search); nhóm 1M–10M bản ghi (thẻ HF qua search) | CC BY 4.0 (thẻ HF qua search, trên bản sao `sky5268`, đánh dấu trùng với `Dingdong-Inc`; chưa xác minh trên bản gốc) | 4 |
| 9 | [Amazon Last Mile Routing Challenge](https://registry.opendata.aws/amazon-last-mile-challenges) | c | tuyến giao hàng | 9.184 tuyến, 5 vùng đô thị Mỹ (2018) | Dung lượng: không tìm thấy (có thể đo bằng `aws s3 ls --summarize --human-readable --recursive s3://amazon-last-mile-challenges`); 6.112 tuyến train + 3.072 tuyến eval (search) | CC BY-NC 4.0 (theo bản sao DagsHub; chưa xác minh) | 2 |
| 10 | [Tmall/Tianchi repeat buyers (IJCAI-15)](https://ira.lib.polyu.edu.hk/handle/10397/103877?mode=full) | b | hành vi mua lặp | log 6 tháng đến "Double Eleven" | Dung lượng: không tìm thấy | chưa xác minh | 1 |
| 11 | [Food delivery time (Kaggle, Ấn Độ ~45.000 bản ghi; bản "multi-factor")](https://arxiv.org/pdf/2503.15177) | b | ETA giao đồ ăn | ~45.000 bản ghi, 19 feature (1 bộ) | Dung lượng: không tìm thấy; ~45.000 bản ghi | chưa xác minh | 2 |
| 12 | [SynDelay](https://arxiv.org/pdf/2509.05325) | c | delay giao hàng tổng hợp | từ nguồn ~180.000 dòng | Dung lượng: không tìm thấy; ~180.000 dòng | chưa xác minh | 2 |
| 13 | [GIFT-Eval](https://github.com/SalesforceAIResearch/gift-eval) | c | benchmark foundation models | 23 dataset, >144.000 chuỗi, 177 triệu điểm | Dung lượng tổng: không tìm thấy; file lẻ từ ~96 kB đến ~208 MB (HF qua search); nhóm 100K–1M (GiftEval) và 1M–10M (Pretrain) bản ghi | code Apache-2.0; dữ liệu `Salesforce/GiftEval` + `GiftEvalPretrain` trên HF (đã xác minh, nguồn: README GitHub); license dữ liệu chưa xác minh | 4 |
| 14 | [Monash Time Series Forecasting Archive](https://arxiv.org/abs/2105.06643v1) | c | kho benchmark | 20 (arXiv v1) hay 25 (NeurIPS D&B) dataset; bản Zenodo mới có thể nhiều hơn (chưa xác minh) | Dung lượng: không tìm thấy | CC BY 4.0 (theo loader HF, nguồn thứ cấp); Zenodo record 4656093 | 3 |
| 15 | [M4](https://github.com/Mcompetitions/M4-methods) | c | benchmark | thư mục `Dataset` (Train/Test/Info) trong GitHub `Mcompetitions/M4-methods`; số chuỗi không nêu ở README (chưa xác minh) | Dung lượng: không tìm thấy | license không ghi ở README; chưa xác minh | 3 |
| 16 | Foundation models: Chronos-2, TimesFM, Moirai (chưa có link) | d | mô hình, không phải dữ liệu | – | – | xem mục 4d | 3 |
| 17 | [DataCo Smart Supply Chain](https://data.mendeley.com/datasets/8gx2fvg2k6) | b | đơn TMĐT + giao hàng/chuỗi cung ứng | ~180K dòng đơn (README Mendeley: `DataCoSupplyChainDataset.csv`; số chính xác chưa xác minh) + `tokenized_access_logs.csv` (469.977 dòng, theo bản Kaggle mirror) | 180.519 dòng (Research Square qua search); `tokenized_access_logs.csv` 4,24 MB (Kaggle mirror qua search); CSV chính: dung lượng không tìm thấy | CC BY 4.0 (đã xác minh, nguồn: trang so sánh phiên bản Mendeley, v5) | 3 |
| 18 | [Nixtla `datasetsforecast` (loader)](https://github.com/Nixtla/datasetsforecast) | c | thư viện tải M3/M4/M5/Favorita/Hierarchical/Long-horizon/PHM2008 | – | – | loader MIT (đã xác minh, nguồn: GitHub); license từng dataset: xem THIRD_PARTY_LICENSES | 3 |

### (a) Dữ liệu Việt Nam (ghi chú)

Chủ dự án xác nhận không cần bộ dữ liệu tiếng Việt: dùng dữ liệu tiếng Anh/quốc tế (mục b, c) để chọn phương pháp, sau đó thay bằng dữ liệu mô phỏng/thật của chính hệ thống. Nếu sau này cần số liệu vĩ mô VN, nguồn có thể thử (**chưa xác minh**): Tổng cục Thống kê (doanh số bán lẻ theo tháng), Cục TMĐT và Kinh tế số, các báo cáo e-Conomy SEA. Dự án cần tự tạo dữ liệu mô phỏng theo mùa vụ Việt Nam (Tết, 11/11, 12/12, ngày đôi) — mục 5.

### (b) TMĐT / bán lẻ công khai

**1. M5 Forecasting – Accuracy (Walmart)**
- Link: [trang cuộc thi M5 (ĐH Nicosia)](https://www.unic.ac.cy/iff/research/forecasting/m5-competition/), [bài kết quả](https://pure.unic.ac.cy/en/publications/m5-accuracy-competition-results-findings-and-conclusions/), bản PDF [M5 accuracy competition](https://statmodeling.stat.columbia.edu/wp-content/uploads/2021/10/M5_accuracy_competition.pdf); trang Kaggle: chưa mở.
- 42.840 chuỗi doanh số đơn vị theo ngày của Walmart, 12 mức tổng hợp (mặt hàng, phòng, danh mục, cửa hàng, bang CA/TX/WI), dự báo 28 ngày (30.490 chuỗi mức thấp nhất); kèm giá, khuyến mãi, thứ trong tuần, sự kiện (Super Bowl, Lễ Tình nhân, Easter chính thống…). Phần lớn chuỗi ngắt quãng. Chạy 2/3–30/6/2020. Mốc chia chuẩn (một nguồn): huấn luyện đến 24/4/2016, validation tới 22/5/2016, test tới 19/6/2016. Metric chính WRMSSE; có track phân vị (pinball loss 9 mức). License: chưa xác minh.
- Cột (theo mô tả cuộc thi, chưa xác minh tên file): chuỗi bán theo ngày, lịch có sự kiện, giá theo tuần/cửa hàng.
- Ngôn ngữ: dữ liệu số (doanh số, giá, lịch) nên giữ nguyên.
- Ánh xạ: `item_id` ↔ `product_id`; `dept/cat` ↔ `category_id`; `store` ↔ `store_id`; `sell_price` ↔ `unit_price_minor`; `snap/event` ↔ lịch Tết/ngày đôi.
- ★5. Hạn chế: dữ liệu bán lẻ vật lý Mỹ, không có kênh online/tracking; không có hết hàng rõ ràng.

**2. Corporación Favorita**
- Link: [trang Kaggle](https://www.kaggle.com/c/favorita-grocery-sales-forecasting) (theo bài; chưa mở), [Nixtla datasetsforecast](https://nixtlaverse.nixtla.io/datasetsforecast/favorita.html), [bài WaveNet](https://ar5iv.labs.arxiv.org/html/1803.04037) (hạng 2). Ecuador, ~4.400 mặt hàng, 54 cửa hàng, từ 1/2013 đến giữa 8/2017; có cờ khuyến mãi (`onpromotion`), giao dịch, giá dầu, cửa hàng, ngày lễ. Train 125.497.040 dòng, test 3.370.464 (một nguồn). Bản Nixtla: 371.312 chuỗi có phân cấp địa lý. Dùng: cờ khuyến mãi, ngày lễ, chuỗi dài — tốt cho chia theo thời gian và hierarchical. ★4. Hạn chế: Ecuador, tạp hoá; license/điều khoản chưa xác minh; tải qua Kaggle CLI.

**3. Rossmann Store Sales**
- Dự báo doanh số ngày tới 6 tuần cho 1.115 cửa hàng drugstore Đức (huấn luyện 1/1/2013–31/7/2015; test 8–9/2015; có khuyến mãi, ngày lễ, thông tin đối thủ; một số cửa hàng đóng cửa sửa chữa). Nguồn mô tả: [notebook mlcourse.ai](https://nbviewer.org/github/Yorko/mlcourse.ai/blob/main/jupyter_english/projects_indiv/Rossman_store_sales_Syrovatskiy_Ilya.ipynb), [NYC Data Science](https://nycdatascience.com/blog/student-works/gradient-boosters-and-the-rossmann-project). ★3. Hạn chế: cấp cửa hàng, không phải sản phẩm; license chưa xác minh.

**4. Store Item Demand Forecasting Challenge**
- 10 cửa hàng × 50 mặt hàng, 1/2013–12/2017, dự báo 3 tháng đầu 2018; metric SMAPE; cuộc thi playground không huy chương ([tổng quan](https://book.st-hakky.com/en/data-science/store-item-demand-forecast-overview)). Chỉ có `date, store, item, sales`. Dùng: kiểm tra nhanh pipeline. ★2. Hạn chế: sạch/tổng hợp, không phản ánh bán lẻ thực.

**5. Online Retail II (UCI)**
- Link: [UCI 502](https://archive.ics.uci.edu/dataset/502/online+retail+ii). 1.067.371 dòng giao dịch của một nhà bán lẻ online ở UK, 12/2009–12/2011, **CC BY 4.0** (trang UCI; bản OpenML ghi CC0 — dùng theo UCI); trích dẫn Chen, D. (2012), DOI 10.24432/C5CG6D. Cột: mã hoá đơn, mã sản phẩm, mô tả, số lượng, thời điểm, đơn giá, mã khách, quốc gia (tên cột chính xác: chưa xác minh trong phiên). Dùng: dữ liệu giao dịch TMĐT thật gần nhất (có hoá đơn trả hàng âm); ánh xạ `Invoice` ↔ `order_id`, `StockCode` ↔ `product_id`, `Quantity` ↔ `qty`, `Price` ↔ `unit_price_minor`. ★4. Hạn chế: nhà bán sỉ/quà tặng, nhiều SKU thưa; không có tồn kho/giao hàng.

**7. Instacart Market Basket**
- >3 triệu đơn, >200.000 người dùng (ẩn danh), có cờ `reordered`; nguồn: cuộc thi Kaggle 2017 ([một repo mô tả](https://github.com/archd3sai/Instacart-Market-Basket-Analysis)). License: chưa xác minh (một thư viện ghi "research use" thứ cấp). Dùng: chu kỳ mua lặp (hàng tiêu dùng) -> phát hiện khi nào khách cần mua lại và dự báo cầu lặp. ★3. Hạn chế: không có tồn kho/giá/thời gian giao.

**8. Tmall (IJCAI-15 repeat buyers) / Tianchi**
- Log mua sắm ẩn danh 6 tháng trước Double Eleven; tỉ lệ mua lặp ~6,1% (theo Tmall trích trong bài); AUC tốt nhất một luận văn 0,6521. Dùng: ít cho dự báo cầu (phục vụ scope 13). Không tìm thấy bộ nhu cầu chuỗi thời gian của Alibaba hay tồn kho JD.com công khai trong phiên. ★1.

### (c) Benchmark / bài báo học thuật

**9. FreshRetailNet-50K**
- Link: [arXiv 2505.16319](https://arxiv.org/abs/2505.16319v1) (xem thêm [bản tổng hợp](https://sotaverified.org/papers/freshretailnet-50k-a-stockout-annotated)). 50.000 chuỗi cửa hàng–sản phẩm bán hàng tươi theo giờ, 898 cửa hàng/18 thành phố, **có trạng thái tồn từng giờ** để tách "không có cầu" khỏi "hết hàng". Dùng: duy nhất bộ công khai ánh xạ trực tiếp ý tưởng `fact_inventory.level` -> nhu cầu bị kiểm duyệt. ★4. Hạn chế: hàng tươi Trung Quốc (chưa xác minh chi tiết); dữ liệu số theo giờ, không có văn bản tự nhiên nên không bị ràng buộc ngôn ngữ; license CC BY 4.0 theo thẻ HF qua search; chưa mở được trang tải. 898 cửa hàng/18 thành phố, 865 SKU hàng tươi, chuỗi 90 ngày theo giờ (thẻ HF qua search).

**10. Amazon Last Mile Routing Research Challenge**
- Link: [AWS Open Data](https://registry.opendata.aws/amazon-last-mile-challenges), [MIT challenge](https://routingchallenge.mit.edu/routing-challenge-dataset-now-publicly-available), [PDF mô tả dữ liệu](https://cdn.amazon.science/1f/02/349d1f594f8d8f97b21ebc02c7b2/2021-amazon-last-mile-routing-research-challenge-data-set.pdf). Mức tuyến/điểm dừng/gói của 9.184 tuyến tài xế Amazon năm 2018 tại 5 vùng đô thị Mỹ, ID đã tái sinh ngẫu nhiên, vị trí bị làm mờ. License: bản sao DagsHub ghi CC BY-NC 4.0 và bản quyền Amazon — **chưa xác minh trên trang chính thức**; phi thương mại. Trích dẫn: Merchán và cộng sự, Transportation Science 2022, DOI 10.1287/trsc.2022.1173. ★2 (ta chưa tự vận hành giao hàng; dùng khi có module tuyến).

**11. Dữ liệu ETA giao đồ ăn / bưu kiện**
- Kaggle giao đồ ăn Ấn Độ ~45.000 bản ghi/19 feature, mục tiêu phút giao ([arXiv 2503.15177](https://arxiv.org/pdf/2503.15177)); bộ "multi-factor" ([gts.ai](https://gts.ai/dataset-download/food-delivery-times-a-multi-factor-dataset/)) nguồn gốc không rõ; [SynDelay arXiv 2509.05325](https://arxiv.org/pdf/2509.05325) tổng hợp từ ~180.000 dòng thật với nhãn early/on-time/delayed. Cainiao DeepETA ([AAAI](https://ojs.aaai.org/index.php/AAAI/article/view/3856/3734)) không công bố dữ liệu (theo phiên). Không tìm thấy bộ dữ liệu bưu kiện DHL/JD Logistics công khai (không tra cụ thể). ★2. Hạn chế: thời gian giao đồ ăn (phút) khác bưu kiện (ngày).

**12. GIFT-Eval**
- Link: [arXiv 2410.10393](https://arxiv.org/pdf/2410.10393), GitHub `SalesforceAIResearch/gift-eval`, HF `Salesforce/GiftEval`, [blog](https://www.salesforce.com/blog/gift-eval-time-series-benchmark/). 23 dataset (blog ghi 28), >144.000 chuỗi, 177 triệu điểm, 7 miền, 10 tần suất; có tập pretrain không rò rỉ (~230 tỉ điểm). Dùng: giao thức so sánh foundation model; ★4 (tham chiếu).

**13. Monash Time Series Forecasting Archive**
- [NeurIPS 2021 D&B](https://datasets-benchmarks-proceedings.neurips.cc/paper/2021/hash/eddea82ad2755b24c4e168c5fc2ebd40-Abstract-round2.html), [arXiv 2105.06643](https://arxiv.org/abs/2105.06643v1): kho dataset chuẩn hoá (20 hoặc 25 dataset tuỳ nguồn); các bản thêm trên Zenodo (Vehicle Trips, Temperature Rain, Oikolab Weather…). Dùng: chọn thuật toán nền tảng. ★3.

**14. M4.** Dữ liệu nằm trong thư mục `Dataset` (Train, Test và file Info có metadata từng chuỗi, gồm Frequency để tính MASE) của repo GitHub [Mcompetitions/M4-methods](https://github.com/Mcompetitions/M4-methods) (đã xác minh, nguồn: README). Số chuỗi và license: README không nêu (chưa xác minh). Cũng tải được qua `datasetsforecast`. ★3 (benchmark nền).

**16. DataCo Smart Supply Chain.** [Mendeley Data 8gx2fvg2k6](https://data.mendeley.com/datasets/8gx2fvg2k6): đơn hàng bán Clothing/Sports/Electronic Supplies kèm thông tin giao hàng và log truy cập; **CC BY 4.0** (đã xác minh, nguồn: trang so sánh phiên bản Mendeley, DOI 10.17632/8gx2fvg2k6.5, 12/3/2019). Khoảng 180K dòng (các repo ghi "180K+", số chính xác chưa xác minh). Dùng: thử nghiệm dự đoán trễ giao/rủi ro giao hàng, join với log truy cập. ★3. Hạn chế: dữ liệu tổng hợp/công ty cung cấp, chất lượng nhãn trễ cần kiểm tra.

**17. Nixtla `datasetsforecast`.** [GitHub](https://github.com/Nixtla/datasetsforecast): loader (MIT) cho Favorita, Hierarchical, Longhorizon, M3, M4, M5, PHM2008 (đã xác minh, nguồn: README). Dùng để nạp nhanh các benchmark trên cùng định dạng. ★3.

**15. Tài liệu về tồn kho/phân cấp.** Spiliotis và cộng sự (IJPE 2021): phương pháp ước lượng thực nghiệm đơn giản cho phân vị cho kết quả tồn kho tốt hơn mô hình phức tạp; hầu hết bài WRMSSE/pinball không đo chi phí tồn kho -> cần mô phỏng riêng. Bài [arXiv 2606.26774](https://arxiv.org/pdf/2606.26774) (preprint) báo cáo top-down xác suất end-to-end tốt trên M5. Bộ dữ liệu Dunnhumby "The Complete Journey" và Walmart Recruiting Store Sales: **không tìm thấy trang trong phiên**.

### (d) Mô hình nền tảng chuỗi thời gian (tham chiếu, không phải dữ liệu)

- **Chronos-2 (Amazon):** nhiều nguồn thứ cấp ghi Apache 2.0, dùng được thương mại ([so sánh](https://www.beri.net/article/timesfm-vs-chronos-vs-moirai-time-series-foundation-model-licence-bigquery-sagemaker-cost-2026), [KDnuggets](https://www.kdnuggets.com/5-time-series-foundation-models-you-are-missing-out-on)).
- **TimesFM (Google):** các nguồn mâu thuẫn — TimesFM 2.5 Apache 2.0; phiên bản 3.0 bị mô tả là phi thương mại/chỉ production trong BigQuery. **Phải đọc model card chính thức.**
- **Moirai (Salesforce):** Moirai 2.0 được mô tả là phi thương mại (CC BY-NC 4.0).
- ★3 như baseline zero-shot; kiểm tra license trước khi triển khai.

## 5. Dữ liệu tự tạo / synthetic từ chính hệ thống

1. **Bộ sinh nhu cầu có mùa vụ VN:** mỗi sản phẩm có mức nền (Poisson/Negative Binomial), độ dốc xu hướng, mùa theo tuần (cuối tuần), theo tháng, **Tết Nguyên đán** (tăng trước, rơi mạnh sau), ngày đôi (9/9, 11/11, 12/12), lương, thời tiết; thưa (nhiều ngày bằng 0) cho 60–70% sản phẩm; khuyến mãi làm tăng cầu có độ co giãn giá. Tham số hiệu chuẩn theo M5 (phân bố ngắt quãng) và Online Retail II.
2. **Tồn kho kiểm duyệt:** mô phỏng nhập hàng và giữ chỗ giống thật (`inventory`, `reserved`, `sold`, ledger); khi `inventory = 0` doanh số quan sát = 0 dù cầu thật > 0 -> giữ **cầu thật** ở cột riêng làm nhãn vàng (có được nhờ synthetic) để đo mô hình xử lý stockout — đây là lợi thế của synthetic.
3. **Sinh qua luồng thật của hệ thống:** chạy generator gọi API/outbox thật (đặt đơn, ship, deliver) để `fact_*` được lấp qua đúng pipeline (kiểm thử toàn tuyến); ghi `synthetic=true`; chỉ dùng với `SEED_DEMO_DATA=true`.
4. **ETA:** sinh `shipped_at -> delivered_at` theo phân bố log-normal phụ thuộc cặp (tỉnh kho, tỉnh nhận), `carrier`, ngày lễ/cuối tuần, tải đột biến 11/11; thêm hiệu ứng `AUTO_DELIVER_DAYS` (7) để tái hiện nhãn nhiễu (buyer không bấm xác nhận); giữ cột `true_delivery_time` làm vàng.
5. **Chia train/valid/test theo thời gian:** rolling-origin; hold-out ít nhất một chu kỳ mùa vụ lớn (Tết, 11/11) vào test; thêm nhóm "sản phẩm mới" (cold start) chỉ xuất hiện ở test; không xáo trộn ngẫu nhiên. Với ETA: chia theo `shipped_at`, tránh dùng feature chỉ biết sau khi giao.
6. **Dữ liệu thật dần dần:** khi tích luỹ ≥ 6–12 tháng, thay dần synthetic; đo gap giữa kết quả synthetic và thật.

## 6. Lộ trình dùng dữ liệu đề xuất

1. **Giai đoạn 0:** dựng pipeline đánh giá (rolling-origin, WRMSSE/pinball) trên M5 và Favorita; thử seasonal naive, Croston, LightGBM, và một foundation model (Chronos) làm baseline.
2. **Giai đoạn 1:** thử logic stockout/censored demand trên FreshRetailNet-50K (nếu tải được) và trên synthetic có cầu thật.
3. **Giai đoạn 2:** áp lên `fact_order_items` + `fact_inventory` của hệ thống (demo rồi thật): cảnh báo `days_of_cover`, gợi ý `reorder_qty`; mô phỏng chính sách để đo chi phí.
4. **Giai đoạn 3 (ETA):** baseline theo trung vị tuyến trên DataCo/dữ liệu synthetic (mục 5) để chọn feature/mô hình, rồi huấn luyện lại trên `orders.shipped_at/delivered_at` thật; xử lý nhãn `AUTO_DELIVER_DAYS`.
5. **Giai đoạn 4:** tín hiệu sớm từ `events_raw` (view/cart) cải thiện dự báo ngắn hạn; tuyến giao (Amazon Last Mile) chỉ khi có nhu cầu.

## 7. Rủi ro, license, thiên lệch, lưu ý pháp lý/PII

- **License hạn chế thương mại:** Amazon Last Mile (NC theo bản sao), TimesFM 3.0/Moirai (non-commercial); M5/Favorita/Instacart/TalkingData là dữ liệu cuộc thi — đọc điều khoản. Online Retail II (CC BY 4.0) rõ ràng nhất.
- **Domain gap:** M5/Favorita/Rossmann là cửa hàng vật lý; ETA ở Việt Nam phụ thuộc hãng vận chuyển, ngoại thành/nông thôn, mùa mưa, Tết — ngoài phân bố các bộ công khai.
- **Stockout bias:** huấn luyện trên doanh số quan sát khi hết hàng sẽ dự báo thấp (dự báo "lượng bán" thay vì "nhu cầu").
- **Nhãn ETA nhiễu:** `delivered_at` có thể là xác nhận tự động; thiếu GPS/mốc hãng vận chuyển.
- **Hierarchical/tổng hợp:** WRMSSE bị chi phối bởi tầng trên; shop nhỏ có series thưa nên cần chỉ số riêng.
- **Cold start & thay đổi giá/khuyến mãi:** dự báo gãy khi seller đổi giá/ngưng bán (`status`).
- **PII:** dữ liệu chuỗi thời gian tổng hợp theo sản phẩm ít PII; riêng ETA dùng `shipping_address` -> chỉ dùng cấp phường/quận/tỉnh, không lưu địa chỉ chi tiết trong tập huấn luyện.
- **Hành động tự động:** không tự đặt hàng nhập thay seller; chỉ gợi ý, ghi `inventory_ledgers` qua `adjust` có lý do như hiện nay.

## 8. Nguồn tham khảo (URL)

- M5: https://www.unic.ac.cy/iff/research/forecasting/m5-competition/ ; https://pure.unic.ac.cy/en/publications/m5-accuracy-competition-results-findings-and-conclusions/ ; https://statmodeling.stat.columbia.edu/wp-content/uploads/2021/10/M5_accuracy_competition.pdf ; https://github.com/rruss2/M5_competition
- Favorita: https://nixtlaverse.nixtla.io/datasetsforecast/favorita.html ; https://ar5iv.labs.arxiv.org/html/1803.04037 ; https://forums.fast.ai/t/corporacion-favorita-grocery-sales-forecasting/8359
- Rossmann: https://nbviewer.org/github/Yorko/mlcourse.ai/blob/main/jupyter_english/projects_indiv/Rossman_store_sales_Syrovatskiy_Ilya.ipynb ; https://nycdatascience.com/blog/student-works/gradient-boosters-and-the-rossmann-project
- Store Item: https://book.st-hakky.com/en/data-science/store-item-demand-forecast-overview
- Online Retail II: https://archive.ics.uci.edu/dataset/502/online+retail+ii
- Instacart: https://github.com/archd3sai/Instacart-Market-Basket-Analysis
- FreshRetailNet-50K: https://arxiv.org/abs/2505.16319v1
- Amazon Last Mile: https://registry.opendata.aws/amazon-last-mile-challenges ; https://routingchallenge.mit.edu/routing-challenge-dataset-now-publicly-available ; https://cdn.amazon.science/1f/02/349d1f594f8d8f97b21ebc02c7b2/2021-amazon-last-mile-routing-research-challenge-data-set.pdf
- ETA: https://arxiv.org/pdf/2503.15177 ; https://arxiv.org/pdf/2509.05325 ; https://ojs.aaai.org/index.php/AAAI/article/view/3856/3734
- GIFT-Eval: https://arxiv.org/pdf/2410.10393 ; https://www.salesforce.com/blog/gift-eval-time-series-benchmark/
- Monash: https://arxiv.org/abs/2105.06643v1 ; https://datasets-benchmarks-proceedings.neurips.cc/paper/2021/hash/eddea82ad2755b24c4e168c5fc2ebd40-Abstract-round2.html
- Foundation models: https://www.beri.net/article/timesfm-vs-chronos-vs-moirai-time-series-foundation-model-licence-bigquery-sagemaker-cost-2026 ; https://www.kdnuggets.com/5-time-series-foundation-models-you-are-missing-out-on
- Phân cấp/xác suất: https://arxiv.org/pdf/2606.26774 ; https://arxiv.org/pdf/2103.08250
- Tmall repeat buyers (thứ cấp): https://ira.lib.polyu.edu.hk/handle/10397/103877?mode=full
- Chưa tra được: Dunnhumby, Walmart Recruiting; license chính thức trên Kaggle của M5/Favorita/Rossmann (Kaggle không mở được); không cần dataset bán lẻ Việt Nam (xem mục (a)). Thêm nguồn: [DataCo](https://data.mendeley.com/datasets/8gx2fvg2k6), [M4-methods](https://github.com/Mcompetitions/M4-methods), [datasetsforecast](https://github.com/Nixtla/datasetsforecast), [Instacart blog](https://tech.instacart.com/3-million-instacart-orders-open-sourced-d40d29ead6f2).
