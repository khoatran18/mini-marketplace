"""Chế độ `synthetic`: sinh người dùng / store / danh mục / sản phẩm, rồi mô phỏng phiên duyệt web -> giỏ -> đơn."""
from __future__ import annotations

import dataclasses
import datetime as dt
import random
from typing import Dict, List, Optional

from . import commerce
from .commerce import Buyer, Ids, Intent, Product
from .events import Sink, Visitor, new_visitor
from .util import UTC, Weighted, hex_id, local_midnight_utc
from .world import People

# (danh mục cha, [(danh mục con, từ khoá tên, thương hiệu, khoảng giá VND)])
CATALOG = [
    ("Điện thoại & Phụ kiện", [("Điện thoại", ["Điện thoại", "Smartphone"], ["Samsung", "Xiaomi", "Oppo", "Realme"], (2_000_000, 25_000_000)),
                               ("Ốp lưng & Cường lực", ["Ốp lưng", "Kính cường lực"], ["Spigen", "Baseus", "UAG"], (50_000, 600_000)),
                               ("Sạc & Cáp", ["Sạc nhanh", "Cáp USB-C", "Pin dự phòng"], ["Anker", "Baseus", "Ugreen"], (80_000, 1_200_000))]),
    ("Laptop & Máy tính", [("Laptop", ["Laptop", "Ultrabook"], ["Asus", "Dell", "Lenovo", "HP"], (9_000_000, 45_000_000)),
                           ("Chuột & Bàn phím", ["Chuột không dây", "Bàn phím cơ"], ["Logitech", "Razer", "Keychron"], (150_000, 3_500_000)),
                           ("Màn hình", ["Màn hình 24 inch", "Màn hình 27 inch"], ["LG", "Dell", "AOC"], (2_500_000, 12_000_000))]),
    ("Thời trang nam", [("Áo nam", ["Áo thun", "Áo sơ mi", "Áo khoác"], ["Routine", "Coolmate", "Owen"], (120_000, 900_000)),
                        ("Quần nam", ["Quần jean", "Quần kaki", "Quần short"], ["Levi's", "Coolmate", "Routine"], (180_000, 1_100_000)),
                        ("Giày nam", ["Giày sneaker", "Giày da"], ["Biti's", "Nike", "Adidas"], (350_000, 3_500_000))]),
    ("Thời trang nữ", [("Váy & Đầm", ["Váy xòe", "Đầm công sở"], ["Canifa", "Yame", "IVY moda"], (200_000, 1_500_000)),
                       ("Áo nữ", ["Áo kiểu", "Áo len"], ["Canifa", "Yame", "Mango"], (130_000, 800_000)),
                       ("Túi xách", ["Túi tote", "Balo mini"], ["Juno", "Vascara", "Charles"], (250_000, 2_800_000))]),
    ("Nhà cửa & Đời sống", [("Đồ bếp", ["Nồi chiên không dầu", "Bộ nồi inox"], ["Lock&Lock", "Philips", "Sunhouse"], (150_000, 4_500_000)),
                            ("Đồ gia dụng", ["Máy hút bụi", "Quạt đứng"], ["Xiaomi", "Panasonic", "Electrolux"], (400_000, 7_000_000)),
                            ("Trang trí", ["Đèn ngủ", "Khung ảnh"], ["Rạng Đông", "Ikea", "Tikilife"], (60_000, 900_000))]),
    ("Sách & Văn phòng phẩm", [("Sách", ["Sách kỹ năng", "Tiểu thuyết"], ["NXB Trẻ", "Nhã Nam", "Alpha Books"], (45_000, 320_000)),
                               ("Văn phòng phẩm", ["Sổ tay", "Bút gel"], ["Thiên Long", "Campus", "Pentel"], (10_000, 250_000))]),
]
ADJ = ["Cao cấp", "Chính hãng", "Mới 2026", "Bản đặc biệt", "Tiết kiệm", "Phiên bản giới hạn", "Bền đẹp", "Bán chạy"]
HOUR_W = [1, 1, .5, .5, .5, 1, 2, 3, 4, 5, 5, 5, 6, 5, 4, 4, 5, 6, 8, 10, 11, 10, 6, 3]
DOW_W = [1.0, 1.0, 1.0, 1.0, 1.1, 1.3, 1.3]


def slugify(s: str) -> str:
    import unicodedata
    s = s.replace("đ", "d").replace("Đ", "D")
    s = unicodedata.normalize("NFD", s)
    s = "".join(c for c in s if unicodedata.category(c) != "Mn").lower()
    return "".join(c if c.isalnum() else "-" for c in s).strip("-").replace("--", "-")


@dataclasses.dataclass
class Params:
    buyers: int = 200
    sellers: int = 6
    products: int = 120
    sessions_per_day: int = 150
    visitors: int = 400
    consent_rate: float = 0.9


def build_catalog(out, ids: Ids, rng: random.Random, p: Params, store_ids: List[int], start: dt.datetime):
    """Trả (products, category_of_product_index, leaf_slug_list)."""
    leaves = []  # (category_id, slug, nouns, brands, price range, name)
    created = start - dt.timedelta(days=45)
    n = 0
    for pname, children in CATALOG:
        n += 1
        pid = ids.next("categories")
        out["categories"].append({"id": pid, "parent_id": 0, "name": pname, "slug": "dg-" + slugify(pname), "sort": n,
                                  "active": True, "created_at": created, "updated_at": created})
        for i, (cname, nouns, brands, pr) in enumerate(children):
            cid = ids.next("categories")
            slug = "dg-" + slugify(pname) + "-" + slugify(cname)
            out["categories"].append({"id": cid, "parent_id": pid, "name": cname, "slug": slug, "sort": i + 1, "active": True,
                                      "created_at": created, "updated_at": created})
            leaves.append((cid, slug, nouns, brands, pr, cname))
    products: List[Product] = []
    leaf_of: List[int] = []
    per_store_n: Dict[int, int] = {}
    for k in range(p.products):
        li = k % len(leaves)
        cid, slug, nouns, brands, (lo, hi), cname = leaves[li]
        store = store_ids[rng.randrange(len(store_ids))]
        per_store_n[store] = per_store_n.get(store, 0) + 1
        brand = rng.choice(brands)
        noun = rng.choice(nouns)
        # giá log-đều, làm tròn theo bậc cho giống giá thật
        import math
        price = math.exp(rng.uniform(math.log(lo), math.log(hi)))
        step = 1000 if price >= 100_000 else 500
        price = max(step, int(round(price / step)) * step)
        r = rng.random()
        status = "active" if r < 0.92 else "hidden" if r < 0.97 else "draft"
        name = "%s %s %s %d" % (noun, brand, rng.choice(ADJ), rng.randint(1, 999))
        pid = ids.next("products")
        products.append(Product(
            id=pid, store_id=store, price_minor=price * 100, category_id=cid, name=name,
            sku="dg-%d-%04d" % (store - ids.base, per_store_n[store]), brand=brand,
            created_at=created + dt.timedelta(minutes=k), init_stock=rng.randint(20, 300), status=status,
            description="%s %s thuộc nhóm %s (dữ liệu mẫu datagen)." % (noun, brand, cname), weight_g=rng.randint(80, 2500)))
        leaf_of.append(li)
    return products, leaf_of, leaves


def generate(out, ids: Ids, rng: random.Random, p: Params, start_day: dt.date, end_day: dt.date):
    start = local_midnight_utc(start_day)
    end = local_midnight_utc(end_day + dt.timedelta(days=1))  # loại trừ
    people = People(out, ids, rng, start)
    for i in range(p.sellers):
        people.add_seller(i)
    for i in range(p.buyers):
        people.add_buyer("%04d" % (i + 1))
    products, leaf_of, leaves = build_catalog(out, ids, rng, p, people.sellers, start)
    active = [i for i, pr in enumerate(products) if pr.status == "active"]
    order_idx = list(active)
    rng.shuffle(order_idx)
    pop = {pi: 1.0 / (rank + 1) ** 0.9 for rank, pi in enumerate(order_idx)}  # Zipf theo thứ hạng ngẫu nhiên
    by_leaf: Dict[int, List[int]] = {}
    for pi in active:
        by_leaf.setdefault(leaf_of[pi], []).append(pi)
    leaf_w = {li: Weighted([pop[pi] for pi in lst]) for li, lst in by_leaf.items()}
    all_w = Weighted([pop[pi] for pi in active])
    new_w = Weighted([1.0] * len(active))
    buyer_w = Weighted([1.0 / (i + 1) ** 0.6 for i in range(len(people.buyers))]) if people.buyers else None

    sink = Sink(rng)
    intents: List[Intent] = []
    seq = 0
    buyer_vis: Dict[int, Visitor] = {}
    anon_vis: Dict[int, Visitor] = {}
    hour_w = Weighted(HOUR_W)

    def visitor_for(bi: Optional[int], ai: int) -> Visitor:
        if bi is not None:
            if bi not in buyer_vis:
                buyer_vis[bi] = new_visitor(rng, people.buyer_anon[bi], people.buyers[bi].account_id, p.consent_rate)
            return buyer_vis[bi]
        if ai not in anon_vis:
            anon_vis[ai] = new_visitor(rng, "dg-a-" + hex_id(rng, 8), 0, p.consent_rate)
        return anon_vis[ai]

    day = start_day
    while day <= end_day:
        day0 = local_midnight_utc(day)
        n = max(1, int(round(p.sessions_per_day * DOW_W[day.weekday()] * rng.uniform(0.85, 1.15))))
        for _ in range(n):
            hour = hour_w.pick(rng)
            t = day0 + dt.timedelta(seconds=hour * 3600 + rng.randint(0, 3599), milliseconds=rng.randint(0, 999))
            bi = buyer_w.pick(rng) if (buyer_w and rng.random() < 0.35) else None
            v = visitor_for(bi, rng.randrange(p.visitors))
            seq += 1
            _session(sink, rng, v, bi, t, products, by_leaf, leaf_w, leaves, leaf_of, all_w, new_w, active, intents, seq, end)
        day += dt.timedelta(days=1)

    commerce.build(out, ids, rng, products, people.buyers, intents, end)
    commerce.product_rows(out, products)
    return products, sink.rows


def _session(sink, rng, v, bi, t, products, by_leaf, leaf_w, leaves, leaf_of, all_w, new_w, active, intents, seq, end):
    sid = "dg-s-" + hex_id(rng, 6)
    cur = [t]

    def tick(lo=1.0, hi=20.0):
        cur[0] = cur[0] + dt.timedelta(milliseconds=int(rng.uniform(lo, hi) * 1000))
        return cur[0]

    sink.emit(t, "page_view", v, sid, "home_trending", "/", "")
    cart: Dict[int, int] = {}
    loops = 1
    while loops < 4 and rng.random() < 0.55:
        loops += 1
    last_path = "/"
    for _ in range(loops):
        r = rng.random()
        if r < 0.3:
            li = rng.randrange(len(leaves))
            q = rng.choice(leaves[li][2]).lower()
            surface, path = "search_results", "/search?q=" + q.replace(" ", "+")
            pool = by_leaf.get(li) or []
            sink.emit(tick(), "search", v, sid, surface, "/search", last_path, 0, 0, {"q": q, "results": len(pool)})
            shown = [pool[i] for i in leaf_w[li].pick_distinct(rng, 8)] if pool else []
            if not shown:
                shown = [active[i] for i in all_w.pick_distinct(rng, 8)]
        elif r < 0.6:
            li = rng.randrange(len(leaves))
            surface, path = "category_page", "/category/" + leaves[li][1]
            pool = by_leaf.get(li) or []
            sink.emit(tick(), "page_view", v, sid, surface, path, last_path)
            shown = [pool[i] for i in leaf_w[li].pick_distinct(rng, 8)] if pool else [active[i] for i in all_w.pick_distinct(rng, 8)]
        elif r < 0.85:
            surface, path = "home_trending", "/"
            shown = [active[i] for i in all_w.pick_distinct(rng, 8)]
        else:
            surface, path = "home_new", "/"
            shown = [active[i] for i in new_w.pick_distinct(rng, 8)]
        for pos, pi in enumerate(shown, start=1):
            sink.emit(tick(0.05, 0.5), "impression", v, sid, surface, path, "", products[pi].id, pos, {})
        clicks = 0
        for pos, pi in enumerate(shown, start=1):
            if clicks < 2 and rng.random() < 0.28 / (pos ** 0.6):
                clicks += 1
                pr = products[pi]
                sink.emit(tick(), "product_click", v, sid, surface, path, "", pr.id, pos, {})
                ppath = "/products/%d" % pr.id
                sink.emit(tick(0.2, 1.5), "product_view", v, sid, "pdp", ppath, path, pr.id, 0, {})
                sink.emit(tick(5, 60), "dwell", v, sid, "pdp", ppath, path, pr.id, 0, {"ms": rng.randint(3000, 90000)})
                if rng.random() < 0.2:
                    q = rng.choice([1, 1, 1, 2, 3])
                    cart[pi] = min(cart.get(pi, 0) + q, 5)
                    sink.emit(tick(1, 8), "add_to_cart", v, sid, "pdp", ppath, path, pr.id, 0, {"qty": q})
                last_path = ppath
    if cart:
        if bi is None:
            if rng.random() < 0.1:
                pi = next(iter(cart))
                sink.emit(tick(), "remove_from_cart", v, sid, "cart", "/cart", last_path, products[pi].id, 0, {})
            return
        if rng.random() < 0.6:
            sink.emit(tick(), "checkout_start", v, sid, "checkout", "/checkout", "/cart", 0, 0, {"lines": len(cart)})
            if rng.random() < 0.7:
                method = commerce.choose_method(rng)
                if method != "COD":
                    sink.emit(tick(5, 40), "checkout_submit", v, sid, "checkout", "/checkout", "", 0, 0, {"method": method})
                    sink.emit(tick(1, 3), "payment_page_view", v, sid, "checkout", "/payment", "/checkout", 0, 0, {"method": method})
                else:
                    sink.emit(tick(5, 40), "checkout_submit", v, sid, "checkout", "/checkout", "", 0, 0, {"method": method})
                intents.append(Intent(cur[0], bi, sorted(cart.items()), method, seq))
