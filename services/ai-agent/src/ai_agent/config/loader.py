"""Nạp cấu hình: .env -> configs/config.{APP_ENV}.yaml -> thay ${VAR} -> Config (đọc bằng dấu chấm).

Mẫu lấy từ dự án time-aware-agentic-memory (src/tam/config/loader.py). Yaml chỉ chứa cấu trúc và tham số, KHÔNG chứa secret:
chỗ cần secret ghi `${TÊN_BIẾN:-}` và giá trị thật nằm ở biến môi trường (deploy/.env, Swarm secret).
"""
from __future__ import annotations

import logging
import os
import re
from collections.abc import Iterator
from functools import lru_cache
from pathlib import Path
from typing import Any

import yaml
from dotenv import load_dotenv

logger = logging.getLogger(__name__)

DEFAULT_ENV = "dev"
CONFIG_DIR_NAME = "configs"

# ${VAR} (bắt buộc) hoặc ${VAR:-mặc_định}
_ENV_PATTERN = re.compile(r"\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}")
# khoá có tên chứa các từ này bị che khi in / ghi ra file
_SECRET_KEY = re.compile(r"(key|secret|token|password)", re.IGNORECASE)


class ConfigError(Exception):
    """Lỗi nạp cấu hình; thông điệp nêu rõ file và khoá sai."""


class Config:
    """Bọc dict lồng nhau: cfg.llm.profiles.x hoặc cfg["llm"]["profiles"]["x"]."""

    def __init__(self, data: dict[str, Any], path: str = "") -> None:
        self._data = data
        self._path = path

    def _wrap(self, name: str, value: Any) -> Any:
        if isinstance(value, dict):
            return Config(value, f"{self._path}.{name}" if self._path else name)
        return value

    def __getattr__(self, name: str) -> Any:
        if name.startswith("_"):
            raise AttributeError(name)
        try:
            return self._wrap(name, self._data[name])
        except KeyError:
            where = f"{self._path}.{name}" if self._path else name
            raise AttributeError(f"Thiếu khoá cấu hình '{where}'; khoá hiện có: {sorted(self._data)}") from None

    def __getitem__(self, name: str) -> Any:
        return self._wrap(name, self._data[name])

    def get(self, name: str, default: Any = None) -> Any:
        return self._wrap(name, self._data[name]) if name in self._data else default

    def __contains__(self, name: object) -> bool:
        return name in self._data

    def __iter__(self) -> Iterator[str]:
        return iter(self._data)

    def keys(self):
        return self._data.keys()

    def items(self):
        return ((k, self._wrap(k, v)) for k, v in self._data.items())

    def to_dict(self, masked: bool = True) -> dict[str, Any]:
        """Bản dict thường; masked=True che giá trị của khoá có tên key/secret/token/password."""
        return _mask(self._data) if masked else _copy(self._data)

    def __repr__(self) -> str:
        return f"Config({self.to_dict(masked=True)!r})"


def _copy(node: Any) -> Any:
    if isinstance(node, dict):
        return {k: _copy(v) for k, v in node.items()}
    if isinstance(node, list):
        return [_copy(v) for v in node]
    return node


def _mask(node: Any, key: str = "") -> Any:
    if isinstance(node, dict):
        return {k: _mask(v, str(k)) for k, v in node.items()}
    if isinstance(node, list):
        return [_mask(v, key) for v in node]
    if node and isinstance(node, str) and _SECRET_KEY.search(key):
        return "********"
    return node


def project_root() -> Path:
    """Gốc service = thư mục tổ tiên đầu tiên có `configs/` (đổi được bằng AI_AGENT_ROOT)."""
    override = os.getenv("AI_AGENT_ROOT")
    if override:
        return Path(override).resolve()
    for parent in Path(__file__).resolve().parents:
        if (parent / CONFIG_DIR_NAME).is_dir():
            return parent
    return Path.cwd()


def _expand_env(node: Any, path: str, missing: list[str]) -> Any:
    """Thay ${VAR} / ${VAR:-default} trong mọi chuỗi; ghi lại biến thiếu. Chuỗi chỉ gồm ${...} mà rỗng -> None."""
    if isinstance(node, dict):
        return {k: _expand_env(v, f"{path}.{k}" if path else str(k), missing) for k, v in node.items()}
    if isinstance(node, list):
        return [_expand_env(v, f"{path}[{i}]", missing) for i, v in enumerate(node)]
    if isinstance(node, str) and _ENV_PATTERN.search(node):

        def _sub(match: re.Match[str]) -> str:
            name, default = match.group(1), match.group(2)
            value = os.getenv(name)
            if value is not None and value != "":
                return value
            if default is not None:
                return default
            missing.append(f"{path}: thiếu biến môi trường {name}")
            return ""

        result = _ENV_PATTERN.sub(_sub, node)
        return result if result.strip() else None
    return node


def load_config(env: str | None = None, config_dir: Path | str | None = None) -> Config:
    """Nạp cấu hình cho môi trường `env` (mặc định biến APP_ENV, rồi 'dev')."""
    root = project_root()
    load_dotenv(root / ".env", override=False)  # biến đã có sẵn trong môi trường được ưu tiên hơn .env

    env = env or os.getenv("APP_ENV") or DEFAULT_ENV
    directory = Path(config_dir or os.getenv("AI_AGENT_CONFIG_DIR") or root / CONFIG_DIR_NAME)
    path = directory / f"config.{env}.yaml"
    if not path.is_file():
        available = sorted(p.name for p in directory.glob("config.*.yaml")) if directory.is_dir() else []
        raise ConfigError(f"Không tìm thấy {path} (APP_ENV={env!r}); file hiện có: {available}")

    try:
        raw = yaml.safe_load(path.read_text(encoding="utf-8"))
    except yaml.YAMLError as exc:
        raise ConfigError(f"{path} không phải YAML hợp lệ: {exc}") from exc
    if not isinstance(raw, dict):
        raise ConfigError(f"{path} phải là một mapping ở cấp cao nhất")

    missing: list[str] = []
    expanded = _expand_env(raw, "", missing)
    if missing:
        raise ConfigError(f"{path} có biến môi trường chưa đặt:\n  " + "\n  ".join(missing))

    declared = (expanded.get("app") or {}).get("env")
    if declared is not None and declared != env:
        logger.warning("app.env=%r trong %s không khớp APP_ENV=%r", declared, path.name, env)
    return Config(expanded)


@lru_cache(maxsize=1)
def get_config() -> Config:
    """Bản dùng chung cho cả tiến trình (nạp một lần)."""
    return load_config()
