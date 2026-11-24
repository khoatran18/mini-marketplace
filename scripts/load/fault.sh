#!/usr/bin/env bash
# Chèn một lỗi vào hệ thống ĐANG CHỊU TẢI (kiểm tra chịu lỗi). Chạy nền từ perf-one.sh (biến FAULT, FAULT_AT).
#   scripts/load/fault.sh HÀNH_ĐỘNG TRỄ_GIÂY FILE_NHẬT_KÝ FILE_KẾT_QUẢ_CCU
# Chờ đến khi ccu.mjs in "[ổn định] bắt đầu đo" (= giây 0 của bảng cửa sổ 10 s) trong FILE_KẾT_QUẢ_CCU, ngủ TRỄ_GIÂY rồi chèn lỗi.
# HÀNH_ĐỘNG:
#   kill:<service>   kill MỘT container của service Swarm (vd kill:auth-service); Swarm dựng lại task theo restart_policy
#   redis-flush      FLUSHALL trên Redis (Redis mất dữ liệu: kiểm tra logic dựng lại khóa pwd_version, R25)
#   kafka-kill:<n>   kill broker n (1-3)
#   postgres-kill    kill container Postgres (Swarm dựng lại; dữ liệu ở volume còn nguyên)
# Nhật ký: mỗi dòng "epoch HÀNH_ĐỘNG ...", gồm mốc trước/sau lệnh để canh với cửa sổ 10 s của ccu.
act="${1:?thiếu HÀNH_ĐỘNG}"; delay="${2:?thiếu TRỄ_GIÂY}"; log="${3:?thiếu FILE_NHẬT_KÝ}"; ccu="${4:?thiếu FILE_KẾT_QUẢ_CCU}"
: > "$log"
note() { echo "$(date +%s.%N | cut -c1-14) $*" >> "$log"; }
note "chờ ccu bắt đầu đo ($ccu)"
for _ in $(seq 1 1800); do grep -q '\[ổn định\] bắt đầu đo' "$ccu" 2>/dev/null && break; sleep 1; done
grep -q '\[ổn định\] bắt đầu đo' "$ccu" 2>/dev/null || { note "KHÔNG THẤY mốc bắt đầu đo, huỷ"; exit 1; }
note "ccu bắt đầu đo (giây 0); chờ $delay s rồi chèn lỗi $act"
sleep "$delay"
cid() { docker ps -q -f "name=$1" | head -1; }
case "$act" in
  kill:*)
    svc="${act#kill:}"; c=$(cid "marketplace_${svc}\\.")
    [ -n "$c" ] || { note "không có container của $svc"; exit 1; }
    note "TRƯỚC kill $svc container=${c:0:12} tên=$(docker inspect -f '{{.Name}}' "$c")"
    docker kill "$c" > /dev/null; note "SAU kill $svc" ;;
  redis-flush)
    c=$(cid "marketplace-infra_redis"); [ -n "$c" ] || { note "không có container redis"; exit 1; }
    note "TRƯỚC FLUSHALL (số khóa: $(docker exec "$c" redis-cli DBSIZE))"
    docker exec "$c" redis-cli FLUSHALL > /dev/null; note "SAU FLUSHALL (số khóa: $(docker exec "$c" redis-cli DBSIZE))" ;;
  kafka-kill:*)
    n="${act#kafka-kill:}"; c=$(cid "marketplace-infra_broker${n}\\.")
    [ -n "$c" ] || { note "không có container broker$n"; exit 1; }
    note "TRƯỚC kill broker$n container=${c:0:12}"; docker kill "$c" > /dev/null; note "SAU kill broker$n" ;;
  postgres-kill)
    c=$(cid "marketplace-infra_postgres\\."); [ -n "$c" ] || { note "không có container postgres"; exit 1; }
    note "TRƯỚC kill postgres container=${c:0:12}"; docker kill "$c" > /dev/null; note "SAU kill postgres" ;;
  *) note "hành động lạ: $act"; exit 1 ;;
esac
# Log Swarm xoay vòng nhanh dưới tải (đã mất đoạn chèn lỗi ở lượt kill auth 2026-11-24): chụp log 40 s trước tới ~90 s sau lúc chèn, vào <nhãn>.mid-<service>.log
since=$(date -u -d "@$(( $(date +%s) - 40 ))" +%Y-%m-%dT%H:%M:%SZ)
( sleep 90; for s in traefik api-gateway auth-service order-service payment-service product-service user-service; do
    timeout 60 docker service logs --since "$since" --timestamps "marketplace_$s" > "${log%.fault.log}.mid-$s.log" 2>&1
  done ) &
midp=$!
# theo dõi hồi phục: mỗi 2 giây ghi số bản đang chạy của mọi service cho tới 150 s sau khi chèn
for _ in $(seq 1 75); do
  note "bản chạy: $(docker service ls --format '{{.Name}}={{.Replicas}}' | grep -v -E 'obs_' | sed 's/marketplace[-_a-z]*_//' | awk -F'[=/ ]' '$2!=$3{printf "%s=%s/%s ", $1,$2,$3}')"
  sleep 2
done
wait $midp 2>/dev/null
note "xong theo dõi và chụp log giữa lượt"
