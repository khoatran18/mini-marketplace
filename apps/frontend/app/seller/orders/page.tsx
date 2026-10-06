import { OrdersList } from '../../../components/orders/OrdersList';

export default function SellerOrdersPage() {
  return (
    <div className="grid gap-5">
      <header className="grid gap-1">
        <h1 className="text-2xl font-bold text-text">Hộp thư đơn hàng</h1>
        <p className="text-sm text-muted">Chỉ gồm đơn của cửa hàng bạn. Đơn đã thanh toán hoặc COD đã xác nhận cần được giao.</p>
      </header>
      <OrdersList scope="seller" />
    </div>
  );
}
