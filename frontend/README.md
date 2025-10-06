# Mini Marketplace Frontend

Giao diện Next.js để thao tác với hệ thống backend trong thư mục `api-gateway`. Ứng dụng tập
trung vào các luồng cơ bản đã được mô tả trong tài liệu OpenAPI (đăng nhập, quản lý sản phẩm và đơn
hàng).

## Khởi chạy cục bộ

```bash
cd frontend
npm install
npm run dev
```

Ứng dụng sử dụng biến môi trường `NEXT_PUBLIC_API_BASE_URL` để xác định địa chỉ API Gateway. Mặc định
là `http://localhost:8080`. Ngoài ra cần khai báo `NEXT_PUBLIC_JWT_SECRET` (hoặc tương đương) để
frontend có thể giải mã và gia hạn access token dựa trên refresh token.

## Cấu trúc chính

- `app/` – cấu trúc App Router của Next.js với các trang đăng nhập, đăng ký, danh sách sản phẩm và bảng
  điều khiển.
- `components/` – các thành phần tái sử dụng như thanh điều hướng, form tạo sản phẩm/đơn hàng và lớp
  bảo vệ yêu cầu đăng nhập.
- `lib/api.ts` – wrapper đơn giản cho các endpoint của API Gateway dựa trên tài liệu OpenAPI.

> Lưu ý: Các yêu cầu tới API cần Bearer Token. Sau khi đăng nhập thành công, token sẽ được lưu trong
> `localStorage`, tự động gia hạn bằng refresh token khi sắp hết hạn và gửi kèm ID người dùng cho các
> lời gọi kế tiếp.
