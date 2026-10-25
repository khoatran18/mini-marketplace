# Phân tích khách hàng: phân khúc, CLV, churn, uplift/khuyến mãi, định giá

> Phân khúc khách hàng (RFM), dự đoán CLV, churn/repeat buyer, uplift cho khuyến mãi/voucher và tối ưu giá. **Trạng thái: chưa triển khai trong hệ thống; đây là tài liệu nghiên cứu dữ liệu.** Ngày tra cứu: 2026-10-25.
>
> Ghi chú phương pháp: WebFetch không mở được Kaggle/UCI/HF trực tiếp (DNS); thông tin lấy từ WebSearch (trang UCI, TFDS, scikit-uplift docs, GitHub). Số liệu nào không kiểm chứng ghi "chưa xác minh". Phiên bổ sung (2026-10-25): xác minh thêm bằng WebSearch (nguồn ghi "kết quả search"); chủ dự án xác nhận dùng bộ tiếng Anh/quốc tế là được. Quy tắc ngôn ngữ (2026-10-26): chỉ giữ bộ có nội dung văn bản bằng tiếng Anh hoặc tiếng Việt, hoặc không có văn bản (số/ẩn danh); đã loại Olist (tiếng Bồ Đào Nha). Cột "Quy mô / dung lượng" ghi số mẫu và dung lượng tải kèm nguồn (README/UCI/TFDS/docs qua search); "dung lượng: không tìm thấy" nghĩa là không thấy trong kết quả search.

| Liên quan | File |
|---|---|
| Gợi ý sản phẩm | [01_RECOMMENDATION_CLICK.md](01_RECOMMENDATION_CLICK.md) |
| Text-to-SQL / analytics QA | [04_ANALYTICS_QA_TEXT2SQL.md](04_ANALYTICS_QA_TEXT2SQL.md) |
| Gian lận/rủi ro | [08_FRAUD_ANOMALY_RISK.md](08_FRAUD_ANOMALY_RISK.md) |
| Dự báo nhu cầu/logistics | [09_DEMAND_FORECAST_LOGISTICS.md](09_DEMAND_FORECAST_LOGISTICS.md) |
| Eval agent (tool phân tích cho seller/admin) | [16_AGENT_TOOL_USE_EVAL.md](16_AGENT_TOOL_USE_EVAL.md) |

## 1. Mục tiêu và vị trí trong hệ thống

- **Seller**: phân khúc người mua của shop mình (khách mới, trung thành, sắp rời bỏ), gợi ý voucher/giá cho từng nhóm, tác động của khuyến mãi.
- **Admin**: churn và CLV toàn sàn, hiệu quả chiến dịch voucher, nhạy cảm giá theo danh mục.
- **Buyer**: voucher cá nhân hoá (chỉ khi có lợi ích rõ và kiểm soát công bằng).
- **Input/Output**: lịch sử mua + hành vi → điểm churn, CLV 12 tháng, nhóm phân khúc, uplift từng khách, giá/độ co giãn đề xuất.
- **Dữ liệu của hệ thống**: ClickHouse `fact_order_items` (dòng đơn hàng: sản phẩm, shop, tiền) và `fact_order_status` (vòng đời trạng thái: PAID/DELIVERED/huỷ/hoàn), `events_raw` (view, add-to-cart, tìm kiếm), bảng đơn/giỏ trong order-service, `domain_events` từ payment (hoàn tiền). Quy ước doanh thu của dự án: subtotal đơn PAID (online) / DELIVERED (COD) trừ hoàn tiền — nhãn churn/CLV phải dùng cùng định nghĩa. Tiền VND, múi giờ Asia/Ho_Chi_Minh.

## 2. Hướng làm đề xuất (baseline → nâng cao)

1. **Baseline**: RFM + K-means/quantile segment; thống kê mô tả (tần suất mua lại, khoảng cách giữa các đơn).
2. **CLV**: mô hình xác suất BG/NBD + Gamma-Gamma (thư viện Lifetimes, theo bài tham khảo tìm được) → học máy (GBM) trên đặc trưng RFM + hành vi.
3. **Churn/repeat buyer**: định nghĩa churn theo khoảng không mua (không dùng nhãn thuê bao như Telco); GBM, calibration; survival analysis.
4. **Uplift**: T-learner/X-learner, causal forest; bắt buộc có nhóm đối chứng ngẫu nhiên (A/B) cho voucher.
5. **Định giá**: ước lượng độ co giãn (log-log, mô hình nhân quả), sau đó bandit/tối ưu có ràng buộc (biên lợi nhuận, giá sàn, ngân sách voucher). Chỉ gợi ý cho seller; seller tự quyết (tool ghi qua `action_proposal`).

## 3. Metric đánh giá

- Churn/repeat: AUC, PR-AUC, lift@decile, Brier/calibration.
- CLV: MAE/RMSE, Spearman, độ phủ top-decile giá trị, sai số tổng doanh thu theo nhóm.
- Phân khúc: silhouette (tham khảo), ổn định theo thời gian, khả năng hành động (có chiến dịch khác nhau).
- Uplift: **Qini coefficient / AUUC**, uplift@k, policy value (off-policy).
- Giá: sai số dự báo cầu, doanh thu/lợi nhuận mô phỏng, độ an toàn (không tụt biên dưới ngưỡng).
- Lưu ý từ tổng hợp 2026 (UpliftBench, kết quả search): trên Hillstrom, X5, MegaFon, khác biệt giữa các mô hình nằm trong nhiễu giữa fold; đừng kết luận từ một bộ nhỏ.

## 4. Bộ dữ liệu

### Bảng tổng hợp

| # | Bộ dữ liệu | Nhóm | Quy mô / dung lượng (theo nguồn đã tra) | License | Phù hợp |
|---|---|---|---|---|---|
| 1 | [Online Retail II (UCI)](https://archive.ics.uci.edu/dataset/502/online+retail+ii) | b | 1,067,371 dòng (2009-12 đến 2011-12); dung lượng: 43.5 MB, file online_retail_II.xlsx (trang UCI qua search) | CC BY 4.0 | ★★★★★ |
| 2 | [Dunnhumby The Complete Journey](https://dunnhumby.com/sourcefiles) | b | ~2,500 hộ, 2 năm (bản R: 2,469 hộ, 1 năm); số dòng giao dịch: "hàng triệu" (Mendeley qua search); dung lượng: không tìm thấy (trang dunnhumby chỉ cảnh báo file lớn tải 5+ phút) | Chưa xác minh gốc; gói R CC0 | ★★★★ |
| 3 | [Tmall / IJCAI-15 Repeat Buyers](https://groups.google.com/g/resys/c/oK8DzKHho1c) | b | Log 6 tháng đến "Double 11"; số liệu các paper không thống nhất (một paper: 424,170 user, 1,090,390 item, 54,925,330 tương tác; paper khác nêu số khác, chưa xác minh); tỉ lệ mua lại ~6.1% (theo 1 paper); dung lượng: không tìm thấy | Chưa xác minh | ★★★★ |
| 4 | [Instacart Market Basket](https://www.instacart.com/datasets/grocery-shopping-2017) | b | 3,346,083 đơn, 206,209 user, 49,685 sản phẩm, ~33.8M dòng sản phẩm-trong-đơn (BigML qua search), 6 file CSV; dung lượng: không tìm thấy | Điều khoản Instacart (chưa xác minh) | ★★★ |
| 5 | [Criteo Uplift](https://ailab.criteo.com/criteo-uplift-prediction-dataset/) | c | 25,309,483 dòng (TFDS); dung lượng: 459 MB nén CSV (TFDS qua search); bản ~13M dòng (Fabric/Kaggle mirror); scikit-uplift có tham số `percent10` (mẫu 10%); 12 đặc trưng f0-f11 + treatment + visit + conversion | Điều khoản Criteo Research (chưa xác minh) | ★★★★ |
| 6 | [Hillstrom MineThatData](https://blog.minethatdata.com/2008/03/minethatdata-e-mail-analytics-and-data.html) | c | 64,000 khách, 3 nhóm email; dung lượng: 433 KB nén / 4,935 KB giải nén CSV (docs scikit-uplift qua search) | TFDS; điều khoản gốc chưa xác minh | ★★★★ |
| 7 | [X5 RetailHero](https://scikit-uplift.readthedocs.io/en/latest/api/datasets/fetch_x5.html) | c/d | clients, train (treatment_flg, target), purchases; số khách/dòng và dung lượng: không tìm thấy (docs scikit-uplift không nêu) | Chưa xác minh | ★★★★ |
| 8 | [Acquire Valued Shoppers (Kaggle)](https://github.com/auduno/Kaggle-Acquire-Valued-Shoppers-Challenge) | b | ~350M dòng, >300K khách, 22 GB (README GitHub lời giải) | Luật cuộc thi | ★★★ |
| 9 | [Retailrocket e-commerce](https://www.kaggle.com/datasets/retailrocket/ecommerce-dataset) | b | 2,756,101 sự kiện (2,664,312 view, 69,332 addtocart, 22,457 transaction), 1,407,580 visitor, 417,053 item trong file thuộc tính (nguồn thứ cấp qua search); dung lượng: không tìm thấy | Chưa xác minh | ★★★ |
| 10 | [IBM Telco Customer Churn](https://data.mendeley.com/datasets/phsxg9ssrf) | c | 7,043 khách, 21 cột; dung lượng: ~550 KB (bản CSV re-upload trên HF, qua search, xấp xỉ) | IBM sample (chưa xác minh) | ★★ |
| 11 | [Santander Customer Transaction](https://arxiv.org/pdf/2406.19380) | c | 200,000 dòng, 200 đặc trưng ẩn danh; dung lượng: không tìm thấy | Luật cuộc thi | ★ |
| 12 | [Open Bandit Dataset (ZOZOTOWN)](https://arxiv.org/abs/2008.07146) | c | ~26M dòng impression (research.zozo.com qua search; cộng bảng paper ~26.7M); 80 item (All), 34 (Men), 46 (Women); dung lượng: không tìm thấy | CC BY 4.0 nhưng paper ghi chỉ cho nghiên cứu phi thương mại (nguồn: paper/research.zozo.com qua search) | ★★★ |
| 13 | [Kaggle Retail Price Optimization](https://baselight.app/u/kaggle/dataset/suddharshan_retail_price_optimization) | b | Số dòng chưa xác minh; dung lượng: không tìm thấy (bộ nhỏ) | Chưa xác minh | ★★ |
| 14 | [ViEcomRec (Việt Nam)](https://doras.dcu.ie/29693/1/viecomrec.pdf) | a | 369,099 tương tác, sản phẩm sữa rửa mặt (paper); dung lượng: không tìm thấy | Chưa xác minh | ★★ |
| 15 | [REES46 / Kaggle "E-commerce behavior data from multi category store"](https://www.kaggle.com/datasets/mkechinov/ecommerce-behavior-data-from-multi-category-store) | b | Log sự kiện view/cart/remove_from_cart/purchase; ~9 GB cho 2 file Oct+Nov 2019 (hướng dẫn YDB qua search); tháng 10/2019 ~42M sự kiện (một phân tích Zerve qua search), tổng số sự kiện chưa xác minh | Chưa xác minh (một nhận xét ghi phi thương mại; không phải văn bản license) | ★★★★ |
| 16 | [M5 Forecasting (Walmart)](https://www.unic.ac.cy/unic-walmart-and-kaggle-launch-m5-forecasting-competition/) | b | 42,840 chuỗi doanh số phân cấp (30,490 ở mức thấp nhất) x 1,941 ngày, có giá bán, khuyến mãi, sự kiện; dung lượng: không tìm thấy | Chưa xác minh (Kaggle/M-competitions) | ★★★ (giá, nhu cầu) |

### (a) Tiếng Việt / Việt Nam
- Ghi chú trung tính: các bộ churn/CLV chính ở mục (b)/(c) là tiếng Anh/quốc tế. Bộ Việt Nam công khai gần nhất trong các nguồn đã tra: **ViEcomRec** (https://doras.dcu.ie/29693/1/viecomrec.pdf): dữ liệu sàn TMĐT VN, 369,099 tương tác user-item trong một ngành hàng; thiên về gợi ý, thời gian/lặp mua cần kiểm tra (chưa xác minh); dung lượng: không tìm thấy. Một bài báo churn B2B Shopee Việt Nam (https://nghiencuu.tapchikinhtetaichinh.vn/du-bao-hanh-vi-roi-bo-khach-hang-b2b-bang-hoc-may-trong-boi-canh-thuong-mai-dien-tu-tai-viet-nam-159927.html) báo Logistic Regression accuracy 84%, F1 0.85; dữ liệu không công khai (theo search). ★2.

### (b) TMĐT / miền liên quan

**Online Retail II (UCI)** — https://archive.ics.uci.edu/dataset/502/online+retail+ii
- Nhà bán lẻ online Anh, giao dịch 2009-12-01 đến 2011-12-09, 1,067,371 dòng, CC BY 4.0 (bản sao OpenML ghi CC0, hãy trích bản UCI; DOI 10.24432/C5CG6D). Cột (chuẩn): Invoice, StockCode, Description, Quantity, InvoiceDate, Price, Customer ID, Country (chưa xác minh trong phiên).
- **Quy mô / dung lượng**: 1,067,371 dòng; file online_retail_II.xlsx 43.5 MB (UCI qua search). Dùng: RFM, BG/NBD, CLV. Hạn chế: khách B2B/wholesale, hàng trả có Invoice "C", nhiều dòng không có Customer ID (một paper giữ 77.2% sau lọc), đơn vị GBP. ★5.

**Dunnhumby The Complete Journey** — https://dunnhumby.com/sourcefiles
- ~2,500 hộ thường xuyên mua, 2 năm giao dịch cửa hàng, có nhân khẩu học và lịch sử tiếp thị trực tiếp/chiến dịch cho một số hộ; gói R `completejourney` (CC0) dùng 2,469 hộ/1 năm. **Quy mô / dung lượng**: số dòng 'hàng triệu' (Mendeley qua search); dung lượng tải: không tìm thấy. Dùng: phân khúc, đáp ứng khuyến mãi/coupon. Hạn chế: siêu thị truyền thống, không phải TMĐT. ★4.

**Tmall IJCAI-15 Repeat Buyers** — thông báo https://groups.google.com/g/resys/c/oK8DzKHho1c (đã thấy qua search); dữ liệu Tianchi
- Log mua/giỏ/yêu thích/xem 6 tháng quanh "Double 11", dự đoán người mua mua lại sau khuyến mãi; mất cân bằng (6.1% mua lại theo một paper). **Quy mô / dung lượng**: xem bảng (số liệu các paper không thống nhất); dung lượng: không tìm thấy. Truy cập Tianchi có thể cần đăng ký (chưa xác minh). ★4.

**Instacart** — https://www.instacart.com/datasets/grocery-shopping-2017 (qua bản ghi TIB); 3.4M đơn, 206K user, mỗi user 4-100 đơn, 134 aisle/21 department, `days_since_prior_order`. **Quy mô / dung lượng**: ~33.8M dòng sản phẩm-trong-đơn (BigML qua search); dung lượng: không tìm thấy. Dùng: chu kỳ mua lại. Hạn chế: tạp hoá, không giá/ngày tuyệt đối. ★3.

**Acquire Valued Shoppers Challenge** — https://github.com/auduno/Kaggle-Acquire-Valued-Shoppers-Challenge (lời giải; dữ liệu trên Kaggle): dự đoán ai mua lại sau coupon hoàn tiền; **Quy mô / dung lượng**: 22 GB, ~350M dòng, >300K khách (README GitHub). AUC công bố trong một nghiên cứu 0.60936 (chứng tỏ bài toán khó). Quá lớn cho dự án; có thể lấy mẫu. ★3.

**Retailrocket** — kaggle.com/retailrocket/ecommerce-dataset (qua search): log e-commerce thật, giá trị băm; 2,756,101 sự kiện gồm 2,664,312 view, 69,332 addtocart, 22,457 transaction từ 1,407,580 visitor, 417,053 item trong file thuộc tính; thời gian ghi 4-4.5 tháng tuỳ nguồn (nguồn thứ cấp: bài báo/repo dùng lại, qua search); dung lượng: không tìm thấy; license chưa xác minh. Hợp với funnel hơn là CLV. ★3.

**REES46 "E-commerce behavior data from multi category store"** — https://www.kaggle.com/mkechinov/ecommerce-behavior-data-from-multi-category-store (người tải: Michael Kechinov, dữ liệu từ nền tảng REES46): log sự kiện view/cart/remove_from_cart/purchase kèm event time, product, category, brand, price, user, session; file 2019-Nov.csv ~9 GB (theo hướng dẫn nhập của YDB, qua search); tháng 10/2019 ~42M sự kiện (phân tích Zerve qua search). Tổng sự kiện và license chưa xác minh (không thấy văn bản license trong kết quả). Có brand, price và session nên dùng được cho RFM, funnel, phân khúc; thiếu thông tin hoàn trả. ★4.

**M5 Forecasting (Walmart)** — 42,840 chuỗi doanh số phân cấp, 30,490 chuỗi mức thấp nhất, kèm giá bán, khuyến mãi, ngày trong tuần, sự kiện đặc biệt (nguồn: kết quả search về cuộc thi M5). Dùng cho dự báo cầu và ước lượng độ co giãn giá theo giá bán thay đổi; không có khách hàng nên không dùng cho CLV. **Quy mô / dung lượng**: 30,490 chuỗi x 1,941 ngày; dung lượng: không tìm thấy. License chưa xác minh. ★3.

**Kaggle Retail Price Optimization** (qua mirror https://baselight.app/u/kaggle/dataset/suddharshan_retail_price_optimization): chưa xác minh cột/quy mô; dung lượng: không tìm thấy. Nhỏ, dùng demo độ co giãn. ★2.

### (c) Benchmark / paper học thuật

**Criteo Uplift Prediction** — https://ailab.criteo.com/criteo-uplift-prediction-dataset/ ; paper Diemert et al. 2018 "A large scale benchmark for uplift modeling".
- **Quy mô / dung lượng**: 25,309,483 dòng, 459 MB nén CSV (TFDS qua search). 25,309,483 dòng theo TFDS (bản khác ~13M dòng theo Microsoft Fabric/Kaggle mirror), đặc trưng ẩn danh (TFDS ghi 11 trong văn bản nhưng liệt kê f0-f11 = 12; Microsoft/Kaggle ghi 12), cờ treatment, nhãn visit và conversion, từ kiểm thử ngẫu nhiên; phiên bản v2.1 ~13.9M dòng theo một paper 2026 (https://arxiv.org/pdf/2608.00915; chưa đối chiếu). Điều khoản Criteo Research (tải, không phân phối lại; chưa xác minh chi tiết). Đặc trưng ẩn danh, quảng cáo chứ không phải voucher. ★4.

**Hillstrom MineThatData** — https://blog.minethatdata.com/2008/03/minethatdata-e-mail-analytics-and-data.html
- **Quy mô / dung lượng**: 64,000 dòng; 433 KB nén / 4,935 KB giải nén (docs scikit-uplift qua search). 64,000 khách mua trong 12 tháng, chia ngẫu nhiên 1/3 email Nam, 1/3 email Nữ, 1/3 không email; theo dõi 2 tuần. Cột: recency, history, segment, channel, zip code, conversion, spend. Có trong TFDS và scikit-uplift. Nhỏ, tín hiệu uplift yếu; tốt để dạy/kiểm pipeline. ★4.

**X5 RetailHero** — scikit-uplift `fetch_x5`: https://scikit-uplift.readthedocs.io/en/latest/api/datasets/fetch_x5.html . Gồm clients, train (treatment_flg, target, customer_id), purchases (lịch sử mua trước khi gửi truyền thông); hackathon RetailHero của X5 Retail Group mùa đông 2019 (nguồn: docs scikit-uplift qua search); số khách, số dòng và dung lượng: không tìm thấy (docs trang search không nêu). Có lịch sử mua thô → dùng cho đặc trưng RFM + uplift. ★4.

**Telco churn (IBM)** — https://data.mendeley.com/datasets/phsxg9ssrf : 7,043 khách, 21 cột (~550 KB, bản re-upload HF qua search), nhãn Churn Yes/No; TotalCharges cần ép số, 11 giá trị thiếu ở một số bản. Churn kiểu thuê bao, khác TMĐT. ★2.

**Santander Customer Transaction** — 200,000 mẫu, 200 đặc trưng ẩn danh (dung lượng: không tìm thấy), mất cân bằng; chỉ để thử GBM, không có ngữ nghĩa kinh doanh. ★1.

**Open Bandit Dataset** — https://arxiv.org/abs/2008.07146 (NeurIPS 2021 D&B): log bandit trên ZOZOTOWN (thời trang), nhiều chính sách (Bernoulli TS, Random), kèm pipeline OPE. Dùng cho đánh giá chính sách ngoại tuyến (off-policy), liên quan voucher/gợi ý. Quy mô ~26M dòng (research.zozo.com qua search), 80 item; dung lượng: không tìm thấy; CC BY 4.0 nhưng paper nêu mục đích phi thương mại (xem bảng); chưa đối chiếu điều khoản trang ZOZO Research. ★3.

**scikit-uplift datasets** — https://www.uplift-modeling.com/en/latest/api/datasets/fetch_criteo.html : loader cho Criteo, Lenta, X5, Hillstrom, MegaFon (không thấy loader Starbucks). Tiện để tải.

### (d) Blog / cuộc thi / công ty
- Towards Data Science, "Buy Til You Die" CLV bằng Lifetimes: https://towardsdatascience.com/buy-til-you-die-predict-customer-lifetime-value-in-python-9701bfd4ddc0/ (hướng dẫn CLV bằng Lifetimes; bài gốc dùng Olist nên chỉ lấy phần phương pháp, không dùng dữ liệu Olist).
- Các thư viện tham chiếu (không phải dữ liệu): Lifetimes, scikit-uplift, Open Bandit Pipeline.

## 5. Dữ liệu tự tạo / synthetic từ chính hệ thống

- **Nhãn churn nội bộ**: từ `fact_order_status` tính ngày mua cuối; churn = không có đơn hợp lệ (PAID online/DELIVERED COD, trừ hoàn) trong N ngày, N chọn theo phân phối khoảng cách giữa các đơn của từng danh mục; tách cửa sổ thời gian (quan sát/dự báo) để tránh rò rỉ.
- **CLV**: doanh thu thuần 90/365 ngày sau mốc cắt.
- **Voucher A/B**: để có uplift thật, phát voucher cho nhóm ngẫu nhiên (có nhóm đối chứng) và ghi `treatment_flg` + thời điểm vào `events_raw`/bảng chiến dịch; không có A/B thì không đáng tin dùng mô hình uplift.
- **Sinh dữ liệu mô phỏng** để kiểm pipeline: mô phỏng khách theo BG/NBD với tham số biết trước, so sánh tham số khôi phục; dùng `SEED_DEMO_DATA=true`, không dùng ở production.
- **Giá**: thí nghiệm giá nhỏ (±5%) theo SKU ngẫu nhiên để ước lượng độ co giãn nhân quả.

## 6. Lộ trình dùng dữ liệu đề xuất

1. Online Retail II (+ Dunnhumby/REES46 khi cần dữ liệu thêm): dựng pipeline RFM/BG-NBD/GBM và kiểm tra cách định nghĩa nhãn.
2. Uplift: Hillstrom (nhỏ) rồi Criteo/X5 để so sánh S/T/X-learner, đo Qini; nhớ cảnh báo nhiễu.
3. Dunnhumby và Tmall cho đáp ứng khuyến mãi / mua lại sau sale.
4. Open Bandit cho đánh giá chính sách ngoại tuyến.
5. Chuyển sang dữ liệu nội bộ khi đủ lịch sử và có A/B voucher; kiểm định lại trên dữ liệu Việt Nam (mùa vụ Tết, 11.11, 12.12, COD).

## 7. Rủi ro, license, thiên lệch, lưu ý pháp lý/PII

- **Chuyển miền**: dữ liệu Anh/Trung Quốc/Mỹ/Nhật, hành vi khác VN (COD, hoàn/huỷ cao, khuyến mãi theo đợt).
- **Định nghĩa churn/CLV**: dễ rò rỉ nhãn; phải theo quy ước doanh thu của dự án (PAID/DELIVERED, trừ hoàn).
- **Giá**: tối ưu giá cá nhân hoá có rủi ro công bằng và người tiêu dùng; nên chỉ gợi ý giá theo nhóm sản phẩm, minh bạch; tránh phân biệt đối xử theo thuộc tính nhạy cảm. Kiểm tra quy định cạnh tranh/bảo vệ người tiêu dùng (cần tư vấn pháp lý).
- **PII**: ID khách, địa chỉ, SĐT; dùng ID băm và gộp; tuân thủ pháp luật bảo vệ dữ liệu cá nhân VN. Seller chỉ thấy khách của shop mình (ownership từ JWT).
- **License**: Criteo, Instacart, các bộ Kaggle có điều khoản riêng; kiểm tra trước khi dùng thương mại. Mirror có thể ghi license khác bản gốc (OpenML Online Retail II).
- **Thiên lệch sống sót**: dữ liệu chỉ gồm khách đã mua; uplift thiếu nhóm đối chứng cho kết quả sai.

## 8. Nguồn tham khảo

- Online Retail II: https://archive.ics.uci.edu/dataset/502/online+retail+ii
- Dunnhumby: https://dunnhumby.com/sourcefiles ; https://cran.r-project.org/web/packages/completejourney/vignettes/completejourney.html
- Tmall IJCAI-15: https://groups.google.com/g/resys/c/oK8DzKHho1c
- Instacart: https://service.tib.eu/ldmservice/dataset/instacart-dataset
- Criteo: https://www.tensorflow.org/datasets/catalog/criteo ; https://scikit-uplift.readthedocs.io/en/latest/api/datasets/fetch_criteo.html ; https://arxiv.org/pdf/2608.00915
- Hillstrom: https://www.tensorflow.org/datasets/catalog/hillstrom ; https://blog.minethatdata.com/2008/03/minethatdata-e-mail-analytics-and-data.html
- X5: https://scikit-uplift.readthedocs.io/en/latest/api/datasets/fetch_x5.html
- Acquire Valued Shoppers: https://github.com/auduno/Kaggle-Acquire-Valued-Shoppers-Challenge
- Telco: https://data.mendeley.com/datasets/phsxg9ssrf
- Santander: https://arxiv.org/pdf/2406.19380 (tổng hợp)
- Open Bandit: https://arxiv.org/abs/2008.07146
- ViEcomRec: https://doras.dcu.ie/29693/1/viecomrec.pdf ; churn Shopee B2B: link ở mục 4(a)
- CLV Lifetimes: https://towardsdatascience.com/buy-til-you-die-predict-customer-lifetime-value-in-python-9701bfd4ddc0/
