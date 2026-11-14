"""Persona = dữ liệu (prompt, role LLM, các role token được dùng), không phải code riêng. Thêm persona = thêm một thư mục + một dòng
trong personas/__init__.py. Bộ tool của persona KHÔNG khai báo ở đây mà lấy từ gateway theo role trong token (GET /ai/tools)."""
from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class Persona:
    name: str                    # buyer | seller
    llm_role: str                # khoá trong llm.roles
    token_roles: tuple[str, ...] # role trong token agent được dùng persona này
    prompt: str

    @staticmethod
    def load(name: str, llm_role: str, token_roles: tuple[str, ...], directory: Path) -> "Persona":
        text = (directory / "prompt.md").read_text(encoding="utf-8")
        policy_file = directory / "policy.md"
        policy = policy_file.read_text(encoding="utf-8").strip() if policy_file.is_file() else ""
        if not policy or policy.startswith("TODO"):
            policy = "(Chưa có tài liệu chính sách. Khi được hỏi về chính sách, nói rõ bạn chưa có thông tin chính thức và gợi ý liên hệ cửa hàng.)"
        return Persona(name, llm_role, token_roles, text.replace("{policy}", policy))
