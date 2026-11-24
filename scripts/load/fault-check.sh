#!/usr/bin/env bash
# Kiểm TOÀN VẸN DỮ LIỆU sau một lượt chèn lỗi (chỉ đọc). Chạy sau khi tải kết thúc và outbox đã có thời gian bắt kịp.
#   scripts/load/fault-check.sh FILE_RA
# Các kiểm tra (đơn vị: số dòng):
#  1. đơn trả trước (không COD) không bị hủy mà KHÔNG có dòng payment cùng checkout_id, nhóm theo trạng thái đơn, chỉ đơn cũ hơn 60 s
#  2. đơn PENDING cũ hơn 60 s (kẹt, chưa được product-service xác nhận hoặc từ chối)
#  3. trạng thái đơn và hàng đợi outbox còn lại (PENDING/DEAD) của order, payment, product
#  4. tổng quan: số đơn, số payment
pg=$(docker ps -q -f name=marketplace-infra_postgres | head -1)
[ -n "$pg" ] || { echo "KHÔNG có container postgres"; exit 1; }
q() { echo "-- $1"; docker exec "$pg" psql -U postgres -At -F ' | ' -c "$2" 2>&1; }
{
echo "KIỂM DỮ LIỆU $(date '+%F %T')"
q "tổng đơn / tổng payment" "select (select count(*) from orders), (select count(*) from payments)"
q "đơn trả trước, không hủy, KHÔNG có payment, cũ hơn 60 s (trạng thái | số đơn)" \
  "select o.status, count(*) from orders o where o.payment_method <> 'COD' and o.status not in ('CANCELED','CANCELLED') and o.created_at < now() - interval '60 seconds' and not exists (select 1 from payments p where p.checkout_id = o.checkout_id) group by 1 order by 2 desc"
q "đơn PENDING cũ hơn 60 s" "select count(*) from orders where status = 'PENDING' and created_at < now() - interval '60 seconds'"
q "trạng thái đơn" "select status, count(*) from orders group by 1 order by 2 desc"
q "outbox order (trạng thái | số dòng)" "select status, count(*) from order_domain_events group by 1 order by 1"
q "outbox payment (trạng thái | số dòng)" "select status, count(*) from payment_domain_events group by 1 order by 1"
q "outbox product (trạng thái | số dòng)" "select status, count(*) from product_domain_events group by 1 order by 1"
} > "$1" 2>&1
cat "$1"
