import { ProtectedContent } from '../../components/ProtectedContent';
import { ProfilePageClient } from './ProfilePageClient';

export default function ProfilePage() {
  return (
    <ProtectedContent>
      <ProfilePageClient />
    </ProtectedContent>
  );
}
