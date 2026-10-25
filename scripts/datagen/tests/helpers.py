"""Hạ tầng test: dựng DB bằng đúng các file migration goose của các service."""
import glob
import os
import re
import subprocess
import shlex
import uuid

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", ".."))
SERVICES = ["auth", "user", "product", "order", "payment"]

# DATAGEN_TEST_PSQL: lệnh psql tới server Postgres 16 (không cần -d), ví dụ "docker exec -i dg-pg psql -U postgres"
PSQL = os.environ.get("DATAGEN_TEST_PSQL", "")
# DATAGEN_TEST_CH_URL: ClickHouse 24.8 HTTP, ví dụ http://127.0.0.1:18123
CH_URL = os.environ.get("DATAGEN_TEST_CH_URL", "")


def goose_up(path):
    sql = open(path, encoding="utf-8").read()
    up = sql.split("-- +goose Down")[0]
    return up


def run_psql(db, sql, base=None):
    cmd = shlex.split(base or PSQL) + ["-X", "-q", "-v", "ON_ERROR_STOP=1", "-d", db]
    p = subprocess.run(cmd, input=sql, text=True, capture_output=True)
    if p.returncode != 0:
        raise RuntimeError(p.stderr)
    return p.stdout


def create_db():
    name = "dgtest_" + uuid.uuid4().hex[:10]
    run_psql("postgres", "CREATE DATABASE %s;" % name)
    for s in SERVICES:
        files = sorted(glob.glob(os.path.join(ROOT, "services", s + "-service", "migrations", "*.sql")))
        assert files, "không thấy migration của " + s
        for f in files:
            run_psql(name, goose_up(f))
    return name


def drop_db(name):
    run_psql("postgres", "DROP DATABASE IF EXISTS %s WITH (FORCE);" % name)


def psql_cmd_for(db):
    return PSQL + " -d " + db



def scalar(db, sql):
    cmd = shlex.split(PSQL) + ["-X", "-q", "-A", "-t", "-v", "ON_ERROR_STOP=1", "-d", db, "-c", sql]
    p = subprocess.run(cmd, capture_output=True, text=True)
    if p.returncode != 0:
        raise RuntimeError(p.stderr)
    return p.stdout.strip()


def events_raw_ddl_from_go(db):
    """Lấy DDL events_raw thật từ analytics-service/internal/store/schema.go (migration 1)."""
    src = open(os.path.join(ROOT, "services", "analytics-service", "internal", "store", "schema.go"), encoding="utf-8").read()
    m = re.search(r"CREATE TABLE IF NOT EXISTS events_raw \(.*?INTERVAL %\[2\]d MONTH", src, re.S)
    assert m, "không tìm thấy DDL events_raw trong schema.go"
    ddl = m.group(0).replace("%[1]s", "DateTime64(3, 'UTC')").replace("%[2]d", "13")
    return ddl.replace("events_raw", db + ".events_raw", 1)
