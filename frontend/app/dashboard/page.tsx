import { ProtectedContent } from '../../components/ProtectedContent';
import { DashboardContent } from './DashboardContent';

export default function DashboardPage() {
  return (
    <ProtectedContent allowedRoles={['seller_admin', 'seller_employee']}>
      <DashboardContent />
    </ProtectedContent>
  );
}
