import { ProtectedContent } from '../../components/ProtectedContent';
import { CheckoutClient } from './CheckoutClient';

export default function CheckoutPage() {
  return (
    <ProtectedContent allowedRoles={['buyer']}>
      <CheckoutClient />
    </ProtectedContent>
  );
}
