"""Ghi / xoá Postgres bằng SQL trực tiếp, qua một lệnh `psql` (không cần driver Python).

Lệnh psql lấy từ --psql hoặc biến DATAGEN_PSQL, ví dụ:
    psql "host=localhost port=5432 user=postgres password=postgres dbname=marketplace sslmode=disable"
    docker exec -i mm-pg psql -U postgres -d marketplace
"""
from __future__ import annotations

import shlex
import subprocess
from typing import Dict, Iterable, List

from .util import lit

# Thứ tự ghi (cha trước con). Cột lấy từ khoá dict của dòng đầu tiên.
TABLE_ORDER = ["accounts", "sellers", "buyers", "addresses", "categories", "products", "inventory_ledgers", "checkouts",
               "orders", "order_items", "order_status_histories", "payments", "attempts", "refunds"]
CHUNK = 400


def new_out() -> Dict[str, List[dict]]:
    return {t: [] for t in TABLE_ORDER}


def sql_script(out: Dict[str, List[dict]]) -> str:
    parts = ["BEGIN;"]
    for table in TABLE_ORDER:
        rows = out.get(table) or []
        if not rows:
            continue
        cols = list(rows[0].keys())
        head = "INSERT INTO %s (%s) VALUES\n" % (table, ", ".join(cols))
        for i in range(0, len(rows), CHUNK):
            chunk = rows[i:i + CHUNK]
            vals = []
            for r in chunk:
                if list(r.keys()) != cols:
                    raise ValueError("dòng %s có tập cột khác dòng đầu" % table)
                vals.append("(" + ", ".join(lit(r[c]) for c in cols) + ")")
            parts.append(head + ",\n".join(vals) + ";")
    parts.append("COMMIT;")
    return "\n".join(parts) + "\n"


RESET_SQL = r"""
BEGIN;
CREATE TEMP TABLE dg_acc ON COMMIT DROP AS SELECT id, store_id FROM accounts WHERE username LIKE 'dg\_%';
CREATE TEMP TABLE dg_co ON COMMIT DROP AS SELECT id FROM checkouts WHERE id LIKE 'co\_dg%';
CREATE TEMP TABLE dg_ord ON COMMIT DROP AS SELECT id FROM orders WHERE checkout_id IN (SELECT id FROM dg_co);
CREATE TEMP TABLE dg_pay ON COMMIT DROP AS SELECT id FROM payments WHERE checkout_id IN (SELECT id FROM dg_co);
DELETE FROM refunds WHERE payment_id IN (SELECT id FROM dg_pay);
DELETE FROM attempts WHERE payment_id IN (SELECT id FROM dg_pay);
DELETE FROM payments WHERE id IN (SELECT id FROM dg_pay);
DELETE FROM order_status_histories WHERE order_id IN (SELECT id FROM dg_ord);
DELETE FROM order_items WHERE order_id IN (SELECT id FROM dg_ord);
DELETE FROM orders WHERE id IN (SELECT id FROM dg_ord);
DELETE FROM checkouts WHERE id IN (SELECT id FROM dg_co);
DELETE FROM inventory_ledgers WHERE product_id IN (SELECT id FROM products WHERE sku LIKE 'dg-%');
DELETE FROM products WHERE sku LIKE 'dg-%';
DELETE FROM categories WHERE slug LIKE 'dg-%';
DELETE FROM addresses WHERE user_id IN (SELECT id FROM dg_acc);
DELETE FROM buyers WHERE user_id IN (SELECT id FROM dg_acc);
DELETE FROM sellers WHERE id IN (SELECT store_id FROM dg_acc WHERE store_id > 0);
DELETE FROM accounts WHERE id IN (SELECT id FROM dg_acc);
COMMIT;
"""

COUNT_SQL = "SELECT count(*) FROM accounts WHERE username LIKE 'dg\\_%'"


class Psql:
    def __init__(self, cmd: str):
        self.cmd = shlex.split(cmd)

    def _run(self, sql: str, extra: Iterable[str] = ()) -> str:
        p = subprocess.run(self.cmd + ["-X", "-q", "-v", "ON_ERROR_STOP=1"] + list(extra), input=sql, text=True,
                           capture_output=True)
        if p.returncode != 0:
            raise RuntimeError("psql lỗi (%d): %s" % (p.returncode, p.stderr.strip()[:2000]))
        return p.stdout

    def exec(self, sql: str) -> None:
        self._run(sql, ["-o", "/dev/null"])

    def query(self, sql: str) -> List[List[str]]:
        out = self._run(sql, ["-A", "-t", "-F", "\t"])
        return [line.split("\t") for line in out.splitlines() if line.strip() != ""]

    def datagen_account_count(self) -> int:
        return int(self.query(COUNT_SQL)[0][0])

    def reset(self) -> None:
        self.exec(RESET_SQL)
