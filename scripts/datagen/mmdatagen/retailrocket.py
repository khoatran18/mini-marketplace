"""Chế độ `retailrocket`: nạp events.csv (+ category_tree.csv, item_properties_part*.csv) của bộ RetailRocket.

Ánh xạ (docs/design/07_DATA_PIPELINE.md §4):
  view -> product_view, addtocart -> add_to_cart, transaction -> đơn hàng (checkout/orders/order_items/trạng thái/thanh toán);
  visitorid -> anonymous_id "dg-rr-<visitorid>" và (với người có giao dịch + một phần nhỏ còn lại) tài khoản buyer dg_buyer_rr_<visitorid>;
  itemid -> products.id qua bảng ánh xạ trả về (ghi ra CSV bởi CLI).
RetailRocket không có giá, tên, tồn kho, người bán: tất cả được sinh tất định từ itemid. Thời gian gốc (2015) được co giãn tuyến
tính vào khoảng [start, end] để khớp "lịch sử gần đây".
"""
from __future__ import annotations

import csv
import datetime as dt
import glob
import hashlib
import math
import os
import random
from typing import Dict, List, Optional, Tuple

from . import commerce
from .commerce import Ids, Intent, Product
from .events import Sink, Visitor, new_visitor
from .synthetic import ADJ, slugify
from .util import UTC, hex_id, local_midnight_utc
from .world import People

SESSION_GAP_MS = 30 * 60 * 1000
BRANDS = ["Aurora", "Bamboo", "Cosmo", "Delta", "Echo", "Fuji", "Gaia", "Helio"]


def _h(s: str) -> int:
    return int.from_bytes(hashlib.sha1(s.encode()).digest()[:8], "big")


def _read_csv(path: str):
    with open(path, newline="", encoding="utf-8") as f:
        yield from csv.DictReader(f)


def load_events(path: str) -> List[Tuple[int, int, str, int, Optional[int]]]:
    rows = []
    for i, r in enumerate(_read_csv(path)):
        ev = (r.get("event") or "").strip().lower()
        if ev not in ("view", "addtocart", "transaction"):
            continue
        tx = r.get("transactionid") or ""
        rows.append((int(r["timestamp"]), i, ev, int(r["itemid"]), int(r["visitorid"]), int(float(tx)) if tx.strip() else None))
    rows.sort(key=lambda x: (x[0], x[1]))
    return [(t, v, ev, it, tx) for t, _, ev, it, v, tx in rows]  # (ts_ms, visitor, event, item, tx)


def load_categories(rr_dir: str):
    tree: Dict[int, int] = {}
    p = os.path.join(rr_dir, "category_tree.csv")
    if os.path.exists(p):
        for r in _read_csv(p):
            par = (r.get("parentid") or "").strip()
            tree[int(r["categoryid"])] = int(float(par)) if par else 0
    item_cat: Dict[int, Tuple[int, int]] = {}
    for p in sorted(glob.glob(os.path.join(rr_dir, "item_properties_part*.csv"))):
        for r in _read_csv(p):
            if r.get("property") == "categoryid":
                ts, it = int(r["timestamp"]), int(r["itemid"])
                if it not in item_cat or ts >= item_cat[it][0]:
                    item_cat[it] = (ts, int(float(r["value"])))
    return tree, {k: v[1] for k, v in item_cat.items()}


def generate(out, ids: Ids, rng: random.Random, rr_dir: str, sellers: int, buyer_share: float, start_day: dt.date, end_day: dt.date):
    start = local_midnight_utc(start_day)
    end = local_midnight_utc(end_day + dt.timedelta(days=1))
    span_ms = int((end - start).total_seconds() * 1000) - 1000
    events = load_events(os.path.join(rr_dir, "events.csv"))
    if not events:
        raise SystemExit("events.csv rỗng hoặc không có dòng hợp lệ")
    tree, item_cat = load_categories(rr_dir)
    t_min, t_max = events[0][0], events[-1][0]

    def scale(ts_ms: int) -> dt.datetime:
        frac = 0.0 if t_max == t_min else (ts_ms - t_min) / (t_max - t_min)
        return start + dt.timedelta(milliseconds=int(frac * span_ms))

    mapping: List[Tuple[str, str, str, str]] = []
    people = People(out, ids, rng, start)
    for i in range(sellers):
        people.add_seller(i)

    # --- danh mục: chỉ các danh mục được dùng + tổ tiên của chúng
    items = sorted({e[3] for e in events})
    used = {item_cat[i] for i in items if i in item_cat}
    need = set()
    for c in used:
        while c and c not in need:
            need.add(c)
            c = tree.get(c, 0)
    created = start - dt.timedelta(days=45)
    cat_id: Dict[int, int] = {}
    for n, c in enumerate(sorted(need), start=1):
        cat_id[c] = ids.next("categories")
    for n, c in enumerate(sorted(need), start=1):
        par = tree.get(c, 0)
        out["categories"].append({"id": cat_id[c], "parent_id": cat_id.get(par, 0), "name": "RR danh mục %d" % c,
                                  "slug": "dg-rr-%d" % c, "sort": n, "active": True, "created_at": created, "updated_at": created})
        mapping.append(("category", str(c), str(cat_id[c]), ""))
    unc = ids.next("categories")
    out["categories"].append({"id": unc, "parent_id": 0, "name": "RR chưa phân loại", "slug": "dg-rr-uncategorized", "sort": 999,
                              "active": True, "created_at": created, "updated_at": created})

    # --- sản phẩm
    products: List[Product] = []
    pidx: Dict[int, int] = {}
    for it in items:
        h = _h("item:%d" % it)
        store = people.sellers[h % len(people.sellers)]
        price = math.exp(math.log(20_000) + (h >> 8) % 10_000 / 10_000 * (math.log(5_000_000) - math.log(20_000)))
        price = max(1000, int(round(price / 1000)) * 1000)
        pid = ids.next("products")
        pidx[it] = len(products)
        brand = BRANDS[h % len(BRANDS)]
        products.append(Product(
            id=pid, store_id=store, price_minor=price * 100, category_id=cat_id.get(item_cat.get(it, 0), unc),
            name="Sản phẩm RR %d %s" % (it, ADJ[(h >> 4) % len(ADJ)]), sku="dg-rr-%d" % it, brand=brand,
            created_at=created + dt.timedelta(seconds=len(products)), init_stock=5 + (h >> 16) % 96, status="active",
            description="Mặt hàng RetailRocket itemid=%d (giá/tên giả lập)." % it, weight_g=100 + (h >> 20) % 2000))
        mapping.append(("item", str(it), str(pid), "sku=dg-rr-%d" % it))

    # --- buyers: người có giao dịch + một phần nhỏ còn lại (chọn theo hash của visitorid, ổn định)
    tx_visitors = sorted({e[1] for e in events if e[2] == "transaction"})
    tx_set = set(tx_visitors)
    all_visitors = sorted({e[1] for e in events})
    buyer_of: Dict[int, int] = {}
    for v in all_visitors:
        if v in tx_set or (_h("buyer:%d" % v) % 10_000) < buyer_share * 10_000:
            buyer_of[v] = people.add_buyer("rr_%d" % v)
            mapping.append(("visitor", str(v), str(people.buyers[buyer_of[v]].account_id), "buyer=dg_buyer_rr_%d" % v))
        else:
            mapping.append(("visitor", str(v), "dg-rr-%d" % v, "anonymous_id"))

    # --- sự kiện + giao dịch
    sink = Sink(rng)
    visitors: Dict[int, Visitor] = {}
    sess: Dict[int, Tuple[str, int]] = {}  # visitor -> (session_id, last_ms)
    txs: Dict[int, dict] = {}
    for ts_ms, v, ev, it, tx in events:
        t = scale(ts_ms)
        if ev == "transaction":
            d = txs.setdefault(tx if tx is not None else -(len(txs) + 1), {"t": t, "v": v, "lines": {}})
            d["lines"][pidx[it]] = d["lines"].get(pidx[it], 0) + 1
            continue
        if v not in visitors:
            bi = buyer_of.get(v)
            visitors[v] = new_visitor(rng, "dg-rr-%d" % v, people.buyers[bi].account_id if bi is not None else 0, 1.0)
        s = sess.get(v)
        if s is None or ts_ms - s[1] > SESSION_GAP_MS:
            s = ("dg-s-" + hex_id(rng, 6), ts_ms)
        sess[v] = (s[0], ts_ms)
        pr = products[pidx[it]]
        path = "/products/%d" % pr.id
        if ev == "view":
            sink.emit(t, "product_view", visitors[v], s[0], "pdp", path, "", pr.id, 0, {"src": "retailrocket"})
        else:
            sink.emit(t, "add_to_cart", visitors[v], s[0], "pdp", path, "", pr.id, 0, {"qty": 1, "src": "retailrocket"})
    intents = []
    for n, (k, d) in enumerate(sorted(txs.items(), key=lambda kv: (kv[1]["t"], kv[0]))):
        intents.append(Intent(d["t"], buyer_of[d["v"]], sorted(d["lines"].items()), commerce.choose_method(rng), n))
    commerce.build(out, ids, rng, products, people.buyers, intents, end)
    commerce.product_rows(out, products)
    return products, sink.rows, mapping

