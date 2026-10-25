"""Lấy DDL migration THẬT của analytics-service bằng cách đọc schema.go (schema đổi thì test đổi theo)."""
import os
import re

SCHEMA_GO = os.path.join(os.path.dirname(__file__), "..", "..", "..", "services", "analytics-service", "internal", "store", "schema.go")


def migrations(retention_months=120):
    src = open(SCHEMA_GO, encoding="utf-8").read()
    ts = re.search(r'const ts = "([^"]+)"', src).group(1)
    out = []
    for m in re.finditer(r"fmt\.Sprintf\(`(.*?)`,\s*ts(?:,\s*retentionMonths)?\)", src, re.S):
        ddl = m.group(1).replace("%[1]s", ts).replace("%[2]d", str(retention_months))
        out.append(ddl)
    if len(out) < 7:
        raise RuntimeError(f"chỉ tách được {len(out)} migration từ schema.go")
    return out
