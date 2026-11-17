#!/usr/bin/env bash
# Kiểm tra công cụ RCA bằng token hệ thống THẬT (ADR-47) sau khi deploy. KHÔNG gọi LLM, không tốn tiền, chỉ đọc.
# Bắt đúng loại lỗi từng làm phiên RCA đầy lỗi công cụ: đường dẫn công cụ /ops/* bị gateway từ chối (400 id không hợp lệ, 403 ngoài /ops/*).
#   1. đổi AI_SYSTEM_KEY lấy token hệ thống (POST /ai/system-token)
#   2. lấy danh sách công cụ (GET /ai/tools) rồi gọi mọi công cụ GET dưới /ops/ không cần tham số: phải trả 2xx
#   3. ops_get_playbook với slug thật lấy từ ops_list_playbooks: phải trả 2xx (slug có chữ, không phải số)
#   4. một đường ngoài /ops/ (GET /admin/analytics/data-health) phải bị 403: chứng tỏ token hệ thống vẫn bị giới hạn
# Khoá hệ thống đọc từ $SMOKE_SYSTEM_KEY; nếu trống thì đọc AI_SYSTEM_KEY trong deploy/.env (đọc trong shell, không in ra).
# Usage: scripts/smoke-rca-tools.sh [base_url]     mặc định $SMOKE_BASE_URL hoặc http://api.marketplace.swarm.localhost
# Thoát 0 khi mọi bước đạt; 1 khi có bước lỗi; 2 khi thiếu khoá.
set -uo pipefail
base="${1:-${SMOKE_BASE_URL:-http://api.marketplace.swarm.localhost}}"
root="$(cd "$(dirname "$0")/.." && pwd)"
key="${SMOKE_SYSTEM_KEY:-}"
if [ -z "$key" ] && [ -f "$root/deploy/.env" ]; then key="$(grep -E '^AI_SYSTEM_KEY=' "$root/deploy/.env" | head -1 | cut -d= -f2-)"; fi
[ -z "$key" ] && { echo "SKIP: chưa có SMOKE_SYSTEM_KEY hoặc AI_SYSTEM_KEY trong deploy/.env"; exit 2; }
export SMOKE_BASE="$base" SMOKE_KEY="$key"
python3 - <<'PY'
import json, os, sys, urllib.request, urllib.error

base, key = os.environ["SMOKE_BASE"].rstrip("/"), os.environ["SMOKE_KEY"]
fail = 0


def call(method, path, headers=None):
    req = urllib.request.Request(base + path, method=method, headers=headers or {})
    try:
        with urllib.request.urlopen(req, timeout=20) as r:
            return r.status, r.read()
    except urllib.error.HTTPError as e:
        return e.code, e.read()
    except Exception as e:  # noqa: BLE001
        return 0, str(type(e).__name__).encode()


def report(ok, label, detail=""):
    global fail
    print(("OK   " if ok else "FAIL ") + label + (f"  {detail}" if detail and not ok else ""))
    fail |= 0 if ok else 1


st, body = call("POST", "/ai/system-token", {"X-System-Key": key, "Accept": "application/json"})
if st != 200:
    report(False, "đổi khoá lấy token hệ thống", f"HTTP {st}")
    sys.exit(1)
auth = {"Authorization": "Bearer " + json.loads(body)["token"], "Accept": "application/json"}
report(True, "token hệ thống")

st, body = call("GET", "/ai/tools", auth)
if st != 200:
    report(False, "GET /ai/tools bằng token hệ thống", f"HTTP {st}")
    sys.exit(1)
data = json.loads(body)
tools = data if isinstance(data, list) else data.get("tools") or data.get("scopes") or []
ops = [t for t in tools if t.get("method") == "GET" and t.get("path", "").startswith("/ops/")]
report(bool(ops), f"có công cụ /ops/ trong danh sách ({len(ops)})")
playbook = None
for t in ops:
    if ":" in t["path"]:
        if t["path"] == "/ops/playbooks/:id":
            playbook = t
        continue  # công cụ có tham số đường dẫn cần giá trị thật: chỉ kiểm playbook ở bước 3
    st, _ = call("GET", t["path"], auth)
    report(200 <= st < 300, f'{t["name"]}  GET {t["path"]}', f"HTTP {st}")

if playbook is not None:
    st, body = call("GET", "/ops/playbooks", auth)
    items = []
    if st == 200:
        d = json.loads(body)
        items = d if isinstance(d, list) else d.get("playbooks") or d.get("items") or []
    slugs = [i.get("id") or i.get("slug") for i in items if isinstance(i, dict)]
    slugs = [s for s in slugs if isinstance(s, str) and not s.isdigit()]
    if not slugs:
        report(False, "ops_get_playbook: không lấy được slug từ ops_list_playbooks", f"HTTP {st}")
    else:
        bad = [s for s in slugs if not (200 <= call("GET", "/ops/playbooks/" + s, auth)[0] < 300)]
        report(not bad, f"ops_get_playbook với {len(slugs)} slug thật", "lỗi: " + ", ".join(bad))

st, _ = call("GET", "/admin/analytics/data-health", auth)
report(st == 403, "đường ngoài /ops/ bị từ chối (403)", f"nhận HTTP {st}")
sys.exit(fail)
PY
