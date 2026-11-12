#!/usr/bin/env bash
# Sao lưu Postgres (nguồn sự thật) + kiểm thử khôi phục. Chạy được từ cron.
# Usage: scripts/backup-postgres.sh [backup|verify|restore <file>|cron-install]
#   backup        pg_dump -Fc -> $BACKUP_DIR/mm-<giờ>.dump, kiểm tra bằng pg_restore --list, xoá bản cũ quá $BACKUP_KEEP_DAYS ngày (mặc định: backup)
#   verify        khôi phục bản mới nhất vào DB tạm "restore_check" (KHÔNG đụng DB thật), so số bảng và số dòng với DB thật, rồi xoá DB tạm
#   restore FILE  khôi phục vào DB thật (--clean). Phải dừng các service ghi trước: docker stack rm marketplace
#   cron-install  in dòng crontab mẫu (không tự cài)
# Biến: BACKUP_DIR (mặc định ~/mm-backups), BACKUP_KEEP_DAYS (7), POSTGRES_USER (postgres), POSTGRES_DB (postgres)
# Đọc mật khẩu: không cần, vì chạy trong container qua socket cục bộ (peer/trust trong image postgres).
set -euo pipefail
cmd="${1:-backup}"
dir="${BACKUP_DIR:-$HOME/mm-backups}"
keep="${BACKUP_KEEP_DAYS:-7}"
user="${POSTGRES_USER:-postgres}"
db="${POSTGRES_DB:-postgres}"
scratch="restore_check"

cid() {
  local c; c="$(docker ps -q -f name=marketplace-infra_postgres | head -1)"
  [ -n "$c" ] || { echo "không thấy container marketplace-infra_postgres đang chạy" >&2; exit 1; }
  echo "$c"
}
psqlx() { docker exec -i "$(cid)" psql -U "$user" -v ON_ERROR_STOP=1 -At "$@"; }

# đếm dòng mọi bảng user (schema public) -> "bảng=số_dòng" theo thứ tự tên
counts() {
  local d="$1" t
  for t in $(psqlx -d "$d" -c "select tablename from pg_tables where schemaname='public' order by 1"); do
    echo "$t=$(psqlx -d "$d" -c "select count(*) from \"$t\"")"
  done
}

latest() { ls -1t "$dir"/mm-*.dump 2>/dev/null | head -1; }

case "$cmd" in
  backup)
    mkdir -p "$dir"; chmod 700 "$dir"
    out="$dir/mm-$(date +%Y%m%d-%H%M%S).dump"
    docker exec "$(cid)" pg_dump -U "$user" -Fc "$db" > "$out.tmp"
    # file rỗng/hỏng thì bỏ, không để bản lỗi thay bản tốt
    docker exec -i "$(cid)" pg_restore --list < "$out.tmp" > /dev/null
    mv "$out.tmp" "$out"; chmod 600 "$out"
    find "$dir" -name 'mm-*.dump' -mtime +"$keep" -delete
    echo "OK $out ($(du -h "$out" | cut -f1)); còn $(ls -1 "$dir"/mm-*.dump | wc -l) bản"
    ;;
  verify)
    f="$(latest)"; [ -n "$f" ] || { echo "chưa có bản backup trong $dir" >&2; exit 1; }
    echo "kiểm thử khôi phục: $f -> DB tạm $scratch"
    psqlx -d postgres -c "drop database if exists $scratch" >/dev/null
    psqlx -d postgres -c "create database $scratch" >/dev/null
    trap 'psqlx -d postgres -c "drop database if exists '"$scratch"'" >/dev/null || true' EXIT
    docker exec -i "$(cid)" pg_restore -U "$user" -d "$scratch" --no-owner < "$f"
    a="$(counts "$db")"; b="$(counts "$scratch")"
    nt="$(echo "$b" | grep -c . || true)"
    if [ "$a" = "$b" ]; then
      echo "KHỚP: $nt bảng, số dòng giống DB thật"
    else
      # DB thật có thể đã ghi thêm sau bản backup: chỉ cảnh báo chênh lệch, nhưng lỗi nếu thiếu bảng
      echo "$a" > "${TMPDIR:-/tmp}/mm-live.counts"; echo "$b" > "${TMPDIR:-/tmp}/mm-restored.counts"
      miss="$(diff <(echo "$a" | cut -d= -f1) <(echo "$b" | cut -d= -f1) || true)"
      [ -z "$miss" ] || { echo "THIẾU/THỪA bảng sau khôi phục:"; echo "$miss"; exit 1; }
      echo "KHÁC số dòng (DB thật ghi thêm sau backup?) — đủ $nt bảng:"
      diff <(echo "$a") <(echo "$b") || true
    fi
    ;;
  restore)
    f="${2:?usage: $0 restore <file.dump>}"
    [ -f "$f" ] || { echo "không có file $f" >&2; exit 1; }
    echo "Sẽ GHI ĐÈ DB '$db' bằng $f. Đã dừng các service ghi chưa?"
    read -r -p "Gõ 'yes' để tiếp tục: " ans; [ "$ans" = yes ] || { echo "Hủy."; exit 0; }
    docker exec -i "$(cid)" pg_restore -U "$user" -d "$db" --clean --if-exists --no-owner < "$f"
    echo "xong; deploy lại: ./scripts/deploy.sh"
    ;;
  cron-install)
    root="$(cd "$(dirname "$0")/.." && pwd)"
    echo "# mỗi ngày 02:30 backup, 02:45 kiểm thử khôi phục; thêm bằng: crontab -e"
    echo "30 2 * * * $root/scripts/backup-postgres.sh backup >> \$HOME/mm-backups/backup.log 2>&1"
    echo "45 2 * * * $root/scripts/backup-postgres.sh verify >> \$HOME/mm-backups/backup.log 2>&1"
    ;;
  *) echo "usage: $0 [backup|verify|restore <file>|cron-install]"; exit 1 ;;
esac
