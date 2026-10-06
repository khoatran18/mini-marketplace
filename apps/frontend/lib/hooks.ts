'use client';

import { useCallback, useEffect, useRef, useState, type DependencyList } from 'react';
import { useAuth } from '../components/auth/AuthProvider';

export interface RequestState<T> {
  data: T | null;
  error: string | null;
  status: number | null;
  loading: boolean;
  reload: () => void;
}

/**
 * Loads data with the current access token (when signed in). Waits until the session has been restored, ignores
 * stale responses and exposes loading / error / data / reload. `auth: 'optional'` is for public endpoints: the token
 * is sent when there is one. `auth: 'required'` (default) waits for a token.
 */
export function useApiData<T>(
  loader: (token: string | null) => Promise<T>,
  deps: DependencyList,
  options: { auth?: 'required' | 'optional' | 'none'; enabled?: boolean } = {}
): RequestState<T> {
  const { auth = 'required', enabled = true } = options;
  const { ready, accessToken, getValidAccessToken } = useAuth();
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [status, setStatus] = useState<number | null>(null);
  const [loading, setLoading] = useState(enabled);
  const [tick, setTick] = useState(0);
  const loaderRef = useRef(loader);
  loaderRef.current = loader;

  useEffect(() => {
    if (!enabled || !ready) return;
    let cancelled = false;
    setLoading(true);
    setError(null);
    setStatus(null);
    void (async () => {
      try {
        let token: string | null = null;
        if (auth !== 'none' && accessToken) {
          token = await getValidAccessToken();
        }
        if (auth === 'required' && !token) {
          throw new Error('Phiên đăng nhập đã hết hạn, vui lòng đăng nhập lại.');
        }
        const result = await loaderRef.current(token);
        if (!cancelled) setData(result);
      } catch (err) {
        if (!cancelled) {
          setError((err as Error).message);
          setStatus((err as { status?: number }).status ?? null);
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, ready, accessToken, auth, tick, ...deps]);

  const reload = useCallback(() => setTick((value) => value + 1), []);
  return { data, error, status, loading, reload };
}

/** Returns a function that runs `fn` with a fresh access token (for button actions). */
export function useAuthedAction() {
  const { getValidAccessToken } = useAuth();
  return useCallback(
    async <T,>(fn: (token: string) => Promise<T>): Promise<T> => {
      const token = await getValidAccessToken();
      if (!token) throw new Error('Phiên đăng nhập đã hết hạn, vui lòng đăng nhập lại.');
      return fn(token);
    },
    [getValidAccessToken]
  );
}
