"""Dữ liệu mẫu tự sinh (deterministic) + bản tham chiếu độc lập để so với kết quả export."""
import random

import export as ex

DAY = 86400 * 1000
START = ex.parse_when("2026-10-01")
END = ex.parse_when("2026-10-11")
GAP = 24 * 3600 * 1000
TYPES = ["product_view", "product_click", "add_to_cart", "remove_from_cart", "page_view", "search"]
NO_CONSENT_USERS = {4, 8, 12}


def generate(seed=7):
    rnd = random.Random(seed)
    events, n = [], 0
    for s in range(60):
        uid = rnd.choice([0, 0] + list(range(1, 13)))
        if uid:
            consent = 0 if uid in NO_CONSENT_USERS else 1
            anon = f"anon-u{uid}"
        else:
            anon = f"anon-{s}"
            consent = 0 if s % 5 == 0 else 1
        sid = f"sess-{s}"
        t = START + rnd.randrange(0, 9 * DAY)
        for _ in range(rnd.randint(4, 14)):
            t += rnd.randrange(1000, 600000)
            n += 1
            events.append(dict(event_id=f"ev-{n}", event_type=rnd.choice(TYPES), ts=t, ts_server=t + 50, anonymous_id=anon,
                               session_id=sid if s % 9 else "", user_id=uid, role="buyer" if uid else "anonymous",
                               store_id=0, surface="pdp", path=f"/p/{rnd.randint(1, 15)}", referrer="",
                               product_id=rnd.choice([0] + list(range(1, 16))), position=rnd.randint(0, 5), props="{}",
                               device_type="mobile", os="android", country="VN", ua_family="UASECRET",
                               analytics_consent=consent, ip_hash=f"IPSECRET{n}", app_version="1.0"))
    dups = [dict(e, ts_server=e["ts_server"] + 1000) for e in events[:6]]  # trùng event_id => FINAL phải khử
    dim = []
    for pid in range(1, 14):  # 14,15 không có trong dim
        dim.append(dict(product_id=pid, store_id=1, name=f"P{pid}", sku=f"S{pid}", category_id=100 + pid % 4,
                        brand="old-brand", price_minor=1000, status="active", updated_at=START))
        dim.append(dict(product_id=pid, store_id=1, name=f"P{pid}", sku=f"S{pid}", category_id=100 + pid % 4,
                        brand=f"brand{pid % 3}", price_minor=1999 * 100 + pid * 7, status="active", updated_at=START + DAY))
    status, items, item_id = [], [], 0
    by_user = {}
    for e in events:
        if e["analytics_consent"] and e["user_id"]:
            by_user.setdefault(e["user_id"], []).append(e["ts"])
    for oid in range(1, 41):
        buyer = rnd.randint(1, 12)
        tl = by_user.get(buyer)
        if tl and oid % 5:
            t0 = rnd.choice(tl) + rnd.randrange(60000, 20 * 60000)
        else:
            t0 = START + rnd.randrange(0, 9 * DAY)
        pm = "COD" if oid % 3 == 0 else "MOMO"
        base = dict(order_id=oid, checkout_id=f"c{oid}", buyer_id=buyer, store_id=1 + oid % 2, payment_method=pm,
                    payment_status="", subtotal_minor=0, shipping_fee_minor=0, total_minor=0)
        status.append(dict(base, status="CREATED", at=t0))
        if oid % 7 != 0:
            if pm == "COD":
                status.append(dict(base, status="DELIVERED", at=t0 + 2 * DAY // 2))
            else:
                status.append(dict(base, status="PAID", at=t0 + 90000))
                if oid % 4 == 0:
                    status.append(dict(base, status="DELIVERED", at=t0 + DAY))
        for _ in range(rnd.randint(1, 3)):
            item_id += 1
            items.append(dict(order_id=oid, item_id=item_id, store_id=base["store_id"], product_id=rnd.randint(1, 15),
                              category_id=100 + item_id % 4, qty=rnd.randint(1, 3), unit_price_minor=rnd.randint(1, 900) * 100 + 50,
                              at=t0))
    return dict(events=events, dups=dups, dim=dim, status=status, items=items)


# ---- tham chiếu độc lập (Python thuần, cùng định nghĩa nghiệp vụ) ----
def recognized(d):
    first = {}
    for s in d["status"]:
        k = (s["order_id"], s["status"])
        first[k] = min(first.get(k, 1 << 62), s["at"])
    pm = {s["order_id"]: (s["payment_method"], s["buyer_id"], s["store_id"]) for s in d["status"]}
    out = {}
    for oid, (m, buyer, store) in pm.items():
        need = "DELIVERED" if m == "COD" else "PAID"
        if (oid, need) in first:
            out[oid] = dict(rec_at=first[(oid, need)], buyer=buyer, store=store, pm=m)
    return out


def session_of(d, buyer, rec_at):
    best = None
    for e in d["events"]:
        if e["analytics_consent"] == 1 and e["user_id"] == buyer and e["ts"] <= rec_at:
            if best is None or e["ts"] >= best["ts"]:
                best = e
    if best and rec_at - best["ts"] <= GAP:
        return best["session_id"] or best["anonymous_id"]
    return None


def ref_otto(d, a, b):
    rows = []
    for e in d["events"]:
        if e["analytics_consent"] == 1 and e["product_id"] > 0 and e["event_type"] in ("product_click", "add_to_cart") and a <= e["ts"] < b:
            rows.append([ex.stable_int("session", e["session_id"] or e["anonymous_id"]), e["product_id"], e["ts"],
                         "clicks" if e["event_type"] == "product_click" else "carts"])
    for oid, o in recognized(d).items():
        if not (a <= o["rec_at"] < b):
            continue
        s = session_of(d, o["buyer"], o["rec_at"])
        if s is None:
            continue
        for it in d["items"]:
            if it["order_id"] == oid:
                rows.append([ex.stable_int("session", s), it["product_id"], o["rec_at"], "orders"])
    return rows


def dim_latest(d):
    cur = {}
    for r in d["dim"]:
        if r["product_id"] not in cur or r["updated_at"] >= cur[r["product_id"]]["updated_at"]:
            cur[r["product_id"]] = r
    return cur


def ref_rees46(d, a, b):
    dim = dim_latest(d)
    rows = []
    mp = {"product_view": "view", "add_to_cart": "cart", "remove_from_cart": "remove_from_cart"}
    for e in d["events"]:
        if e["analytics_consent"] == 1 and e["product_id"] > 0 and e["event_type"] in mp and a <= e["ts"] < b:
            p = dim.get(e["product_id"])
            user = e["user_id"] or ex.stable_int("anon", e["anonymous_id"])
            rows.append([ex.fmt_utc_s(e["ts"]) + " UTC", mp[e["event_type"]], e["product_id"], p["category_id"] if p else 0,
                         p["brand"] if p else "", p["price_minor"] if p else 0, user,
                         ex.stable_int("session", e["session_id"] or e["anonymous_id"])])
    for oid, o in recognized(d).items():
        if not (a <= o["rec_at"] < b):
            continue
        s = session_of(d, o["buyer"], o["rec_at"])
        if s is None:
            continue
        for it in d["items"]:
            if it["order_id"] == oid:
                p = dim.get(it["product_id"])
                rows.append([ex.fmt_utc_s(o["rec_at"]) + " UTC", "purchase", it["product_id"],
                             it["category_id"] or (p["category_id"] if p else 0), p["brand"] if p else "",
                             it["unit_price_minor"], o["buyer"], ex.stable_int("session", s)])
    return rows


def ref_orders(d, a, b):
    consenting = {e["user_id"] for e in d["events"] if e["analytics_consent"] == 1 and e["user_id"]}
    rows = []
    for oid, o in recognized(d).items():
        if not (a <= o["rec_at"] < b):
            continue
        for it in d["items"]:
            if it["order_id"] == oid:
                rows.append([ex.iso_local(o["rec_at"]), ex.local_date(o["rec_at"]), oid, it["item_id"],
                             o["buyer"] if o["buyer"] in consenting else 0, o["store"], it["product_id"], it["category_id"],
                             it["qty"], it["unit_price_minor"], it["unit_price_minor"] * it["qty"], o["pm"]])
    return rows


def ref_raw_ids(d, a, b):
    return {e["event_id"] for e in d["events"] if e["analytics_consent"] == 1 and a <= e["ts"] < b}
