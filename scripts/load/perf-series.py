#!/usr/bin/env python3
"""Lấy CHUỖI THEO THỜI GIAN từ Prometheus cho cả lượt đo (soak, spike), để thấy xu hướng (RAM tăng dần, lag tích lũy, kết nối...), không chỉ trung bình.
Dùng:  python3 scripts/load/perf-series.py OUT.json START_EPOCH [END_EPOCH] [STEP_GIÂY=30]    (cần stack obs đang chạy; gọi TRƯỚC khi dọn)
Mỗi chuỗi là {nhãn: [[t, giá_trị], ...]}; các truy vấn: RAM và CPU theo service, lag outbox, lag consumer Kafka, kết nối Postgres theo trạng thái,
thời gian chờ pool DB, số goroutine theo job, commit/s của Postgres."""
import json, subprocess, sys, time, urllib.parse

out, t0 = sys.argv[1], int(float(sys.argv[2]))
t1 = int(float(sys.argv[3])) if len(sys.argv) > 3 else int(time.time())
step = int(sys.argv[4]) if len(sys.argv) > 4 else 30
prom = subprocess.run(["docker", "ps", "-q", "-f", "name=marketplace-obs_prometheus"], capture_output=True, text=True).stdout.split()
if not prom:
    sys.exit("không thấy container prometheus")
SVC = 'container_label_com_docker_swarm_service_name'
Q = {
    "ram_mib_by_service": f'sum by ({SVC})(container_memory_working_set_bytes{{{SVC}!=""}}) / 1048576',
    "cpu_cores_by_service": f'sum by ({SVC})(rate(container_cpu_usage_seconds_total{{{SVC}!=""}}[1m]))',
    "outbox_lag_s": "max by (job)(mm_outbox_oldest_pending_seconds)",
    "kafka_consumer_lag": "sum by (consumergroup, topic)(kafka_consumergroup_lag)",
    "pg_connections_by_state": "sum by (state)(pg_stat_activity_count)",
    "db_pool_wait_s_per_s": "sum by (job)(rate(mm_db_pool_wait_seconds_total[1m]))",
    "goroutines_by_job": "sum by (job)(go_goroutines)",
    "pg_commits_per_s": "sum(rate(pg_stat_database_xact_commit[1m]))",
}
res = {"window": [t0, t1], "step": step, "series": {}}
for name, q in Q.items():
    url = f"http://localhost:9090/api/v1/query_range?query={urllib.parse.quote(q)}&start={t0}&end={t1}&step={step}"
    r = subprocess.run(["docker", "exec", prom[0], "wget", "-qO-", url], capture_output=True, text=True)
    try:
        data = json.loads(r.stdout)["data"]["result"]
    except Exception:
        res["series"][name] = {"err": (r.stdout + r.stderr)[:200]}
        continue
    res["series"][name] = {",".join(f"{k}={v}" for k, v in x["metric"].items()) or "total": [[int(a), round(float(b), 3)] for a, b in x["values"]] for x in data}
json.dump(res, open(out, "w"), separators=(",", ":"))
print("đã ghi", out, {k: len(v) for k, v in res["series"].items()})
