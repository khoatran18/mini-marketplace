'use client';

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import type {
  LoginInput,
  LoginOutput,
  RegisterInput,
  RegisterOutput
} from '../../lib/types';
import { loginRequest, registerRequest } from '../../lib/api';

export interface AuthState {
  accessToken: string | null;
  refreshToken: string | null;
  username: string | null;
  role: string | null;
}

interface AuthContextValue extends AuthState {
  login: (input: LoginInput) => Promise<LoginOutput>;
  register: (input: RegisterInput) => Promise<RegisterOutput>;
  logout: () => void;
}

const defaultState: AuthState = {
  accessToken: null,
  refreshToken: null,
  username: null,
  role: null
};

const AuthContext = createContext<AuthContextValue | undefined>(undefined);

const STORAGE_KEY = 'mini-marketplace-auth';

function loadPersistedState(): AuthState {
  if (typeof window === 'undefined') {
    return defaultState;
  }

  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) {
      return defaultState;
    }

    const parsed = JSON.parse(raw) as AuthState;
    return {
      accessToken: parsed.accessToken ?? null,
      refreshToken: parsed.refreshToken ?? null,
      username: parsed.username ?? null,
      role: parsed.role ?? null
    };
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

  useEffect(() => {
    setState(loadPersistedState());
  }, []);

  const login = useCallback(async (input: LoginInput) => {
    const output = await loginRequest(input);
    const nextState: AuthState = {
      accessToken: output.access_token ?? null,
      refreshToken: output.refresh_token ?? null,
      username: input.username,
      role: input.role
    };
    setState(nextState);
    persistState(nextState);
    return output;
  }, []);

  const register = useCallback(async (input: RegisterInput) => {
    return registerRequest(input);
  }, []);

  const logout = useCallback(() => {
    setState(defaultState);
    persistState(defaultState);
  }, []);

  const value = useMemo(
    () => ({
      ...state,
      login,
      register,
      logout
    }),
    [state, login, register, logout]
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
