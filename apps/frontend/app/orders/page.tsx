import { OrdersList } from '../../components/orders/OrdersList';
import { ProtectedContent } from '../../components/ProtectedContent';
import { PageTitle } from '../../components/layout/PageTitle';

export default function OrdersPage() {
  return (
    <ProtectedContent allowedRoles={['buyer']}>
      <div className="grid gap-5">
        <header className="card grid gap-1">
          <PageTitle icon="orders">Đơn hàng của tôi</PageTitle>
          <p className="text-sm text-muted">Theo dõi trạng thái, thanh toán, huỷ đơn và yêu cầu đổi trả.</p>
        </header>
        <OrdersList scope="buyer" />
      </div>
    </ProtectedContent>
  );
}
