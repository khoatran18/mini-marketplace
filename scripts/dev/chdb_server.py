#!/usr/bin/env python3
"""Tiny ClickHouse HTTP-interface emulator on top of chDB (embedded ClickHouse engine).

For TESTS ONLY (CI / sandboxes without Docker): it lets analytics-service run its real SQL against a real
ClickHouse engine. Implements what the service uses: GET/POST /ping, /?query=...&param_x=... (+ body appended to
the query, as for `INSERT ... FORMAT JSONEachRow`), `database` param, HTTP basic auth ignored.

    pip install chdb
    python3 scripts/dev/chdb_server.py --port 8123 --path /var/tmp/chdb-data
    TEST_CLICKHOUSE_URL=http://127.0.0.1:8123 go test ./...      (in services/analytics-service)
"""
import argparse
import threading
import urllib.parse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import chdb.session as chs

LOCK = threading.Lock()  # chDB sessions are not thread safe


class Handler(BaseHTTPRequestHandler):
    session = None

    def log_message(self, *a):  # quiet
        pass

    def _reply(self, code, body, ctype="text/plain; charset=UTF-8"):
        data = body if isinstance(body, bytes) else body.encode()
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def _handle(self):
        url = urllib.parse.urlparse(self.path)
        if url.path == "/ping":
            return self._reply(200, "Ok.\n")
        qs = urllib.parse.parse_qs(url.query, keep_blank_values=True)
        length = int(self.headers.get("Content-Length") or 0)
        body = self.rfile.read(length).decode() if length else ""
        query = qs.get("query", [""])[0]
        sql = (query + "\n" + body) if query else body
        if not sql.strip():
            return self._reply(400, "Empty query")
        params = {k[6:]: v[0] for k, v in qs.items() if k.startswith("param_")}
        db = qs.get("database", [""])[0]
        try:
            with LOCK:
                if db:
                    self.session.query(f"USE {db}")
                res = self.session.query(sql, params=params) if params else self.session.query(sql)
            return self._reply(200, str(res).encode() if res is not None else b"")
        except Exception as e:  # noqa: BLE001
            return self._reply(500, f"{e}\n")

    do_GET = _handle
    do_POST = _handle


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--port", type=int, default=8123)
    ap.add_argument("--path", default="/var/tmp/chdb-data")
    a = ap.parse_args()
    Handler.session = chs.Session(a.path)
    print(f"chdb server on :{a.port} data={a.path}", flush=True)
    ThreadingHTTPServer(("127.0.0.1", a.port), Handler).serve_forever()


if __name__ == "__main__":
    main()
