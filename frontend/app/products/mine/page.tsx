import { ProtectedContent } from '../../../components/ProtectedContent';
import { MyProductsPageClient } from './MyProductsPageClient';

export default function MyProductsPage() {
  return (
    <ProtectedContent allowedRoles={['seller_admin', 'seller_employee']}>
      <MyProductsPageClient />
    </ProtectedContent>
  );
}
