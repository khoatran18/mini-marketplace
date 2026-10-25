# AI trên nền Mini Marketplace

> **Trạng thái: nghiên cứu / kế hoạch. Chưa có `ai-service`, endpoint hay giao diện AI nào trong code.**

| File | Nội dung |
|---|---|
| [01_AI_SCOPES_OVERVIEW.md](01_AI_SCOPES_OVERVIEW.md) | 16 scope AI khả thi, lĩnh vực (RecSys, IR, RAG, CV, NLP…), từng scope viết thành bài toán riêng (dữ liệu, phương pháp so sánh, metric), đề xuất gộp và logic gộp, bộ dữ liệu nên chọn + cụm tích hợp chung, cách tích hợp lại, thứ tự làm |
| [02_AGENT_ARCHITECTURE.md](02_AGENT_ARCHITECTURE.md) | Khi nào cần agent / LangGraph / LangSmith, lựa chọn thay thế, kiến trúc đề xuất |
| [03_MARKETPLACE_AI_LANDSCAPE.md](03_MARKETPLACE_AI_LANDSCAPE.md) | Shopee, Lazada, Amazon, Alibaba… đang dùng AI vào việc gì (bằng chứng từ kết quả tìm kiếm, **chưa mở đọc trang gốc**) |
| [04_AI_DATA_READINESS.md](04_AI_DATA_READINESS.md) | Nền đã có / còn thiếu gì cho từng scope |
| [06_AGENT_FEATURES.md](06_AGENT_FEATURES.md) | Danh mục tính năng agent cho admin / seller / buyer: tool nào đã có, người xác nhận ở đâu, thứ tự làm |
| [07_COROOT_AND_INCIDENT_AGENT.md](07_COROOT_AND_INCIDENT_AGENT.md) | Coroot (eBPF observability + RCA) là gì, giúp khoanh vùng sự cố ra sao, và kịch bản / dataset / test case cho agent điều tra sự cố: đã có, còn thiếu |
| [05_AI_DRAFT_ORIGINAL.md](05_AI_DRAFT_ORIGINAL.md) | Bản nháp ý tưởng ban đầu (hợp đồng tool, SSE, hệ gợi ý, RAG). Chỉ là điểm xuất phát; khi mâu thuẫn thì các file 01–04 mới hơn |

Bộ dữ liệu để kiểm chứng từng scope: [../dataset/README.md](../dataset/README.md).
