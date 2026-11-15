# ai-agent

Tầng agent của Mini Marketplace (Python 3.12, FastAPI, LangChain). Kế hoạch đầy đủ: [docs/ai/08_AGENT_BUILD_ROADMAP.md](../../docs/ai/08_AGENT_BUILD_ROADMAP.md).

## Hiện có
- **Config**: `configs/config.{dev,prod}.yaml` (không secret, `${VAR:-}` cho biến môi trường), loader `ai_agent/config/loader.py`, secret bị che khi in.
- **LLM profile + role + factory**: `llm.profiles` (mô hình đã cấu hình) và `llm.roles` (buyer, seller, summarizer → tên profile); `llm/registry.py` + `llm/factory.py` + `llm/providers/{openai,anthropic,fake}.py`. DeepSeek/OpenCode đi qua provider `openai` + `base_url`. Thêm nhà cung cấp = một file + một dòng import.
- **Mô hình giả có kịch bản** (`provider: fake`, `script:` trong yaml): chạy cả luồng không gọi mạng; dùng cho dev, CI, test an toàn.
- **Persona buyer, seller** (`personas/*/prompt.md`; chính sách buyer ở `personas/buyer/policy.md`, hiện là TODO). Persona chọn theo **role trong token agent**.
- **Tool lấy từ gateway** (`GET /ai/tools`, bảng `agent/scopes.go`): schema tham số sinh từ bảng, mô hình không thêm được tham số lạ. Hàm bọc tool gọi gateway bằng token agent, tách kết quả: **card đầy đủ → UI**, **bản đã che/cắt, bọc "dữ liệu không tin cậy" → mô hình**.
- **`POST /chat` (SSE)**: `start`, `token`, `tool_call`, `tool_result`, `card`, `citation`, `done`/`error`. Giới hạn: 4 vòng mô hình, 8 lần gọi tool, 60 s/lượt, trần token/lượt, kích thước tin nhắn. Một lượt tại một thời điểm cho mỗi cuộc chat, `client_message_id` chống gửi trùng, kiểm sở hữu `conversation_id`.
- Kho lịch sử **trong bộ nhớ** (mất khi restart).

## Chưa có (làm tiếp)
Kho Postgres, `/metrics`, tool đề xuất hành động (`/ai/proposals`, bước 5), provider thật đã chạy thử (chưa gọi mô hình thật nào), persona admin/operator, RCA, bộ vàng (`evals/`), tóm tắt lịch sử dài.

## Trạng thái kiểm thử
45 test đạt trên Python 3.12 (có `create_agent` + middleware thật và lớp FastAPI), và một lượt thật với OpenCode (buyer, seller; gateway giả) stream từng token, đếm token đúng. **Python 3.10 không stream token** trong LangGraph async (cần >= 3.11): dùng 3.12. Chưa chạy qua gateway thật trên Swarm. Chi tiết: [docs/ai/09_AGENT_BUILD_LOG.md](../../docs/ai/09_AGENT_BUILD_LOG.md) §16.5.

## Chạy cục bộ (không gọi mô hình thật)
```bash
conda create -n ai-agent python=3.12 && conda activate ai-agent
pip install -r requirements-dev.txt
cd services/ai-agent && pytest                       # test (cần đủ requirements)
APP_ENV=dev GATEWAY_URL=http://localhost:8080 \
  PYTHONPATH=src uvicorn ai_agent.main:create_app --factory --port 8090
```
Ở dev, `llm.roles.buyer`/`seller` trỏ tới profile `fake_*`. Đổi sang mô hình thật: đặt khoá trong `.env` (xem `configs/.env.example`) rồi sửa **một dòng** `llm.roles.<role>` thành `deepseek_flash` (hoặc profile bạn thêm).

## Nối với gateway
1. Gateway: `FEATURE_AI_CHAT=true`, `AI_SERVICE_URL=http://ai-agent:8090`, `AGENT_TOKEN_KEYS=...` (khoá chỉ gateway giữ).
2. `./scripts/build-images.sh ai-agent` rồi deploy (`deploy/services.yml` đã có service `ai-agent`, không publish cổng).
3. Luồng: UI → `POST /ai/chat` (JWT người dùng) → gateway cấp token agent → `POST /chat` ở đây → tool gọi lại gateway bằng token agent → SSE trả về UI.

## Bảo mật cần nhớ
- ai-agent **không có khoá ký** nên chỉ đọc (không kiểm chữ ký) token; phải nằm sau gateway trong mạng nội bộ. Token giả bị gateway từ chối (401) ở lần gọi tool đầu tiên.
- Ở prod, khởi động báo lỗi nếu có biến tracing của LangSmith (`LANGSMITH_*`, `LANGCHAIN_TRACING_V2`).
- Không log nội dung chat; log JSON có `run_id`.
