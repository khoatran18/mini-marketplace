import { ConsoleLayout } from '../../components/layout/ConsoleLayout';

export default function SellerLayout({ children }: { children: React.ReactNode }) {
  return (
    <ConsoleLayout
      title="Kênh người bán"
      roles={['seller_admin', 'seller_employee']}
      links={[
        { href: '/seller', label: 'Tổng quan', exact: true },
        { href: '/seller/orders', label: 'Đơn hàng' },
        { href: '/seller/products', label: 'Sản phẩm' },
        { href: '/seller/inventory', label: 'Tồn kho' },
        { href: '/seller/analytics', label: 'Phân tích' }
      ]}
    >
      {children}
    </ConsoleLayout>
  );
}
