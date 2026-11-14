"""Import mọi file provider ở đây: import khiến @register_llm chạy và hàm build được ĐĂNG KÝ (chưa tạo model nào).
Thêm provider mới: viết file trong thư mục này rồi thêm MỘT dòng import bên dưới."""
from ai_agent.llm.providers import anthropic, fake, openai  # noqa: F401
