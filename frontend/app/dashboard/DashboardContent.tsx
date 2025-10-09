'use client';

import { useAuth } from '../../components/auth/AuthProvider';
import { CreateProductForm } from '../../components/CreateProductForm';

export function DashboardContent() {
  const { username, role } = useAuth();
  const isSeller = role === 'seller_admin' || role === 'seller_employee';
  const isBuyer = role === 'buyer';

  return (
    <div className="grid gap-6">
      <header className="card grid gap-2">
        <h1 className="text-2xl font-bold text-slate-900">Bảng điều khiển</h1>
        <p className="text-sm text-slate-600">
          Đăng nhập bởi <strong>{username}</strong> với vai trò <strong>{role}</strong>.
        </p>
      </header>
      {isSeller ? (
        <CreateProductForm />
      ) : (
        <div className="card grid gap-2">
          <h2 className="text-xl font-semibold text-slate-900">Quản lý sản phẩm</h2>
          <p className="text-sm text-slate-600">
            Chỉ tài khoản <strong>seller_admin</strong> hoặc <strong>seller_employee</strong> mới có quyền tạo và chỉnh sửa sản
            phẩm. Vui lòng đăng nhập bằng tài khoản người bán nếu bạn cần truy cập tính năng này.
          </p>
        </div>
      )}
      <div className="card grid gap-2">
        <h2 className="text-xl font-semibold text-slate-900">Đơn hàng</h2>
        {isBuyer ? (
          <p className="text-sm text-slate-600">
            Thêm sản phẩm vào giỏ hàng từ trang <strong>Sản phẩm</strong>, sau đó truy cập mục <strong>Giỏ hàng</strong> để hoàn
            tất đơn hàng.
          </p>
        ) : (
          <p className="text-sm text-slate-600">
            Chỉ tài khoản người mua mới có thể tạo đơn hàng. Bạn vẫn có thể theo dõi các yêu cầu mua hàng của khách trong hệ
            thống quản trị hoặc xem sản phẩm của mình tại mục <strong>Sản phẩm của tôi</strong>.
          </p>
        )}
      </div>
    </div>
  );
}
