import { SystemHealthClient } from './SystemHealthClient';

// access is guarded by app/admin/layout.tsx (AppShell)
export default function AdminSystemPage() {
  return <SystemHealthClient />;
}
