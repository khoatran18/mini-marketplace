"""Tạo chat model từ config: `llm.roles` (chỗ dùng -> tên profile) và `llm.profiles` (tên profile -> provider + tham số).

    get_llm(cfg, "deepseek_flash")   # theo tên profile
    get_llm_for_role(cfg, "buyer")   # tra roles -> profile -> chat model

Tiêm phụ thuộc: runtime và persona nhận mô hình qua tham số; chỉ chỗ lắp ráp (main.py) gọi hàm ở đây.
THÊM PROVIDER: viết file trong providers/ có hàm build gắn @register_llm("tên") + một dòng import trong providers/__init__.py.
"""
from __future__ import annotations

from langchain_core.language_models.chat_models import BaseChatModel

from ai_agent.config.loader import Config
from ai_agent.config.logging import get_logger
from ai_agent.llm import metrics as _metrics
from ai_agent.llm import providers as _providers  # noqa: F401  (nạp provider để chúng tự đăng ký)
from ai_agent.llm.registry import LLM_PROVIDERS

logger = get_logger(__name__)


def get_llm(cfg: Config, profile_name: str) -> BaseChatModel:
    """Tạo chat model theo tên profile. Lỗi cấu hình (thiếu api_key, model_id chưa điền...) chỉ báo ở đây, tức khi
    profile đó thực sự được dùng, và thông điệp luôn nêu tên profile."""
    profiles = cfg.llm.profiles
    if profile_name not in profiles:
        raise ValueError(f"Không có llm.profiles.{profile_name}; hiện có: {sorted(profiles)}")
    profile = profiles[profile_name].to_dict(masked=False)

    provider = profile.get("provider")
    if provider not in LLM_PROVIDERS:
        raise ValueError(f"llm.profiles.{profile_name}: provider={provider!r} chưa hỗ trợ; hiện có: {sorted(LLM_PROVIDERS)}")
    try:
        model = LLM_PROVIDERS[provider](profile)
    except ValueError as exc:
        raise ValueError(f"llm.profiles.{profile_name}: {exc}") from exc
    _metrics.attach(model, profile_name)
    logger.info("llm created", extra={"profile": profile_name, "provider": provider, "model_id": profile.get("model_id")})
    return model


def get_llm_for_role(cfg: Config, role: str) -> BaseChatModel:
    """Tạo chat model cho một vai trò (vd "buyer"): tra `llm.roles[role]` ra tên profile rồi gọi get_llm."""
    roles = cfg.llm.roles
    if role not in roles:
        raise ValueError(f"Không có llm.roles.{role}; hiện có: {sorted(roles)}")
    return get_llm(cfg, roles[role])
