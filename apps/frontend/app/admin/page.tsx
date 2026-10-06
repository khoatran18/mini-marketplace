import Link from 'next/link';
import { AdminOverviewClient } from './AdminOverviewClient';

const links = [
  { href: '/admin/orders', title: 'Đơn hàng', text: 'Xem mọi đơn, huỷ, duyệt hoàn trả' },
  { href: '/admin/payments', title: 'Thanh toán', text: 'Giao dịch mô phỏng, hoàn tiền' },
  { href: '/admin/categories', title: 'Danh mục', text: 'Cây danh mục sản phẩm' },
  { href: '/admin/analytics', title: 'Phân tích', text: 'Doanh thu, traffic, funnel, thanh toán' },
  { href: '/admin/system', title: 'Hệ thống', text: 'Sức khoẻ các service' }
];

export default function AdminHomePage() {
  return (
    <div className="grid gap-6">
      <header className="card grid gap-1">
        <h1 className="text-2xl font-bold text-text">Quản trị</h1>
        <p className="text-sm text-muted">Quyền thật được kiểm tra ở gateway; ẩn/hiện menu chỉ để tiện dùng.</p>
      </header>
      <AdminOverviewClient />
      <ul className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {links.map((link) => (
          <li key={link.href}>
            <Link href={link.href} className="card block h-full p-5 transition hover:border-brand">
              <span className="text-lg font-semibold text-text">{link.title}</span>
              <span className="block text-sm text-muted">{link.text}</span>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
