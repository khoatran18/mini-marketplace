"""Dựng dòng events_raw đúng cột analytics-service/internal/store.EventRow và luật consent của ingest/tracking.go."""
from __future__ import annotations

import dataclasses
import datetime as dt
import json
import random
from typing import Dict, List, Optional

from .util import ch_ts, hex_id

DEVICES = ["desktop", "mobile", "mobile", "tablet"]
OSES = {"desktop": ["Windows", "macOS", "Linux"], "mobile": ["Android", "iOS"], "tablet": ["iPadOS", "Android"]}
UAS = ["Chrome", "Safari", "Firefox", "Edge"]
COLUMNS = ["event_id", "event_type", "ts", "ts_server", "anonymous_id", "session_id", "user_id", "role", "store_id", "surface",
           "path", "referrer", "product_id", "position", "props", "device_type", "os", "country", "ua_family",
           "analytics_consent", "ip_hash", "app_version"]
# ingest chỉ giữ page_view / error_client khi người dùng từ chối consent, và xoá mọi định danh
ANON_TYPES = {"page_view", "error_client"}


@dataclasses.dataclass
class Visitor:
    anonymous_id: str
    user_id: int = 0
    role: str = ""
    consent: bool = True
    device: str = "desktop"
    os: str = "Windows"
    ua: str = "Chrome"
    ip_hash: str = ""


def new_visitor(rng: random.Random, anonymous_id: str, user_id: int = 0, consent_rate: float = 0.9) -> Visitor:
    dev = rng.choice(DEVICES)
    return Visitor(anonymous_id, user_id, "buyer" if user_id else "", rng.random() < consent_rate, dev,
                   rng.choice(OSES[dev]), rng.choice(UAS), hex_id(rng, 8))


class Sink:
    def __init__(self, rng: random.Random):
        self.rng = rng
        self.rows: List[dict] = []
        self.seen = set()

    def _eid(self) -> str:
        while True:
            e = "dg-" + hex_id(self.rng, 10)
            if e not in self.seen:
                self.seen.add(e)
                return e

    def emit(self, t: dt.datetime, etype: str, v: Visitor, session: str, surface: str = "", path: str = "/", referrer: str = "",
             product_id: int = 0, position: int = 0, props: Optional[dict] = None) -> None:
        rng = self.rng
        server = t + dt.timedelta(milliseconds=rng.randint(20, 300))
        row = {"event_id": self._eid(), "event_type": etype, "ts": ch_ts(t), "ts_server": ch_ts(server),
               "anonymous_id": v.anonymous_id, "session_id": session, "user_id": v.user_id, "role": v.role, "store_id": 0,
               "surface": surface, "path": path, "referrer": referrer, "product_id": product_id, "position": position,
               "props": json.dumps(props or {}, ensure_ascii=False, separators=(",", ":"), sort_keys=True),
               "device_type": v.device, "os": v.os, "country": "VN", "ua_family": v.ua,
               "analytics_consent": 1 if v.consent else 0, "ip_hash": v.ip_hash, "app_version": "1.0.0"}
        if not v.consent:
            if etype not in ANON_TYPES:
                self.seen.discard(row["event_id"])
                return
            row.update(anonymous_id="", session_id="", user_id=0, ip_hash="", product_id=0, props="{}")
        self.rows.append(row)
