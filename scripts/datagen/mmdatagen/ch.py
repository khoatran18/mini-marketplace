"""Ghi / xoá events_raw ở ClickHouse bằng HTTP (JSONEachRow), chỉ dùng urllib."""
from __future__ import annotations

import base64
import json
import urllib.error
import urllib.parse
import urllib.request
from typing import List

from .events import COLUMNS

# Bản sao migration 1 của analytics-service/internal/store/schema.go (chỉ dùng khi --ch-ensure-schema).
EVENTS_RAW_DDL = """CREATE TABLE IF NOT EXISTS {db}.events_raw (
  event_id String, event_type LowCardinality(String),
  ts DateTime64(3, 'UTC'), ts_server DateTime64(3, 'UTC'),
  anonymous_id String, session_id String, user_id UInt64, role LowCardinality(String), store_id UInt64,
  surface LowCardinality(String), path String, referrer String,
  product_id UInt64, position UInt16, props String,
  device_type LowCardinality(String), os LowCardinality(String), country LowCardinality(String), ua_family LowCardinality(String),
  analytics_consent UInt8, ip_hash String, app_version LowCardinality(String),
  ingested_at DateTime64(3, 'UTC') DEFAULT now64(3)
) ENGINE = ReplacingMergeTree(ts_server)
  PARTITION BY toYYYYMM(ts) ORDER BY (event_type, anonymous_id, ts, event_id)
  TTL toDateTime(ts) + INTERVAL 13 MONTH"""


class ClickHouse:
    def __init__(self, url: str, db: str, user: str = "default", password: str = ""):
        self.url, self.db, self.user, self.password = url.rstrip("/"), db, user, password

    def _post(self, query: str, body: bytes = b"", params=None) -> str:
        q = {"query": query}
        q.update(params or {})
        req = urllib.request.Request(self.url + "/?" + urllib.parse.urlencode(q), data=body, method="POST")
        if self.user:
            tok = base64.b64encode(("%s:%s" % (self.user, self.password)).encode()).decode()
            req.add_header("Authorization", "Basic " + tok)
        try:
            with urllib.request.urlopen(req, timeout=300) as r:
                return r.read().decode()
        except urllib.error.HTTPError as e:
            raise RuntimeError("ClickHouse HTTP %d: %s" % (e.code, e.read().decode()[:1500]))

    def ensure_schema(self) -> None:
        self._post("CREATE DATABASE IF NOT EXISTS " + self.db)
        self._post(EVENTS_RAW_DDL.format(db=self.db))

    def count_datagen(self) -> int:
        return int(self._post("SELECT count() FROM %s.events_raw WHERE startsWith(event_id, 'dg-') FORMAT TSV" % self.db).strip() or 0)

    def reset(self) -> None:
        self._post("ALTER TABLE %s.events_raw DELETE WHERE startsWith(event_id, 'dg-') SETTINGS mutations_sync = 2" % self.db)

    def insert(self, rows: List[dict], batch: int = 20000) -> None:
        q = "INSERT INTO %s.events_raw (%s) FORMAT JSONEachRow" % (self.db, ", ".join(COLUMNS))
        for i in range(0, len(rows), batch):
            body = "\n".join(json.dumps(r, ensure_ascii=False, separators=(",", ":")) for r in rows[i:i + batch]).encode()
            self._post(q, body)
