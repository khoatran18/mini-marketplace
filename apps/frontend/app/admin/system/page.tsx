import { SystemHealthClient } from './SystemHealthClient';

// access is guarded by app/admin/layout.tsx (ConsoleLayout)
export default function AdminSystemPage() {
  return <SystemHealthClient />;
}
