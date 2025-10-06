import { ProtectedContent } from '../../components/ProtectedContent';
import { DashboardContent } from './DashboardContent';

export default function DashboardPage() {
  return (
    <ProtectedContent>
      <DashboardContent />
    </ProtectedContent>
  );
}
