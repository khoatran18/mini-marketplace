import { AppShell } from '../../components/layout/AppShell';

export default function SellerLayout({ children }: { children: React.ReactNode }) {
  return (
    <AppShell
      title="Kênh người bán"
      roles={['seller_admin', 'seller_employee']}
      links={[
        { href: '/seller', label: 'Tổng quan', icon: 'home', exact: true },
        { href: '/seller/orders', label: 'Đơn hàng', icon: 'orders' },
        { href: '/seller/products', label: 'Sản phẩm', icon: 'package' },
        { href: '/seller/inventory', label: 'Tồn kho', icon: 'box' },
        { href: '/seller/analytics', label: 'Phân tích', icon: 'chart' }
      ]}
    >
      {children}
    </AppShell>
  );
}
