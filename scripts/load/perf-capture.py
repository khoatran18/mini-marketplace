#!/usr/bin/env python3
"""Thu MỌI chỉ số cần để tìm nguyên nhân cú đứng của Postgres (R23), mỗi giây, ghi JSON từng dòng. Chỉ dùng thư viện chuẩn, không cần root.
Dùng:  python3 scripts/load/perf-capture.py FILE.jsonl [giây_giữa_các_mẫu=1]      (chạy nền, dừng bằng SIGTERM/kill)

Mỗi dòng gồm:
  host   : /proc/stat (CPU từng loại, tổng), loadavg, PSI cpu/io/memory (tổng us), meminfo (Dirty, Writeback...), vmstat (dirty/writeback/reclaim),
           /proc/diskstats của đĩa, thống kê journal ext4 (/proc/fs/jbd2), xung nhịp CPU (tb/nhỏ nhất) và nhiệt độ cao nhất (hwmon)
  cg     : mỗi container swarm: cpu.stat (usage/user/system us, nr_periods, nr_throttled, throttled_usec), cpu.pressure, io.stat (byte và số thao tác đọc/ghi), memory.current
  pgproc : MỌI tiến trình trong container Postgres: nhãn (tiêu đề tiến trình, vd "COMMIT", "idle in transaction", "autovacuum worker", "walwriter"), trạng thái,
           utime/stime (tick, tách user và kernel), số lần chuyển ngữ cảnh tự nguyện/không tự nguyện, wchan (hàm kernel đang chờ)
  top    : các tiến trình khác trên máy dùng CPU >= 2 tick trong mẫu (kernel thread như jbd2, kworker, kswapd; node của máy tải; dockerd...)
  sql    : pg_stat_wal, pg_stat_bgwriter, pg_stat_io (theo loại tiến trình), pg_stat_database, truy vấn đang chạy nhóm theo sự kiện chờ, khoá chưa cấp, số kết nối theo trạng thái
Các giá trị cộng dồn (tick, us, byte, số lần) được ghi nguyên, phân tích tính hiệu giữa hai mẫu. Chi phí: ~1 psql + quét /proc mỗi giây.
"""
import glob, json, os, re, signal, subprocess, sys, time

OUT = sys.argv[1]
IV = float(sys.argv[2]) if len(sys.argv) > 2 else 1.0
DISK = os.environ.get("CAPTURE_DISK", "nvme0n1")
JBD = glob.glob("/proc/fs/jbd2/*/info")
stop = False
signal.signal(signal.SIGTERM, lambda *a: globals().__setitem__("stop", True))
signal.signal(signal.SIGINT, lambda *a: globals().__setitem__("stop", True))

SQL = """select json_build_object(
 'wal',(select row_to_json(w) from pg_stat_wal w),
 'bgw',(select row_to_json(b) from pg_stat_bgwriter b),
 'db',(select row_to_json(d) from (select xact_commit,xact_rollback,blks_read,blks_hit,tup_returned,tup_fetched,tup_inserted,tup_updated,tup_deleted,temp_files,temp_bytes,deadlocks,blk_read_time,blk_write_time,sessions,sessions_abandoned from pg_stat_database where datname='postgres') d),
 'io',(select json_agg(json_build_object('bt',backend_type,'obj',object,'ctx',context,'r',reads,'w',writes,'wt',write_time,'f',fsyncs,'ft',fsync_time,'ev',evictions)) from pg_stat_io where writes>0 or fsyncs>0 or reads>0),
 'act',(select json_agg(x) from (select backend_type bt,state st,wait_event_type wt,wait_event we,left(regexp_replace(query,'\\s+',' ','g'),70) q,count(*) n from pg_stat_activity where pid<>pg_backend_pid() and state is distinct from 'idle' group by 1,2,3,4,5 order by n desc limit 15) x),
 'locks',(select json_agg(y) from (select locktype,mode,granted,count(*) n from pg_locks where not granted group by 1,2,3) y),
 'conn',(select json_object_agg(coalesce(state,'null'),c) from (select state,count(*) c from pg_stat_activity where backend_type='client backend' group by 1) z),
 'addr',(select json_object_agg(coalesce(host(client_addr),'local'),c) from (select client_addr,count(*) c from pg_stat_activity where backend_type='client backend' group by 1) a))"""


def rd(path):
    try:
        with open(path) as f:
            return f.read()
    except OSError:
        return ""


def containers():
    """{id_dài: tên service} cho container của dự án."""
    out = subprocess.run(["docker", "ps", "--no-trunc", "--format", '{{.ID}} {{.Label "com.docker.swarm.service.name"}}'], capture_output=True, text=True).stdout
    return {l.split()[0]: l.split()[1] for l in out.splitlines() if len(l.split()) == 2}


def scope(cid):
    for p in (f"/sys/fs/cgroup/system.slice/docker-{cid}.scope", f"/sys/fs/cgroup/docker/{cid}"):
        if os.path.isdir(p):
            return p
    return None


def kv(text):
    d = {}
    for l in text.splitlines():
        p = l.split()
        if len(p) >= 2 and p[1].lstrip("-").isdigit():
            d[p[0]] = int(p[1])
    return d


def psi(path):
    r = {}
    for l in rd(path).splitlines():
        m = re.match(r"(some|full) .*total=(\d+)", l)
        if m:
            r[m.group(1)] = int(m.group(2))
    return r


def host():
    h = {"stat": [int(x) for x in rd("/proc/stat").splitlines()[0].split()[1:]], "load": rd("/proc/loadavg").split()[:3]}
    h["psi"] = {k: psi(f"/proc/pressure/{k}") for k in ("cpu", "io", "memory")}
    mi = kv(rd("/proc/meminfo").replace(":", ""))
    h["mem"] = {k: mi.get(k) for k in ("MemAvailable", "Dirty", "Writeback", "Cached", "Slab")}
    vm = kv(rd("/proc/vmstat"))
    h["vm"] = {k: vm.get(k) for k in ("nr_dirty", "nr_writeback", "nr_dirtied", "nr_written", "pgscan_direct", "pgscan_kswapd", "allocstall_normal", "pgmajfault", "compact_stall")}
    for l in rd("/proc/diskstats").splitlines():
        p = l.split()
        if len(p) > 3 and p[2] == DISK:
            h["disk"] = [int(x) for x in p[3:]]
    j = {}
    for path in JBD[:1]:
        t = rd(path)
        m = re.search(r"(\d+) transactions", t)
        j["tx"] = int(m.group(1)) if m else None
        for name, key in (("running transaction", "run_ms"), ("logging transaction", "log_ms"), ("flushing data", "flush_ms")):
            m = re.search(r"(\d+)ms " + name, t)
            if m:
                j[key] = int(m.group(1))
        m = re.search(r"(\d+)us average transaction commit time", t)
        if m:
            j["commit_us"] = int(m.group(1))
    h["jbd2"] = j
    fr = [int(rd(p) or 0) for p in glob.glob("/sys/devices/system/cpu/cpu[0-9]*/cpufreq/scaling_cur_freq")]
    if fr:
        h["freq_khz"] = [round(sum(fr) / len(fr)), min(fr)]
    tm = [int(rd(p) or 0) for p in glob.glob("/sys/class/hwmon/hwmon*/temp*_input")]
    if tm:
        h["temp_mC_max"] = max(tm)
    return h


def cgroups(cmap):
    r = {}
    for cid, name in cmap.items():
        s = scope(cid)
        if not s:
            continue
        c = kv(rd(s + "/cpu.stat"))
        io = {"rb": 0, "wb": 0, "rio": 0, "wio": 0}
        for l in rd(s + "/io.stat").splitlines():
            for k, kk in (("rbytes", "rb"), ("wbytes", "wb"), ("rios", "rio"), ("wios", "wio")):
                m = re.search(k + r"=(\d+)", l)
                if m:
                    io[kk] += int(m.group(1))
        cur = {"cpu": {k: c.get(k) for k in ("usage_usec", "user_usec", "system_usec", "nr_periods", "nr_throttled", "throttled_usec")},
               "psi": psi(s + "/cpu.pressure").get("some"), "io": io, "mem": int(rd(s + "/memory.current") or 0)}
        if name in r:  # nhiều bản cùng tên service: CỘNG các bộ đếm (trước đây bản cuối đè các bản trước, mất số đo của các bản còn lại)
            old = r[name]
            for k, v in cur["cpu"].items():
                old["cpu"][k] = (old["cpu"].get(k) or 0) + (v or 0)
            for k, v in cur["io"].items():
                old["io"][k] = old["io"].get(k, 0) + v
            old["mem"] += cur["mem"]
            old["replicas"] = old.get("replicas", 1) + 1
        else:
            r[name] = cur
    return r


def proc_stat(pid):
    t = rd(f"/proc/{pid}/stat")
    i = t.rfind(")")
    if i < 0:
        return None
    f = t[i + 2:].split()
    return t[t.find("(") + 1:i], f[0], int(f[11]), int(f[12])  # comm, state, utime, stime


def pg_procs(pg_scope):
    out = {}
    for pid in rd(pg_scope + "/cgroup.procs").split():
        st = proc_stat(pid)
        if not st:
            continue
        title = rd(f"/proc/{pid}/cmdline").replace("\0", " ").strip()
        title = re.sub(r"^postgres: ", "", title)
        status = rd(f"/proc/{pid}/status")
        v = re.search(r"voluntary_ctxt_switches:\s+(\d+)", status)
        n = re.search(r"nonvoluntary_ctxt_switches:\s+(\d+)", status)
        out[pid] = [title[:70], st[1], st[2], st[3], int(v.group(1)) if v else 0, int(n.group(1)) if n else 0, rd(f"/proc/{pid}/wchan")[:30]]
    return out


prev_top = {}


def top_procs(skip):
    global prev_top
    cur, out = {}, {}
    for d in os.listdir("/proc"):
        if not d.isdigit() or d in skip:
            continue
        st = proc_stat(d)
        if not st:
            continue
        cur[d] = (st[2] + st[3], st[0])
        if d in prev_top:
            dl = cur[d][0] - prev_top[d][0]
            if dl >= 2:
                out[d] = [st[0], dl]
    prev_top = cur
    return out


def sql(pg_id):
    if not pg_id:
        return None
    r = subprocess.run(["docker", "exec", pg_id, "psql", "-U", "postgres", "-At", "-c", SQL], capture_output=True, text=True)
    try:
        return json.loads(r.stdout)
    except Exception:
        return {"err": (r.stdout + r.stderr)[:200]}


def main():
    cmap, last_map = containers(), time.time()
    f = open(OUT, "w")
    ipmap = {}
    for cid, name in cmap.items():
        ip = subprocess.run(["docker", "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}", cid], capture_output=True, text=True).stdout.split()
        for i in ip:
            ipmap[i] = name
    f.write(json.dumps({"meta": {"ipmap": ipmap}}) + "\n")
    while not stop:
        t0 = time.time()
        if t0 - last_map > 30:
            cmap, last_map = containers(), t0
        pg_long = next((c for c, n in cmap.items() if n == "marketplace-infra_postgres"), None)
        pg_scope = scope(pg_long) if pg_long else None
        pgp = pg_procs(pg_scope) if pg_scope else {}
        rec = {"t": round(t0, 3), "host": host(), "cg": cgroups(cmap), "pgproc": pgp, "top": top_procs(set(pgp)), "sql": sql(pg_long[:12] if pg_long else None)}
        rec["dt"] = round(time.time() - t0, 3)
        f.write(json.dumps(rec, separators=(",", ":")) + "\n")
        f.flush()
        time.sleep(max(0.0, IV - (time.time() - t0)))
    f.close()


main()
