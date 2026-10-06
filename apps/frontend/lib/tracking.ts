// Tracking SDK (docs/platform/07-ui-design.md §5, 03-data-model.md §2, 05-analytics-monitoring.md).
//
//   track(type, props?, ctx?)  queue an event (never throws, never blocks the UI)
//   identify(userId)           link the anonymous id to the signed-in user (POST /events/identify)
//   setConsent(analytics)      without consent only page_view / error_client are collected, without identifiers
//
// Events are batched (flush every 5 s or at 20 events, at most 50 per request), sent with `fetch(keepalive)` and
// `sendBeacon` when the tab is hidden, retried with exponential back-off and then dropped. The gateway endpoint may not
// exist yet: a 404/405/501 switches the SDK off for this page load, silently. The server adds user_id / role / store_id
// from the JWT; the client never sends them.

import { getBaseUrl } from './api';

export type TrackEventType =
  | 'page_view'
  | 'impression'
  | 'product_click'
  | 'product_view'
  | 'dwell'
  | 'search'
  | 'search_click'
  | 'filter_apply'
  | 'sort_change'
  | 'add_to_cart'
  | 'remove_from_cart'
  | 'cart_update'
  | 'wishlist_add'
  | 'wishlist_remove'
  | 'checkout_start'
  | 'checkout_submit'
  | 'payment_page_view'
  | 'error_client';

export type Surface =
  | 'home_trending'
  | 'home_new'
  | 'category_page'
  | 'search_results'
  | 'pdp'
  | 'cart'
  | 'checkout'
  | 'orders'
  | 'seller_console'
  | 'admin_console';

export interface TrackContext {
  surface?: Surface | string;
  item?: { product_id?: number; position?: number };
}

export interface TrackingEnvelope {
  event_id: string;
  event_type: TrackEventType;
  ts_client: string;
  anonymous_id: string;
  session_id: string;
  surface?: string;
  page: { path: string; referrer: string };
  item?: { product_id?: number; position?: number };
  props: Record<string, unknown>;
  device: { type: string; os: string; viewport: string };
  consent: { analytics: boolean };
  app_version: string;
}

export type DebugState = 'queued' | 'sending' | 'sent' | 'failed' | 'dropped';

export interface DebugEntry {
  event: TrackingEnvelope;
  state: DebugState;
  attempts: number;
  note?: string;
}

export const BATCH_INTERVAL_MS = 5000;
export const BATCH_SIZE = 20;
export const MAX_BATCH = 50; // server limit per request
export const MAX_ATTEMPTS = 3;
export const SESSION_TTL_MS = 30 * 60 * 1000;
const NO_CONSENT_TYPES: TrackEventType[] = ['page_view', 'error_client'];
const DEBUG_LIMIT = 200;

/** Everything the tracker needs from the outside world: injectable, so batching can be unit tested. */
export interface TrackerEnv {
  now: () => number;
  uuid: () => string;
  baseUrl: string;
  enabled: boolean;
  appVersion: string;
  getToken: () => string | null;
  context: () => { path: string; referrer: string; device: { type: string; os: string; viewport: string } };
  storage: { get(key: string): string | null; set(key: string, value: string): void };
  cookie: { get(name: string): string | null; set(name: string, value: string, maxAgeSeconds: number): void };
  send: (url: string, body: string, token: string | null) => Promise<{ status: number }>;
  beacon: (url: string, body: string) => boolean;
  setTimer: (fn: () => void, ms: number) => unknown;
  clearTimer: (handle: unknown) => void;
  random: () => number;
}

function rid(env: TrackerEnv): string {
  return env.uuid().replace(/-/g, '').slice(0, 12);
}

export class Tracker {
  private queue: DebugEntry[] = [];
  private log: DebugEntry[] = [];
  private timer: unknown = null;
  private flushing = false;
  private seenImpressions = new Set<string>();
  private identified = new Set<string>();
  private listeners = new Set<() => void>();
  private disabledReason: string | null = null;
  private consent: boolean;
  private sessionCache: { id: string; last: number } | null = null;
  private anonCache: string | null = null;

  constructor(private env: TrackerEnv) {
    this.consent = env.storage.get('mm_consent') !== 'denied';
  }

  // ---- identifiers -------------------------------------------------------------------------------------------

  anonymousId(): string {
    if (this.anonCache) return this.anonCache;
    let id = this.env.cookie.get('mm_aid') ?? this.env.storage.get('mm_aid');
    if (!id) id = `a_${rid(this.env)}`;
    // refresh the 13 month first-party cookie on every start
    this.env.cookie.set('mm_aid', id, 60 * 60 * 24 * 395);
    this.env.storage.set('mm_aid', id);
    this.anonCache = id;
    return id;
  }

  sessionId(): string {
    const now = this.env.now();
    if (!this.sessionCache) {
      try {
        const raw = this.env.storage.get('mm_sid');
        if (raw) this.sessionCache = JSON.parse(raw) as { id: string; last: number };
      } catch {
        this.sessionCache = null;
      }
    }
    if (!this.sessionCache || now - this.sessionCache.last > SESSION_TTL_MS) {
      this.sessionCache = { id: `s_${rid(this.env)}`, last: now };
    } else {
      this.sessionCache.last = now;
    }
    this.env.storage.set('mm_sid', JSON.stringify(this.sessionCache));
    return this.sessionCache.id;
  }

  // ---- public API --------------------------------------------------------------------------------------------

  setConsent(analytics: boolean) {
    this.consent = analytics;
    this.env.storage.set('mm_consent', analytics ? 'granted' : 'denied');
    if (!analytics) {
      // drop identifying events that are still waiting
      this.queue = this.queue.filter((entry) => NO_CONSENT_TYPES.includes(entry.event.event_type));
    }
    this.notify();
  }

  hasConsent(): boolean {
    return this.consent;
  }

  track(type: TrackEventType, props: Record<string, unknown> = {}, ctx: TrackContext = {}): TrackingEnvelope | null {
    try {
      if (!this.env.enabled || this.disabledReason) return null;
      if (!this.consent && !NO_CONSENT_TYPES.includes(type)) return null;

      // impressions: once per session per surface/product/position
      if (type === 'impression') {
        const key = `${ctx.surface ?? ''}:${ctx.item?.product_id ?? ''}:${ctx.item?.position ?? ''}`;
        if (this.seenImpressions.has(key)) return null;
        this.seenImpressions.add(key);
      }

      const page = this.env.context();
      const event: TrackingEnvelope = {
        event_id: this.env.uuid(),
        event_type: type,
        ts_client: new Date(this.env.now()).toISOString(),
        // without consent nothing identifies the visitor
        anonymous_id: this.consent ? this.anonymousId() : '',
        session_id: this.consent ? this.sessionId() : '',
        ...(ctx.surface ? { surface: ctx.surface } : {}),
        page: { path: page.path, referrer: page.referrer },
        ...(ctx.item ? { item: ctx.item } : {}),
        props,
        device: page.device,
        consent: { analytics: this.consent },
        app_version: this.env.appVersion
      };
      this.enqueue(event);
      return event;
    } catch {
      return null; // tracking must never break the app
    }
  }

  identify(userId: number | string | null | undefined) {
    try {
      if (!this.env.enabled || this.disabledReason || !this.consent || userId === null || userId === undefined) return;
      const key = String(userId);
      if (this.identified.has(key)) return;
      const token = this.env.getToken();
      if (!token) return; // the endpoint needs a signed-in user
      this.identified.add(key);
      const body = JSON.stringify({ anonymous_id: this.anonymousId(), session_id: this.sessionId() });
      this.env
        .send(`${this.env.baseUrl}/events/identify`, body, token)
        .then((res) => {
          if (res.status >= 400) this.identified.delete(key);
          this.handleStatus(res.status);
        })
        .catch(() => this.identified.delete(key));
    } catch {
      // ignore
    }
  }

  /** Forget the identity link (logout) so the next sign-in is sent again. */
  reset() {
    this.identified.clear();
  }

  pending(): number {
    return this.queue.length;
  }

  isDisabled(): string | null {
    if (!this.env.enabled) return 'NEXT_PUBLIC_TRACKING_ENABLED=false';
    return this.disabledReason;
  }

  /** Re-enable after the endpoint was found missing (used by /dev/tracking). */
  resume() {
    this.disabledReason = null;
    this.notify();
  }

  getLog(): DebugEntry[] {
    return this.log;
  }

  clearLog() {
    this.log = [];
    this.notify();
  }

  subscribe(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  }

  // ---- batching ----------------------------------------------------------------------------------------------

  private notify() {
    this.listeners.forEach((listener) => {
      try {
        listener();
      } catch {
        // ignore
      }
    });
  }

  private enqueue(event: TrackingEnvelope) {
    const entry: DebugEntry = { event, state: 'queued', attempts: 0 };
    this.queue.push(entry);
    this.log = [entry, ...this.log].slice(0, DEBUG_LIMIT);
    this.notify();
    if (this.queue.length >= BATCH_SIZE) {
      void this.flush();
    } else if (this.timer === null) {
      this.timer = this.env.setTimer(() => {
        this.timer = null;
        void this.flush();
      }, BATCH_INTERVAL_MS);
    }
  }

  private handleStatus(status: number) {
    // the gateway endpoint is built in parallel: "not there yet" turns the SDK off quietly
    if (status === 404 || status === 405 || status === 501) {
      this.disabledReason = `endpoint unavailable (HTTP ${status})`;
      this.queue.forEach((entry) => {
        entry.state = 'dropped';
        entry.note = this.disabledReason ?? undefined;
      });
      this.queue = [];
      this.notify();
    }
  }

  /** Sends the queue now (in batches of up to 50). Resolves when finished; never rejects. */
  async flush(): Promise<void> {
    if (this.timer !== null) {
      this.env.clearTimer(this.timer);
      this.timer = null;
    }
    if (this.flushing || this.queue.length === 0) return;
    this.flushing = true;
    try {
      while (this.queue.length > 0 && !this.disabledReason) {
        const batch = this.queue.splice(0, MAX_BATCH);
        await this.sendBatch(batch);
      }
    } finally {
      this.flushing = false;
      this.notify();
    }
  }

  private async sendBatch(batch: DebugEntry[]) {
    batch.forEach((entry) => {
      entry.state = 'sending';
      entry.attempts += 1;
    });
    this.notify();
    const body = JSON.stringify({ events: batch.map((entry) => entry.event) });
    try {
      const res = await this.env.send(`${this.env.baseUrl}/events`, body, this.env.getToken());
      if (res.status >= 200 && res.status < 300) {
        batch.forEach((entry) => (entry.state = 'sent'));
      } else if (res.status === 404 || res.status === 405 || res.status === 501) {
        batch.forEach((entry) => {
          entry.state = 'dropped';
          entry.note = `HTTP ${res.status}`;
        });
        this.handleStatus(res.status);
      } else if (res.status >= 400 && res.status < 500 && res.status !== 429) {
        batch.forEach((entry) => {
          entry.state = 'dropped';
          entry.note = `HTTP ${res.status}`;
        }); // a bad request will not get better by retrying
      } else {
        this.retry(batch, `HTTP ${res.status}`);
      }
    } catch (err) {
      this.retry(batch, (err as Error)?.message ?? 'network error');
    }
    this.notify();
  }

  private retry(batch: DebugEntry[], note: string) {
    const again: DebugEntry[] = [];
    batch.forEach((entry) => {
      entry.note = note;
      if (entry.attempts >= MAX_ATTEMPTS) {
        entry.state = 'failed';
      } else {
        entry.state = 'queued';
        again.push(entry);
      }
    });
    if (again.length === 0) return;
    this.queue.unshift(...again);
    // exponential back-off with jitter: 2 s, 4 s ...
    const attempts = again[0].attempts;
    const delay = Math.min(30_000, 1000 * 2 ** attempts) + Math.floor(this.env.random() * 500);
    if (this.timer === null) {
      this.timer = this.env.setTimer(() => {
        this.timer = null;
        void this.flush();
      }, delay);
    }
  }

  /** Tab hidden / page unloading: hand the queue to sendBeacon (no auth header: the JWT is optional on /events). */
  flushBeacon() {
    if (this.queue.length === 0 || this.disabledReason || !this.env.enabled) return;
    try {
      while (this.queue.length > 0) {
        const batch = this.queue.splice(0, MAX_BATCH);
        const ok = this.env.beacon(`${this.env.baseUrl}/events`, JSON.stringify({ events: batch.map((entry) => entry.event) }));
        batch.forEach((entry) => {
          entry.attempts += 1;
          entry.state = ok ? 'sent' : 'failed';
          entry.note = ok ? 'beacon' : 'beacon rejected';
        });
        if (!ok) break;
      }
    } catch {
      // ignore
    }
    this.notify();
  }
}

// ---- browser wiring ------------------------------------------------------------------------------------------------

function deviceInfo() {
  const ua = navigator.userAgent;
  const width = window.innerWidth;
  const type = /Mobi|Android|iPhone|iPod/i.test(ua) ? 'mobile' : /iPad|Tablet/i.test(ua) || (width >= 600 && width < 1024) ? 'tablet' : 'desktop';
  const os = /Windows/i.test(ua)
    ? 'Windows'
    : /iPhone|iPad|iPod/i.test(ua)
      ? 'iOS'
      : /Android/i.test(ua)
        ? 'Android'
        : /Mac OS X|Macintosh/i.test(ua)
          ? 'macOS'
          : /Linux/i.test(ua)
            ? 'Linux'
            : 'other';
  return { type, os, viewport: `${width}x${window.innerHeight}` };
}

function newUuid(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') return crypto.randomUUID();
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0;
    return (c === 'x' ? r : (r & 0x3) | 0x8).toString(16);
  });
}

function browserEnv(): TrackerEnv {
  return {
    now: () => Date.now(),
    uuid: newUuid,
    baseUrl: getBaseUrl(),
    enabled: process.env.NEXT_PUBLIC_TRACKING_ENABLED !== 'false',
    appVersion: `web-${process.env.NEXT_PUBLIC_APP_VERSION ?? '0.1.0'}`,
    getToken: () => authTokenProvider?.() ?? null,
    context: () => ({ path: window.location.pathname, referrer: document.referrer ? safePath(document.referrer) : '', device: deviceInfo() }),
    storage: {
      get: (key) => {
        try {
          return window.localStorage.getItem(key);
        } catch {
          return null;
        }
      },
      set: (key, value) => {
        try {
          window.localStorage.setItem(key, value);
        } catch {
          // ignore
        }
      }
    },
    cookie: {
      get: (name) => {
        const match = document.cookie.match(new RegExp(`(?:^|; )${name}=([^;]+)`));
        return match ? decodeURIComponent(match[1]) : null;
      },
      set: (name, value, maxAge) => {
        document.cookie = `${name}=${encodeURIComponent(value)}; path=/; max-age=${maxAge}; SameSite=Lax`;
      }
    },
    send: async (url, body, tokenValue) => {
      const res = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', ...(tokenValue ? { Authorization: `Bearer ${tokenValue}` } : {}) },
        body,
        keepalive: body.length < 60_000,
        cache: 'no-store'
      });
      return { status: res.status };
    },
    beacon: (url, body) => typeof navigator.sendBeacon === 'function' && navigator.sendBeacon(url, new Blob([body], { type: 'application/json' })),
    setTimer: (fn, ms) => window.setTimeout(fn, ms),
    clearTimer: (handle) => window.clearTimeout(handle as number),
    random: Math.random
  };
}

function safePath(url: string): string {
  try {
    const parsed = new URL(url);
    return parsed.origin === window.location.origin ? parsed.pathname : parsed.origin; // keep third-party referrers coarse
  } catch {
    return '';
  }
}

let authTokenProvider: (() => string | null) | null = null;

/** The auth layer registers how to read the current access token (for /events/identify and attributed events). */
export function setTrackingTokenProvider(provider: (() => string | null) | null) {
  authTokenProvider = provider;
}

let tracker: Tracker | null = null;

/** Lazily created singleton; null on the server. */
export function getTracker(): Tracker | null {
  if (typeof window === 'undefined') return null;
  if (!tracker) {
    tracker = new Tracker(browserEnv());
    const flushOnHide = () => {
      if (document.visibilityState === 'hidden') tracker?.flushBeacon();
    };
    document.addEventListener('visibilitychange', flushOnHide);
    window.addEventListener('pagehide', () => tracker?.flushBeacon());
  }
  return tracker;
}

export function track(type: TrackEventType, props: Record<string, unknown> = {}, ctx: TrackContext = {}) {
  getTracker()?.track(type, props, ctx);
}

export function identify(userId: number | string | null | undefined) {
  getTracker()?.identify(userId);
}

export function setConsent(analytics: boolean) {
  getTracker()?.setConsent(analytics);
}

/** Best-effort surface for a pathname (used when a caller does not pass one). */
export function surfaceForPath(path: string): Surface | undefined {
  if (path === '/') return 'home_trending';
  if (path.startsWith('/categories')) return 'category_page';
  if (path.startsWith('/products/')) return 'pdp';
  if (path.startsWith('/products')) return 'search_results';
  if (path.startsWith('/cart')) return 'cart';
  if (path.startsWith('/checkout') || path.startsWith('/pay')) return 'checkout';
  if (path.startsWith('/orders')) return 'orders';
  if (path.startsWith('/seller')) return 'seller_console';
  if (path.startsWith('/admin')) return 'admin_console';
  return undefined;
}
