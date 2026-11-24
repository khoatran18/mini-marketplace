#!/usr/bin/env bash
# Lấy mẫu mỗi giây trạng thái ghi của Postgres và đĩa trong lúc đo (chạy nền, kết thúc bằng kill). CHỈ để chẩn đoán payment lag (R23).
#   scripts/load/perf-pgsample.sh FILE_RA [thiết_bị_đĩa]     (mặc định nvme0n1)
# FILE_RA, mỗi dòng: epoch | checkpoint theo giờ/theo WAL đầy | thời gian ghi/sync checkpoint (ms) | buffers checkpoint/backend | wal bytes, fpi, buffers_full, số lần write/sync, ms sync |
#           số truy vấn đang chờ WALSync/WALWrite/đang chạy | đĩa: ghi hoàn tất, sector ghi, ms ghi, đang chạy, flush hoàn tất, ms flush
# FILE_RA.tables, mỗi 5 giây: 8 bảng có seq_tup_read lớn nhất (quét tuần tự: dấu hiệu kế hoạch truy vấn xấu khi chưa có thống kê) cùng số dòng, dòng chết, số lần autoanalyze/autovacuum.
# FILE_RA.act (mỗi giây): truy vấn/tiến trình không rảnh của Postgres kèm sự kiện chờ (gồm autovacuum worker); FILE_RA.top (mỗi 2 giây): tiến trình tốn CPU nhất trong container.
# Cần track_io_timing/track_wal_io_timing (bật bằng ALTER SYSTEM trong lượt chẩn đoán) và `log_checkpoints` để có dòng log.
out="${1:?thiếu FILE_RA}"; dev="${2:-nvme0n1}"
pg=$(docker ps -q -f name=marketplace-infra_postgres | head -1)
q="select extract(epoch from now())::bigint, b.checkpoints_timed, b.checkpoints_req, b.checkpoint_write_time::bigint, b.checkpoint_sync_time::bigint, b.buffers_checkpoint, b.buffers_backend, w.wal_bytes::bigint, w.wal_fpi, w.wal_buffers_full, w.wal_write, w.wal_sync, w.wal_sync_time::bigint, (select count(*) from pg_stat_activity where wait_event='WALSync'), (select count(*) from pg_stat_activity where wait_event='WALWrite'), (select count(*) from pg_stat_activity where state='active' and backend_type='client backend') from pg_stat_bgwriter b, pg_stat_wal w"
qt="select extract(epoch from now())::bigint, relname, seq_scan, seq_tup_read, idx_scan, n_live_tup, n_dead_tup, autoanalyze_count, autovacuum_count from pg_stat_user_tables order by seq_tup_read desc limit 8"
qa="select backend_type, state, coalesce(wait_event_type,'-'), coalesce(wait_event,'-'), left(regexp_replace(query,'\\s+',' ','g'),90), count(*) from pg_stat_activity where pid<>pg_backend_pid() and state<>'idle' group by 1,2,3,4,5 order by 6 desc limit 12"
i=0
while true; do
  r=$(docker exec "$pg" psql -U postgres -At -F' ' -c "$q" 2>&1 | head -1)
  d=$(awk -v d="$dev" '$3==d{print $8, $10, $11, $12, $19, $20}' /proc/diskstats)
  echo "$r | $d"
  if [ $((i % 5)) = 0 ]; then docker exec "$pg" psql -U postgres -At -F' ' -c "$qt" >> "$out.tables" 2>&1; fi
  echo "## $(date +%s)" >> "$out.act"; docker exec "$pg" psql -U postgres -At -F' | ' -c "$qa" >> "$out.act" 2>&1
  if [ $((i % 2)) = 0 ]; then { echo "## $(date +%s)"; docker exec "$pg" top -b -n 1 2>/dev/null | sed -n '1,3p;8,17p'; } >> "$out.top"; fi
  i=$((i+1)); sleep 1
done > "$out"
