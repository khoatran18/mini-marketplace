# Regenerating protobuf code

> **Cách nhanh (tiếng Việt):** `scripts/protogen/gen.sh <service-dir>` (vd. `scripts/protogen/gen.sh payment-service`) tự làm toàn bộ quy trình bên dưới: dựng `validate.proto` từ descriptor (`scripts/protogen/printer`), chạy `buf`/`protoc-gen-go`/`protoc-gen-go-grpc` cục bộ (cài vào `.tools/`, đúng phiên bản trong header file sinh) và chép `.pb.go` tới mọi thư mục `out:` của `services/<svc>/buf.gen.yaml` (gói của chính nó + bản sao ở gateway/consumer). Cần `go` và truy cập Go module proxy lần đầu. Sáu service có proto: `auth`, `user`, `product`, `order`, `payment`, `analytics` (gateway chỉ là consumer).

`.pb.go` files are generated and must not be edited. The normal flow is `buf generate` from the owning service (`services/<svc>/buf.gen.yaml`, remote plugins on buf.build and the `buf.build/bufbuild/protovalidate` dependency). When buf.build is not reachable (offline, restricted proxy) use the local method below; it reproduces the current output **byte-for-byte** (verified for `auth.proto`), so it is safe.

## Tools (versions must match the headers of the existing generated files)
```bash
export GOBIN=$PWD/.tools
go install github.com/bufbuild/buf/cmd/buf@v1.47.2
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.9       # see "protoc-gen-go vX" in auth.pb.go
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1       # see "protoc-gen-go-grpc vX" in auth_grpc.pb.go
```

## Workspace
```
work/
  buf.yaml                      # version: v2, modules: [validate, auth]
  buf.gen.yaml                  # plugins: local protoc-gen-go and protoc-gen-go-grpc, out: out, opt: paths=source_relative
  validate/buf/validate/validate.proto   # reconstructed (below), go_package kept unchanged
  auth/auth.proto               # copy of services/auth-service/pkg/proto/auth.proto
```
`validate.proto` is printed from the descriptor compiled into the Go module the services already depend on (`buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go`), using `github.com/jhump/protoreflect/desc/protoprint` on `validate.File_buf_validate_validate_proto`.

## Generate and distribute
```bash
cd work && buf generate auth            # writes out/auth.pb.go and out/auth_grpc.pb.go
# copy the two files to every destination listed in services/auth-service/buf.gen.yaml:
#   auth-service/pkg/pb, api-gateway/pkg/pb/authservice,
#   product-service|order-service|user-service/pkg/client/authclient
```
Always first regenerate the **unchanged** proto and diff against the repo (must be identical) before editing the proto.

## History
- Role `admin` was added to `LoginRequest` and `ChangePasswordRequest` (`string.in`), not to `RegisterRequest`/`Account` (admin can never self-register). See security.md.
- `product.proto`: thêm `Product` (mô tả, danh mục, brand, tags, ảnh, status, sku, ngưỡng tồn, reserved/sold/version, `stock_level`), danh mục, `SearchProducts`, `SetProductStatus`, `AdjustInventory`, `GetInventoryLedger`, `ListLowStock`.
- `order.proto`: viết lại cho checkout theo store, giỏ hàng, `ApplyAction`, `ListOrders/GetOrder/CountOrders`, `PreviewCheckout`.
- `user.proto`: thêm RPC địa chỉ (`UpsertAddress`, `ListAddresses`, `GetAddress`, `DeleteAddress`).
- `payment.proto` (mới, :50057) và `analytics.proto` (mới, :50055: `IngestEvents`, `Query`) – sinh vào `pkg/pb` của chính chúng và `api-gateway/pkg/pb/{paymentservice,analyticsservice}`.
