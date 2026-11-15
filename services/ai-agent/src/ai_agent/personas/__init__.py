from __future__ import annotations

from pathlib import Path

from ai_agent.personas.base import Persona

_HERE = Path(__file__).parent


def load_personas() -> dict[str, Persona]:
    """name -> Persona. Chọn persona theo ROLE TRONG TOKEN, không theo lời người dùng."""
    return {
        "buyer": Persona.load("buyer", "buyer", ("buyer",), _HERE / "buyer", hide_internals=True),
        "seller": Persona.load("seller", "seller", ("seller_admin", "seller_employee"), _HERE / "seller", hide_internals=True),
        "admin": Persona.load("admin", "admin", ("admin",), _HERE / "admin"),
        "operator": Persona.load("operator", "operator", ("operator",), _HERE / "operator"),
        # RCA (ADR-46): không có role token riêng (token_roles rỗng nên chat không bao giờ chọn nó); chạy bằng token operator, chỉ đọc
        "rca": Persona.load("rca", "rca", (), _HERE / "rca", allow_actions=False),
    }


def persona_for_role(personas: dict[str, Persona], token_role: str) -> Persona | None:
    for p in personas.values():
        if token_role in p.token_roles:
            return p
    return None
