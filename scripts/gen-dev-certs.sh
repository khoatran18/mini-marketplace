#!/usr/bin/env bash
# Generate a self-signed dev certificate for Traefik and the gateway (never committed).
set -euo pipefail
out="$(dirname "$0")/../deploy/traefik/certs"
mkdir -p "$out"
openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
  -keyout "$out/local.key" -out "$out/local.crt" \
  -subj "/CN=marketplace.swarm.localhost" \
  -addext "subjectAltName=DNS:marketplace.swarm.localhost,DNS:localhost,IP:127.0.0.1"
echo "Wrote $out/local.crt and local.key"
