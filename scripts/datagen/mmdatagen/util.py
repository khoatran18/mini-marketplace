"""Tiện ích chung: thời gian, literal SQL, mẫu ngẫu nhiên có trọng số."""
from __future__ import annotations

import bisect
import datetime as dt
import json
import random
from typing import Any, List, Sequence

UTC = dt.timezone.utc
# Múi giờ nền tảng cố định Asia/Ho_Chi_Minh = UTC+7, không có DST.
VN_OFFSET = dt.timedelta(hours=7)
PREFIX = "dg"  # mọi dữ liệu do datagen tạo mang tiền tố này (username dg_, sku dg-, slug dg-, checkout co_dg, event dg-)


def parse_day(s: str) -> dt.date:
    return dt.date.fromisoformat(s)


def local_midnight_utc(d: dt.date) -> dt.datetime:
    """00:00 giờ Việt Nam của ngày d, đổi sang UTC (aware)."""
    return dt.datetime(d.year, d.month, d.day, tzinfo=UTC) - VN_OFFSET


def pg_ts(t: dt.datetime) -> str:
    return t.astimezone(UTC).strftime("%Y-%m-%d %H:%M:%S.%f+00")


def ch_ts(t: dt.datetime) -> str:
    return t.astimezone(UTC).strftime("%Y-%m-%d %H:%M:%S.") + "%03d" % (t.microsecond // 1000)


def money(minor: int) -> str:
    """Số tiền NUMERIC(18,2) dạng chuỗi từ đơn vị nhỏ (1/100 VND)."""
    sign = "-" if minor < 0 else ""
    m = abs(minor)
    return "%s%d.%02d" % (sign, m // 100, m % 100)


def lit(v: Any) -> str:
    """Literal SQL an toàn (standard_conforming_strings = on, mặc định của Postgres 16)."""
    if v is None:
        return "NULL"
    if isinstance(v, bool):
        return "TRUE" if v else "FALSE"
    if isinstance(v, int):
        return str(v)
    if isinstance(v, Money):
        return v.s
    if isinstance(v, dt.datetime):
        return "'" + pg_ts(v) + "'"
    if isinstance(v, (dict, list)):
        return "'" + json.dumps(v, ensure_ascii=False, separators=(",", ":"), sort_keys=True).replace("'", "''") + "'::jsonb"
    s = str(v).replace("'", "''")
    if "\x00" in s:
        raise ValueError("NUL trong chuỗi")
    return "'" + s + "'"


class Money:
    """Đánh dấu giá trị NUMERIC(18,2) để lit() in đúng dạng 123000.00."""
    __slots__ = ("s",)

    def __init__(self, minor: int):
        self.s = money(minor)


class Weighted:
    """Chọn phần tử theo trọng số (CDF + bisect), tất định theo Random truyền vào."""

    def __init__(self, weights: Sequence[float]):
        self.cum: List[float] = []
        t = 0.0
        for w in weights:
            t += w
            self.cum.append(t)
        self.total = t

    def pick(self, rng: random.Random) -> int:
        return min(bisect.bisect_left(self.cum, rng.random() * self.total), len(self.cum) - 1)

    def pick_distinct(self, rng: random.Random, k: int) -> List[int]:
        k = min(k, len(self.cum))
        out: List[int] = []
        seen = set()
        guard = 0
        while len(out) < k and guard < k * 50:
            guard += 1
            i = self.pick(rng)
            if i not in seen:
                seen.add(i)
                out.append(i)
        return out


def hex_id(rng: random.Random, nbytes: int = 8) -> str:
    return "%0*x" % (nbytes * 2, rng.getrandbits(nbytes * 8))
