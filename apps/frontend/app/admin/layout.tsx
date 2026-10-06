import { ConsoleLayout } from '../../components/layout/ConsoleLayout';

export default function AdminLayout({ children }: { children: React.ReactNode }) {
  return (
    <ConsoleLayout
      title="Quản trị"
      roles={['admin']}
      links={[
        { href: '/admin', label: 'Tổng quan', exact: true },
        { href: '/admin/orders', label: 'Đơn hàng' },
        { href: '/admin/payments', label: 'Thanh toán' },
        { href: '/admin/categories', label: 'Danh mục' },
        { href: '/admin/analytics', label: 'Phân tích' },
        { href: '/admin/system', label: 'Hệ thống' }
      ]}
    >
      {children}
    </ConsoleLayout>
  );
}
