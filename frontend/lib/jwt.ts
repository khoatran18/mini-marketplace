import { decodeJwt, type JWTPayload } from 'jose';
import type { Role } from './types';

interface AuthTokenPayload extends JWTPayload {
  UserID?: number | string;
  userID?: number | string;
  userId?: number | string;
  userid?: number | string;
  sub?: number | string;
  Username?: string;
  username?: string;
  Role?: string;
  role?: string;
  Type?: string;
  type?: string;
}

export interface DecodedAuthToken {
  userId: number | null;
  username: string | null;
  role: Role | null;
  expiresAt: number | null;
  tokenType: string | null;
}

const validRoles: Role[] = ['buyer', 'seller_admin', 'seller_employee'];

function coerceUserId(value: unknown): number | null {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === 'string') {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) {
      return parsed;
    }
  }
  return null;
}

function coerceRole(value: unknown): Role | null {
  if (typeof value !== 'string') {
    return null;
  }
  const normalized = value as Role;
  return validRoles.includes(normalized) ? normalized : null;
}

export function decodeAuthToken(token: string | null): DecodedAuthToken {
  if (!token) {
    return { userId: null, username: null, role: null, expiresAt: null, tokenType: null };
  }

  try {
    const payload = decodeJwt(token) as AuthTokenPayload;
    const userId =
      coerceUserId(payload.UserID) ??
      coerceUserId(payload.userID) ??
      coerceUserId(payload.userId) ??
      coerceUserId(payload.userid) ??
      coerceUserId(payload.sub);

    const username =
      typeof payload.Username === 'string'
        ? payload.Username
        : typeof payload.username === 'string'
          ? payload.username
          : null;

    const role = coerceRole(payload.Role ?? payload.role);

    const expiresAt = typeof payload.exp === 'number' ? payload.exp : null;

    const tokenType =
      typeof payload.Type === 'string'
        ? payload.Type
        : typeof payload.type === 'string'
          ? payload.type
          : null;

    return {
      userId,
      username,
      role,
      expiresAt,
      tokenType
    };
  } catch (error) {
    console.warn('Failed to decode auth token', error);
    return { userId: null, username: null, role: null, expiresAt: null, tokenType: null };
  }
}

export function isTokenExpired(expiresAt: number | null, bufferSeconds = 15): boolean {
  if (!expiresAt) {
    return true;
  }
  const nowInSeconds = Math.floor(Date.now() / 1000);
  return expiresAt - bufferSeconds <= nowInSeconds;
}
