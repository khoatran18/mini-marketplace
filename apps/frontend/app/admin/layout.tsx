import { AppShell } from '../../components/layout/AppShell';

// `dev` runs the system (ops flow, health) but must not see business data: the gateway returns 403 for those routes,
// the menu simply does not offer them.
export default function AdminLayout({ children }: { children: React.ReactNode }) {
  return (
    <AppShell
      title="Quản trị"
      roles={['admin', 'dev']}
      links={[
        { href: '/admin', label: 'Tổng quan', icon: 'home', exact: true, roles: ['admin'] },
        { href: '/admin/orders', label: 'Đơn hàng', icon: 'orders', roles: ['admin'] },
        { href: '/admin/payments', label: 'Thanh toán', icon: 'payments', roles: ['admin'] },
        { href: '/admin/categories', label: 'Danh mục', icon: 'tag', roles: ['admin'] },
        { href: '/admin/analytics', label: 'Phân tích', icon: 'chart', roles: ['admin'] },
        { href: '/admin/system', label: 'Hệ thống', icon: 'map' }
      ]}
    >
      {children}
    </AppShell>
  );
}
