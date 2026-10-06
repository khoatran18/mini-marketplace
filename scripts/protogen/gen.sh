#!/usr/bin/env bash
# Regenerate the protobuf Go code of one service WITHOUT needing buf.build.
#   scripts/protogen/gen.sh product-service
# Reads services/<svc>/pkg/proto/*.proto and the `out:` directories of services/<svc>/buf.gen.yaml
# (own pb package + every consumer copy), runs a local buf with local protoc-gen-go / protoc-gen-go-grpc
# (versions pinned below = the ones recorded in the generated file headers) and copies the result.
# Needs: go, network access to the Go module proxy (first run installs the tools into .tools/).
set -euo pipefail
svc="${1:?usage: $0 <service-dir, e.g. product-service>}"
root="$(cd "$(dirname "$0")/../.." && pwd)"
svcdir="$root/services/$svc"
tools="$root/.tools"
[ -d "$svcdir/pkg/proto" ] || { echo "no $svcdir/pkg/proto"; exit 1; }

mkdir -p "$tools"
export GOBIN="$tools"
[ -x "$tools/buf" ] || go install github.com/bufbuild/buf/cmd/buf@v1.47.2
[ -x "$tools/protoc-gen-go" ] || go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.9
[ -x "$tools/protoc-gen-go-grpc" ] || go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1

work="$(mktemp -d)"; trap 'rm -rf "$work"' EXIT
mkdir -p "$work/validate/buf/validate" "$work/svc" "$work/out"
(cd "$root/scripts/protogen/printer" && GOFLAGS=-mod=mod go run . > "$work/validate/buf/validate/validate.proto")
cp "$svcdir"/pkg/proto/*.proto "$work/svc/"
cat > "$work/buf.yaml" <<YAML
version: v2
modules:
  - path: validate
  - path: svc
YAML
cat > "$work/buf.gen.yaml" <<YAML
version: v2
plugins:
  - local: $tools/protoc-gen-go
    out: out
    opt: paths=source_relative
  - local: $tools/protoc-gen-go-grpc
    out: out
    opt: paths=source_relative
YAML
(cd "$work" && "$tools/buf" generate svc)

# every distinct output directory of buf.gen.yaml (relative to the service dir)
grep -E '^[[:space:]]+out:' "$svcdir/buf.gen.yaml" | awk '{print $2}' | sort -u | while read -r out; do
  mkdir -p "$svcdir/$out"
  cp "$work"/out/*.pb.go "$svcdir/$out/"
  echo "wrote $(ls "$work"/out/*.pb.go | wc -l) files to services/$svc/$out"
done
