import { OrdersList } from '../../../components/orders/OrdersList';

export default function AdminOrdersPage() {
  return (
    <div className="grid gap-5">
      <header className="grid gap-1">
        <h1 className="text-2xl font-bold text-text">Tất cả đơn hàng</h1>
        <p className="text-sm text-muted">Hỗ trợ chăm sóc khách hàng: xem mọi đơn, huỷ, duyệt hoặc từ chối hoàn trả.</p>
      </header>
      <OrdersList scope="admin" />
    </div>
  );
}
