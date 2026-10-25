#!/usr/bin/env python3
"""D5 datagen: sinh dữ liệu lịch sử cho Postgres (SQL trực tiếp) và ClickHouse events_raw (HTTP).

    python3 scripts/datagen/datagen.py synthetic --seed 1 --start 2026-07-01 --end 2026-09-30 --psql "psql ..." --reset
    python3 scripts/datagen/datagen.py retailrocket --rr-dir scripts/datagen/fixtures --seed 1 --dry-run
    python3 scripts/datagen/datagen.py reset --psql "psql ..."      # chỉ xoá dữ liệu do datagen tạo (tiền tố dg)
Xem README.md.
"""
from __future__ import annotations

import argparse
import csv
import datetime as dt
import json
import os
import random
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from mmdatagen import pg as pgmod, retailrocket, synthetic  # noqa: E402
from mmdatagen.ch import ClickHouse  # noqa: E402
from mmdatagen.commerce import Ids  # noqa: E402
from mmdatagen.util import parse_day  # noqa: E402

HERE = os.path.dirname(os.path.abspath(__file__))


def build_parser() -> argparse.ArgumentParser:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="mode", required=True)

    def common(p, load=True):
        p.add_argument("--psql", default=os.environ.get("DATAGEN_PSQL", ""), help="lệnh psql (hoặc env DATAGEN_PSQL)")
        p.add_argument("--ch-url", default=os.environ.get("CLICKHOUSE_URL", "http://127.0.0.1:8123"))
        p.add_argument("--ch-db", default=os.environ.get("CLICKHOUSE_DB", "marketplace"))
        p.add_argument("--ch-user", default=os.environ.get("CLICKHOUSE_USER", "default"))
        p.add_argument("--ch-password", default=os.environ.get("CLICKHOUSE_PASSWORD", ""))
        p.add_argument("--skip-pg", action="store_true", help="không ghi/xoá Postgres")
        p.add_argument("--skip-ch", action="store_true", help="không ghi/xoá ClickHouse")
        if load:
            today = dt.date.today()
            p.add_argument("--seed", type=int, default=42)
            p.add_argument("--end", default=(today - dt.timedelta(days=1)).isoformat(), help="ngày cuối (giờ VN, gồm cả ngày này)")
            p.add_argument("--start", default=None, help="ngày đầu (mặc định end-59)")
            p.add_argument("--id-base", type=int, default=5_000_000, help="id khoá chính bắt đầu từ đây (tránh dữ liệu thật)")
            p.add_argument("--dry-run", action="store_true", help="sinh và in thống kê, không ghi gì")
            p.add_argument("--reset", action="store_true", help="xoá dữ liệu datagen cũ (tiền tố dg) trước khi ghi")
            p.add_argument("--ch-ensure-schema", action="store_true", help="tạo DB + events_raw nếu chưa có (bản sao migration 1)")
            p.add_argument("--sql-out", help="ghi thêm SQL Postgres ra file này")
            p.add_argument("--events-out", help="ghi thêm events_raw dạng NDJSON ra file này")
            p.add_argument("--map-file", help="ghi bảng ánh xạ CSV (retailrocket); mặc định out/<mode>_map.csv")

    s = sub.add_parser("synthetic", help="nguồn nhân tạo")
    common(s)
    s.add_argument("--buyers", type=int, default=200)
    s.add_argument("--sellers", type=int, default=6)
    s.add_argument("--products", type=int, default=120)
    s.add_argument("--sessions-per-day", type=int, default=150)
    s.add_argument("--visitors", type=int, default=400, help="số khách vãng lai (anonymous) trong hồ")
    s.add_argument("--consent-rate", type=float, default=0.9)

    r = sub.add_parser("retailrocket", help="nạp bộ RetailRocket (events.csv, category_tree.csv, item_properties_part*.csv)")
    common(r)
    r.add_argument("--rr-dir", required=True, help="thư mục chứa events.csv")
    r.add_argument("--sellers", type=int, default=6)
    r.add_argument("--buyer-share", type=float, default=0.1, help="tỉ lệ visitor không giao dịch vẫn có tài khoản buyer")

    x = sub.add_parser("reset", help="chỉ xoá dữ liệu do datagen tạo")
    common(x, load=False)
    return ap


def generate(args):
    """Trả (out, events, mapping, products)."""
    end_day = parse_day(args.end)
    start_day = parse_day(args.start) if args.start else end_day - dt.timedelta(days=59)
    if start_day > end_day:
        raise SystemExit("--start phải <= --end")
    rng = random.Random(args.seed)
    ids = Ids(args.id_base)
    out = pgmod.new_out()
    mapping = []
    if args.mode == "synthetic":
        p = synthetic.Params(args.buyers, args.sellers, args.products, args.sessions_per_day, args.visitors, args.consent_rate)
        products, events = synthetic.generate(out, ids, rng, p, start_day, end_day)
    else:
        products, events, mapping = retailrocket.generate(out, ids, rng, args.rr_dir, args.sellers, args.buyer_share, start_day, end_day)
    events.sort(key=lambda e: (e["ts"], e["event_id"]))
    return out, events, mapping, products


def write_map(path, mapping):
    os.makedirs(os.path.dirname(os.path.abspath(path)), exist_ok=True)
    with open(path, "w", newline="", encoding="utf-8") as f:
        w = csv.writer(f)
        w.writerow(["kind", "source_id", "target", "note"])
        w.writerows(mapping)


def main(argv=None) -> int:
    args = build_parser().parse_args(argv)
    pg = pgmod.Psql(args.psql) if args.psql and not args.skip_pg else None
    ch = None if args.skip_ch else ClickHouse(args.ch_url, args.ch_db, args.ch_user, args.ch_password)

    if args.mode == "reset":
        if pg:
            pg.reset()
            print("Postgres: đã xoá dữ liệu datagen")
        if ch:
            ch.reset()
            print("ClickHouse: đã xoá events_raw có event_id 'dg-*'")
        return 0

    t0 = time.time()
    out, events, mapping, _ = generate(args)
    stats = {t: len(r) for t, r in out.items()}
    stats["events_raw"] = len(events)
    print(json.dumps(stats, ensure_ascii=False))
    if args.sql_out:
        with open(args.sql_out, "w", encoding="utf-8") as f:
            f.write(pgmod.sql_script(out))
    if args.events_out:
        with open(args.events_out, "w", encoding="utf-8") as f:
            for e in events:
                f.write(json.dumps(e, ensure_ascii=False, separators=(",", ":")) + "\n")
    if args.dry_run:
        print("dry-run: không ghi gì (%.1fs)" % (time.time() - t0))
        return 0
    if mapping or args.map_file:
        write_map(args.map_file or os.path.join(HERE, "out", "%s_map.csv" % args.mode), mapping)

    if not pg and not ch:
        raise SystemExit("cần --psql (hoặc DATAGEN_PSQL) và/hoặc ClickHouse; dùng --dry-run để chỉ sinh")
    if args.reset:
        if pg:
            pg.reset()
        if ch:
            ch.reset()
    if pg and pg.datagen_account_count() > 0:
        raise SystemExit("Postgres đã có dữ liệu datagen; chạy lại với --reset")
    if ch:
        if args.ch_ensure_schema:
            ch.ensure_schema()
        if ch.count_datagen() > 0:
            raise SystemExit("ClickHouse đã có events datagen; chạy lại với --reset")
    if pg:
        pg.exec(pgmod.sql_script(out))
        print("Postgres: đã ghi %d dòng" % sum(stats[t] for t in out))
    if ch:
        ch.insert(events)
        print("ClickHouse: đã ghi %d events" % len(events))
    print("xong (%.1fs)" % (time.time() - t0))
    return 0


if __name__ == "__main__":
    sys.exit(main())
