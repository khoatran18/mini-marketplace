// Client HTTP mỏng trên fetch có sẵn của Node 18+. Tự chờ rồi thử lại khi gặp 429 (rate limit theo IP/phút).
import { config } from './config.mjs';

export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

export async function call(method, path, { token, body, headers = {}, retry429 = 2, raw = false } = {}) {
  for (let attempt = 0; ; attempt++) {
    let res;
    try {
      res = await fetch(config.api + path, {
        method,
        headers: {
          ...(body !== undefined ? { 'Content-Type': 'application/json' } : {}),
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
          ...headers,
        },
        body: body === undefined ? undefined : raw ? body : JSON.stringify(body),
        signal: AbortSignal.timeout(20000),
      });
    } catch (e) {
      return { status: 0, body: null, headers: new Headers(), error: String(e.cause?.code ?? e.message) };
    }
    if (res.status === 429 && attempt < retry429) {
      const wait = Number(res.headers.get('retry-after')) || 15;
      console.log(`    (429 ${method} ${path}: chờ ${wait}s rồi thử lại)`);
      await sleep(wait * 1000);
      continue;
    }
    let json = null;
    const text = await res.text();
    try { json = text ? JSON.parse(text) : null; } catch { json = { _text: text }; }
    return { status: res.status, body: json, headers: res.headers };
  }
}

export const get = (path, o) => call('GET', path, o);
export const post = (path, body, o = {}) => call('POST', path, { ...o, body });
export const put = (path, body, o = {}) => call('PUT', path, { ...o, body });
export const patch = (path, body, o = {}) => call('PATCH', path, { ...o, body });
export const del = (path, o) => call('DELETE', path, o);

// Giải mã payload JWT (không kiểm chữ ký - chỉ để đọc UserID/Role)
export function jwtClaims(token) {
  try { return JSON.parse(Buffer.from(token.split('.')[1], 'base64url').toString()); } catch { return {}; }
}
