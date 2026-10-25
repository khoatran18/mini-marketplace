import csv
import glob
import hashlib
import json
import os
import shutil
import sys
import tempfile
import unittest
import urllib.parse
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.join(HERE, ".."))
sys.path.insert(0, HERE)

import export as ex  # noqa: E402
import ddl  # noqa: E402
import testdata as td  # noqa: E402

URL = os.environ.get("TEST_CLICKHOUSE_URL")
USER = os.environ.get("TEST_CLICKHOUSE_USER", "default")
PASSWORD = os.environ.get("TEST_CLICKHOUSE_PASSWORD", "")

try:
    import pyarrow  # noqa: F401
    HAVE_PYARROW = True
except ImportError:
    HAVE_PYARROW = False


def http(sql, db=None, data=None):
    q = {}
    if db:
        q["database"] = db
    if data is not None:
        q["query"] = sql
        body = data.encode()
    else:
        body = sql.encode()
    req = urllib.request.Request(URL + "/?" + urllib.parse.urlencode(q), data=body, method="POST",
                                 headers={"X-ClickHouse-User": USER, "X-ClickHouse-Key": PASSWORD})
    urllib.request.urlopen(req, timeout=120).read()


def insert(db, table, rows):
    ms_cols = {"ts", "ts_server", "at", "updated_at"}
    out = []
    for r in rows:
        r = dict(r)
        for c in ms_cols & set(r):
            r[c] = ex.fmt_utc_ms(r[c])
        out.append(json.dumps(r))
    http(f"INSERT INTO {table} FORMAT JSONEachRow", db, "\n".join(out))


def read_csv(path):
    with open(path, newline="", encoding="utf-8") as f:
        return list(csv.reader(f))


class PureTests(unittest.TestCase):
    def test_stable_int_is_fixed(self):
        # giá trị cố định: đổi thuật toán số hoá sẽ làm hỏng khả năng so sánh giữa các lần xuất
        self.assertEqual(ex.stable_int("session", "x"), int.from_bytes(hashlib.sha256(b"session:x").digest()[:8], "big") & 0x7FFFFFFFFFFFFFFF)
        self.assertEqual(ex.stable_int("session", "abc"), ex.stable_int("session", "abc"))
        self.assertNotEqual(ex.stable_int("session", "abc"), ex.stable_int("anon", "abc"))

    def test_money(self):
        self.assertEqual(ex.money(12345), "123.45")
        self.assertEqual(ex.money(5), "0.05")

    def test_split_fraction(self):
        a, b = ex.parse_when("2026-10-01"), ex.parse_when("2026-10-11")
        sp = ex.parse_split("train=0.7,valid=0.2,test=0.1", a, b)
        self.assertEqual([s[0] for s in sp], ["train", "valid", "test"])
        self.assertEqual(sp[0][1], a)
        self.assertEqual(sp[-1][2], b)
        for (_, _, h1), (_, l2, _) in zip(sp, sp[1:]):
            self.assertEqual(h1, l2)
            self.assertEqual(h1 % 1000, 0)

    def test_split_ranges_and_overlap(self):
        a, b = ex.parse_when("2026-10-01"), ex.parse_when("2026-10-11")
        sp = ex.parse_split("train=2026-10-01..2026-10-08,valid=2026-10-08..2026-10-10,test=2026-10-10..2026-10-11", a, b)
        self.assertEqual(len(sp), 3)
        with self.assertRaises(ex.ExportError):
            ex.parse_split("train=2026-10-01..2026-10-09,valid=2026-10-08..2026-10-10", a, b)
        with self.assertRaises(ex.ExportError):
            ex.parse_split("train=0.9,valid=0.3", a, b)
        with self.assertRaises(ex.ExportError):
            ex.parse_split("train=0.5,valid=2026-10-08..2026-10-10", a, b)

    def test_bad_args(self):
        with self.assertRaises(ex.ExportError):
            ex.parse_when("01/10/2026")
        self.assertEqual(ex.main(["--from", "2026-10-01", "--to", "2026-10-02", "--profile", "otto", "--out", "/nonexistent-dir-x/o",
                                  "--clickhouse-url", "http://127.0.0.1:1"]), 2)

    def test_password_masked(self):
        ch = ex.CH("http://u:secretpw@host:8123", "default", "", "db")
        self.assertNotIn("secretpw", ch.masked_url())
        ch = ex.CH("http://host:8123", "tester", "secretpw", "db")
        self.assertNotIn("secretpw", ch.masked_url())

    @unittest.skipIf(HAVE_PYARROW, "pyarrow có sẵn")
    def test_parquet_without_pyarrow_clear_error(self):
        ch = ex.CH("http://127.0.0.1:1", "d", "", "db")
        with self.assertRaises(ex.ExportError) as c:
            ex.run_export(ch, "otto", td.START, td.END, "parquet", tempfile.mkdtemp())
        self.assertIn("pyarrow", str(c.exception))


@unittest.skipUnless(URL, "đặt TEST_CLICKHOUSE_URL (xem tests/run_tests.sh)")
class ClickHouseTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.db = f"export_test_{os.getpid()}"
        http(f"CREATE DATABASE {cls.db}")
        for ddl_sql in ddl.migrations():
            http(ddl_sql, cls.db)
        cls.d = td.generate()
        d = cls.d
        insert(cls.db, "events_raw", d["events"])
        insert(cls.db, "events_raw", d["dups"])
        insert(cls.db, "dim_products", d["dim"])
        insert(cls.db, "fact_order_status", d["status"])
        insert(cls.db, "fact_order_items", d["items"])
        insert(cls.db, "fact_order_items", d["items"])  # ghi đôi: FINAL phải khử
        cls.ch = ex.CH(URL, USER, PASSWORD, cls.db)
        cls.tmp = tempfile.mkdtemp()

    @classmethod
    def tearDownClass(cls):
        try:
            http(f"DROP DATABASE IF EXISTS {cls.db}")
        finally:
            shutil.rmtree(cls.tmp, ignore_errors=True)

    def run_ex(self, profile, name, split=None, fmt="csv", a=td.START, b=td.END):
        out = os.path.join(self.tmp, name)
        shutil.rmtree(out, ignore_errors=True)
        m = ex.run_export(self.ch, profile, a, b, fmt, out, split)
        return out, m

    def check_profile(self, profile, ref):
        out, m = self.run_ex(profile, profile + "_plain")
        rows = read_csv(os.path.join(out, f"{profile}.csv"))
        header, body = rows[0], rows[1:]
        self.assertEqual(header, [c["name"] for c in m["columns"]])
        expected = ref(self.d, td.START, td.END)
        self.assertGreater(len(expected), 20, "dữ liệu mẫu quá nghèo")
        self.assertEqual(len(body), len(expected))
        self.assertEqual(m["files"][0]["rows"], len(expected))
        # so chính xác (đa tập hợp), sau khi chuẩn hoá về chuỗi giống CSV
        types = [c["type"] for c in m["columns"]]

        def norm(r):
            res = []
            for v, t in zip(r, types):
                if t == "float64":
                    res.append(v if isinstance(v, str) else ex.money(v))
                else:
                    res.append(str(v))
            return res

        # tham chiếu 'money' ở dạng số nguyên 1/100
        prof_cols = ex.PROFILE_DEFS[profile]["columns"]
        def norm_ref(r):
            return [ex.money(v) if t == "money" else str(v) for v, (_, t) in zip(r, prof_cols)]
        self.assertEqual(sorted(map(tuple, body)), sorted(tuple(norm_ref(r)) for r in expected))
        # kiểu: cột int64 parse được
        for r in body[:50]:
            for v, t in zip(r, types):
                if t == "int64":
                    int(v)
                elif t == "float64":
                    float(v)
        return out, m, body

    def test_otto(self):
        out, m, body = self.check_profile("otto", td.ref_otto)
        self.assertEqual({r[3] for r in body}, {"clicks", "carts", "orders"})
        ts = [int(r[2]) for r in body]
        self.assertEqual(ts, sorted(ts))

    def test_rees46(self):
        out, m, body = self.check_profile("rees46", td.ref_rees46)
        self.assertEqual({r[1] for r in body}, {"view", "cart", "remove_from_cart", "purchase"})
        self.assertTrue(all(r[0].endswith(" UTC") for r in body))
        # dim_products FINAL: phiên bản mới nhất thắng
        self.assertNotIn("old-brand", {r[4] for r in body})

    def test_orders(self):
        out, m, body = self.check_profile("orders", td.ref_orders)
        self.assertIn("0", {r[4] for r in body}, "buyer không consent phải bị ẩn thành 0")

    def test_raw_columns_and_consent(self):
        out, m = self.run_ex("raw", "raw")
        rows = read_csv(os.path.join(out, "raw.csv"))
        header, body = rows[0], rows[1:]
        for banned in ex.EXCLUDED_COLUMNS:
            self.assertNotIn(banned, header)
        ids = {r[header.index("event_id")] for r in body}
        self.assertEqual(ids, td.ref_raw_ids(self.d, td.START, td.END))
        self.assertEqual(len(body), len(ids), "phải khử trùng (FINAL)")
        noconsent = {e["event_id"] for e in self.d["events"] if not e["analytics_consent"]}
        self.assertTrue(noconsent)
        self.assertFalse(ids & noconsent)

    def test_no_pii_in_any_output(self):
        for p in ex.PROFILES:
            out, _ = self.run_ex(p, p + "_pii", split="train=0.8,valid=0.1,test=0.1")
            for f in glob.glob(os.path.join(out, "*")):
                with open(f, encoding="utf-8") as fh:
                    txt = fh.read()
                for needle in ("IPSECRET", "UASECRET", PASSWORD):
                    self.assertNotIn(needle, txt, f)

    def test_idempotent(self):
        for p in ex.PROFILES:
            o1, m1 = self.run_ex(p, p + "_r1", split="train=0.7,valid=0.15,test=0.15")
            o2, m2 = self.run_ex(p, p + "_r2", split="train=0.7,valid=0.15,test=0.15")
            self.assertEqual([f["sha256"] for f in m1["files"]], [f["sha256"] for f in m2["files"]])
            for f in m1["files"]:
                self.assertEqual(open(os.path.join(o1, f["name"]), "rb").read(), open(os.path.join(o2, f["name"]), "rb").read())

    def test_split_by_time_no_leakage(self):
        for p, tscol, conv in (("otto", "ts", int), ("rees46", "event_time", str), ("orders", "order_ts", str), ("raw", "ts", str)):
            out, m = self.run_ex(p, p + "_split", split="train=0.6,valid=0.2,test=0.2")
            self.assertEqual([f["split"] for f in m["files"]], ["train", "valid", "test"])
            rng = []
            for f in m["files"]:
                rows = read_csv(os.path.join(out, f["name"]))
                col = rows[0].index(tscol)
                vals = [conv(r[col]) for r in rows[1:]]
                self.assertEqual(len(vals), f["rows"])
                rng.append((min(vals), max(vals)) if vals else None)
            self.assertTrue(all(rng), f"{p}: split rỗng {rng}")
            self.assertLess(rng[0][1], rng[1][0], p)
            self.assertLess(rng[1][1], rng[2][0], p)
            # tổng các split = bản không chia
            _, mp = self.run_ex(p, p + "_whole")
            self.assertEqual(sum(f["rows"] for f in m["files"]), mp["total_rows"])
            # kiểm theo ms ở manifest
            self.assertLess(m["files"][0]["ts_max_utc"], m["files"][1]["ts_min_utc"])
            self.assertLess(m["files"][1]["ts_max_utc"], m["files"][2]["ts_min_utc"])

    def test_split_date_ranges(self):
        out, m = self.run_ex("otto", "otto_ranges",
                             split="train=2026-10-01..2026-10-07,valid=2026-10-07..2026-10-09,test=2026-10-09..2026-10-11")
        self.assertEqual(m["rows_outside_split_dropped"], 0)
        _, whole = self.run_ex("otto", "otto_whole2")
        self.assertEqual(m["total_rows"], whole["total_rows"])

    def test_manifest_valid(self):
        out, m = self.run_ex("rees46", "manifest", split="train=0.8,valid=0.1,test=0.1")
        mj = json.load(open(os.path.join(out, "manifest.json"), encoding="utf-8"))
        for k in ("version", "created_at", "range", "profile", "files", "columns", "split", "clickhouse", "filters"):
            self.assertIn(k, mj)
        self.assertEqual(mj["version"], "snapshot-2026-10-01_2026-10-11")
        self.assertEqual(mj["profile"], "rees46")
        self.assertEqual(mj["range"]["timezone"], "Asia/Ho_Chi_Minh")
        self.assertNotIn(PASSWORD, json.dumps(mj))
        for f in mj["files"]:
            p = os.path.join(out, f["name"])
            self.assertEqual(f["sha256"], hashlib.sha256(open(p, "rb").read()).hexdigest())
            self.assertEqual(f["bytes"], os.path.getsize(p))
            self.assertEqual(f["rows"], len(read_csv(p)) - 1)
        self.assertEqual(mj["total_rows"], sum(f["rows"] for f in mj["files"]))
        self.assertEqual({c["type"] for c in mj["columns"]}, {"int64", "string", "float64"})

    def test_range_filter(self):
        a, b = td.START + 3 * td.DAY, td.START + 5 * td.DAY
        out, m = self.run_ex("otto", "otto_sub", a=a, b=b)
        rows = read_csv(os.path.join(out, "otto.csv"))[1:]
        self.assertEqual(len(rows), len(td.ref_otto(self.d, a, b)))
        self.assertTrue(all(a <= int(r[2]) < b for r in rows))

    def test_wrong_password_clear_error(self):
        bad = ex.CH(URL, USER, "wrong-pass", self.db)
        with self.assertRaises(ex.ExportError):
            ex.run_export(bad, "raw", td.START, td.END, "csv", os.path.join(self.tmp, "bad"))

    @unittest.skipUnless(HAVE_PYARROW, "cần pyarrow")
    def test_parquet(self):
        import pyarrow.parquet as pq
        for p in ex.PROFILES:
            o1, m1 = self.run_ex(p, p + "_pq1", fmt="parquet", split="train=0.8,valid=0.1,test=0.1")
            o2, m2 = self.run_ex(p, p + "_pq2", fmt="parquet", split="train=0.8,valid=0.1,test=0.1")
            self.assertEqual([f["sha256"] for f in m1["files"]], [f["sha256"] for f in m2["files"]])
            _, mc = self.run_ex(p, p + "_pqcsv", split="train=0.8,valid=0.1,test=0.1")
            for fp, fc in zip(m1["files"], mc["files"]):
                t = pq.read_table(os.path.join(o1, fp["name"]))
                self.assertEqual(t.num_rows, fp["rows"])
                self.assertEqual(fp["rows"], fc["rows"])
                names = [c["name"] for c in m1["columns"]]
                self.assertEqual(t.column_names, names)
                for field, c in zip(t.schema, m1["columns"]):
                    want = {"int64": "int64", "string": "string", "float64": "double"}[c["type"]]
                    self.assertEqual(str(field.type), want)


if __name__ == "__main__":
    unittest.main()
