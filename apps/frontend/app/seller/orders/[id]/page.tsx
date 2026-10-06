import { notFound } from 'next/navigation';
import { OrderDetail } from '../../../../components/orders/OrderDetail';
import { ProtectedContent } from '../../../../components/ProtectedContent';

export default function OrderDetailPage({ params }: { params: { id: string } }) {
  const id = Number(params.id);
  if (!Number.isInteger(id) || id <= 0) {
    notFound();
  }
  return (
    <ProtectedContent allowedRoles={['seller_admin', 'seller_employee']}>
      <OrderDetail scope="seller" orderId={id} />
    </ProtectedContent>
  );
}
