#!/usr/bin/env python3
"""Send synthetic browsing traffic to the gateway's POST /events (DEV ONLY) so the analytics dashboards have data.

    python3 scripts/dev/gen_traffic.py --api https://api.marketplace.swarm.localhost --visitors 200 --products 1-60

Each visitor gets an anonymous_id and one or more sessions: page_view → impressions → product_click/product_view →
(sometimes) add_to_cart → checkout_start; plus some searches (a few with 0 results). Only CLIENT event types are
sent: orders and payments always come from the real order/payment services (place orders through the UI or API).
Self-signed dev certificates are accepted (--insecure, default on).
"""
import argparse, json, random, ssl, time, urllib.request, uuid
from datetime import datetime, timezone

ap = argparse.ArgumentParser()
ap.add_argument("--api", default="http://localhost:8080")
ap.add_argument("--visitors", type=int, default=100)
ap.add_argument("--products", default="1-50", help="product id range, e.g. 1-60")
ap.add_argument("--seed", type=int, default=1)
ap.add_argument("--secure", action="store_true", help="verify TLS certificates")
a = ap.parse_args()
random.seed(a.seed)
lo, hi = (int(x) for x in a.products.split("-"))
ctx = None if a.secure else ssl._create_unverified_context()
QUERIES = ["áo thun", "giày", "tai nghe", "laptop", "bàn phím", "xyzabc", "sách", "balo", "điện thoại", "áo khoác"]
DEVICES = [("mobile", "Android"), ("mobile", "iOS"), ("desktop", "Windows"), ("desktop", "macOS"), ("tablet", "iOS")]


def send(events):
    req = urllib.request.Request(a.api.rstrip("/") + "/events", data=json.dumps({"events": events}).encode(),
                                 headers={"Content-Type": "application/json"}, method="POST")
    with urllib.request.urlopen(req, context=ctx, timeout=10) as r:
        return json.load(r)


def ev(t, aid, sid, dev, path, surface=None, pid=0, pos=0, props=None, consent=True):
    return {"event_id": str(uuid.uuid4()), "event_type": t, "ts_client": datetime.now(timezone.utc).isoformat(),
            "anonymous_id": aid, "session_id": sid, "surface": surface or "", "page": {"path": path, "referrer": random.choice(["", "", "https://google.com/", "https://facebook.com/"])},
            "item": {"product_id": pid, "position": pos}, "props": props or {}, "device": {"type": dev[0], "os": dev[1]},
            "consent": {"analytics": consent}, "app_version": "gen-0.1"}


sent = rejected = 0
for v in range(a.visitors):
    aid, dev, consent = f"a_{uuid.uuid4().hex[:10]}", random.choice(DEVICES), random.random() > 0.1
    for _ in range(random.choice([1, 1, 2, 3])):
        sid, batch = f"s_{uuid.uuid4().hex[:10]}", []
        batch.append(ev("page_view", aid, sid, dev, "/", "home_trending", consent=consent))
        if random.random() < 0.5:
            q = random.choice(QUERIES)
            n = 0 if q == "xyzabc" else random.randint(1, 30)
            batch.append(ev("search", aid, sid, dev, "/search", "search_results", props={"q": q, "result_count": n}, consent=consent))
        for pos in range(random.randint(1, 6)):
            pid = random.randint(lo, hi)
            batch.append(ev("impression", aid, sid, dev, "/", "home_trending", pid, pos, consent=consent))
            if random.random() < 0.5:
                batch.append(ev("product_click", aid, sid, dev, "/", "home_trending", pid, pos, consent=consent))
                batch.append(ev("product_view", aid, sid, dev, f"/products/{pid}", "pdp", pid, consent=consent))
                if random.random() < 0.25:
                    batch.append(ev("add_to_cart", aid, sid, dev, f"/products/{pid}", "pdp", pid, props={"qty": 1}, consent=consent))
                    if random.random() < 0.5:
                        batch.append(ev("checkout_start", aid, sid, dev, "/checkout", "checkout", consent=consent))
        for i in range(0, len(batch), 50):
            res = send(batch[i:i + 50])
            sent += res.get("accepted", 0)
            rejected += res.get("rejected", 0)
    time.sleep(0.01)
print(f"accepted={sent} rejected={rejected} (rejected = events of visitors without consent, by design)")
