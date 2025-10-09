import { ProtectedContent } from '../../components/ProtectedContent';
import { CartPageClient } from './CartPageClient';

export default function CartPage() {
  return (
    <ProtectedContent allowedRoles={['buyer']}>
      <CartPageClient />
    </ProtectedContent>
  );
}
