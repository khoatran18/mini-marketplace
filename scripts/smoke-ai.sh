#!/usr/bin/env bash
# Smoke test trợ lý AI sau khi deploy: mỗi role đăng nhập, hỏi MỘT câu qua /ai/chat và kiểm tra luồng SSE kết thúc bằng `done`.
# Bắt lỗi kiểu "vai trò này chưa được hỗ trợ" (persona chưa bật), thiếu tool của role, gateway thiếu AI_SERVICE_URL, mô hình lỗi.
# CÓ gọi LLM thật (mỗi role một câu ngắn). Tài khoản lấy từ biến môi trường, không có mật khẩu nào nằm trong file này:
#   SMOKE_BUYER_USER/_PASS   (mặc định buyer1/password: tài khoản demo khi SEED_DEMO_DATA=true)
#   SMOKE_SELLER_USER/_PASS  (mặc định seller1/password)
#   SMOKE_ADMIN_USER/_PASS   và SMOKE_DEV_USER/_PASS (không có mặc định: role nào thiếu biến thì bị bỏ qua, in SKIP)
# Usage: scripts/smoke-ai.sh [base_url]     base_url mặc định $SMOKE_BASE_URL hoặc http://api.marketplace.swarm.localhost
# Thoát 0 khi mọi role đã chạy đều đạt; 1 khi có role lỗi hoặc không role nào chạy.
set -uo pipefail
base="${1:-${SMOKE_BASE_URL:-http://api.marketplace.swarm.localhost}}"
timeout="${SMOKE_TIMEOUT:-90}"

fail=0; ran=0
check() { # role user pass question
  local role="$1" user="$2" pass="$3" question="$4" token out
  if [ -z "$user" ] || [ -z "$pass" ]; then echo "SKIP $role (chưa có SMOKE_${role^^}_USER/_PASS)"; return; fi
  token=$(curl -fsS --max-time 15 -H 'Content-Type: application/json' -d "$(printf '{"username":"%s","password":"%s"}' "$user" "$pass")" "$base/auth/login" 2>/dev/null \
    | python3 -c 'import json,sys; print(json.load(sys.stdin).get("access_token",""))' 2>/dev/null)
  if [ -z "$token" ]; then echo "FAIL $role: đăng nhập không được ($user)"; fail=1; return; fi
  ran=$((ran+1))
  out=$(curl -sS --max-time "$timeout" -N -H "Authorization: Bearer $token" -H 'Content-Type: application/json' \
    -d "$(python3 -c 'import json,sys; print(json.dumps({"message": sys.argv[1]}))' "$question")" "$base/ai/chat" 2>&1)
  if printf '%s' "$out" | grep -q '^event: done'; then
    echo "OK   $role"
  else
    echo "FAIL $role: không thấy 'event: done'. Phản hồi (cắt 300 ký tự):"; printf '%s' "$out" | head -c 300; echo; fail=1
  fi
}

check buyer  "${SMOKE_BUYER_USER-buyer1}"   "${SMOKE_BUYER_PASS-password}"   "Chính sách đổi trả của cửa hàng thế nào?"
check seller "${SMOKE_SELLER_USER-seller1}" "${SMOKE_SELLER_PASS-password}"   "Cho mình xem sản phẩm sắp hết hàng."
check admin  "${SMOKE_ADMIN_USER-}"         "${SMOKE_ADMIN_PASS-}"            "Sức khoẻ hệ thống hiện giờ thế nào?"
check dev    "${SMOKE_DEV_USER-}"           "${SMOKE_DEV_PASS-}"              "Service nào đang chậm nhất?"

[ "$ran" -eq 0 ] && { echo "Không role nào chạy được."; exit 1; }
exit "$fail"
