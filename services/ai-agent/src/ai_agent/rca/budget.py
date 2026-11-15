"""Ngân sách MỘT sự cố: số vòng, lượt gọi mô hình, số truy vấn, thời gian. Đọc từ khoá `rca:` của config (có mặc định)."""
from __future__ import annotations

from dataclasses import dataclass
from typing import Any

from ai_agent.config.loader import Config


@dataclass(frozen=True)
class Budget:
    max_rounds: int = 3           # số vòng collect -> hypothesize -> decide
    max_model_calls: int = 12
    max_queries: int = 25         # số lần gọi tool đọc (gateway)
    max_seconds: int = 300
    max_hypotheses: int = 3
    confident_at: float = 0.6     # độ tin cậy của giả thuyết đầu từ mức này thì đủ để đề xuất
    max_pending_proposals: int = 2
    verify_retries: int = 1       # verify thất bại: thêm đúng một vòng điều tra rồi báo người
    verify_wait_sec: int = 180    # chờ sau khi hành động chạy xong rồi mới verify (bộ theo dõi áp dụng)
    wait_timeout_sec: int = 1800  # chờ người (xác nhận / duyệt) tối đa
    poll_interval_sec: int = 30
    auto_critical: bool = True    # alert critical: agent tự vào điều tra
    daily_auto_runs: int = 10     # trần số phiên tự chạy mỗi ngày

    @classmethod
    def from_config(cls, cfg: Config | dict[str, Any] | None) -> "Budget":
        raw = cfg._data.get("rca", {}) if isinstance(cfg, Config) else (cfg or {})
        base = cls()
        fields = {k: type(getattr(base, k))(raw[k]) for k in cls.__dataclass_fields__ if k in raw}
        return cls(**fields)

    def exceeded(self, state: dict[str, Any], now: float) -> str | None:
        """Tên giới hạn đã chạm (để ghi vào dòng thời gian), None nếu còn."""
        if state.get("model_calls", 0) >= self.max_model_calls:
            return "model_calls"
        if state.get("queries", 0) >= self.max_queries:
            return "queries"
        if now - state.get("started_at", now) >= self.max_seconds:
            return "seconds"
        return None
