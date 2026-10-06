import { TrackingDebugClient } from './TrackingDebugClient';

export default function DevTrackingPage() {
  // The tool is for development; a production-like build hides it unless NEXT_PUBLIC_APP_ENV=dev.
  const enabled = process.env.NODE_ENV !== 'production' || process.env.NEXT_PUBLIC_APP_ENV === 'dev';
  if (!enabled) {
    return <div className="card text-sm text-muted">Trang này chỉ có ở môi trường dev.</div>;
  }
  return <TrackingDebugClient />;
}
