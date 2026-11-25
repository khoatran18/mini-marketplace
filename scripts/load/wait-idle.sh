#!/usr/bin/env bash
# Chờ máy rảnh (CPU toàn máy < NGƯỠNG % trong 3 mẫu 5 s liên tiếp) trước khi đo: tiến trình ngoài (đánh giá/ IDE) làm lệch số đo (CPU toàn máy > 75% thì kém tin cậy).
#   scripts/load/wait-idle.sh [NGƯỠNG=25] [TỐI_ĐA_GIÂY=1800]     mã thoát 0 = rảnh, 1 = hết giờ vẫn bận (vẫn in cảnh báo)
thr="${1:-25}"; max="${2:-1800}"; ok=0; t0=$(date +%s)
cpu() { read -r _ a b c d e f g h _ < /proc/stat; i1=$((d+e)); t1=$((a+b+c+d+e+f+g+h)); sleep 5; read -r _ a b c d e f g h _ < /proc/stat; i2=$((d+e)); t2=$((a+b+c+d+e+f+g+h)); echo $(( 100*( (t2-t1)-(i2-i1) )/(t2-t1) )); }
while [ $(( $(date +%s)-t0 )) -lt "$max" ]; do
  u=$(cpu); if [ "$u" -lt "$thr" ]; then ok=$((ok+1)); else ok=0; fi
  [ "$ok" -ge 3 ] && { echo "máy rảnh (CPU $u%)"; exit 0; }
done
echo "CẢNH BÁO: sau ${max}s máy vẫn bận (CPU $u%)"; exit 1
