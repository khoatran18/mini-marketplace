#!/usr/bin/env python3
"""D3 export: ClickHouse -> file huấn luyện có phiên bản (CSV / Parquet) + manifest.json.

Thiết kế: docs/design/07_DATA_PIPELINE.md §6. Hướng dẫn: scripts/export/README.md.
Chỉ dùng thư viện chuẩn; Parquet cần thêm `pyarrow` (tuỳ chọn).
"""
from __future__ import annotations

import argparse
import bisect
import csv
import datetime as dt
import hashlib
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
from decimal import Decimal

TOOL_VERSION = "1"
TZ_NAME = "Asia/Ho_Chi_Minh"
TZ_OFFSET = dt.timedelta(hours=7)  # múi giờ nền tảng cố định, không DST
UTC = dt.timezone.utc
SPLIT_NAMES = ("train", "valid", "test")
PROFILES = ("otto", "rees46", "orders", "raw")

# Cột KHÔNG BAO GIỜ xuất (PII / định danh thiết bị).
EXCLUDED_COLUMNS = ["ip_hash", "ua_family", "email", "phone"]


class ExportError(Exception):
    pass


# --------------------------------------------------------------------------- thời gian

def parse_when(s: str) -> int:
    """'YYYY-MM-DD' hoặc 'YYYY-MM-DDTHH:MM[:SS]' (giờ Asia/Ho_Chi_Minh) -> epoch ms."""
    s = s.strip()
    for fmt in ("%Y-%m-%d", "%Y-%m-%dT%H:%M", "%Y-%m-%dT%H:%M:%S", "%Y-%m-%d %H:%M:%S"):
        try:
            d = dt.datetime.strptime(s, fmt)
        except ValueError:
            continue
        return int((d - TZ_OFFSET).replace(tzinfo=UTC).timestamp()) * 1000
    raise ExportError(f"thời gian không hợp lệ: {s!r} (dùng YYYY-MM-DD hoặc YYYY-MM-DDTHH:MM:SS, múi giờ {TZ_NAME})")


def fmt_utc_ms(ms: int) -> str:
    d = dt.datetime.fromtimestamp(ms / 1000.0, UTC) if ms % 1000 else dt.datetime.fromtimestamp(ms // 1000, UTC)
    return d.strftime("%Y-%m-%d %H:%M:%S") + ".%03d" % (ms % 1000)


def fmt_utc_s(ms: int) -> str:
    return dt.datetime.fromtimestamp(ms // 1000, UTC).strftime("%Y-%m-%d %H:%M:%S")


def local_date(ms: int) -> str:
    return (dt.datetime.fromtimestamp(ms // 1000, UTC) + TZ_OFFSET).strftime("%Y-%m-%d")


def iso_local(ms: int) -> str:
    return (dt.datetime.fromtimestamp(ms // 1000, UTC) + TZ_OFFSET).strftime("%Y-%m-%dT%H:%M:%S+07:00")


def stable_int(prefix: str, value: str) -> int:
    """Số hoá ổn định (63 bit) từ chuỗi: cùng chuỗi -> cùng số ở mọi lần chạy / máy."""
    h = hashlib.sha256((prefix + ":" + value).encode("utf-8")).digest()
    return int.from_bytes(h[:8], "big") & 0x7FFFFFFFFFFFFFFF


def money(minor: int) -> str:
    sign = "-" if minor < 0 else ""
    minor = abs(int(minor))
    return f"{sign}{minor // 100}.{minor % 100:02d}"


# --------------------------------------------------------------------------- ClickHouse HTTP

class CH:
    def __init__(self, url: str, user: str, password: str, database: str, timeout: int = 600):
        u = urllib.parse.urlsplit(url)
        if u.scheme not in ("http", "https") or not u.hostname:
            raise ExportError(f"URL ClickHouse không hợp lệ: {url!r}")
        self.user = urllib.parse.unquote(u.username) if u.username else user
        self.password = urllib.parse.unquote(u.password) if u.password else password
        netloc = u.hostname + (f":{u.port}" if u.port else "")
        self.base = f"{u.scheme}://{netloc}{u.path.rstrip('/')}/"
        self.database = database
        self.timeout = timeout

    def masked_url(self) -> str:
        u = urllib.parse.urlsplit(self.base)
        auth = f"{self.user}:***@" if self.password else (f"{self.user}@" if self.user and self.user != "default" else "")
        return f"{u.scheme}://{auth}{u.netloc}{u.path}".rstrip("/")

    def _req(self, sql: str, fmt: str):
        q = {"database": self.database, "join_use_nulls": "0", "output_format_json_quote_64bit_integers": "0"}
        body = sql if fmt == "" else sql.rstrip().rstrip(";") + " FORMAT " + fmt
        headers = {"X-ClickHouse-User": self.user or "default"}
        if self.password:
            headers["X-ClickHouse-Key"] = self.password
        req = urllib.request.Request(self.base + "?" + urllib.parse.urlencode(q), data=body.encode("utf-8"), headers=headers, method="POST")
        try:
            return urllib.request.urlopen(req, timeout=self.timeout)
        except urllib.error.HTTPError as e:
            msg = e.read().decode("utf-8", "replace").strip()
            raise ExportError(f"ClickHouse HTTP {e.code}: {msg[:600]}") from None
        except urllib.error.URLError as e:
            raise ExportError(f"không kết nối được ClickHouse ({self.masked_url()}): {e.reason}") from None

    def stream(self, sql: str):
        """Trả về iterator các dòng (list) từ JSONCompactEachRow."""
        with self._req(sql, "JSONCompactEachRow") as resp:
            for line in resp:
                line = line.strip()
                if line:
                    yield json.loads(line)

    def scalar(self, sql: str):
        with self._req(sql, "JSONCompactEachRow") as resp:
            line = resp.readline().strip()
            return json.loads(line)[0] if line else None


# --------------------------------------------------------------------------- SQL

def _recognized_orders_cte() -> str:
    # Định nghĩa doanh thu (ADR-22): COD -> DELIVERED, còn lại -> PAID; thời điểm = lần đầu đạt trạng thái đó.
    return """rec AS (
  SELECT order_id, any(buyer_id) AS buyer_id, any(payment_method) AS pm, any(store_id) AS sid,
         minIf(at, status = 'PAID') AS paid_at, countIf(status = 'PAID') AS np,
         minIf(at, status = 'DELIVERED') AS del_at, countIf(status = 'DELIVERED') AS nd
  FROM fact_order_status FINAL GROUP BY order_id),
recog AS (
  SELECT order_id, buyer_id, sid, pm, if(pm = 'COD', del_at, paid_at) AS rec_at
  FROM rec WHERE if(pm = 'COD', nd > 0, np > 0))"""


def _ords_with_session_cte(a: int, b: int, gap_ms: int) -> str:
    """Đơn được ghi nhận trong [a,b) gắn với phiên cuối cùng (consent=1) của chính buyer trước đó <= gap."""
    return _recognized_orders_cte() + f""",
ords AS (
  SELECT r.order_id AS order_id, r.rec_at AS rec_at, r.buyer_id AS buyer_id, e.s AS s, e.ets AS ets
  FROM (SELECT order_id, buyer_id, rec_at FROM recog
        WHERE rec_at >= fromUnixTimestamp64Milli({a}, 'UTC') AND rec_at < fromUnixTimestamp64Milli({b}, 'UTC')) AS r
  ASOF LEFT JOIN (
    SELECT user_id, ts, ts AS ets, if(session_id != '', session_id, anonymous_id) AS s
    FROM events_raw FINAL WHERE analytics_consent = 1 AND user_id > 0) AS e
  ON r.buyer_id = e.user_id AND r.rec_at >= e.ts
  WHERE e.s != '' AND toUnixTimestamp64Milli(r.rec_at) - toUnixTimestamp64Milli(e.ets) <= {gap_ms})"""


def _event_range(a: int, b: int) -> str:
    return f"ts >= fromUnixTimestamp64Milli({a}, 'UTC') AND ts < fromUnixTimestamp64Milli({b}, 'UTC')"


def sql_otto(a, b, o):
    return f"""WITH {_ords_with_session_cte(a, b, o['order_session_gap_ms'])}
SELECT * FROM (
  SELECT toUnixTimestamp64Milli(ts) AS t, if(session_id != '', session_id, anonymous_id) AS s, product_id AS aid,
         if(event_type = 'product_click', 'clicks', 'carts') AS ty, event_id AS k
  FROM events_raw FINAL
  WHERE analytics_consent = 1 AND product_id > 0 AND event_type IN ('product_click', 'add_to_cart') AND {_event_range(a, b)}
  UNION ALL
  SELECT toUnixTimestamp64Milli(o.rec_at) AS t, o.s AS s, i.product_id AS aid, 'orders' AS ty,
         concat(toString(o.order_id), '-', toString(i.item_id)) AS k
  FROM ords AS o INNER JOIN (SELECT order_id, item_id, product_id FROM fact_order_items FINAL) AS i ON o.order_id = i.order_id
) ORDER BY t, k"""


def sql_rees46(a, b, o):
    return f"""WITH {_ords_with_session_cte(a, b, o['order_session_gap_ms'])},
dim AS (SELECT product_id, category_id AS dcat, brand AS dbrand, price_minor AS dprice FROM dim_products FINAL)
SELECT * FROM (
  SELECT toUnixTimestamp64Milli(e.ts) AS t,
         multiIf(e.event_type = 'product_view', 'view', e.event_type = 'add_to_cart', 'cart', 'remove_from_cart') AS ty,
         e.product_id AS pid, d.dcat AS cat, d.dbrand AS brand, d.dprice AS price_minor,
         e.user_id AS uid, e.anonymous_id AS anon, if(e.session_id != '', e.session_id, e.anonymous_id) AS s, e.event_id AS k
  FROM (SELECT * FROM events_raw FINAL
        WHERE analytics_consent = 1 AND product_id > 0 AND event_type IN ('product_view', 'add_to_cart', 'remove_from_cart')
          AND {_event_range(a, b)}) AS e
  LEFT JOIN dim AS d ON e.product_id = d.product_id
  UNION ALL
  SELECT toUnixTimestamp64Milli(o.rec_at) AS t, 'purchase' AS ty, i.product_id AS pid,
         if(i.category_id > 0, i.category_id, d.dcat) AS cat, d.dbrand AS brand, i.unit_price_minor AS price_minor,
         o.buyer_id AS uid, '' AS anon, o.s AS s,
         concat(toString(o.order_id), '-', toString(i.item_id)) AS k
  FROM ords AS o
  INNER JOIN (SELECT order_id, item_id, product_id, category_id, unit_price_minor FROM fact_order_items FINAL) AS i ON o.order_id = i.order_id
  LEFT JOIN dim AS d ON i.product_id = d.product_id
) ORDER BY t, k"""


def sql_orders(a, b, o):
    return f"""WITH {_recognized_orders_cte()},
consenting AS (SELECT DISTINCT user_id FROM events_raw FINAL WHERE analytics_consent = 1 AND user_id > 0)
SELECT toUnixTimestamp64Milli(r.rec_at) AS t, r.order_id AS order_id, i.item_id AS item_id,
       if(r.buyer_id IN (SELECT user_id FROM consenting), r.buyer_id, 0) AS buyer_id, r.sid AS store_id,
       i.product_id AS product_id, i.category_id AS category_id, i.qty AS qty, i.unit_price_minor AS unit_price_minor,
       r.pm AS payment_method
FROM recog AS r
INNER JOIN (SELECT order_id, item_id, product_id, category_id, qty, unit_price_minor FROM fact_order_items FINAL) AS i ON r.order_id = i.order_id
WHERE r.rec_at >= fromUnixTimestamp64Milli({a}, 'UTC') AND r.rec_at < fromUnixTimestamp64Milli({b}, 'UTC')
ORDER BY t, order_id, item_id"""


RAW_COLS = ["event_id", "event_type", "anonymous_id", "session_id", "user_id", "role", "store_id", "surface", "path",
            "referrer", "product_id", "position", "props", "device_type", "os", "country", "app_version"]


def sql_raw(a, b, o):
    cols = ", ".join(RAW_COLS)
    return f"""SELECT toUnixTimestamp64Milli(ts) AS t, toUnixTimestamp64Milli(ts_server) AS tsrv, {cols}
FROM events_raw FINAL WHERE analytics_consent = 1 AND {_event_range(a, b)}
ORDER BY t, event_id"""


# --------------------------------------------------------------------------- profile: cột + biến đổi
# Mỗi profile: columns [(tên, kiểu)], sql(a,b,opts), transform(row_thô) -> dòng xuất. row_thô[0] = ts (ms) để chia split.

def tr_otto(r):
    t, s, aid, ty, _k = r
    return [stable_int("session", s), int(aid), int(t), ty]


def tr_rees46(r):
    t, ty, pid, cat, brand, price_minor, uid, anon, s, _k = r
    user = int(uid) if int(uid) > 0 else stable_int("anon", anon)
    return [fmt_utc_s(t) + " UTC", ty, int(pid), int(cat or 0), brand or "", int(price_minor or 0), user, stable_int("session", s)]


def tr_orders(r):
    t, order_id, item_id, buyer, store, pid, cat, qty, up, pm = r
    return [iso_local(t), local_date(t), int(order_id), int(item_id), int(buyer), int(store), int(pid), int(cat),
            int(qty), int(up), int(up) * int(qty), pm]


def tr_raw(r):
    t, tsrv = r[0], r[1]
    rest = r[2:]
    return [fmt_utc_ms(t), fmt_utc_ms(tsrv)] + list(rest)


# kiểu: int64 | string | money (nội bộ số nguyên 1/100; CSV "123.45", Parquet float64)
PROFILE_DEFS = {
    "otto": dict(
        columns=[("session", "int64"), ("aid", "int64"), ("ts", "int64"), ("type", "string")],
        sql=sql_otto, transform=tr_otto,
        notes="ts = epoch mili-giây UTC; type: clicks|carts|orders; session = số hoá ổn định (sha256) của session_id; "
              "orders gắn vào phiên cuối của buyer (consent=1) trong order_session_gap trước thời điểm ghi nhận đơn."),
    "rees46": dict(
        columns=[("event_time", "string"), ("event_type", "string"), ("product_id", "int64"), ("category_id", "int64"),
                 ("brand", "string"), ("price", "money"), ("user_id", "int64"), ("user_session", "int64")],
        sql=sql_rees46, transform=tr_rees46,
        notes="event_time UTC giây; price đơn vị chính (VND, 2 số lẻ); category_id/brand/price ghép dim_products FINAL "
              "(purchase: giá/danh mục lấy từ dòng đơn); user_id = id tài khoản, hoặc số hoá ổn định của anonymous_id nếu chưa đăng nhập."),
    "orders": dict(
        columns=[("order_ts", "string"), ("order_date", "string"), ("order_id", "int64"), ("item_id", "int64"),
                 ("buyer_id", "int64"), ("store_id", "int64"), ("product_id", "int64"), ("category_id", "int64"),
                 ("qty", "int64"), ("unit_price", "money"), ("line_amount", "money"), ("payment_method", "string")],
        sql=sql_orders, transform=None,
        notes="mỗi dòng = một sản phẩm của một đơn đã ghi nhận doanh thu (PAID online / DELIVERED COD), ts = thời điểm ghi nhận; "
              f"order_date theo {TZ_NAME}; buyer_id = 0 nếu buyer không có consent phân tích; chưa trừ hoàn tiền."),
    "raw": dict(
        columns=[("ts", "string"), ("ts_server", "string"), ("event_id", "string"), ("event_type", "string"),
                 ("anonymous_id", "string"), ("session_id", "string"), ("user_id", "int64"), ("role", "string"),
                 ("store_id", "int64"), ("surface", "string"), ("path", "string"), ("referrer", "string"),
                 ("product_id", "int64"), ("position", "int64"), ("props", "string"), ("device_type", "string"),
                 ("os", "string"), ("country", "string"), ("app_version", "string")],
        sql=sql_raw, transform=tr_raw,
        notes="events_raw FINAL, chỉ analytics_consent=1; bỏ ip_hash, ua_family; ts UTC mili-giây."),
}
PROFILE_DEFS["orders"]["transform"] = tr_orders


# --------------------------------------------------------------------------- split

def parse_split(spec: str | None, a: int, b: int):
    """-> list[(name, lo_ms, hi_ms)] nửa mở [lo,hi), hoặc [] nếu không chia."""
    if not spec:
        return []
    items = {}
    for part in spec.split(","):
        if "=" not in part:
            raise ExportError(f"--split-by-time sai cú pháp: {part!r}")
        k, v = part.split("=", 1)
        k, v = k.strip(), v.strip()
        if k not in SPLIT_NAMES:
            raise ExportError(f"tên split không hợp lệ: {k!r} (chỉ {', '.join(SPLIT_NAMES)})")
        if k in items:
            raise ExportError(f"split {k!r} lặp lại")
        items[k] = v
    names = [n for n in SPLIT_NAMES if n in items]
    ranged = ["…" if ".." in items[n] else "" for n in names]
    if all(ranged):
        out = []
        for n in names:
            lo, hi = items[n].split("..", 1)
            out.append((n, parse_when(lo), parse_when(hi)))
        for n, lo, hi in out:
            if lo >= hi:
                raise ExportError(f"split {n}: khoảng rỗng")
        for (n1, _l1, h1), (n2, l2, _h2) in zip(out, out[1:]):
            if h1 > l2:
                raise ExportError(f"split {n1} và {n2} chồng lấn thời gian (rò rỉ)")
        return out
    if any(ranged):
        raise ExportError("--split-by-time: không trộn tỉ lệ với khoảng ngày")
    try:
        fr = [float(items[n]) for n in names]
    except ValueError:
        raise ExportError("--split-by-time: tỉ lệ phải là số, ví dụ train=0.8,valid=0.1,test=0.1") from None
    if any(f <= 0 for f in fr) or sum(fr) > 1.0 + 1e-9:
        raise ExportError("--split-by-time: tỉ lệ phải > 0 và tổng <= 1")
    span, cum, lo, out = b - a, 0.0, a, []
    for n, f in zip(names, fr):
        cum += f
        hi = b if abs(cum - 1.0) < 1e-9 else a + int(span * cum)
        hi = hi // 1000 * 1000  # cắt ở ranh giới giây: file giây (rees46) cũng không trùng giây giữa hai split
        out.append((n, lo, hi))
        lo = hi
    for n, lo, hi in out:
        if lo >= hi:
            raise ExportError(f"split {n}: khoảng rỗng (khoảng --from/--to quá ngắn?)")
    return out


# --------------------------------------------------------------------------- sink

class CsvSink:
    def __init__(self, path, columns):
        self.path, self.columns = path, columns
        self.f = open(path, "w", newline="", encoding="utf-8")
        self.w = csv.writer(self.f, lineterminator="\n")
        self.w.writerow([c for c, _ in columns])
        self.money_idx = [i for i, (_, t) in enumerate(columns) if t == "money"]

    def write(self, row):
        if self.money_idx:
            row = list(row)
            for i in self.money_idx:
                row[i] = money(row[i])
        self.w.writerow(row)

    def close(self):
        self.f.close()


def _pyarrow():
    try:
        import pyarrow as pa  # noqa
        import pyarrow.parquet as pq  # noqa
        return pa, pq
    except ImportError:
        raise ExportError("--format parquet cần thư viện pyarrow, chưa cài. Cài bằng: pip install pyarrow "
                          "(nên trong venv), hoặc dùng --format csv.") from None


class ParquetSink:
    BATCH = 50000

    def __init__(self, path, columns):
        self.pa, self.pq = _pyarrow()
        self.columns = columns
        typemap = {"int64": self.pa.int64(), "string": self.pa.string(), "money": self.pa.float64()}
        self.schema = self.pa.schema([(c, typemap[t]) for c, t in columns])
        self.writer = self.pq.ParquetWriter(path, self.schema, compression="snappy")
        self.buf = []

    def write(self, row):
        self.buf.append(row)
        if len(self.buf) >= self.BATCH:
            self._flush()

    def _flush(self):
        if not self.buf:
            return
        cols = []
        for i, (_, t) in enumerate(self.columns):
            vals = [r[i] for r in self.buf]
            if t == "money":
                vals = [int(v) / 100.0 for v in vals]
            cols.append(vals)
        self.writer.write_table(self.pa.table(cols, schema=self.schema))
        self.buf = []

    def close(self):
        self._flush()
        self.writer.close()


def sha256_file(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


# --------------------------------------------------------------------------- chạy xuất

def run_export(ch: CH, profile, a, b, fmt, out_dir, split_spec=None, version=None, opts=None, created_at=None):
    if profile not in PROFILE_DEFS:
        raise ExportError(f"profile không hợp lệ: {profile}")
    if fmt not in ("csv", "parquet"):
        raise ExportError(f"format không hợp lệ: {fmt}")
    if fmt == "parquet":
        _pyarrow()  # lỗi sớm, rõ ràng
    if a >= b:
        raise ExportError("--from phải nhỏ hơn --to")
    opts = dict(opts or {})
    opts.setdefault("order_session_gap_ms", 24 * 3600 * 1000)
    pdef = PROFILE_DEFS[profile]
    splits = parse_split(split_spec, a, b)
    os.makedirs(out_dir, exist_ok=True)
    ext = "csv" if fmt == "csv" else "parquet"
    if splits:
        targets = [(n, lo, hi, f"{profile}_{n}.{ext}") for n, lo, hi in splits]
    else:
        targets = [(None, a, b, f"{profile}.{ext}")]
    sinks = {}
    sink_cls = CsvSink if fmt == "csv" else ParquetSink
    for n, lo, hi, fname in targets:
        sinks[fname] = sink_cls(os.path.join(out_dir, fname), pdef["columns"])
    stats = {fname: dict(rows=0, ts_min=None, ts_max=None) for *_, fname in targets}
    los = [t[1] for t in targets]
    dropped = 0
    try:
        for raw in ch.stream(pdef["sql"](a, b, opts)):
            ts = int(raw[0])
            idx = bisect.bisect_right(los, ts) - 1
            if idx < 0 or ts >= targets[idx][2]:
                dropped += 1
                continue
            fname = targets[idx][3]
            sinks[fname].write(pdef["transform"](raw))
            st = stats[fname]
            st["rows"] += 1
            st["ts_min"] = ts if st["ts_min"] is None else min(st["ts_min"], ts)
            st["ts_max"] = ts if st["ts_max"] is None else max(st["ts_max"], ts)
    finally:
        for s in sinks.values():
            s.close()
    files = []
    for n, lo, hi, fname in targets:
        p = os.path.join(out_dir, fname)
        st = stats[fname]
        files.append({
            "name": fname, "split": n, "rows": st["rows"], "bytes": os.path.getsize(p), "sha256": sha256_file(p),
            "range_utc": [fmt_utc_ms(lo), fmt_utc_ms(hi)],
            "ts_min_utc": fmt_utc_ms(st["ts_min"]) if st["ts_min"] is not None else None,
            "ts_max_utc": fmt_utc_ms(st["ts_max"]) if st["ts_max"] is not None else None,
        })
    ver = version or f"snapshot-{local_date(a)}_{local_date(b)}"
    manifest = {
        "manifest_version": 1,
        "tool": f"scripts/export/export.py v{TOOL_VERSION}",
        "version": ver,
        "created_at": (created_at or dt.datetime.now(UTC)).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "profile": profile,
        "format": fmt,
        "range": {"from": iso_local(a), "to_exclusive": iso_local(b), "timezone": TZ_NAME,
                  "from_utc": fmt_utc_ms(a), "to_exclusive_utc": fmt_utc_ms(b)},
        "clickhouse": {"url": ch.masked_url(), "database": ch.database, "user": ch.user},
        "columns": [{"name": c, "type": ("float64" if t == "money" else t)} for c, t in pdef["columns"]],
        "column_notes": pdef["notes"],
        "split": [{"name": n, "from_utc": fmt_utc_ms(lo), "to_exclusive_utc": fmt_utc_ms(hi)} for n, lo, hi in splits] or None,
        "rows_outside_split_dropped": dropped,
        "files": files,
        "total_rows": sum(f["rows"] for f in files),
        "filters": {"analytics_consent": 1, "dedupe": "FINAL (ReplacingMergeTree)", "excluded_columns": EXCLUDED_COLUMNS,
                    "order_session_gap_hours": opts["order_session_gap_ms"] / 3600000.0},
    }
    mp = os.path.join(out_dir, "manifest.json")
    with open(mp, "w", encoding="utf-8") as f:
        json.dump(manifest, f, ensure_ascii=False, indent=2, sort_keys=False)
        f.write("\n")
    return manifest


def main(argv=None):
    p = argparse.ArgumentParser(description="Xuất dữ liệu huấn luyện từ ClickHouse (D3).")
    p.add_argument("--from", dest="from_", required=True, help="bắt đầu (gồm), YYYY-MM-DD[THH:MM:SS], giờ Asia/Ho_Chi_Minh")
    p.add_argument("--to", required=True, help="kết thúc (không gồm)")
    p.add_argument("--format", choices=["csv", "parquet"], default="csv")
    p.add_argument("--profile", choices=PROFILES, required=True)
    p.add_argument("--out", required=True, help="thư mục đầu ra")
    p.add_argument("--split-by-time", help="train=0.8,valid=0.1,test=0.1 hoặc train=2026-10-01..2026-10-08,valid=...,test=...")
    p.add_argument("--version", help="ghi đè tên phiên bản (mặc định snapshot-<from>_<to>)")
    p.add_argument("--order-session-gap-hours", type=float, default=24.0,
                   help="otto/rees46: đơn chỉ gắn vào phiên của buyer cách tối đa N giờ (mặc định 24)")
    p.add_argument("--clickhouse-url", default=os.environ.get("CLICKHOUSE_URL", "http://127.0.0.1:8123"))
    p.add_argument("--user", default=os.environ.get("CLICKHOUSE_USER", "default"))
    p.add_argument("--password", default=os.environ.get("CLICKHOUSE_PASSWORD", ""))
    p.add_argument("--db", default=os.environ.get("CLICKHOUSE_DB", "marketplace"))
    args = p.parse_args(argv)
    try:
        ch = CH(args.clickhouse_url, args.user, args.password, args.db)
        m = run_export(ch, args.profile, parse_when(args.from_), parse_when(args.to), args.format, args.out,
                       args.split_by_time, args.version,
                       {"order_session_gap_ms": int(args.order_session_gap_hours * 3600000)})
    except (ExportError, OSError) as e:
        print(f"lỗi: {e}", file=sys.stderr)
        return 2
    for f in m["files"]:
        print(f"{f['name']}: {f['rows']} dòng sha256={f['sha256'][:12]}")
    print(f"manifest: {os.path.join(args.out, 'manifest.json')} (version {m['version']})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
