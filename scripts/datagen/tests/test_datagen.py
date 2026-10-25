"""Chạy: python3 -m unittest discover -s scripts/datagen/tests -t scripts/datagen -v

Không cần hạ tầng: các test "UnitInMemory".
Cần hạ tầng (tự bỏ qua nếu thiếu env):
  DATAGEN_TEST_PSQL    ví dụ  "docker exec -i dg-pg psql -U postgres"
  DATAGEN_TEST_CH_URL  ví dụ  http://127.0.0.1:18123
"""
import hashlib
import json
import os
import sys
import unittest
import urllib.request
import urllib.parse
import uuid

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.dirname(HERE))
sys.path.insert(0, HERE)

import datagen  # noqa: E402
from mmdatagen import pg as pgmod  # noqa: E402
from mmdatagen.util import lit, Money  # noqa: E402
import helpers  # noqa: E402

import tempfile  # noqa: E402
MAP = os.path.join(tempfile.gettempdir(), "dg_test_map.csv")
FIX = os.path.join(os.path.dirname(HERE), "fixtures")
SMALL = ["--seed", "11", "--start", "2026-08-01", "--end", "2026-08-20", "--buyers", "40", "--sellers", "3",
         "--products", "30", "--sessions-per-day", "40", "--visitors", "60"]
RR = ["--rr-dir", FIX, "--seed", "6", "--start", "2026-08-01", "--end", "2026-08-31"]


def gen(mode, extra):
    args = datagen.build_parser().parse_args([mode] + extra + ["--dry-run"])
    out, events, mapping, products = datagen.generate(args)
    return out, events, mapping


def digest(out, events):
    h = hashlib.sha256(pgmod.sql_script(out).encode())
    h.update(json.dumps(events, sort_keys=True).encode())
    return h.hexdigest()


class UnitInMemory(unittest.TestCase):
    def test_lit(self):
        self.assertEqual(lit("o'brien"), "'o''brien'")
        self.assertEqual(lit(None), "NULL")
        self.assertEqual(lit(Money(12345600)), "123456.00")
        self.assertEqual(lit({"a": "x'y"}), "'{\"a\":\"x''y\"}'::jsonb")

    def test_same_seed_same_data(self):
        a = digest(*gen("synthetic", SMALL)[:2])
        b = digest(*gen("synthetic", SMALL)[:2])
        self.assertEqual(a, b)
        c = digest(*gen("synthetic", ["--seed", "12"] + SMALL[2:])[:2])
        self.assertNotEqual(a, c)

    def test_prefixes_and_ids(self):
        out, events, _ = gen("synthetic", SMALL)
        self.assertTrue(all(a["username"].startswith("dg_") for a in out["accounts"]))
        self.assertTrue(all(p["sku"].startswith("dg-") for p in out["products"]))
        self.assertTrue(all(c["slug"].startswith("dg-") for c in out["categories"]))
        self.assertTrue(all(c["id"].startswith("co_dg") for c in out["checkouts"]))
        self.assertTrue(all(e["event_id"].startswith("dg-") for e in events))
        self.assertEqual(len({e["event_id"] for e in events}), len(events))
        for t in ("orders", "products", "payments", "inventory_ledgers"):
            ids = [r["id"] for r in out[t]]
            self.assertEqual(len(ids), len(set(ids)))

    def test_semantics_in_memory(self):
        out, events, _ = gen("synthetic", SMALL)
        by_order = {}
        for h in out["order_status_histories"]:
            by_order.setdefault(h["order_id"], []).append(h)
        pay_by_co = {p["checkout_id"]: p for p in out["payments"]}
        statuses = set()
        for o in out["orders"]:
            statuses.add(o["status"])
            hist = by_order[o["id"]]
            self.assertEqual(hist[0]["to_status"], "PENDING")
            self.assertEqual(hist[-1]["to_status"] if hist[-1]["reason"] != "return_rejected" else "DELIVERED", o["status"])
            self.assertTrue(all(a["at"] <= b["at"] for a, b in zip(hist, hist[1:])))
            if o["payment_method"] == "COD":
                self.assertNotIn(o["checkout_id"], pay_by_co)
            else:
                self.assertIn(o["checkout_id"], pay_by_co)
        # có đủ các nhánh nghiệp vụ chính
        for s in ("DELIVERED", "CANCELED"):
            self.assertIn(s, statuses)
        self.assertTrue(any(o["payment_method"] == "COD" for o in out["orders"]))
        self.assertTrue(out["payments"] and out["refunds"] is not None)

    def test_events_schema(self):
        _, events, _ = gen("synthetic", SMALL)
        from mmdatagen.events import COLUMNS
        types = set()
        for e in events:
            self.assertEqual(list(e.keys()), COLUMNS)
            types.add(e["event_type"])
            if e["event_type"] == "impression":
                self.assertGreaterEqual(e["position"], 1)
                self.assertTrue(e["surface"])
            if not e["analytics_consent"]:
                self.assertEqual((e["event_type"], e["anonymous_id"], e["user_id"]), ("page_view", "", 0))
            else:
                self.assertTrue(e["session_id"] and e["anonymous_id"])
            self.assertLessEqual(e["ts"], e["ts_server"])
            self.assertTrue("2026-07-31" <= e["ts"][:10] <= "2026-08-20")
        for t in ("page_view", "impression", "product_click", "product_view", "add_to_cart", "checkout_start"):
            self.assertIn(t, types)
        self.assertIn(0, {e["analytics_consent"] for e in events})

    def test_retailrocket_fixture(self):
        a = gen("retailrocket", RR)
        b = gen("retailrocket", RR)
        self.assertEqual(digest(a[0], a[1]), digest(b[0], b[1]))
        out, events, mapping = a
        kinds = {m[0] for m in mapping}
        self.assertEqual(kinds, {"category", "item", "visitor"})
        # 7 dòng transaction trong fixture -> 4 giao dịch -> 4 checkout; view/addtocart -> sự kiện, transaction không thành sự kiện
        self.assertEqual(len(out["checkouts"]), 4)
        self.assertEqual({e["event_type"] for e in events}, {"product_view", "add_to_cart"})
        lines = open(os.path.join(FIX, "events.csv")).read().splitlines()
        n_rows = len(lines) - 1
        n_tx_rows = sum(1 for l in lines if ",transaction," in l)
        self.assertEqual(len(events), n_rows - n_tx_rows)
        # itemid -> product id qua ánh xạ; sản phẩm 355908 được mua 2 lần trong một giao dịch => quantity 2
        imap = {m[1]: int(m[2]) for m in mapping if m[0] == "item"}
        q = [i["quantity"] for i in out["order_items"] if i["product_id"] == imap["355908"]]
        self.assertIn(2, q)
        # thời gian được co giãn vào [start, end]
        self.assertTrue("2026-07-31" <= min(e["ts"] for e in events)[:10] and max(e["ts"] for e in events)[:10] <= "2026-08-31")
        # danh mục mới nhất của 355908 là 809 (giá trị property có timestamp lớn hơn thắng)
        cat809 = [c for c in out["categories"] if c["slug"] == "dg-rr-809"][0]["id"]
        p = [p for p in out["products"] if p["sku"] == "dg-rr-355908"][0]
        self.assertEqual(p["category_id"], cat809)


@unittest.skipUnless(helpers.PSQL, "cần DATAGEN_TEST_PSQL")
class PostgresIntegration(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.dbs = []

    @classmethod
    def tearDownClass(cls):
        for d in cls.dbs:
            helpers.drop_db(d)

    def newdb(self):
        d = helpers.create_db()
        self.dbs.append(d)
        return d

    def load(self, db, mode, extra, *more):
        rc = datagen.main([mode] + extra + ["--psql", helpers.psql_cmd_for(db), "--skip-ch",
                                            "--map-file", MAP] + list(more))
        self.assertEqual(rc, 0)

    def table_hashes(self, db):
        res = {}
        for t in pgmod.TABLE_ORDER:
            res[t] = helpers.scalar(db, "SELECT count(*)||':'||coalesce(md5(string_agg(x::text, '|' ORDER BY x::text)),'') FROM %s x" % t)
        return res

    def test_load_twice_identical_and_constraints(self):
        d1, d2 = self.newdb(), self.newdb()
        self.load(d1, "synthetic", SMALL)
        self.load(d2, "synthetic", SMALL)
        h1, h2 = self.table_hashes(d1), self.table_hashes(d2)
        self.assertEqual(h1, h2)
        self.assertTrue(all(not v.startswith("0:") for k, v in h1.items()), h1)
        self.check_business(d1)

    def check_business(self, db, lo="2026-07-31", hi="2026-08-21"):
        def zero(sql, msg):
            self.assertEqual(helpers.scalar(db, sql), "0", msg)

        # tiền khớp
        zero("SELECT count(*) FROM orders WHERE total_price <> subtotal + shipping_fee", "total = subtotal + ship")
        zero("SELECT count(*) FROM orders o WHERE subtotal <> (SELECT coalesce(sum(line_total),0) FROM order_items i WHERE i.order_id=o.id)", "subtotal = sum(line)")
        zero("SELECT count(*) FROM order_items WHERE line_total <> price * quantity", "line_total")
        zero("SELECT count(*) FROM checkouts c WHERE total <> (SELECT sum(total_price) FROM orders o WHERE o.checkout_id=c.id)", "checkout.total")
        zero("SELECT count(*) FROM payments p WHERE amount_minor <> (SELECT round(sum(total_price)*100) FROM orders o WHERE o.checkout_id=p.checkout_id)", "payment.amount")
        zero("SELECT count(*) FROM payments p WHERE refunded_minor <> coalesce((SELECT sum(amount_minor) FROM refunds r WHERE r.payment_id=p.id),0)", "refunded_minor")
        zero("SELECT count(*) FROM payments WHERE refunded_minor > amount_minor", "refund <= amount")
        # đơn online đã PAID (có lịch sử PAID) -> payment thành công
        zero("""SELECT count(*) FROM orders o WHERE o.payment_method <> 'COD' AND EXISTS (SELECT 1 FROM order_status_histories h WHERE h.order_id=o.id AND h.to_status='PAID')
                AND NOT EXISTS (SELECT 1 FROM payments p WHERE p.checkout_id=o.checkout_id AND p.status IN ('SUCCEEDED','PARTIALLY_REFUNDED','REFUNDED') AND p.paid_at IS NOT NULL AND p.paid_at = o.paid_at)""", "online PAID <-> payment")
        zero("SELECT count(*) FROM orders WHERE payment_method <> 'COD' AND status IN ('SHIPPED','DELIVERED','REFUND_REQUESTED','REFUNDED') AND paid_at IS NULL", "online da giao phai da tra")
        # COD DELIVERED: payment_status PAID, không có dòng payments
        zero("SELECT count(*) FROM orders WHERE payment_method='COD' AND status='DELIVERED' AND payment_status <> 'PAID'", "COD delivered paid")
        zero("SELECT count(*) FROM orders o JOIN payments p ON p.checkout_id=o.checkout_id WHERE o.payment_method='COD'", "COD khong co payment")
        zero("SELECT count(*) FROM orders o WHERE payment_method <> 'COD' AND NOT EXISTS (SELECT 1 FROM payments p WHERE p.checkout_id=o.checkout_id)", "online co payment")
        # mỗi refund thuộc một order của checkout đó, order đã hủy sau PAID hoặc REFUNDED
        zero("""SELECT count(*) FROM refunds r JOIN payments p ON p.id=r.payment_id JOIN orders o ON o.id=r.order_id
                WHERE o.checkout_id <> p.checkout_id OR o.status NOT IN ('CANCELED','REFUNDED') OR o.payment_status <> 'REFUNDED'""", "refund <-> order")
        zero("""SELECT count(*) FROM orders o WHERE o.payment_status='REFUNDED' AND NOT EXISTS (SELECT 1 FROM refunds r WHERE r.order_id=o.id)""", "REFUNDED co refund")
        # tồn kho
        zero("SELECT count(*) FROM products WHERE inventory < 0 OR reserved < 0 OR sold < 0", "ton >= 0")
        zero("SELECT count(*) FROM inventory_ledgers WHERE balance_after < 0", "balance >= 0")
        zero("""SELECT count(*) FROM products p WHERE p.inventory <> coalesce((SELECT sum(delta) FROM inventory_ledgers l WHERE l.product_id=p.id),0)""", "inventory = sum(delta)")
        zero("""SELECT count(*) FROM products p WHERE p.inventory <> coalesce((SELECT balance_after FROM inventory_ledgers l WHERE l.product_id=p.id ORDER BY l.at DESC, l.id DESC LIMIT 1),0)""", "balance cuoi")
        zero("""SELECT count(*) FROM products p WHERE p.reserved <> coalesce((SELECT sum(i.quantity) FROM order_items i JOIN orders o ON o.id=i.order_id
                WHERE i.product_id=p.id AND o.status IN ('PENDING','AWAITING_PAYMENT','CONFIRMED','PAID')),0)""", "reserved = don chua xuat kho")
        zero("""SELECT count(*) FROM products p WHERE p.sold <> coalesce((SELECT sum(i.quantity) FROM order_items i JOIN orders o ON o.id=i.order_id
                WHERE i.product_id=p.id AND EXISTS (SELECT 1 FROM order_status_histories h WHERE h.order_id=o.id AND h.to_status='SHIPPED')),0)""", "sold = don da SHIPPED")
        zero("""SELECT count(*) FROM (SELECT product_id, at, id, sum(delta) OVER (PARTITION BY product_id ORDER BY at, id) run, balance_after FROM inventory_ledgers) z WHERE run <> balance_after""", "balance chay dung")
        # FK logic
        zero("SELECT count(*) FROM order_items i WHERE NOT EXISTS (SELECT 1 FROM orders o WHERE o.id=i.order_id)", "item->order")
        zero("SELECT count(*) FROM order_items i JOIN orders o ON o.id=i.order_id JOIN products p ON p.id=i.product_id WHERE p.seller_id <> o.store_id OR i.store_id <> o.store_id", "item->product->store")
        zero("SELECT count(*) FROM orders o WHERE NOT EXISTS (SELECT 1 FROM accounts a WHERE a.id=o.buyer_id AND a.role='buyer')", "order->buyer")
        zero("SELECT count(*) FROM orders o WHERE NOT EXISTS (SELECT 1 FROM sellers s WHERE s.id=o.store_id)", "order->store")
        zero("SELECT count(*) FROM accounts a WHERE a.role='seller_admin' AND NOT EXISTS (SELECT 1 FROM sellers s WHERE s.id=a.store_id)", "seller account->store")
        zero("SELECT count(*) FROM accounts a WHERE a.role='buyer' AND NOT EXISTS (SELECT 1 FROM buyers b WHERE b.user_id=a.id)", "buyer account->profile")
        zero("SELECT count(*) FROM addresses a WHERE NOT EXISTS (SELECT 1 FROM buyers b WHERE b.user_id=a.user_id)", "address->buyer")
        zero("SELECT count(*) FROM products p WHERE NOT EXISTS (SELECT 1 FROM categories c WHERE c.id=p.category_id)", "product->category")
        zero("SELECT count(*) FROM inventory_ledgers l WHERE NOT EXISTS (SELECT 1 FROM products p WHERE p.id=l.product_id)", "ledger->product")
        zero("SELECT count(*) FROM attempts a WHERE NOT EXISTS (SELECT 1 FROM payments p WHERE p.id=a.payment_id)", "attempt->payment")
        zero("SELECT count(*) FROM orders WHERE created_at < '%s' OR updated_at > '%s'" % (lo, hi), "thoi gian trong khoang")
        zero("SELECT count(*) FROM order_status_histories h WHERE h.at > '%s'" % hi, "history trong khoang")
        # dữ liệu có đủ nhánh
        for st in ("DELIVERED", "CANCELED"):
            self.assertNotEqual(helpers.scalar(db, "SELECT count(*) FROM orders WHERE status='%s'" % st), "0", st)
        self.assertNotEqual(helpers.scalar(db, "SELECT count(*) FROM refunds"), "0")
        self.assertNotEqual(helpers.scalar(db, "SELECT count(*) FROM orders WHERE payment_method='COD' AND status='DELIVERED'"), "0")
        self.assertNotEqual(helpers.scalar(db, "SELECT count(*) FROM orders WHERE payment_method<>'COD' AND status='PAID' OR status='AWAITING_PAYMENT'") , "-1")
        # mật khẩu bcrypt cố định
        self.assertEqual(helpers.scalar(db, "SELECT count(DISTINCT password) FROM accounts"), "1")

    def test_larger_run_constraints(self):
        d = self.newdb()
        big = ["--seed", "3", "--start", "2026-06-01", "--end", "2026-08-31", "--buyers", "300", "--sellers", "5",
               "--products", "80", "--sessions-per-day", "120", "--visitors", "300"]
        self.load(d, "synthetic", big)
        self.check_business(d, "2026-05-31", "2026-09-01")
        st = {r for r in helpers.scalar(d, "SELECT string_agg(DISTINCT status, ',') FROM orders").split(",")}
        self.assertTrue({"DELIVERED", "CANCELED", "EXPIRED", "REFUNDED"} <= st, st)
        ps = set(helpers.scalar(d, "SELECT string_agg(DISTINCT status, ',') FROM payments").split(","))
        self.assertTrue({"SUCCEEDED", "EXPIRED"} <= ps, ps)

    def test_reset_only_removes_datagen(self):
        d = self.newdb()
        helpers.run_psql(d, """
          INSERT INTO accounts (username,password,role,pwd_version,store_id) VALUES ('real_user','x','buyer',1,0);
          INSERT INTO categories (name,slug) VALUES ('Real','real-cat');
          INSERT INTO products (name,price,seller_id,inventory,attributes,sku) VALUES ('Real P',1.00,1,3,'{}','real-sku');
          INSERT INTO sellers (name,bank_account,tax_code,description,phone,address) VALUES ('Real shop','1','1','d','1','a');""")
        before = self.table_hashes(d)
        self.load(d, "synthetic", SMALL)
        self.assertNotEqual(self.table_hashes(d), before)
        # nạp lần hai không có --reset phải từ chối
        with self.assertRaises(SystemExit):
            self.load(d, "synthetic", SMALL)
        rc = datagen.main(["reset", "--psql", helpers.psql_cmd_for(d), "--skip-ch"])
        self.assertEqual(rc, 0)
        self.assertEqual(self.table_hashes(d), before)
        # --reset khi nạp = xoá rồi nạp lại, kết quả giống hệt lần đầu
        self.load(d, "synthetic", SMALL)
        first = self.table_hashes(d)
        self.load(d, "synthetic", SMALL, "--reset")
        self.assertEqual(self.table_hashes(d), first)

    def test_retailrocket_fixture_loads(self):
        d = self.newdb()
        self.load(d, "retailrocket", RR)
        h = self.table_hashes(d)
        self.assertTrue(h["orders"].startswith("6:"), h["orders"])
        self.assertTrue(h["checkouts"].startswith("4:"))
        self.check_business_rr(d)
        datagen.main(["reset", "--psql", helpers.psql_cmd_for(d), "--skip-ch"])
        self.assertTrue(all(v.startswith("0:") for v in self.table_hashes(d).values()))

    def check_business_rr(self, db):
        self.assertEqual(helpers.scalar(db, "SELECT count(*) FROM products WHERE inventory<0 OR reserved<0 OR sold<0"), "0")
        self.assertEqual(helpers.scalar(db, "SELECT count(*) FROM orders WHERE total_price <> subtotal + shipping_fee"), "0")
        self.assertEqual(helpers.scalar(db, "SELECT count(*) FROM products p WHERE p.inventory <> (SELECT coalesce(sum(delta),0) FROM inventory_ledgers l WHERE l.product_id=p.id)"), "0")


@unittest.skipUnless(helpers.PSQL and helpers.CH_URL, "cần DATAGEN_TEST_PSQL và DATAGEN_TEST_CH_URL")
class ClickHouseIntegration(unittest.TestCase):
    def ch(self, sql, body=b""):
        req = urllib.request.Request(helpers.CH_URL + "/?" + urllib.parse.urlencode({"query": sql}), data=body, method="POST")
        with urllib.request.urlopen(req, timeout=60) as r:
            return r.read().decode().strip()

    def setUp(self):
        self.db = "dgtest_" + uuid.uuid4().hex[:8]
        self.ch("CREATE DATABASE " + self.db)
        self.ch(helpers.events_raw_ddl_from_go(self.db))  # DDL thật của analytics-service

    def tearDown(self):
        self.ch("DROP DATABASE IF EXISTS " + self.db)

    def run_cli(self, mode, extra, *more):
        return datagen.main([mode] + extra + ["--skip-pg", "--ch-url", helpers.CH_URL, "--ch-db", self.db,
                                              "--map-file", MAP] + list(more))

    def test_events_counts_and_reset(self):
        _, events, _ = gen("synthetic", SMALL)
        self.assertEqual(self.run_cli("synthetic", SMALL), 0)
        n = lambda w="1": int(self.ch("SELECT count() FROM %s.events_raw WHERE %s" % (self.db, w)))
        self.assertEqual(n(), len(events))
        self.assertEqual(n("startsWith(event_id,'dg-')"), len(events))
        self.assertEqual(n("analytics_consent=0"), sum(1 for e in events if not e["analytics_consent"]))
        self.assertEqual(n("event_type='impression' AND position>=1 AND surface<>''"), sum(1 for e in events if e["event_type"] == "impression"))
        self.assertEqual(n("ts < '2026-07-31 00:00:00' OR ts > '2026-08-21 00:00:00'"), 0)
        self.assertEqual(n("ts_server < ts"), 0)
        # một event "thật" không bị --reset đụng tới
        self.ch("INSERT INTO %s.events_raw (event_id,event_type,ts,ts_server) VALUES ('real-1','page_view',now(),now())" % self.db)
        with self.assertRaises(SystemExit):
            self.run_cli("synthetic", SMALL)
        self.assertEqual(datagen.main(["reset", "--skip-pg", "--ch-url", helpers.CH_URL, "--ch-db", self.db]), 0)
        self.assertEqual(n(), 1)
        self.assertEqual(self.run_cli("synthetic", SMALL, "--reset"), 0)
        self.assertEqual(n(), len(events) + 1)

    def test_retailrocket_events_and_ensure_schema(self):
        db2 = self.db + "_b"
        try:
            rc = datagen.main(["retailrocket"] + RR + ["--skip-pg", "--ch-url", helpers.CH_URL, "--ch-db", db2, "--ch-ensure-schema",
                                                      "--map-file", MAP])
            self.assertEqual(rc, 0)
            _, events, _ = gen("retailrocket", RR)
            self.assertEqual(int(self.ch("SELECT count() FROM %s.events_raw" % db2)), len(events))
            self.assertEqual(self.ch("SELECT count() FROM system.columns WHERE database='%s' AND table='events_raw'" % db2), "23")
        finally:
            self.ch("DROP DATABASE IF EXISTS " + db2)


class CrossCheck(unittest.TestCase):
    @unittest.skipUnless(helpers.PSQL and helpers.CH_URL, "cần cả hai")
    def test_ch_ddl_matches_datagen_copy(self):
        from mmdatagen.ch import EVENTS_RAW_DDL
        norm = lambda s: " ".join(s.split())
        real = helpers.events_raw_ddl_from_go("X").replace("X.events_raw", "events_raw")
        ours = EVENTS_RAW_DDL.format(db="X").replace("X.events_raw", "events_raw")
        self.assertEqual(norm(real).replace("DEFAULT now64(3)", "DEFAULT now64(3)"), norm(ours))


if __name__ == "__main__":
    unittest.main()
