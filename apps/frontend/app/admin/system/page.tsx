import { ProtectedContent } from '../../../components/ProtectedContent';
import { SystemHealthClient } from './SystemHealthClient';

export default function AdminSystemPage() {
  return (
    <ProtectedContent allowedRoles={['admin']}>
      <SystemHealthClient />
    </ProtectedContent>
  );
}
