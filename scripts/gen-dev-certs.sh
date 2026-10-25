#!/usr/bin/env bash
# Generate a self-signed dev certificate for Traefik (never committed). Only needed with PUBLIC_SCHEME=https (default is http).
set -euo pipefail
out="$(dirname "$0")/../deploy/traefik/certs"
mkdir -p "$out"
openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
  -keyout "$out/local.key" -out "$out/local.crt" \
  -subj "/CN=marketplace.swarm.localhost" \
  -addext "subjectAltName=DNS:marketplace.swarm.localhost,DNS:api.marketplace.swarm.localhost,DNS:dashboard.marketplace.swarm.localhost,DNS:localhost,IP:127.0.0.1"
echo "Wrote $out/local.crt and local.key"
cat <<MSG

Trình duyệt phải TIN cert này, nếu không trang web gọi API sẽ báo "Failed to fetch" (gateway gửi HSTS nên Chrome không cho
bấm "tiếp tục" ở api.*). Chrome/Chromium trên Linux:
  sudo apt install -y libnss3-tools
  certutil -d sql:\$HOME/.pki/nssdb -A -t "C,," -n marketplace-local -i "$out/local.crt"
Rồi đóng hẳn và mở lại Chrome. (Firefox: Settings > Certificates > Authorities > Import.)
MSG
