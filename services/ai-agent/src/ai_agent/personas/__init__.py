from __future__ import annotations

from pathlib import Path

from ai_agent.personas.base import Persona

_HERE = Path(__file__).parent


def load_personas() -> dict[str, Persona]:
    """name -> Persona. Chọn persona theo ROLE TRONG TOKEN, không theo lời người dùng."""
    return {
        "buyer": Persona.load("buyer", "buyer", ("buyer",), _HERE / "buyer"),
        "seller": Persona.load("seller", "seller", ("seller_admin", "seller_employee"), _HERE / "seller"),
    }


def persona_for_role(personas: dict[str, Persona], token_role: str) -> Persona | None:
    for p in personas.values():
        if token_role in p.token_roles:
            return p
    return None
