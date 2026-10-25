# Sinh lại mã protobuf

> **Cách nhanh (tiếng Việt):** `scripts/protogen/gen.sh <service-dir>` (vd. `scripts/protogen/gen.sh payment-service`) tự làm toàn bộ quy trình bên dưới: dựng `validate.proto` từ descriptor (`scripts/protogen/printer`), chạy `buf`/`protoc-gen-go`/`protoc-gen-go-grpc` cục bộ (cài vào `.tools/`, đúng phiên bản trong header file sinh) và chép `.pb.go` tới mọi thư mục `out:` của `services/<svc>/buf.gen.yaml` (gói của chính nó + bản sao ở gateway/consumer). Cần `go` và truy cập Go module proxy lần đầu. Sáu service có proto: `auth`, `user`, `product`, `order`, `payment`, `analytics` (gateway chỉ là consumer).

Các file `.pb.go` là mã sinh tự động và không được sửa tay. Quy trình thông thường là chạy `buf generate` từ service sở hữu (`services/<svc>/buf.gen.yaml`, plugin remote trên buf.build và dependency `buf.build/bufbuild/protovalidate`). Khi không truy cập được buf.build (offline, proxy hạn chế) hãy dùng cách cục bộ bên dưới; cách này tái tạo kết quả hiện tại **giống từng byte** (đã kiểm chứng với `auth.proto`) nên an toàn.

## Công cụ (phiên bản phải khớp header của các file đã sinh)
```bash
export GOBIN=$PWD/.tools
go install github.com/bufbuild/buf/cmd/buf@v1.47.2
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.9       # xem "protoc-gen-go vX" trong auth.pb.go
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1       # xem "protoc-gen-go-grpc vX" trong auth_grpc.pb.go
```

## Thư mục làm việc
```
work/
  buf.yaml                      # version: v2, modules: [validate, auth]
  buf.gen.yaml                  # plugins: protoc-gen-go và protoc-gen-go-grpc cục bộ, out: out, opt: paths=source_relative
  validate/buf/validate/validate.proto   # dựng lại (xem dưới), giữ nguyên go_package
  auth/auth.proto               # bản sao của services/auth-service/pkg/proto/auth.proto
```
`validate.proto` được in ra từ descriptor đã biên dịch trong Go module mà các service vốn đã phụ thuộc (`buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go`), bằng `github.com/jhump/protoreflect/desc/protoprint` trên `validate.File_buf_validate_validate_proto`.

## Sinh và phân phối
```bash
cd work && buf generate auth            # ghi out/auth.pb.go và out/auth_grpc.pb.go
# chép hai file tới mọi đích liệt kê trong services/auth-service/buf.gen.yaml:
#   auth-service/pkg/pb, api-gateway/pkg/pb/authservice,
#   product-service|order-service|user-service/pkg/client/authclient
```
Luôn sinh lại proto **chưa sửa** trước và so sánh (diff) với repo (phải giống hệt) rồi mới sửa proto.

## Lịch sử
- Role `admin` được thêm vào `LoginRequest` và `ChangePasswordRequest` (`string.in`), không thêm vào `RegisterRequest`/`Account` (admin không bao giờ tự đăng ký). Xem 05_SECURITY.md.
- `product.proto`: thêm `Product` (mô tả, danh mục, brand, tags, ảnh, status, sku, ngưỡng tồn, reserved/sold/version, `stock_level`), danh mục, `SearchProducts`, `SetProductStatus`, `AdjustInventory`, `GetInventoryLedger`, `ListLowStock`.
- `order.proto`: viết lại cho checkout theo store, giỏ hàng, `ApplyAction`, `ListOrders/GetOrder/CountOrders`, `PreviewCheckout`.
- `user.proto`: thêm RPC địa chỉ (`UpsertAddress`, `ListAddresses`, `GetAddress`, `DeleteAddress`).
- `payment.proto` (mới, :50057) và `analytics.proto` (mới, :50055: `IngestEvents`, `Query`) – sinh vào `pkg/pb` của chính chúng và `api-gateway/pkg/pb/{paymentservice,analyticsservice}`.
