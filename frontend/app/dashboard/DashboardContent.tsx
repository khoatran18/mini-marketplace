'use client';

import { useAuth } from '../../components/auth/AuthProvider';
import { CreateProductForm } from '../../components/CreateProductForm';
import { CreateOrderForm } from '../../components/CreateOrderForm';

export function DashboardContent() {
  const { username, role } = useAuth();
  const isSeller = role === 'seller_admin' || role === 'seller_employee';
  const isBuyer = role === 'buyer';

  return (
    <div className="grid" style={{ gap: '1.5rem' }}>
      <header className="card" style={{ display: 'grid', gap: '0.5rem' }}>
        <h1 style={{ margin: 0 }}>Bảng điều khiển</h1>
        <p style={{ margin: 0 }}>
          Đăng nhập bởi <strong>{username}</strong> với vai trò <strong>{role}</strong>.
        </p>
      </header>
      {isSeller ? (
        <CreateProductForm />
      ) : (
        <div className="card">
          <h2 style={{ marginTop: 0 }}>Quản lý sản phẩm</h2>
          <p style={{ marginBottom: 0 }}>
            Chỉ tài khoản <strong>seller_admin</strong> hoặc <strong>seller_employee</strong> mới có quyền tạo
            và chỉnh sửa sản phẩm. Vui lòng đăng nhập bằng tài khoản người bán nếu bạn cần truy cập
            tính năng này.
          </p>
        </div>
      )}
      {isBuyer ? (
        <div className="card">
          <h2 style={{ marginTop: 0 }}>Đơn hàng của bạn</h2>
          <p style={{ marginBottom: 0 }}>
            Thêm sản phẩm vào giỏ hàng từ trang Sản phẩm, sau đó truy cập mục{' '}
            <strong>Giỏ hàng</strong> để hoàn tất đơn hàng thông qua API Gateway.
          </p>
        </div>
      ) : (
        <CreateOrderForm />
      )}
    </div>
  );
}
