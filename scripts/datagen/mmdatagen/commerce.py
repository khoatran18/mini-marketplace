"""Vòng đời đơn / thanh toán / hoàn tiền và mô phỏng tồn kho theo thời gian.

Đầu vào: danh sách `Intent` (khách X mua các dòng hàng Y lúc t, bằng phương thức Z).
Đầu ra: các dòng cho checkouts, orders, order_items, order_status_histories, payments, attempts, refunds,
inventory_ledgers và số liệu cuối cùng (inventory / reserved / sold) của từng sản phẩm.

Ngữ nghĩa bám theo docs/design/02_DATA_MODEL.md (máy trạng thái đơn, máy trạng thái payment, sổ cái tồn kho):
  * mọi bước có thời gian > `end` bị cắt, đơn dừng ở trạng thái cuối cùng đã đạt (đơn mới vẫn "đang chạy");
  * đơn online: PENDING -> AWAITING_PAYMENT -> PAID (payment actor) -> SHIPPED -> DELIVERED ...; COD: CONFIRMED -> ...;
  * tiền: số nguyên đơn vị nhỏ (1/100 VND); NUMERIC(18,2) chỉ là cách in;
  * giữ hàng lúc tạo đơn (reserve), trả hàng khi CANCELED/EXPIRED (release), xuất kho khi SHIPPED (sale);
    hết hàng thì seller nhập thêm (restock) ngay trước lần giữ hàng đó.
"""
from __future__ import annotations

import dataclasses
import datetime as dt
import random
from typing import Any, Dict, List, Optional, Tuple

from .util import Money, hex_id

S = dt.timedelta
FREE_SHIP_OVER_MINOR = 500_000 * 100  # giả định: miễn phí vận chuyển từ 500.000 VND (xem README)
SHIP_FLAT_MINOR = 30_000 * 100
PAYMENT_TTL = S(minutes=15)
CARRIERS = ["GHN", "GHTK", "ViettelPost"]
METHODS = ["COD", "MOCK_CARD", "MOCK_WALLET"]
METHOD_W = [0.45, 0.40, 0.15]
DECLINES = ["card_declined", "insufficient_funds", "do_not_honor", "provider_error"]


@dataclasses.dataclass
class Product:
    id: int
    store_id: int
    price_minor: int
    category_id: int
    name: str
    sku: str
    brand: str
    created_at: dt.datetime
    init_stock: int
    status: str = "active"
    description: str = ""
    weight_g: int = 300
    # kết quả mô phỏng
    inventory: int = 0
    reserved: int = 0
    sold: int = 0
    updated_at: Optional[dt.datetime] = None
    n_ledger: int = 0


@dataclasses.dataclass
class Buyer:
    account_id: int
    name: str
    address: Dict[str, Any]


@dataclasses.dataclass
class Intent:
    t: dt.datetime
    buyer: int  # chỉ số trong list buyers
    lines: List[Tuple[int, int]]  # (chỉ số sản phẩm, số lượng)
    method: str
    seq: int = 0


class Ids:
    def __init__(self, base: int):
        self.base = base
        self.n: Dict[str, int] = {}

    def next(self, table: str) -> int:
        self.n[table] = self.n.get(table, 0) + 1
        return self.base + self.n[table]


def choose_method(rng: random.Random) -> str:
    r = rng.random() * sum(METHOD_W)
    for m, w in zip(METHODS, METHOD_W):
        r -= w
        if r < 0:
            return m
    return METHODS[0]


def _step(status, t, actor, actor_id=0, reason=""):
    return {"status": status, "t": t, "actor": actor, "actor_id": actor_id, "reason": reason}


def build(out: Dict[str, List[dict]], ids: Ids, rng: random.Random, products: List[Product], buyers: List[Buyer],
          intents: List[Intent], end: dt.datetime) -> None:
    """Sinh toàn bộ dòng thương mại. `out` là dict bảng -> list dòng (đã có accounts/products... từ trước)."""
    order = sorted(intents, key=lambda i: (i.t, i.seq))
    ops: List[Tuple[dt.datetime, int, int, str, int, int]] = []  # (t, seq, product_idx, kind, qty, order_id)
    opseq = 0

    def op(t, pidx, kind, qty, oid):
        nonlocal opseq
        opseq += 1
        ops.append((t, opseq, pidx, kind, qty, oid))

    for it in order:
        if it.t >= end:
            continue
        buyer = buyers[it.buyer]
        # nhóm dòng theo store, mỗi store một đơn; thứ tự ổn định theo store_id
        by_store: Dict[int, List[Tuple[int, int]]] = {}
        for pidx, q in it.lines:
            by_store.setdefault(products[pidx].store_id, []).append((pidx, q))
        stores = sorted(by_store)
        online = it.method != "COD"
        checkout_id = "co_dg" + hex_id(rng, 9)
        t0 = it.t
        t1 = t0 + S(seconds=1)  # rời PENDING
        expires = t1 + PAYMENT_TTL

        # --- quyết định cấp checkout
        path = "cod"
        n_fail = 0
        t_paid = None
        if online:
            r = rng.random()
            path = "paid" if r < 0.86 else "expire" if r < 0.94 else "fail_final" if r < 0.965 else "cancel_unpaid"
            if path == "paid":
                n_fail = 0 if rng.random() < 0.8 else rng.randint(1, 2)
                t_paid = t1 + S(seconds=20 + 30 * n_fail + rng.randint(0, 300))
            elif path == "cancel_unpaid":
                t_cancel_unpaid = t1 + S(seconds=rng.randint(60, 600))
        fail_codes = [DECLINES[rng.randrange(len(DECLINES))] for _ in range(5)]

        orders_here = []
        co_total = 0
        for sid in stores:
            oid = ids.next("orders")
            lines = by_store[sid]
            sub = sum(products[p].price_minor * q for p, q in lines)
            ship = 0 if sub >= FREE_SHIP_OVER_MINOR else SHIP_FLAT_MINOR
            total = sub + ship
            co_total += total
            steps = [_step("PENDING", t0, "buyer", buyer.account_id, "")]
            if online:
                steps.append(_step("AWAITING_PAYMENT", t1, "system"))
                if path == "paid":
                    steps.append(_step("PAID", t_paid, "payment"))
                    paid_branch(rng, steps, t_paid, products[lines[0][0]].store_id)
                elif path in ("expire", "fail_final"):
                    steps.append(_step("EXPIRED", expires, "system", 0, "payment_timeout"))
                else:
                    steps.append(_step("CANCELED", t_cancel_unpaid, "buyer", buyer.account_id, "changed_mind"))
            else:
                steps.append(_step("CONFIRMED", t1, "system"))
                cod_branch(rng, steps, t1, products[lines[0][0]].store_id)
            # cắt theo end
            steps = [s for s in steps if s["t"] < end]
            orders_here.append((oid, sid, lines, sub, ship, total, steps))

            # thao tác tồn kho
            for p, q in lines:
                op(t0, p, "reserve", q, oid)
            for s in steps:
                if s["status"] in ("CANCELED", "EXPIRED"):
                    for p, q in lines:
                        op(s["t"], p, "release", q, oid)
                elif s["status"] == "SHIPPED":
                    for p, q in lines:
                        op(s["t"], p, "sale", q, oid)

        # --- ghi dòng DB cho checkout
        out["checkouts"].append({"id": checkout_id, "buyer_id": buyer.account_id, "idempotency_key": "dg-" + hex_id(rng, 8),
                                 "payment_method": it.method, "total": Money(co_total), "created_at": t0})
        paid_t = None
        refunds_here = []
        order_ids_here = []
        for oid, sid, lines, sub, ship, total, steps in orders_here:
            order_ids_here.append(oid)
            row = _order_row(oid, checkout_id, buyer, sid, it.method, sub, ship, total, steps, expires if online else None, rng)
            out["orders"].append(row["order"])
            for h in row["histories"]:
                h["id"] = ids.next("order_status_histories")
                out["order_status_histories"].append(h)
            for p, q in lines:
                pr = products[p]
                out["order_items"].append({
                    "id": ids.next("order_items"), "order_id": oid, "product_id": pr.id, "quantity": q,
                    "price": Money(pr.price_minor), "status": "ACTIVE", "created_at": t0, "updated_at": t0,
                    "product_name": pr.name, "sku": pr.sku, "image_url": "", "store_id": sid,
                    "category_id": pr.category_id, "line_total": Money(pr.price_minor * q)})
            for s in steps:
                if s["status"] == "PAID":
                    paid_t = s["t"]
            refunds_here.extend((oid, total, s) for s in steps if _is_refund_step(s, steps))

        if online:
            _payment_rows(out, ids, rng, checkout_id, buyer, it.method, co_total, order_ids_here, t0, expires, path, n_fail,
                          fail_codes, paid_t, refunds_here, orders_here, end,
                          t_cancel_unpaid if path == "cancel_unpaid" else None)

    _simulate_stock(out, ids, rng, products, ops)


def _is_refund_step(s, steps) -> bool:
    if s["status"] == "REFUNDED":
        return True
    if s["status"] == "CANCELED" and any(x["status"] == "PAID" for x in steps):
        return True
    return False


def paid_branch(rng, steps, t_paid, store_id):
    if rng.random() < 0.06:
        by = "buyer" if rng.random() < 0.7 else "seller"
        steps.append(_step("CANCELED", t_paid + S(seconds=rng.randint(300, 20 * 3600)), by, 0,
                           "changed_mind" if by == "buyer" else "out_of_stock"))
        return
    _ship_deliver(rng, steps, t_paid, store_id, refundable=True)


def cod_branch(rng, steps, t1, store_id):
    if rng.random() < 0.08:
        steps.append(_step("CANCELED", t1 + S(seconds=rng.randint(600, 20 * 3600)), "buyer", 0, "changed_mind"))
        return
    _ship_deliver(rng, steps, t1, store_id, refundable=False)


def _ship_deliver(rng, steps, t, store_id, refundable):
    t_ship = t + S(seconds=rng.randint(2 * 3600, 30 * 3600))
    t_del = t_ship + S(seconds=rng.randint(24 * 3600, 4 * 86400))
    steps.append(_step("SHIPPED", t_ship, "seller", store_id))
    steps.append(_step("DELIVERED", t_del, "seller", store_id))
    if refundable and rng.random() < 0.07:
        t_req = t_del + S(seconds=rng.randint(86400, 5 * 86400))
        steps.append(_step("REFUND_REQUESTED", t_req, "buyer", 0, "damaged_item"))
        t_dec = t_req + S(seconds=rng.randint(2 * 3600, 2 * 86400))
        if rng.random() < 0.7:
            steps.append(_step("REFUNDED", t_dec, "seller", store_id, "return_approved"))
        else:
            steps.append(_step("DELIVERED", t_dec, "seller", store_id, "return_rejected"))


def _order_row(oid, checkout_id, buyer, sid, method, sub, ship, total, steps, expires, rng):
    st = steps[-1]["status"]
    ts = {s["status"]: s["t"] for s in steps if s["status"] not in ("DELIVERED",)}
    delivered_at = None
    for s in steps:
        if s["status"] == "DELIVERED":
            delivered_at = delivered_at or s["t"]
    pay_status = "UNPAID"
    for s in steps:
        if s["status"] == "PAID" or (method == "COD" and s["status"] == "DELIVERED"):
            pay_status = "PAID"
    if any(s["status"] == "REFUNDED" for s in steps) or (
            any(s["status"] == "CANCELED" for s in steps) and any(s["status"] == "PAID" for s in steps)):
        pay_status = "REFUNDED"
    canceled = [s for s in steps if s["status"] == "CANCELED"]
    refund_req = [s for s in steps if s["status"] == "REFUND_REQUESTED"]
    rejected = any(s["reason"] == "return_rejected" for s in steps)
    shipped = ts.get("SHIPPED")
    carrier = CARRIERS[oid % 3] if shipped else ""
    order = {
        "id": oid, "buyer_id": buyer.account_id, "status": st, "total_price": Money(total),
        "created_at": steps[0]["t"], "updated_at": steps[-1]["t"], "checkout_id": checkout_id, "store_id": sid,
        "payment_method": method, "payment_status": pay_status, "subtotal": Money(sub), "shipping_fee": Money(ship),
        "shipping_address": buyer.address, "note": "", "expires_at": expires,
        "paid_at": ts.get("PAID"), "shipped_at": shipped, "delivered_at": delivered_at,
        "canceled_at": canceled[0]["t"] if canceled else None,
        "cancel_reason": (canceled[0]["reason"] if canceled else ""),
        "canceled_by": (canceled[0]["actor"] if canceled else ""),
        "carrier": carrier, "tracking_code": ("DG%010d" % oid) if shipped else "",
        "return_reason": refund_req[0]["reason"] if refund_req else "", "return_rejected": rejected,
    }
    histories = []
    prev = ""
    for s in steps:
        histories.append({"order_id": oid, "from_status": prev, "to_status": s["status"], "actor_type": s["actor"],
                          "actor_id": s["actor_id"], "reason": s["reason"], "at": s["t"]})
        prev = s["status"]
    return {"order": order, "histories": histories}


def _payment_rows(out, ids, rng, checkout_id, buyer, method, amount_minor, order_ids, t0, expires, path, n_fail, fail_codes,
                  paid_t, refunds_here, orders_here, end, t_cancel_unpaid):
    pid = ids.next("payments")
    attempts = []
    status = "REQUIRES_ACTION"
    failure = ""
    paid_at = None
    t1 = t0 + S(seconds=1)
    if path == "paid":
        for k in range(n_fail):
            attempts.append(("card_declined" if method == "MOCK_CARD" else "wallet_rejected", "FAILED", fail_codes[k], t1 + S(seconds=10 * (k + 1))))
        if paid_t is not None:
            attempts.append(("success", "SUCCEEDED", "", paid_t))
            status, paid_at = "SUCCEEDED", paid_t
    elif path == "fail_final":
        for k in range(5):
            attempts.append((fail_codes[k], "FAILED", fail_codes[k], t1 + S(seconds=10 * (k + 1))))
    elif path == "expire":
        pass
    elif path == "cancel_unpaid":
        if t_cancel_unpaid is not None and t_cancel_unpaid < end:
            status = "CANCELED"
    attempts = [a for a in attempts if a[3] < end]
    failed = sum(1 for a in attempts if a[1] == "FAILED")
    if status == "REQUIRES_ACTION":
        if path == "fail_final" and failed >= 5:
            status, failure = "FAILED", attempts[-1][2]
        elif path in ("expire", "fail_final") and expires < end:
            status = "EXPIRED"
    refunded = 0
    refs = []
    for oid, total, s in refunds_here:
        refunded += total
        refs.append((oid, total, s))
    if status == "SUCCEEDED" and refunded:
        status = "REFUNDED" if refunded >= amount_minor else "PARTIALLY_REFUNDED"
    times = [t0] + [a[3] for a in attempts] + [r[2]["t"] for r in refs]
    if status == "EXPIRED":
        times.append(expires)
    if status == "CANCELED":
        times.append(t_cancel_unpaid)
    out["payments"].append({
        "id": pid, "checkout_id": checkout_id, "buyer_id": buyer.account_id, "method": method, "status": status,
        "amount_minor": amount_minor, "currency": "VND", "provider": "mockpay", "provider_ref": "mp_dg" + hex_id(rng, 6),
        "failure_code": failure, "card_last4": "4242" if method == "MOCK_CARD" else "", "failed_count": failed,
        "three_ds_passed": False, "order_ids": sorted(order_ids), "expires_at": expires, "paid_at": paid_at,
        "refunded_minor": refunded, "created_at": t0, "updated_at": max(times)})
    for sc, st, code, at in attempts:
        out["attempts"].append({"id": ids.next("attempts"), "payment_id": pid, "scenario": sc, "status": st,
                                "failure_code": code, "at": at})
    for oid, total, s in refs:
        out["refunds"].append({"id": ids.next("refunds"), "payment_id": pid, "order_id": oid, "refund_key": "dg_refund_%d" % oid,
                               "amount_minor": total, "reason": s["reason"] or "order_canceled", "status": "SUCCEEDED", "at": s["t"]})


def _simulate_stock(out, ids, rng, products, ops):
    ops.sort(key=lambda o: (o[0], o[1]))
    for p in products:
        p.inventory, p.reserved, p.sold = p.init_stock, 0, 0
        p.updated_at = p.created_at
        if p.init_stock > 0:
            out["inventory_ledgers"].append({"id": ids.next("inventory_ledgers"), "product_id": p.id, "delta": p.init_stock,
                                             "reason": "initial", "ref_type": "", "ref_id": "", "balance_after": p.init_stock,
                                             "at": p.created_at})
            p.n_ledger = 1

    def ledger(p, t, delta, reason, rt, rid):
        out["inventory_ledgers"].append({"id": ids.next("inventory_ledgers"), "product_id": p.id, "delta": delta, "reason": reason,
                                         "ref_type": rt, "ref_id": rid, "balance_after": p.inventory, "at": t})
        p.updated_at = t
        p.n_ledger += 1

    for t, _, pidx, kind, qty, oid in ops:
        p = products[pidx]
        if kind == "reserve":
            if p.inventory < qty:
                add = max(2 * qty, rng.randint(50, 200))
                p.inventory += add
                ledger(p, t, add, "restock", "", "")
            p.inventory -= qty
            p.reserved += qty
            ledger(p, t, -qty, "reserve", "order", str(oid))
        elif kind == "release":
            p.inventory += qty
            p.reserved -= qty
            ledger(p, t, qty, "release", "order", str(oid))
        elif kind == "sale":
            p.reserved -= qty
            p.sold += qty
            ledger(p, t, 0, "sale", "order", str(oid))


def product_rows(out, products: List[Product]) -> None:
    """Gọi SAU build(): tồn kho cuối cùng đã nằm trong Product."""
    for p in products:
        out["products"].append({
            "id": p.id, "name": p.name, "price": Money(p.price_minor), "seller_id": p.store_id, "inventory": p.inventory,
            "attributes": {}, "sku": p.sku, "description": p.description, "category_id": p.category_id, "brand": p.brand,
            "tags": [], "image_urls": [], "status": p.status, "low_stock_threshold": 5, "weight_g": p.weight_g,
            "reserved": p.reserved, "sold": p.sold, "version": 1 + max(0, p.n_ledger - 1),
            "created_at": p.created_at, "updated_at": p.updated_at, "deleted_at": None})
