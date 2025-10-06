'use client';

import { useAuth } from '../../components/auth/AuthProvider';
import { CreateProductForm } from '../../components/CreateProductForm';
import { CreateOrderForm } from '../../components/CreateOrderForm';

export function DashboardContent() {
  const { username, role } = useAuth();

  return (
    <div className="grid" style={{ gap: '1.5rem' }}>
      <header className="card" style={{ display: 'grid', gap: '0.5rem' }}>
        <h1 style={{ margin: 0 }}>Bảng điều khiển</h1>
        <p style={{ margin: 0 }}>
          Đăng nhập bởi <strong>{username}</strong> với vai trò <strong>{role}</strong>.
        </p>
      </header>
      <CreateProductForm />
      <CreateOrderForm />
    </div>
  );
}
