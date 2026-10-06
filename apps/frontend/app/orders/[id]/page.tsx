import { notFound } from 'next/navigation';
import { OrderDetail } from '../../../components/orders/OrderDetail';
import { ProtectedContent } from '../../../components/ProtectedContent';

export default function OrderDetailPage({ params }: { params: { id: string } }) {
  const id = Number(params.id);
  if (!Number.isInteger(id) || id <= 0) {
    notFound();
  }
  return (
    <ProtectedContent allowedRoles={['buyer']}>
      <OrderDetail scope="buyer" orderId={id} />
    </ProtectedContent>
  );
}
