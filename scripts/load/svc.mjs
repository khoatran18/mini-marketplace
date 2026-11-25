#!/usr/bin/env node
// Đo CHỊU TẢI TỪNG SERVICE (docs/design/12_PERF_SERVICE.md). Khác ccu.mjs (hành trình người dùng hỗn hợp): mỗi lượt đo chỉ nhắm MỘT loại lời gọi
// ("target"), tải MỞ (n yêu cầu/s cố định, không chờ hệ thống), tăng theo bậc cho tới khi gãy. Tài khoản/token dựng sẵn trước khi tăng tải nên auth không che
// các service khác (trừ khi chính auth là target).
//
//   node scripts/load/svc.mjs --target=product-list,product-detail,search [--rate-stages=100:30,200:30] [--step-secs=30]
//   (thường chạy qua: LOAD_SCRIPT=svc.mjs scripts/load/perf-one.sh NHÃN --target=...)
//
// Target:  auth-login | auth-register | gateway | frontend | product-list | product-detail | search | user | cart | checkout-chain |
//          admin-orders | admin-payments | analytics-ingest | analytics-report | ops | ai-chat
//   checkout-chain = một "đơn vị" là cả chuỗi: thêm vào giỏ -> preview -> đặt đơn -> (không COD) chờ payment tạo bất đồng bộ -> xác nhận thẻ. Đơn vị/s = chuỗi/s.
//   ai-chat dùng mô hình giả (deploy-perf.sh kiểm); KHÔNG bao giờ chạy với mô hình thật.
// Tham số (env hoặc --key=value):
//   API_URL, FRONTEND_URL     như ccu.mjs
//   TARGET                    danh sách target, chạy lần lượt (nghỉ COOLDOWN giây giữa hai target)
//   RATE_STAGES               "yêu cầu/s:giây,..." ghi đè bậc mặc định của MỌI target trong lượt
//   STEP_SECS                 giây mỗi bậc mặc định (30)
//   MAX_INFLIGHT              chặn số yêu cầu cùng bay để máy tải không cạn; vượt thì BỎ và đếm (3000)
//   STOP_ERR_PCT              dừng target khi một bậc có lỗi > ngưỡng % hoặc xong < 50% số gửi (30)
//   COOLDOWN                  giây nghỉ giữa các target (20)
//   ADMIN_USERNAME/ADMIN_PASSWORD  cần cho admin-orders, admin-payments, ops
//   AI_USERS                  số tài khoản cho ai-chat (150); mỗi user có trần token/ngày nên tải cao cần nhiều user hơn, nếu không sẽ có 429
//   OUT                       ghi kết quả JSON
// Mỗi bậc in: gửi, xong, ok/s (thông lượng thật), % lỗi, 429, bỏ, p50/p95/p99/max, CPU máy tải+hệ thống, trễ vòng lặp node. Mốc epoch từng bậc ghi vào JSON
// để đối chiếu với perf-capture (CPU từng container).
import { writeFileSync, readFileSync } from 'node:fs';
import { monitorEventLoopDelay } from 'node:perf_hooks';

const argv = Object.fromEntries(process.argv.slice(2).filter((a) => a.startsWith('--')).map((a) => { const [k, v = '1'] = a.slice(2).split('='); return [k.toUpperCase().replace(/-/g, '_'), v]; }));
const P = (k, d) => argv[k] ?? process.env[k] ?? d;
const API = String(P('API_URL', 'http://api.marketplace.swarm.localhost')).replace(/\/$/, '');
const WEB = String(P('FRONTEND_URL', API.replace('//api.', '//'))).replace(/\/$/, '');
const TARGETS = String(P('TARGET', '')).split(',').filter(Boolean);
const STEP_SECS = Number(P('STEP_SECS', 30)), MAX_INFLIGHT = Number(P('MAX_INFLIGHT', 3000)), STOP_ERR = Number(P('STOP_ERR_PCT', 30)), COOLDOWN = Number(P('COOLDOWN', 20));
const OUT = P('OUT', ''), PRODUCTS = Number(P('PRODUCTS', 30));
const ADMIN_USER = P('ADMIN_USERNAME', ''), ADMIN_PASS = P('ADMIN_PASSWORD', '');
const parseStages = (s) => String(s).split(',').filter(Boolean).map((x) => { const [t, d] = x.split(':'); return { target: Number(t), secs: Number(d) }; });
const RATE_OVERRIDE = P('RATE_STAGES', '');
const PASSWORD = 'LoadPass_1';
const run = Date.now().toString(36);
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const pick = (a) => a[Math.floor(Math.random() * a.length)];
const chance = (p) => Math.random() < p;
const rnd = (n) => Math.floor(Math.random() * n);
const randIp = () => `10.${rnd(256)}.${rnd(256)}.${1 + rnd(254)}`;
const words = ['ao', 'quan', 'giay', 'tui', 'mu', 'dep'];
const pct = (arr, p) => { if (!arr.length) return 0; const a = [...arr].sort((x, y) => x - y); return a[Math.min(a.length - 1, Math.floor((p / 100) * a.length))]; };
const maxOf = (a) => a.reduce((m, x) => (x > m ? x : m), 0);

// Bậc mặc định (yêu cầu/s; chuỗi/s cho checkout-chain; cuộc chat/s cho ai-chat). Chọn rộng để tìm điểm gãy; tăng/giảm bằng --rate-stages nếu cần.
const DEF = {
  'auth-login': [10, 20, 40, 60, 80, 100, 130], 'auth-register': [5, 10, 20, 30, 40, 60],
  gateway: [200, 500, 1000, 1500, 2000], frontend: [5, 10, 20, 40, 60, 80],
  'product-list': [50, 100, 200, 400, 800, 1200], 'product-detail': [50, 100, 200, 400, 800, 1200], search: [20, 50, 100, 200, 400, 600],
  user: [20, 50, 100, 200, 400, 600], cart: [20, 50, 100, 200, 300, 400], 'checkout-chain': [2, 4, 8, 12, 16, 24, 32],
  'admin-orders': [5, 10, 20, 40, 80], 'admin-payments': [5, 10, 20, 40, 80],
  'analytics-ingest': [50, 100, 200, 400, 800], 'analytics-report': [5, 10, 20, 40, 80], ops: [5, 10, 20, 40, 80], 'ai-chat': [1, 2, 4, 8, 12],
};

// ------------------------------------------------------------------ số liệu theo nhãn lời gọi (đặt lại đầu mỗi target, sau bước chuẩn bị)
let stats = {};
const stat = (n) => (stats[n] ??= { n: 0, ok: 0, expected: 0, limited: 0, err: 0, lat: [], codes: {} });
async function call(label, method, path, { token, ip, body, headers = {}, ok = [200], soft = [], timeout = 30000, base = API } = {}) {
  const t0 = performance.now();
  let status = 0, json = null;
  try {
    const res = await fetch(base + path, {
      method, signal: AbortSignal.timeout(timeout),
      headers: { ...(body ? { 'Content-Type': 'application/json' } : {}), ...(token ? { Authorization: `Bearer ${token}` } : {}), 'X-Forwarded-For': ip ?? randIp(), ...headers },
      body: body ? JSON.stringify(body) : undefined,
    });
    status = res.status;
    const txt = await res.text();
    try { json = JSON.parse(txt); } catch { /* không phải JSON */ }
  } catch { status = 0; }
  const ms = performance.now() - t0;
  const s = stat(label); s.n++; s.lat.push(ms); s.codes[status] = (s.codes[status] ?? 0) + 1;
  if (status === 429) s.limited++; else if (ok.includes(status)) s.ok++; else if (soft.includes(status)) s.expected++; else s.err++;
  return { status, json, ms, ok: ok.includes(status), soft: soft.includes(status) };
}

// ------------------------------------------------------------------ tài khoản và token
async function retry(fn) { let r; for (let i = 0; i < 3; i++) { r = await fn(); if (r.status !== 0 && r.status < 500) break; await sleep(1000 * (i + 1)); } return r; }
async function login(username, password, role, ip) {
  const l = await retry(() => call('POST /auth/login', 'POST', '/auth/login', { ip, body: { username, password, role } }));
  if (l.status !== 200) throw new Error(`login ${role} ${l.status}`);
  return { username, password, role, ip, access: l.json.access_token, refresh: l.json.refresh_token, at: Date.now() };
}
let accSeq = 0;
async function account(prefix, role, ip) {
  const username = `${prefix}${run}${(accSeq++).toString(36)}`.slice(0, 16);
  const r = await retry(() => call('POST /auth/register', 'POST', '/auth/register', { ip, body: { username, password: PASSWORD, role } }));
  if (r.status !== 200) throw new Error(`register ${r.status}`);
  return login(username, PASSWORD, role, ip);
}
async function tok(u) { // access token sống ~5 phút: làm mới khi tuổi > 3 phút (một lời gọi làm mới duy nhất cho mỗi user: refresh token xoay vòng)
  if (Date.now() - u.at > 180_000) {
    u.rf ??= (async () => { const r = await call('POST /auth/refresh-token', 'POST', '/auth/refresh-token', { ip: u.ip, body: { refresh_token: u.refresh } }); if (r.status === 200) { u.access = r.json.access_token; u.refresh = r.json.refresh_token ?? u.refresh; u.at = Date.now(); } })().finally(() => { u.rf = null; });
    await u.rf;
  }
  return u.access;
}
async function pmap(n, conc, fn) { // chạy fn(i) cho i<n với tối đa conc đồng thời
  const out = new Array(n); let next = 0;
  await Promise.all(Array.from({ length: Math.min(conc, n) }, async () => { for (;;) { const i = next++; if (i >= n) return; out[i] = await fn(i); } }));
  return out;
}

// ------------------------------------------------------------------ chuẩn bị dùng chung
const W = { owner: null, productIds: [], categoryId: null, admin: null, pools: {} };
async function setupShop() {
  console.log(`[setup] tạo chủ shop + ${PRODUCTS} sản phẩm ...`);
  const ip = '198.51.100.250';
  const owner = await account('lo', 'seller_admin', ip);
  const st = await call('POST /users/sellers', 'POST', '/users/sellers', { ip, token: owner.access, body: { seller: { name: `LOAD shop ${run}`, bank_account: '1', tax_code: '1', description: 'svc test', phone: '0909000111', address: 'x' } } });
  if (st.status !== 200) throw new Error(`tạo store ${st.status} ${JSON.stringify(st.json)}`);
  for (let i = 0; i < 40; i++) { if ((await call('GET /seller/products', 'GET', '/seller/products?page_size=1', { ip, token: owner.access, soft: [403] })).status === 200) break; await sleep(500); }
  W.owner = await login(owner.username, PASSWORD, 'seller_admin', ip); // đăng nhập lại để token mang store_id
  const cats = await call('GET /categories', 'GET', '/categories', { ip });
  W.categoryId = cats.json?.categories?.[0]?.id;
  for (let i = 0; i < PRODUCTS; i++) {
    const r = await call('POST /products', 'POST', '/products', { ip, token: W.owner.access, body: { name: `LOAD ${pick(words)} ${run} ${i}`, price: 50000 + i * 1000, inventory: 1_000_000, description: 'svc', ...(W.categoryId ? { category_id: W.categoryId } : {}) } });
    if (!r.json?.id) throw new Error(`tạo sản phẩm ${r.status} ${JSON.stringify(r.json)}`);
    W.productIds.push(r.json.id);
  }
  if (ADMIN_USER && ADMIN_PASS) { try { W.admin = await login(ADMIN_USER, ADMIN_PASS, 'admin', ip); } catch (e) { console.log(`  (đăng nhập admin thất bại: ${e.message})`); } }
}
// Kho người dùng: pool[kind] = {all, free}. withAddress: địa chỉ mặc định (đặt đơn dùng address_id 0)
async function ensurePool(kind, n, withAddress) {
  const p = (W.pools[kind] ??= { all: [], free: [] });
  const need = n - p.all.length; if (need <= 0) return p;
  console.log(`[setup] tạo ${need} tài khoản ${kind} ...`);
  const made = await pmap(need, 8, async () => {
    const ip = randIp(); const u = await account('lb', 'buyer', ip);
    if (withAddress) { const a = await call('POST /users/me/addresses', 'POST', '/users/me/addresses', { ip, token: u.access, ok: [201], body: { label: 'Nhà', receiver_name: 'Load Test', phone: '0901234567', line1: '1 Test', city: 'HCM', is_default: true } }); if (a.status !== 201) throw new Error(`địa chỉ ${a.status}`); }
    return u;
  });
  p.all.push(...made); p.free.push(...made);
  return p;
}
const lease = (kind) => (kind === 'ai' ? W.pools[kind].free.shift() : W.pools[kind].free.pop()) ?? null; // ai: xoay vòng (FIFO) để trải đều lượt chat, tránh vài user chạm trần token/ngày
const release = (kind, u) => W.pools[kind].free.push(u);
const anyUser = (kind) => pick(W.pools[kind].all);

// ------------------------------------------------------------------ các target: unit(ctx) -> {status, ok, soft?} ; dropUnit khi không có user rảnh
const evtBody = (i) => ({ events: ['product_view', 'impression', 'page_view'].map((t, k) => ({ event_id: `${run}-${i}-${k}-${rnd(1e9)}`, event_type: t, ts_client: new Date().toISOString(), anonymous_id: `a_${run}_${i % 500}`, session_id: `s_${run}_${i % 500}`, surface: 'pdp', page: { path: '/products/1' }, item: { product_id: pick(W.productIds), position: 1 }, device: { type: pick(['desktop', 'mobile']) }, consent: { analytics: true } })) });
const CARD = { card_exp: '12/34', card_cvc: '123' };
const NONE = Symbol('none');
async function withUser(kind, fn) { const u = lease(kind); if (!u) return NONE; try { return await fn(u); } finally { release(kind, u); } }

const TG = {
  'auth-login': { prep: () => ensurePool('buyer', 200, true), unit: () => { const u = anyUser('buyer'); return call('POST /auth/login', 'POST', '/auth/login', { ip: u.ip, body: { username: u.username, password: PASSWORD, role: 'buyer' } }); } },
  'auth-register': { unit: (c) => call('POST /auth/register', 'POST', '/auth/register', { body: { username: `r${c.seq.toString(36)}${run.slice(-6)}`.slice(0, 16), password: PASSWORD, role: 'buyer' } }) },
  gateway: { unit: () => call('GET /health', 'GET', '/health') },
  frontend: { unit: async () => { const path = pick(['/', '/products', `/products/${pick(W.productIds)}`, '/login']); return call(`WEB ${path.replace(/\d+/g, ':id')}`, 'GET', path, { base: WEB }); } },
  'product-list': { unit: () => call('GET /products', 'GET', `/products?page=${1 + rnd(3)}&page_size=10`) },
  'product-detail': { unit: () => call('GET /products/:id', 'GET', `/products/${pick(W.productIds)}`) },
  search: { unit: () => call('GET /search', 'GET', `/search?${pick([`q=${pick(words)}`, `category_id=${W.categoryId ?? ''}`, 'price_min=40000&price_max=90000', 'in_stock=true', `sort=${pick(['relevance', 'price_asc', 'price_desc', 'newest', 'best_selling'])}`])}&page_size=20`) },
  user: { prep: () => ensurePool('buyer', 200, true), unit: async () => { const u = anyUser('buyer'); return call('GET /users/me/addresses', 'GET', '/users/me/addresses', { ip: u.ip, token: await tok(u) }); } },
  cart: { prep: () => ensurePool('buyer', 200, true), unit: () => withUser('buyer', async (u) => call('PUT /cart/items/:id', 'PUT', `/cart/items/${pick(W.productIds)}`, { ip: u.ip, token: await tok(u), body: { quantity: 1 + rnd(2) } })) },
  'checkout-chain': { prep: () => ensurePool('buyer', 400, true), unit: (c) => withUser('buyer', async (u) => {
    const o = { ip: u.ip };
    for (let i = 0, n = 1 + rnd(3); i < n; i++) { const r = await call('PUT /cart/items/:id', 'PUT', `/cart/items/${pick(W.productIds)}`, { ...o, token: await tok(u), body: { quantity: 1 + rnd(2) } }); if (!r.ok) return r; }
    const pv = await call('POST /checkout/preview', 'POST', '/checkout/preview', { ...o, token: await tok(u), body: { from_cart: true } });
    if (!pv.ok || pv.json?.can_order === false) return pv.ok ? { status: 422, ok: false } : pv;
    const cod = chance(0.4), pm = cod ? 'COD' : 'MOCK_CARD';
    const ord = await call(`POST /orders (${pm})`, 'POST', '/orders', { ...o, token: await tok(u), ok: [200, 201], headers: { 'Idempotency-Key': `sv-${run}-${c.seq}` }, body: { from_cart: true, address_id: 0, payment_method: pm, note: 'svc' } });
    if (!ord.ok || cod) return ord;
    const t0 = performance.now(); let pay = ord.json?.payment;
    for (let i = 0; i < 16 && !pay?.id; i++) { await sleep(500); pay = (await call('GET /checkouts/:id', 'GET', `/checkouts/${ord.json.checkout_id}`, { ...o, token: await tok(u) })).json?.payment; }
    if (!pay?.id) { c.step.payTimeouts++; return { status: 599, ok: false }; }
    c.step.pay.push(performance.now() - t0);
    return call('POST /payments/:id/confirm', 'POST', `/payments/${pay.id}/confirm`, { ...o, token: await tok(u), body: { card_number: '4242424242424242', ...CARD } });
  }) },
  'admin-orders': { need: 'admin', unit: async () => call('GET /admin/orders', 'GET', `/admin/orders?${pick(['', 'status=CONFIRMED', 'payment_method=MOCK_CARD', 'sort=newest'])}&page_size=10`, { token: await tok(W.admin) }) },
  'admin-payments': { need: 'admin', unit: async () => call('GET /admin/payments', 'GET', `/admin/payments?${pick(['', 'status=SUCCEEDED', 'method=MOCK_CARD', 'sort=amount_desc'])}&page_size=10`, { token: await tok(W.admin) }) },
  'analytics-ingest': { unit: (c) => call('POST /events', 'POST', '/events', { body: evtBody(c.seq), ok: [202] }) },
  'analytics-report': { unit: async () => { const p = pick(['summary', 'timeseries?granularity=day', 'top-products?sort=revenue', 'funnel']); return call(`GET /seller/analytics/${p.split('?')[0]}`, 'GET', `/seller/analytics/${p}`, { token: await tok(W.owner) }); } },
  ops: { need: 'admin', unit: async () => { const p = pick(['/ops/system/health', '/ops/incidents?limit=20', '/ops/queries', '/ops/audit?limit=20', '/ops/actions']); return call(`GET ${p.split('?')[0]}`, 'GET', p, { token: await tok(W.admin), soft: [503] }); } },
  'ai-chat': { prep: () => ensurePool('ai', Number(P('AI_USERS', 150)), false), unit: (c) => withUser('ai', async (u) => {
    const t0 = performance.now(); let status = 0, ttfb = 0, done = false;
    try {
      const res = await fetch(API + '/ai/chat', { method: 'POST', signal: AbortSignal.timeout(130000), headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${await tok(u)}`, 'X-Forwarded-For': u.ip }, body: JSON.stringify({ message: pick(['Các đơn gần đây của mình thế nào?', 'Mình có đơn nào đang giao không?']) }) });
      status = res.status;
      if (status === 200) { const rd = res.body.getReader(), dec = new TextDecoder(); let text = ''; for (;;) { const { value, done: d } = await rd.read(); if (!ttfb && value) ttfb = performance.now() - t0; if (d) break; text += dec.decode(value, { stream: true }); } done = /^event: done/m.test(text); }
    } catch { status = 0; }
    const ok = status === 200 && done; const s = stat('POST /ai/chat'); const ms = performance.now() - t0; const st = ok ? 200 : status === 200 ? 599 : status;
    s.n++; s.lat.push(ms); s.codes[st] = (s.codes[st] ?? 0) + 1; if (st === 429) s.limited++; else if (ok) s.ok++; else s.err++;
    if (ok) c.step.ttfb.push(ttfb);
    return { status: st, ok, counted: true };
  }) },
};

// ------------------------------------------------------------------ sonde máy tải + hệ thống
function cpuTimes() { const l = readFileSync('/proc/stat', 'utf8').split('\n')[0].trim().split(/\s+/).slice(1).map(Number); return { idle: l[3] + (l[4] ?? 0), total: l.reduce((a, b) => a + b, 0) }; }
const eld = monitorEventLoopDelay({ resolution: 10 }); eld.enable();

// ------------------------------------------------------------------ một bậc tải mở
let inflight = 0;
async function runStep(name, T, g, idx) {
  const step = { idx, rate: g.target, secs: g.secs, t0: Date.now() / 1000, t1: 0, sent: 0, done: 0, ok: 0, expected: 0, limited: 0, err: 0, dropped: 0, lat: [], codes: {}, pay: [], payTimeouts: 0, ttfb: [], hostCpu: 0, eldP99: 0 };
  const c0 = cpuTimes(); eld.reset();
  const t0 = Date.now(), end = t0 + g.secs * 1000, tasks = [];
  console.log(`[svc] ${name} bậc ${idx + 1}: ${g.target}/s x ${g.secs}s bắt đầu ${step.t0.toFixed(0)}`);
  let sent = 0;
  while (Date.now() < end) {
    const due = Math.floor(((Date.now() - t0) * g.target) / 1000) - sent;
    for (let i = 0; i < due; i++) {
      sent++; step.sent++;
      if (inflight >= MAX_INFLIGHT) { step.dropped++; continue; }
      inflight++;
      const ctx = { step, seq: step.sent + idx * 1e6 };
      const t = performance.now();
      tasks.push((async () => {
        let r; try { r = await T.unit(ctx); } catch { r = { status: 0, ok: false }; }
        inflight--;
        if (r === NONE) { step.dropped++; return; } // không có user rảnh trong kho: đếm như bỏ (kho nhỏ hơn mức tải)
        const ms = performance.now() - t;
        step.done++; step.lat.push(ms); step.codes[r.status] = (step.codes[r.status] ?? 0) + 1;
        if (r.status === 429) step.limited++; else if (r.ok) step.ok++; else if (r.soft) step.expected++; else step.err++;
      })());
    }
    await sleep(5);
  }
  step.t1 = Date.now() / 1000;
  const c1 = cpuTimes(); step.hostCpu = +(100 * (1 - (c1.idle - c0.idle) / Math.max(1, c1.total - c0.total))).toFixed(1); step.eldP99 = +(eld.percentile(99) / 1e6).toFixed(1);
  step.tasks = tasks;
  return step;
}
const errPct = (s) => (s.done ? (100 * s.err) / s.done : 0);
function line(s) {
  return `${String(s.idx + 1).padStart(3)} ${String(s.rate).padStart(6)}/s ${String(s.sent).padStart(7)} ${String(s.done).padStart(7)} ${(s.ok / s.secs).toFixed(1).padStart(8)} ${errPct(s).toFixed(1).padStart(6)}% ${String(s.limited).padStart(5)} ${String(s.dropped).padStart(5)} ` +
    `${pct(s.lat, 50).toFixed(0).padStart(6)} ${pct(s.lat, 95).toFixed(0).padStart(6)} ${pct(s.lat, 99).toFixed(0).padStart(6)} ${maxOf(s.lat).toFixed(0).padStart(6)} ${String(s.hostCpu).padStart(6)} ${String(s.eldP99).padStart(6)}  ${JSON.stringify(s.codes)}`;
}
const HEAD = `${'bậc'.padStart(3)} ${'tải'.padStart(8)} ${'gửi'.padStart(7)} ${'xong'.padStart(7)} ${'ok/s'.padStart(8)} ${'lỗi'.padStart(7)} ${'429'.padStart(5)} ${'bỏ'.padStart(5)} ${'p50'.padStart(6)} ${'p95'.padStart(6)} ${'p99'.padStart(6)} ${'max'.padStart(6)} ${'CPUmáy'.padStart(6)} ${'eld99'.padStart(6)}  mã`;

const results = {};
async function runTarget(name) {
  const T = TG[name]; if (!T) { console.log(`target lạ: ${name}`); return; }
  if (T.need === 'admin' && !W.admin) { console.log(`[svc] ${name}: BỎ (thiếu ADMIN_USERNAME/ADMIN_PASSWORD hoặc đăng nhập admin hỏng)`); return; }
  if (T.prep) await T.prep();
  const stages = RATE_OVERRIDE ? parseStages(RATE_OVERRIDE) : DEF[name].map((r) => ({ target: r, secs: STEP_SECS }));
  stats = {}; inflight = 0;
  console.log(`\n========== TARGET ${name}: ${stages.map((g) => `${g.target}/s`).join(' -> ')} (mỗi bậc ${stages[0].secs}s) ==========`);
  const steps = [];
  for (let i = 0; i < stages.length; i++) {
    const s = await runStep(name, T, stages[i], i); steps.push(s);
    const e = errPct(s);
    console.log(`   bậc ${i + 1} xong gửi: gửi ${s.sent}, xong tới giờ ${s.done}, lỗi ${e.toFixed(1)}%, CPU máy ${s.hostCpu}%`);
    if (e > STOP_ERR || (s.sent > 20 && s.done / s.sent < 0.5)) { console.log(`   >>> DỪNG ${name} ở bậc ${i + 1}: lỗi ${e.toFixed(1)}% hoặc xong ${s.done}/${s.sent}`); break; }
  }
  await Promise.race([Promise.all(steps.flatMap((s) => s.tasks)), sleep(60000)]); // chờ yêu cầu còn dở (tối đa 60 s) để bảng cuối đủ số
  console.log(`\n----- KẾT QUẢ ${name} (đơn vị: lời gọi; checkout-chain/ai-chat: cả chuỗi/cuộc chat) -----`);
  console.log(HEAD); for (const s of steps) console.log(line(s));
  for (const s of steps) {
    if (s.pay.length || s.payTimeouts) console.log(`   bậc ${s.idx + 1} payment tạo bất đồng bộ: ${s.pay.length} đơn, p50/p95/max ${pct(s.pay, 50).toFixed(0)}/${pct(s.pay, 95).toFixed(0)}/${maxOf(s.pay).toFixed(0)} ms, quá 8 s chưa có ${s.payTimeouts}`);
    if (s.ttfb.length) console.log(`   bậc ${s.idx + 1} AI byte đầu p50/p95 ${pct(s.ttfb, 50).toFixed(0)}/${pct(s.ttfb, 95).toFixed(0)} ms`);
  }
  console.log('theo nhãn lời gọi (gồm chuỗi phụ):');
  for (const [n, s] of Object.entries(stats).sort(([a], [b]) => a.localeCompare(b))) console.log(`  ${n.padEnd(36)} n=${String(s.n).padStart(7)} ok=${s.ok} dự kiến=${s.expected} 429=${s.limited} lỗi=${s.err} p50/p95/p99 ${pct(s.lat, 50).toFixed(0)}/${pct(s.lat, 95).toFixed(0)}/${pct(s.lat, 99).toFixed(0)} ${JSON.stringify(s.codes)}`);
  const labels = {}; for (const [n, s] of Object.entries(stats)) labels[n] = { n: s.n, ok: s.ok, expected: s.expected, limited: s.limited, err: s.err, p50: +pct(s.lat, 50).toFixed(1), p95: +pct(s.lat, 95).toFixed(1), p99: +pct(s.lat, 99).toFixed(1), max: +maxOf(s.lat).toFixed(1), codes: s.codes };
  results[name] = {
    steps: steps.map((s) => ({ step: s.idx + 1, rate: s.rate, secs: s.secs, t0: s.t0, t1: s.t1, sent: s.sent, done: s.done, ok: s.ok, okPerSec: +(s.ok / s.secs).toFixed(1), expected: s.expected, limited: s.limited, err: s.err, errPct: +errPct(s).toFixed(2), dropped: s.dropped,
      p50: +pct(s.lat, 50).toFixed(1), p95: +pct(s.lat, 95).toFixed(1), p99: +pct(s.lat, 99).toFixed(1), max: +maxOf(s.lat).toFixed(1), hostCpuPct: s.hostCpu, eldP99Ms: s.eldP99, codes: s.codes,
      payLag: s.pay.length ? { n: s.pay.length, p50: +pct(s.pay, 50).toFixed(0), p95: +pct(s.pay, 95).toFixed(0), max: +maxOf(s.pay).toFixed(0), timeoutsOver8s: s.payTimeouts } : null, aiTtfb: s.ttfb.length ? { p50: +pct(s.ttfb, 50).toFixed(0), p95: +pct(s.ttfb, 95).toFixed(0) } : null })),
    labels,
  };
}

(async () => {
  if (!TARGETS.length) { console.log('thiếu --target=...; các target:', Object.keys(TG).join(', ')); process.exit(2); }
  console.log(`SVC: ${TARGETS.join(', ')} ; API ${API}`);
  try { await setupShop(); } catch (e) { console.log('setup hỏng:', e.message); process.exit(1); }
  for (let i = 0; i < TARGETS.length; i++) {
    if (i) { console.log(`[svc] nghỉ ${COOLDOWN}s trước target kế`); await sleep(COOLDOWN * 1000); }
    try { await runTarget(TARGETS[i]); } catch (e) { console.log(`target ${TARGETS[i]} hỏng: ${e.message}`); }
  }
  if (OUT) { // steadyStartMs/steadyEndMs: để perf-diag.py lấy chuỗi Prometheus đúng khoảng (từ bậc đầu tới bậc cuối của mọi target)
    const all = Object.values(results).flatMap((r) => r.steps);
    writeFileSync(OUT, JSON.stringify({ kind: 'svc', api: API, targets: TARGETS, steadyStartMs: Math.min(...all.map((x) => x.t0)) * 1000, steadyEndMs: Math.max(...all.map((x) => x.t1)) * 1000, results }, null, 1));
  }
  process.exit(0);
})();
