import { OrdersList } from '../../../components/orders/OrdersList';
import { PageTitle } from '../../../components/layout/PageTitle';

export default function SellerOrdersPage() {
  return (
    <div className="grid gap-5">
      <header className="grid gap-1">
        <PageTitle icon="orders">Hộp thư đơn hàng</PageTitle>
        <p className="text-sm text-muted">Chỉ gồm đơn của cửa hàng bạn. Đơn đã thanh toán hoặc COD đã xác nhận cần được giao.</p>
      </header>
      <OrdersList scope="seller" />
    </div>
  );
}
