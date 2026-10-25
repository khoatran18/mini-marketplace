# AIOps: phát hiện bất thường, giải thích cảnh báo, root cause

> Admin hỏi/giám sát hệ thống: phát hiện bất thường trên metric Prometheus, giải thích cảnh báo, xác định nguyên nhân gốc cho microservices, phân tích bất thường log. **Trạng thái: chưa triển khai trong hệ thống; đây là tài liệu nghiên cứu dữ liệu.** Ngày tra cứu: 2026-10-25.
>
> Ghi chú phương pháp: WebFetch không mở được Kaggle/HF/arXiv/Zenodo (DNS); thông tin lấy từ WebSearch và GitHub (RCAEval, OpenRCA đọc trực tiếp README). Chỗ không kiểm chứng ghi "chưa xác minh".

| Liên quan | File |
|---|---|
| Gian lận/bất thường nghiệp vụ | [08_FRAUD_ANOMALY_RISK.md](08_FRAUD_ANOMALY_RISK.md) |
| Dự báo nhu cầu/logistics | [09_DEMAND_FORECAST_LOGISTICS.md](09_DEMAND_FORECAST_LOGISTICS.md) |
| Text-to-SQL / analytics QA | [04_ANALYTICS_QA_TEXT2SQL.md](04_ANALYTICS_QA_TEXT2SQL.md) |
| Eval agent (tool `system_metrics`, `list_alerts`) | [16_AGENT_TOOL_USE_EVAL.md](16_AGENT_TOOL_USE_EVAL.md) |

## 1. Mục tiêu và vị trí trong hệ thống

- **Admin** (agent `admin`, tool `system_metrics`, `service_health`, `list_alerts`, `search_audit_log` trong `docs/ai/05_AI_DRAFT_ORIGINAL.md`): hỏi "vì sao latency order-service tăng?", "cảnh báo này nghĩa là gì?", "dịch vụ nào là nguyên nhân?". **Seller** có `list_my_alerts` (cảnh báo nghiệp vụ, không phải hạ tầng).
- **Input**: chuỗi thời gian Prometheus (mỗi service Go có cổng admin :8081 `/metrics`, `/health`, `/ready`), log dịch vụ, cảnh báo Grafana/Alertmanager, trace (nếu bật), sự kiện triển khai. Metric hiện có ví dụ: `mm_ch_insert_lag_seconds`, `mm_ch_rows_dropped_total`, `mm_events_rejected_total`, `mm_payments_total` (một số chưa có theo tài liệu giám sát).
- **Output**: danh sách bất thường xếp hạng, giải thích văn bản kèm bằng chứng (metric/khoảng thời gian), ứng viên nguyên nhân gốc, gợi ý bước tiếp theo (runbook `docs/instruction/03_RUNBOOK.md`). Chỉ đọc; không tự thực hiện thao tác vận hành.
- Hệ thống đã có Kafka, outbox, ClickHouse pipeline: lỗi lan truyền qua hàng đợi (lag tăng, insert thất bại) là kịch bản RCA đặc trưng.

## 2. Hướng làm đề xuất (baseline → nâng cao)

1. **Baseline**: ngưỡng + z-score/EWMA, seasonal decomposition (STL), cảnh báo theo SLO; dùng quy tắc có sẵn của Prometheus.
2. **Thống kê/ML nhẹ**: Isolation Forest, matrix profile, Prophet; mô hình đa biến (SR-CNN, autoencoder) khi cần.
3. **RCA**: đồ thị phụ thuộc dịch vụ (từ cấu hình stack) + hồi quy nhân quả / random walk trên đồ thị (kiểu RCAEval baseline); xếp hạng service nghi vấn.
4. **LLM giải thích**: cung cấp cho LLM dữ liệu đã tóm tắt (top metric lệch, thời điểm bắt đầu, sự kiện deploy), không đổ toàn bộ log; LLM trả lời có trích dẫn bằng chứng. OpenRCA cho thấy LLM agent còn rất yếu ở RCA đầy đủ (xem mục 4), nên LLM chỉ nên là lớp diễn giải trên đầu ra của công cụ thống kê.
5. **Log**: parse mẫu log (Drain) → đếm sự kiện → mô hình bất thường (DeepLog/LogBERT kiểu); dùng Loghub làm thử nghiệm.

## 3. Metric đánh giá

- Phát hiện: **VUS-PR** (khuyến nghị của TSB-AD), best-F1 có point-adjust (cảnh báo: point-adjust thổi phồng điểm), PR-AUC, thời gian đến phát hiện (detection delay).
- NAB: Numenta score (thưởng phát hiện sớm, phạt FP).
- RCA: **Top-k accuracy (AC@k, Avg@k)** của service/metric nguyên nhân; với LLM (OpenRCA): đúng đồng thời thời điểm, thành phần, lý do.
- Giải thích: độ đúng của trích dẫn bằng chứng, đánh giá bởi con người + LLM-judge (xem [16](16_AGENT_TOOL_USE_EVAL.md)).
- Vận hành: tỉ lệ cảnh báo giả/ngày, MTTD, MTTR (khi triển khai).

## 4. Bộ dữ liệu

### Bảng tổng hợp

| # | Bộ dữ liệu | Nhóm | Quy mô (theo nguồn đã tra) | License / truy cập | Phù hợp |
|---|---|---|---|---|---|
| 1 | RCAEval (RE1/RE2/RE3) | c | 735 ca lỗi, 3 hệ thống (Online Boutique, Sock Shop, Train Ticket) | MIT (code+dữ liệu của tác giả); HF ~3.4 GB parquet | ★★★★★ |
| 2 | TSB-AD (kế nhiệm TSB-UAD) | c | 1070 chuỗi, 40 bộ | Mã nguồn mở TheDatumOrg/TSB-AD | ★★★★★ |
| 3 | NAB | c | 58 chuỗi (README: ">50") | MIT (theo diễn đàn/ecosyste.ms) | ★★★★ |
| 4 | SMD (OmniAnomaly) | c | 28 máy x 38 metric, 5 tuần | Repo NetManAIOps; license chưa xác minh | ★★★★★ |
| 5 | SMAP / MSL (NASA) | c | 55x25 / 27x55 (số liệu hay bị đảo) | Nguồn NASA; qua repo OmniAnomaly | ★★ |
| 6 | OpenRCA | c | 335 lỗi, 68 GB telemetry, 3 hệ thống | Repo MIT; dữ liệu Google Drive | ★★★★ |
| 7 | AIOps Challenge 2020 (NetManAIOps) | d | Giai đoạn 1; giai đoạn 2 không có | Chỉ phi thương mại | ★★★ |
| 8 | AIOps Challenge (CCF/Tsinghua/CCB, 2022) | d | Hệ thống e-commerce mô phỏng, 40 instance, 6 node | Chưa xác minh | ★★★★ |
| 9 | Loghub | b | 19+ bộ log (HDFS, BGL, Thunderbird...) | Nghiên cứu/học thuật ("other") | ★★★★ |
| 10 | Yahoo S5 | c | A1 65 chuỗi thật; A2-A4 100 chuỗi tổng hợp | Webscope (cần yêu cầu) | ★★ |
| 11 | Exathlon | c | Spark streaming, 6 loại bất thường | Công khai trên GitHub; license chưa xác minh | ★★★ |
| 12 | LEMMA-RCA | c | Đa miền, có microservices | Chưa xác minh | ★★★ |
| 13 | TrainTicketTrace (SANER 2026) | c | Train Ticket sạch + 9 bản có lỗi | Chưa xác minh | ★★★ |
| 14 | Alibaba cluster-trace-microservices | d | 2021: 20M trace/12h; 2022: ~13B trace/13 ngày | Cần làm khảo sát; chất lượng có vấn đề | ★★ |
| 15 | Azure Public Dataset | d | VM, Functions, LLM inference traces | Chưa rõ (CC-BY-4.0 theo 1 index; chưa xác minh) | ★★ |
| 16 | Google Cluster Data 2019 | d | 8 cell Borg, 2.4 TiB | CC-BY, qua BigQuery | ★ |

### (a) Tiếng Việt / Việt Nam
Không có. Tín hiệu hạ tầng không phụ thuộc ngôn ngữ; chỉ phần diễn giải/trả lời dùng tiếng Việt (prompt và đánh giá nội bộ).

### (b) TMĐT / miền liên quan (microservices thương mại)

**RCAEval** — https://github.com/phamquiluan/RCAEval ; paper https://arxiv.org/pdf/2412.17015 ; Zenodo DOI 10.5281/zenodo.14590730.
- RE1: 375 ca chỉ metric; RE2: 270 ca metric+log+trace; RE3: 90 ca lỗi mức mã. Hệ thống: Online Boutique (12 service, gRPC), Sock Shop (15, HTTP), Train Ticket (64). 15 baseline tái lập được.
- Truy cập: HF Parquet (~3.4 GB, tải theo phần), Figshare, Zenodo (5.2 GB nén ~38 GB). License MIT cho code/dữ liệu của tác giả; baseline bên thứ ba có license riêng. Cảnh báo: schema cột RE1 không đồng nhất giữa hệ thống; tra cứu ngây thơ trả rỗng ở 250/375 ca (theo một nghiên cứu phân tích lại).
- Dùng: validate RCA trên microservices TMĐT giống kiến trúc dự án. ★5.

**AIOps Challenge 2022 (CCF/Tsinghua/CCB)** — mô tả qua https://arxiv.org/pdf/2406.19711 (CHASE): hệ thống e-commerce microservice mô phỏng, 40 instance trên 6 node, metric/log/trace, lỗi mức service/pod/node; thời điểm kết thúc lỗi không được cung cấp. Link tải chưa xác minh. ★4.

**Loghub** — https://github.com/logpai/loghub ; paper https://arxiv.org/pdf/2008.06448 : HDFS, Hadoop, Spark, Zookeeper, OpenStack, BGL, HPC, Thunderbird (nhãn alert theo cột đầu), Linux, Android, Apache, OpenSSH... Log thật không ẩn danh; chỉ cho nghiên cứu/học thuật. Không phải log Go/JSON của dự án. ★4.

### (c) Benchmark / paper học thuật

**TSB-AD (NeurIPS 2024)** — https://github.com/TheDatumOrg/TSB-AD ; https://proceedings.neurips.cc/paper_files/paper/2024/file/c3f3c690b7a99fba16d0efd35cb83b2c-Paper-Datasets_and_Benchmarks_Track.pdf : 1070 chuỗi chất lượng cao từ 40 bộ, đánh giá 40 thuật toán, metric chính VUS-PR; danh sách đánh giá 350 chuỗi đơn biến và 180 đa biến; phát hiện: mô hình đơn giản/thống kê thường tốt. Quan hệ với TSB-UAD chưa xác minh (không thấy trang riêng). ★5.

**NAB** — https://github.com/numenta/NAB ; paper https://www.arxiv.org/pdf/1510.03336 : 58 chuỗi thực+nhân tạo có nhãn, điểm số thưởng phát hiện sớm. Code chuyển AGPL sang MIT (diễn đàn Numenta); kiểm tra LICENSE khi dùng. Nhỏ, cổ điển. ★4.

**SMD / SMAP / MSL** — https://github.com/NetManAIOps/OmniAnomaly : SMD 28 máy x 38 metric (Merlion ghi 33), tỉ lệ bất thường 4.16%, ~708K điểm train/test; SMAP 55 thực thể x 25 chiều; MSL 27 x 55 (con số hay bị hiểu nhầm, xem ESA benchmark https://arxiv.org/pdf/2406.17826 phê bình các bộ NASA). SMD gần nhất với metric máy chủ. ★5 (SMD), ★2 (SMAP/MSL). Cũng có trong Time-Series-Library (https://github.com/thuml/Time-Series-Library, MIT, liệt kê SMD/SMAP; MSL không thấy nêu).

**Yahoo S5** — Webscope https://webscope.sandbox.yahoo.com/catalog.php?datatype=s&did=70 : A1 65 chuỗi traffic thật, A2-A4 100 chuỗi tổng hợp mỗi nhóm. Điều khoản chưa xác minh; có thể cần yêu cầu. ★2.

**OpenRCA** — https://github.com/microsoft/OpenRCA ; ICLR 2025 : 335 lỗi, 3 hệ thống (Telecom, Bank, Market với cloudbed-1/2), >68 GB log/metric/trace; tác tử RCA tốt nhất giải 11.34% (Claude 3.5); nghiên cứu AIOps 2026 báo 3.9%-12.5% trên 5 model. Cần ≥80 GB đĩa, 32 GB RAM; dữ liệu trên Google Drive. Dùng: **đo giới hạn LLM agent**. ★4.

**Exathlon** — https://github.com/exathlonbenchmark/exathlon ; https://arxiv.org/pdf/2010.05073 : vết job streaming Spark, 6 loại bất thường, nhãn khoảng nguyên nhân và khoảng hệ quả, hỗ trợ cả giải thích. ★3.

**LEMMA-RCA** — https://arxiv.org/abs/2406.05375 : tập RCA đa miền/đa phương thức (microservices, hệ thống nước...). Chi tiết microservice chưa xác minh. ★3.

**TrainTicketTrace** — https://conf.researchr.org/details/saner-2026/saner-2026-workshops-tutorials-track/43/ : trace/metric/log Train-Ticket sạch + 9 bản lỗi gieo. ★3.

**Benchmark LLM agent AIOps** (chỉ thấy tên): AIOpsLab (MLSys 2025), ITBench (ICML 2025) trong danh mục https://github.com/Jun-jie-Huang/awesome-LLM-AIOps; chưa xác minh dữ liệu cảnh báo kèm nhãn.

### (d) Blog / cuộc thi / công ty

**AIOps Challenge 2020** — https://github.com/NetManAIOps/AIOps-Challenge-2020-Data : giai đoạn 1 có dữ liệu, giai đoạn 2 không; giới hạn phi thương mại. ★3.

**Alibaba cluster-trace-microservices** — https://github.com/alibaba/clusterdata : v2021 (>20,000 microservice, 12 giờ, 20M trace theo slide Tufts) và v2022 (13 ngày, ~13B trace); cần khảo sát để tải; ICPE 2024 (https://raja.cs.tufts.edu/assets/pdf/huye2024_paper.pdf) chỉ ra bất nhất làm không dựng lại được topo; có bản sửa. Là trace production nhưng không có nhãn lỗi. ★2.

**Azure Public Dataset** — https://github.com/Azure/AzurePublicDataset : VM 2017/2019, Azure Functions 2019, LLM inference traces 11/2023. License không rõ (CC-BY-4.0 theo một index, "nghiên cứu/học thuật" theo index khác; chưa xác minh). Hợp cho dự báo tài nguyên hơn là RCA. ★2.

**Google Cluster Data 2019** — https://github.com/google/cluster-data : 8 cell Borg tháng 5/2019, 2.4 TiB nén, qua BigQuery, CC-BY. ★1.

## 5. Dữ liệu tự tạo / synthetic từ chính hệ thống

- **Thu metric thật**: lưu Prometheus ≥ 30-90 ngày (retention) cho các metric chính của 7 service Go và infra (postgres, redis, kafka, clickhouse). Đây là nguồn quan trọng nhất vì metric Go/gRPC khác dữ liệu công khai.
- **Chaos có kịch bản (nhãn miễn phí)**: tắt Redis, tăng độ trễ Postgres, dừng consumer Kafka để lag, đầy đĩa ClickHouse, thất bại thanh toán mô phỏng (mockpay), 5xx ở gateway. Ghi lại giờ bắt đầu/kết thúc và service gốc làm nhãn; cùng định dạng RCAEval để dùng lại baseline.
- **Tổng hợp thêm**: chèn bất thường nhân tạo (spike, level shift, drift, mất dữ liệu) vào chuỗi sạch để kiểm bộ phát hiện (chấp nhận hạn chế của nhãn tổng hợp).
- **Hỏi đáp mẫu cho agent admin**: 100-200 câu hỏi (tiếng Việt) kèm bằng chứng đúng; bộ này cũng là bộ eval của [16](16_AGENT_TOOL_USE_EVAL.md).
- **Log**: log cấu trúc của dịch vụ Go (JSON) + lỗi gieo; chú ý che PII trước khi lưu.

## 6. Lộ trình dùng dữ liệu đề xuất

1. Phát hiện: chọn thuật toán trên SMD + NAB + một phần TSB-AD (VUS-PR); so với ngưỡng đơn giản.
2. RCA: chạy baseline RCAEval (RE1/RE2) cho Online Boutique/Sock Shop (kiến trúc gần dự án); đo AC@k.
3. LLM: thử OpenRCA một phần nhỏ để biết giới hạn; thiết kế LLM chỉ diễn giải kết quả công cụ.
4. Log: Loghub để thử parser/mô hình, rồi chuyển sang log thật của dự án.
5. Tự tạo kịch bản chaos, dùng làm kiểm thử hồi quy cho tính năng giải thích cảnh báo.

## 7. Rủi ro, license, thiên lệch, lưu ý pháp lý/PII

- **Nhãn kém tin cậy**: các bộ NASA, Yahoo, SMD có lỗi nhãn và thiên lệch được phê bình (ESA/TSB-AD); point-adjust thổi phồng kết quả.
- **Chuyển miền**: Online Boutique/Sock Shop/Train Ticket mô phỏng, lưu lượng sinh bởi load tool; hệ thống thật có mùa vụ (11.11, Tết).
- **License**: AIOps Challenge phi thương mại; Loghub học thuật; Azure/Alibaba điều khoản không rõ; OpenRCA dữ liệu chưa nêu license riêng.
- **Bảo mật**: log/metric có thể chứa định danh khách, địa chỉ, token; không gửi nguyên văn cho LLM ngoài. Agent admin chỉ đọc và tuân thủ RBAC; giải thích do LLM có thể sai (OpenRCA cho thấy hiện tượng hallucinate) nên luôn kèm bằng chứng và cờ độ tin cậy.
- Kích thước lớn (RCAEval 38 GB giải nén, OpenRCA 68 GB): lấy mẫu theo ca, không tải toàn bộ.

## 8. Nguồn tham khảo

- RCAEval: https://github.com/phamquiluan/RCAEval ; https://arxiv.org/pdf/2412.17015 ; https://zenodo.org/records/14504481
- TSB-AD: https://github.com/TheDatumOrg/TSB-AD ; https://nips.cc/virtual/2024/poster/97690
- NAB: https://github.com/numenta/NAB ; https://discourse.numenta.org/t/code-license-is-now-mit/11642
- OmniAnomaly (SMD/SMAP/MSL): https://github.com/NetManAIOps/OmniAnomaly ; https://arxiv.org/pdf/2406.17826
- OpenRCA: https://github.com/microsoft/OpenRCA ; https://iclr.cc/virtual/2025/poster/32093
- AIOps Challenge 2020: https://github.com/NetManAIOps/AIOps-Challenge-2020-Data ; 2022: https://arxiv.org/pdf/2406.19711
- Loghub: https://github.com/logpai/loghub ; https://arxiv.org/pdf/2008.06448
- Yahoo S5 (Webscope, qua trích dẫn): https://webscope.sandbox.yahoo.com/catalog.php?datatype=s&did=70
- Exathlon: https://github.com/exathlonbenchmark/exathlon
- LEMMA-RCA: https://arxiv.org/abs/2406.05375
- TrainTicketTrace: https://conf.researchr.org/details/saner-2026/saner-2026-workshops-tutorials-track/43/TrainTicketTrace-A-Multi-Fault-Distributed-Dataset-for-Microservice-Fault-Detection-
- Alibaba: https://github.com/alibaba/clusterdata ; https://raja.cs.tufts.edu/assets/pdf/huye2024_paper.pdf
- Azure: https://github.com/Azure/AzurePublicDataset
- Google: https://github.com/google/cluster-data
- awesome-LLM-AIOps: https://github.com/Jun-jie-Huang/awesome-LLM-AIOps
- Time-Series-Library: https://github.com/thuml/Time-Series-Library
