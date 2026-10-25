# Bộ dữ liệu để kiểm chứng từng scope AI

> **Trạng thái: tài liệu nghiên cứu, chưa có code AI nào dùng các bộ này.** Mỗi scope trong [../ai/01_AI_SCOPES_OVERVIEW.md](../ai/01_AI_SCOPES_OVERVIEW.md) có một file riêng: mục tiêu, hướng làm, metric, danh sách bộ dữ liệu (kèm link, license, kích thước, độ phù hợp ★1–5, ánh xạ vào bảng / event của hệ thống), cách tự dựng bộ vàng từ dữ liệu của chính hệ thống, lộ trình dùng, rủi ro. Tra cứu ngày 2026-10-25.
> Dữ liệu tiếng Anh / quốc tế đều dùng được để chọn và kiểm phương pháp; phần tiếng Việt (nếu có bộ) được nêu riêng trong từng file.

## Cách đọc độ tin cậy
- Nhãn **"đã xác minh (nguồn …)"**: số liệu / license thấy trong kết quả tra cứu hoặc trang GitHub.
- Nhãn **"chưa xác minh"**: chưa thấy được trang gốc. Công cụ tải trang không mở được Kaggle, HuggingFace, arXiv, Zenodo, nên nhiều mục chỉ dựa vào kết quả WebSearch. **Trước khi tải hoặc trích dẫn, hãy mở trang gốc kiểm lại license và kích thước.**
- Bộ có thể đã đổi license, bị gỡ hoặc yêu cầu đăng ký; đặc biệt các bộ Kaggle/HF.

## Danh sách scope và bộ nên thử đầu tiên

| # | File | Scope | Bộ nên thử đầu tiên (theo các agent nghiên cứu) |
|---|---|---|---|
| 01 | [01_RECOMMENDATION_CLICK.md](01_RECOMMENDATION_CLICK.md) | Gợi ý theo click / giỏ / mua | OTTO (CC BY 4.0), REES46, RetailRocket |
| 02 | [02_PRODUCT_SEARCH_SEMANTIC.md](02_PRODUCT_SEARCH_SEMANTIC.md) | Tìm kiếm ngữ nghĩa | WANDS (MIT), Amazon ESCI (Apache-2.0), VN-MTEB |
| 03 | [03_INVOICE_RECEIPT_OCR.md](03_INVOICE_RECEIPT_OCR.md) | OCR hóa đơn / biên lai | CORD (CC BY 4.0), SROIE, MC-OCR 2021 |
| 04 | [04_ANALYTICS_QA_TEXT2SQL.md](04_ANALYTICS_QA_TEXT2SQL.md) | Hỏi đáp số liệu seller / admin | BIRD, LiveSQLBench, Spider; + tự dựng bộ vàng trên schema ClickHouse |
| 05 | [05_SHOPPER_ASSISTANT_CUSTOMER_SUPPORT.md](05_SHOPPER_ASSISTANT_CUSTOMER_SUPPORT.md) | Trợ lý mua sắm / CSKH | τ-bench / τ²-bench (retail), ABCD, ShoppingBench / WebShop |
| 06 | [06_POLICY_LEGAL_RAG.md](06_POLICY_LEGAL_RAG.md) | RAG luật / chính sách | RAGTruth (MIT), LegalBench-RAG, Zalo Legal Text Retrieval 2021, ALQAC |
| 07 | [07_REVIEW_SENTIMENT_SPAM.md](07_REVIEW_SENTIMENT_SPAM.md) | Review: sentiment, spam | Amazon Reviews, Yelp Open, UIT-ViSFD, ViSpamReviews |
| 08 | [08_FRAUD_ANOMALY_RISK.md](08_FRAUD_ANOMALY_RISK.md) | Gian lận / bất thường | IEEE-CIS, TalkingData, BAF / FDB |
| 09 | [09_DEMAND_FORECAST_LOGISTICS.md](09_DEMAND_FORECAST_LOGISTICS.md) | Dự báo nhu cầu, logistics | M5, Online Retail II (CC BY 4.0), DataCo (CC BY 4.0) |
| 10 | [10_PRODUCT_CATEGORIZATION_ATTRIBUTES.md](10_PRODUCT_CATEGORIZATION_ATTRIBUTES.md) | Phân loại, thuộc tính, trùng lặp | Shopee Product Matching, WDC Products, MAVE (CC BY-NC 4.0) |
| 11 | [11_VISUAL_SEARCH_MULTIMODAL.md](11_VISUAL_SEARCH_MULTIMODAL.md) | Tìm bằng ảnh | Shopee Product Matching, Stanford Online Products, Amazon Berkeley Objects (CC BY 4.0) |
| 12 | [12_CONTENT_MODERATION_AGENT_SAFETY.md](12_CONTENT_MODERATION_AGENT_SAFETY.md) | Kiểm duyệt + an toàn agent | AgentDojo, InjecAgent, WildGuardMix, UIT-ViHSD |
| 13 | [13_CUSTOMER_ANALYTICS_CLV_CHURN_PRICING.md](13_CUSTOMER_ANALYTICS_CLV_CHURN_PRICING.md) | CLV, churn, uplift, giá | Online Retail II, Olist, Hillstrom → Criteo Uplift |
| 14 | [14_AIOPS_ANOMALY_ALERT_EXPLAIN.md](14_AIOPS_ANOMALY_ALERT_EXPLAIN.md) | AIOps, giải thích cảnh báo | RCAEval, SMD, TSB-AD |
| 15 | [15_VIETNAMESE_NLP_QUERY_UNDERSTANDING.md](15_VIETNAMESE_NLP_QUERY_UNDERSTANDING.md) | NLP cho truy vấn (tìm không dấu, sửa lỗi, intent) | VN-MTEB, MASSIVE (có vi-VN), ESCI queries, Viwiki-Spelling |
| 16 | [16_AGENT_TOOL_USE_EVAL.md](16_AGENT_TOOL_USE_EVAL.md) | Đánh giá agent gọi tool | τ²-bench, BFCL, MCP-Bench / MCPMark, GAIA |

## Lưu ý chung
1. **Tách nhiệm vụ**: bộ công khai để *chọn phương pháp* và so baseline; bộ vàng tự dựng từ dữ liệu hệ thống để *kiểm trên miền thật* (cách dựng có trong từng file, mục 5).
2. **License**: nhiều bộ chỉ cho nghiên cứu phi thương mại (ví dụ MAVE CC BY-NC, ViText2SQL chỉ nghiên cứu). Kiểm trước khi dùng cho sản phẩm.
3. **PII / pháp lý**: không đưa dữ liệu người dùng thật vào bộ công khai hay vào prompt của dịch vụ bên ngoài; văn bản pháp luật dùng cho RAG cần kiểm bản quyền / điều khoản và hiệu lực.
4. Bộ vàng cho agent (scope 4, 5, 16) phải gồm cả ca **từ chối vượt quyền** và **prompt injection**, không chỉ ca trả lời đúng.
