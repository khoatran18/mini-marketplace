import { ProtectedContent } from '../../components/ProtectedContent';
import { OrdersPageClient } from './OrdersPageClient';

export default function OrdersPage() {
  return (
    <ProtectedContent allowedRoles={['buyer']}>
      <OrdersPageClient />
    </ProtectedContent>
  );
}
