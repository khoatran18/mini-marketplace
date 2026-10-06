'use client';

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState
} from 'react';
import type {
  LoginInput,
  LoginOutput,
  RegisterInput,
  RegisterOutput,
  Role
} from '../../lib/types';
import { loginRequest, refreshTokenRequest, registerRequest } from '../../lib/api';
import { decodeAuthToken, isTokenExpired } from '../../lib/jwt';

export interface AuthState {
  accessToken: string | null;
  refreshToken: string | null;
  username: string | null;
  role: Role | null;
  userId: number | null;
  accessTokenExpiresAt: number | null;
  refreshTokenExpiresAt: number | null;
}

interface AuthContextValue extends AuthState {
  login: (input: LoginInput) => Promise<LoginOutput>;
  register: (input: RegisterInput) => Promise<RegisterOutput>;
  logout: () => void;
  getValidAccessToken: () => Promise<string | null>;
}

const validRoles: Role[] = ['buyer', 'seller_admin', 'seller_employee'];

const defaultState: AuthState = {
  accessToken: null,
  refreshToken: null,
  username: null,
  role: null,
  userId: null,
  accessTokenExpiresAt: null,
  refreshTokenExpiresAt: null
};

const AuthContext = createContext<AuthContextValue | undefined>(undefined);

const STORAGE_KEY = 'mini-marketplace-auth';

function isRole(value: unknown): value is Role {
  return typeof value === 'string' && validRoles.includes(value as Role);
}

function deriveAuthState(partial: Partial<AuthState>): AuthState {
  const accessToken = partial.accessToken ?? null;
  const refreshToken = partial.refreshToken ?? null;
  const decodedAccess = decodeAuthToken(accessToken);
  const decodedRefresh = decodeAuthToken(refreshToken);

  const fallbackRoleCandidate = partial.role ?? decodedRefresh.role ?? null;
  const sanitizedFallbackRole = isRole(fallbackRoleCandidate) ? fallbackRoleCandidate : null;
  const resolvedRole = decodedAccess.role ?? sanitizedFallbackRole;

  return {
    accessToken,
    refreshToken,
    username: decodedAccess.username ?? partial.username ?? decodedRefresh.username ?? null,
    role: resolvedRole,
    userId: decodedAccess.userId ?? partial.userId ?? decodedRefresh.userId ?? null,
    accessTokenExpiresAt: decodedAccess.expiresAt ?? partial.accessTokenExpiresAt ?? null,
    refreshTokenExpiresAt: decodedRefresh.expiresAt ?? partial.refreshTokenExpiresAt ?? null
  };
}

function loadPersistedState(): AuthState {
  if (typeof window === 'undefined') {
    return defaultState;
  }

  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) {
      return defaultState;
    }

    const parsed = JSON.parse(raw) as Partial<AuthState>;
    return deriveAuthState(parsed);
  } catch (error) {
    console.warn('Failed to parse auth state from storage', error);
    return defaultState;
  }
}

function persistState(state: AuthState) {
  if (typeof window === 'undefined') {
    return;
  }

  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
  } catch (error) {
    console.warn('Failed to persist auth state', error);
  }
}

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<AuthState>(defaultState);
  const refreshPromiseRef = useRef<Promise<string | null> | null>(null);
  const refreshTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const logout = useCallback(() => {
    if (refreshTimeoutRef.current) {
      clearTimeout(refreshTimeoutRef.current);
      refreshTimeoutRef.current = null;
    }
    refreshPromiseRef.current = null;
    setState(defaultState);
    persistState(defaultState);
  }, []);

  useEffect(() => {
    setState(loadPersistedState());
  }, []);

  useEffect(() => {
    if (state.refreshToken && isTokenExpired(state.refreshTokenExpiresAt, 0)) {
      logout();
    }
  }, [state.refreshToken, state.refreshTokenExpiresAt, logout]);

  const login = useCallback(async (input: LoginInput) => {
    const output = await loginRequest(input);
    const resolvedUserIdRaw =
      typeof output.user_id === 'number'
        ? output.user_id
        : typeof output.user_id === 'string'
          ? Number(output.user_id)
          : null;
    const resolvedUserId =
      typeof resolvedUserIdRaw === 'number' && Number.isFinite(resolvedUserIdRaw)
        ? resolvedUserIdRaw
        : null;
    const nextState = deriveAuthState({
      accessToken: output.access_token ?? null,
      refreshToken: output.refresh_token ?? null,
      username: output.username ?? input.username,
      role: output.role ?? input.role,
      userId: resolvedUserId
    });
    setState(nextState);
    persistState(nextState);
    return output;
  }, []);

  const register = useCallback(async (input: RegisterInput) => {
    return registerRequest(input);
  }, []);

  const refreshTokens = useCallback(async () => {
    if (!state.refreshToken || isTokenExpired(state.refreshTokenExpiresAt, 0)) {
      logout();
      return null;
    }

    if (!refreshPromiseRef.current) {
      refreshPromiseRef.current = (async () => {
        try {
          const response = await refreshTokenRequest(state.refreshToken as string);
          let refreshedToken: string | null = response.access_token ?? null;
          setState((previous) => {
            const nextState = deriveAuthState({
              ...previous,
              accessToken: response.access_token ?? null,
              refreshToken: response.refresh_token ?? previous.refreshToken
            });
            persistState(nextState);
            refreshedToken = nextState.accessToken;
            return nextState;
          });
          return refreshedToken;
        } catch (error) {
          console.warn('Failed to refresh tokens', error);
          logout();
          return null;
        }
      })();

      refreshPromiseRef.current.finally(() => {
        refreshPromiseRef.current = null;
      });
    }

    return refreshPromiseRef.current;
  }, [state.refreshToken, state.refreshTokenExpiresAt, logout]);

  useEffect(() => {
    if (typeof window === 'undefined') {
      return;
    }

    if (refreshTimeoutRef.current) {
      clearTimeout(refreshTimeoutRef.current);
      refreshTimeoutRef.current = null;
    }

    if (!state.accessToken || !state.accessTokenExpiresAt) {
      return;
    }

    const now = Date.now();
    const expiry = state.accessTokenExpiresAt * 1000;
    const bufferMs = 30_000;
    const delay = Math.max(expiry - now - bufferMs, 0);

    refreshTimeoutRef.current = setTimeout(() => {
      void refreshTokens();
    }, delay);

    return () => {
      if (refreshTimeoutRef.current) {
        clearTimeout(refreshTimeoutRef.current);
        refreshTimeoutRef.current = null;
      }
    };
  }, [state.accessToken, state.accessTokenExpiresAt, refreshTokens]);

  const getValidAccessToken = useCallback(async () => {
    if (!state.accessToken) {
      return refreshTokens();
    }

    if (!isTokenExpired(state.accessTokenExpiresAt)) {
      return state.accessToken;
    }

    return refreshTokens();
  }, [state.accessToken, state.accessTokenExpiresAt, refreshTokens]);

  const value = useMemo(
    () => ({
      ...state,
      login,
      register,
      logout,
      getValidAccessToken
    }),
    [state, login, register, logout, getValidAccessToken]
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }

  return context;
}
