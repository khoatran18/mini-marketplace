import { OrdersList } from '../../../components/orders/OrdersList';
import { PageTitle } from '../../../components/layout/PageTitle';

export default function AdminOrdersPage() {
  return (
    <div className="grid gap-5">
      <header className="grid gap-1">
        <PageTitle icon="orders">Tất cả đơn hàng</PageTitle>
        <p className="text-sm text-muted">Hỗ trợ chăm sóc khách hàng: xem mọi đơn, huỷ, duyệt hoặc từ chối hoàn trả.</p>
      </header>
      <OrdersList scope="admin" />
    </div>
  );
}
