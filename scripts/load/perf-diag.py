#!/usr/bin/env python3
"""Lấy từ Prometheus các chỉ số lag/kết nối của đúng khoảng ổn định của một lượt đo (gọi NGAY sau perf-run.sh, trước khi gỡ stack).
Dùng:  python3 scripts/load/perf-diag.py <thư mục> <nhãn>
Đọc <thư mục>/<nhãn>.json (steadyStartMs/steadyEndMs), ghi <thư mục>/<nhãn>.diag.json (chuỗi thô) và in tóm tắt (tối đa, trung bình, giá trị cuối).
Chỉ số: lag publish của outbox (mm_outbox_oldest_pending_seconds), lag consumer Kafka, số kết nối Postgres theo trạng thái,
thời gian chờ pool DB, tốc độ publish outbox, commit/s của Postgres.
"""
import json, subprocess, sys, urllib.parse

d, label = sys.argv[1], sys.argv[2]
run = json.load(open(f"{d}/{label}.json"))
t0, t1 = run["steadyStartMs"] / 1000 - 60, run["steadyEndMs"] / 1000 + 90
prom = subprocess.run(["docker", "ps", "-q", "-f", "name=marketplace-obs_prometheus"], capture_output=True, text=True).stdout.split()
if not prom:
    sys.exit("không thấy container prometheus")
QUERIES = {
    "outbox_lag_s": "max by (job)(mm_outbox_oldest_pending_seconds)",
    "outbox_published_per_s": "sum by (job)(rate(mm_outbox_published_total[30s]))",
    "kafka_consumer_lag": "sum by (consumergroup, topic)(kafka_consumergroup_lag)",
    "pg_connections_by_state": "sum by (state)(pg_stat_activity_count)",
    "db_pool_wait_s_per_s": "sum by (job)(rate(mm_db_pool_wait_seconds_total[30s]))",
    "pg_commits_per_s": "sum(rate(pg_stat_database_xact_commit[30s]))",
    "pg_rollbacks_per_s": "sum(rate(pg_stat_database_xact_rollback[30s]))",
    "pg_deadlocks": "sum(pg_stat_database_deadlocks)",
    "disk_write_MBps_by_service": "sum by (container_label_com_docker_swarm_service_name)(rate(container_fs_writes_bytes_total{container_label_com_docker_swarm_service_name!=\"\"}[30s])) / 1e6",
    "disk_write_ops_by_service": "sum by (container_label_com_docker_swarm_service_name)(rate(container_fs_writes_total{container_label_com_docker_swarm_service_name!=\"\"}[30s]))",
}
out = {}
for name, q in QUERIES.items():
    url = f"http://localhost:9090/api/v1/query_range?query={urllib.parse.quote(q)}&start={t0}&end={t1}&step=5"
    r = subprocess.run(["docker", "exec", prom[0], "wget", "-qO-", url], capture_output=True, text=True)
    try:
        out[name] = json.loads(r.stdout)["data"]["result"]
    except Exception:
        out[name] = []
json.dump({"window": [t0, t1], "series": out}, open(f"{d}/{label}.diag.json", "w"))
print(f"DIAG {label} (cửa sổ {t1-t0:.0f} s quanh lượt ổn định; tối đa / trung bình / cuối):")
for name, res in out.items():
    for s in res:
        v = [float(x[1]) for x in s["values"] if x[1] not in ("NaN", "+Inf", "-Inf")]
        if not v or max(v) == 0:
            continue
        tag = ",".join(f"{k}={x}" for k, x in s["metric"].items() if k not in ("__name__", "instance"))
        print(f"  {name}{{{tag}}}: {max(v):.4g} / {sum(v)/len(v):.4g} / {v[-1]:.4g}")
