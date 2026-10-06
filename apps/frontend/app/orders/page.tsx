import { OrdersList } from '../../components/orders/OrdersList';
import { ProtectedContent } from '../../components/ProtectedContent';

export default function OrdersPage() {
  return (
    <ProtectedContent allowedRoles={['buyer']}>
      <div className="grid gap-5">
        <header className="card grid gap-1">
          <h1 className="text-2xl font-bold text-text">Đơn hàng của tôi</h1>
          <p className="text-sm text-muted">Theo dõi trạng thái, thanh toán, huỷ đơn và yêu cầu đổi trả.</p>
        </header>
        <OrdersList scope="buyer" />
      </div>
    </ProtectedContent>
  );
}
