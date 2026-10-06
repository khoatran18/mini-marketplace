'use client';

import { usePathname } from 'next/navigation';
import { useEffect, useRef } from 'react';
import { useAuth } from '../auth/AuthProvider';
import { getTracker, identify, setTrackingTokenProvider, surfaceForPath, track } from '../../lib/tracking';

/** Wires the tracking SDK into the app: page views, identify on login, global client errors. Renders nothing. */
export function TrackingProvider() {
  const pathname = usePathname();
  const { ready, accessToken, userId } = useAuth();
  const tokenRef = useRef<string | null>(null);
  tokenRef.current = accessToken;

  useEffect(() => {
    getTracker();
    setTrackingTokenProvider(() => tokenRef.current);
    return () => setTrackingTokenProvider(null);
  }, []);

  useEffect(() => {
    if (!pathname) return;
    track('page_view', {}, { surface: surfaceForPath(pathname) });
  }, [pathname]);

  useEffect(() => {
    if (!ready) return;
    if (accessToken && userId) {
      identify(userId);
    } else {
      getTracker()?.reset();
    }
  }, [ready, accessToken, userId]);

  useEffect(() => {
    const onError = (event: ErrorEvent) => track('error_client', { code: 'js_error', where: event.filename ? 'script' : 'window', message: String(event.message).slice(0, 200) });
    const onRejection = (event: PromiseRejectionEvent) =>
      track('error_client', { code: 'unhandled_rejection', where: 'promise', message: String((event.reason as Error)?.message ?? event.reason).slice(0, 200) });
    window.addEventListener('error', onError);
    window.addEventListener('unhandledrejection', onRejection);
    return () => {
      window.removeEventListener('error', onError);
      window.removeEventListener('unhandledrejection', onRejection);
    };
  }, []);

  return null;
}
