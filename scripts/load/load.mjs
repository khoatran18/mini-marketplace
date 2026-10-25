#!/usr/bin/env node
// Load / soak cơ bản qua gateway (Node 18+, không phụ thuộc gói ngoài). Mô hình "open loop": phát request theo RPS cố định
// bất kể server trả lời chậm, nên độ trễ đo được phản ánh đúng tình trạng quá tải.
//
//   API_URL=http://localhost:8080 RPS=30 DURATION=60 node scripts/load/load.mjs
//
// Tham số (env hoặc --key=value, ví dụ --rps=50):
//   API_URL        gateway                                   (http://localhost:8080)
//   RPS            tổng request/giây                         (20)
//   DURATION       thời lượng giây (soak: 3600+)             (30)
//   RAMP           giây tăng dần từ 0 tới RPS                (5)
//   MIX            tỉ trọng: browse,search,order,events      (browse:50,search:25,order:5,events:20)
//   BUYERS         số buyer tạo sẵn (mỗi người một token)    (5)
//   PRODUCTS       số sản phẩm của shop test                 (20)
//   MAX_INFLIGHT   trần request đồng thời (vượt thì bỏ & đếm "dropped") (500)
//   MAX_ERR_PCT    ngưỡng lỗi (5xx / mạng / 4xx bất ngờ), vượt => exit 1   (1)
//   P95_MS         nếu đặt: p95 của mọi loại vượt ngưỡng => exit 1
// Chú ý: setup tạo ~(2 + 2*BUYERS) lần /auth/*; nâng AUTH_RATE_LIMIT_PER_MINUTE (mặc định 20) và RATE_LIMIT_PER_MINUTE/
// EVENTS_RATE_LIMIT_PER_MINUTE (giới hạn theo IP) trên stack test, nếu không sẽ thấy nhiều 429 (được báo riêng, không tính lỗi).
// Đơn hàng thử nghiệm dùng COD, shop test có tồn kho rất lớn; dữ liệu tạo ra mang tên "LOAD ...".

const argv = Object.fromEntries(process.argv.slice(2).filter((a) => a.startsWith('--')).map((a) => { const [k, v = '1'] = a.slice(2).split('='); return [k.toUpperCase().replace(/-/g, '_'), v]; }));
const P = (k, d) => argv[k] ?? process.env[k] ?? d;
const API = String(P('API_URL', 'http://localhost:8080')).replace(/\/$/, '');
const RPS = Number(P('RPS', 20)), DURATION = Number(P('DURATION', 30)), RAMP = Number(P('RAMP', 5));
const BUYERS = Number(P('BUYERS', 5)), PRODUCTS = Number(P('PRODUCTS', 20));
const MAX_INFLIGHT = Number(P('MAX_INFLIGHT', 500)), MAX_ERR_PCT = Number(P('MAX_ERR_PCT', 1));
const P95_MS = P('P95_MS', '') === '' ? null : Number(P('P95_MS', ''));
const MIX = String(P('MIX', 'browse:50,search:25,order:5,events:20')).split(',').map((x) => { const [n, w] = x.split(':'); return [n, Number(w)]; });
const PASSWORD = 'LoadPass_1';
const run = Date.now().toString(36).slice(-6);
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function raw(method, path, { token, body, headers = {} } = {}) {
  const res = await fetch(API + path, {
    method, headers: { ...(body ? { 'Content-Type': 'application/json' } : {}), ...(token ? { Authorization: `Bearer ${token}` } : {}), ...headers },
    body: body ? JSON.stringify(body) : undefined, signal: AbortSignal.timeout(15000),
  });
  const txt = await res.text();
  let json = null; try { json = JSON.parse(txt); } catch { /* không phải JSON */ }
  return { status: res.status, body: json };
}
async function auth(path, body, retries = 3) {
  for (let i = 0; i < retries; i++) {
    const r = await raw('POST', path, { body });
    if (r.status !== 429) return r;
    console.log(`  (429 ở ${path}, chờ 20s - hãy nâng AUTH_RATE_LIMIT_PER_MINUTE)`); await sleep(20000);
  }
  throw new Error(`${path} bị 429 liên tục`);
}
async function account(prefix, role) {
  const username = `${prefix}${run}${Math.random().toString(36).slice(2, 5)}`.slice(0, 16);
  await auth('/auth/register', { username, password: PASSWORD, role });
  const l = await auth('/auth/login', { username, password: PASSWORD, role });
  if (l.status !== 200) throw new Error(`đăng nhập ${username} thất bại: ${l.status} ${JSON.stringify(l.body)}`);
  return { access: l.body.access_token, refresh: l.body.refresh_token, at: Date.now() };
}
// access token sống ~5 phút (JWT_EXPIRE_TIME): làm mới khi còn > 60s tuổi 3 phút
async function token(u) {
  if (Date.now() - u.at > 180_000) {
    const r = await auth('/auth/refresh-token', { refresh_token: u.refresh });
    if (r.status === 200) { u.access = r.body.access_token; u.refresh = r.body.refresh_token ?? u.refresh; u.at = Date.now(); }
  }
  return u.access;
}

// ------------------------------------------------------------------ số liệu
const stats = {};
const stat = (n) => (stats[n] ??= { n: 0, ok: 0, limited: 0, err: 0, lat: [], codes: {} });
const win = { n: 0, err: 0 }; // cửa sổ in tiến độ
let inflight = 0, dropped = 0;
function record(name, status, ms, okCodes) {
  const s = stat(name);
  s.n++; s.lat.push(ms); s.codes[status] = (s.codes[status] ?? 0) + 1; win.n++;
  if (status === 429) s.limited++;
  else if (okCodes.includes(status)) s.ok++;
  else { s.err++; win.err++; }
}
const pct = (arr, p) => { if (!arr.length) return 0; const a = [...arr].sort((x, y) => x - y); return a[Math.min(a.length - 1, Math.floor((p / 100) * a.length))]; };

// ------------------------------------------------------------------ dữ liệu & kịch bản
let ctx;
const words = ['ao', 'quan', 'giay', 'tui', 'mu', 'dep'];
const pick = (a) => a[Math.floor(Math.random() * a.length)];
const actions = {
  browse: async () => {
    const r = Math.random();
    if (r < 0.4) return ['GET /products', await raw('GET', `/products?page=${1 + Math.floor(Math.random() * 3)}&page_size=10`), [200]];
    if (r < 0.7) return ['GET /products/:id', await raw('GET', `/products/${pick(ctx.productIds)}`), [200]];
    if (r < 0.85) return ['GET /categories', await raw('GET', '/categories'), [200]];
    return ['GET /products/:id/stock', await raw('GET', `/products/${pick(ctx.productIds)}/stock`), [200]];
  },
  search: async () => ['GET /search', await raw('GET', `/search?q=${encodeURIComponent(pick(words) + ' LOAD')}&sort=${pick(['relevance', 'price_asc', 'newest'])}&page_size=20`), [200]],
  order: async () => {
    const u = pick(ctx.buyers);
    const body = { items: [{ product_id: pick(ctx.productIds), quantity: 1 + Math.floor(Math.random() * 2) }], payment_method: 'COD', address_id: 0 };
    return ['POST /orders (COD)', await raw('POST', '/orders', { token: await token(u), body, headers: { 'Idempotency-Key': `ld-${run}-${(ctx.seq++).toString(36)}-${Math.random().toString(36).slice(2, 8)}` } }), [200, 201]];
  },
  events: async () => {
    const mk = (type, i) => ({ event_id: `${run}-${ctx.seq++}-${i}`, event_type: type, ts_client: new Date().toISOString(), anonymous_id: `a_${run}_${Math.floor(Math.random() * 1000)}`, session_id: `s_${run}_${Math.floor(Math.random() * 300)}`, surface: 'pdp', page: { path: '/products/1' }, item: { product_id: pick(ctx.productIds), position: 1 }, device: { type: 'desktop' }, consent: { analytics: true } });
    return ['POST /events', await raw('POST', '/events', { body: { events: [mk('product_view', 0), mk('impression', 1), mk('page_view', 2)] } }), [202]];
  },
};

async function fire(kind) {
  if (inflight >= MAX_INFLIGHT) { dropped++; return; }
  inflight++;
  const t0 = performance.now();
  let label = kind, status = 0, ok = [200];
  try { const [l, r, o] = await actions[kind](); label = l; status = r.status; ok = o; } catch { status = 0; }
  inflight--;
  record(label, status, performance.now() - t0, ok);
}

// ------------------------------------------------------------------ chạy
async function setup() {
  console.log(`[setup] tạo seller + ${PRODUCTS} sản phẩm + ${BUYERS} buyer ...`);
  const seller = await account('ls', 'seller_admin');
  const st = await raw('POST', '/users/sellers', { token: seller.access, body: { seller: { name: `LOAD shop ${run}`, bank_account: '1', tax_code: '1', description: 'load test', phone: '0909000111', address: 'x' } } });
  if (st.status !== 200) throw new Error(`tạo store: ${st.status} ${JSON.stringify(st.body)}`);
  for (let i = 0; i < 40; i++) { if ((await raw('GET', '/seller/products?page_size=1', { token: seller.access })).status === 200) break; await sleep(500); }
  const cats = await raw('GET', '/categories');
  const cid = cats.body?.categories?.[0]?.id;
  const productIds = [];
  for (let i = 0; i < PRODUCTS; i++) {
    const r = await raw('POST', '/products', { token: seller.access, body: { name: `LOAD ${pick(words)} ${run} ${i}`, price: 50000 + i * 1000, inventory: 1_000_000, description: 'load', ...(cid ? { category_id: cid } : {}) } });
    if (r.body?.id) productIds.push(r.body.id); else throw new Error(`tạo sản phẩm: ${r.status} ${JSON.stringify(r.body)}`);
  }
  const buyers = [];
  for (let i = 0; i < BUYERS; i++) {
    const b = await account('lb', 'buyer');
    const a = await raw('POST', '/users/me/addresses', { token: b.access, body: { label: 'Nhà', receiver_name: 'Load Test', phone: '0901234567', line1: '1 Test', city: 'HCM', is_default: true } });
    if (a.status !== 201) throw new Error(`tạo địa chỉ: ${a.status}`);
    buyers.push(b);
  }
  ctx = { productIds, buyers, seq: 0 };
}

function report() {
  console.log('\n=============== KẾT QUẢ ===============');
  console.log(`${'endpoint'.padEnd(26)} ${'n'.padStart(7)} ${'ok'.padStart(7)} ${'429'.padStart(6)} ${'lỗi'.padStart(6)} ${'p50'.padStart(7)} ${'p95'.padStart(7)} ${'p99'.padStart(7)} ${'max'.padStart(7)}  mã`);
  let total = 0, errs = 0, limited = 0, slow = [];
  for (const [name, s] of Object.entries(stats).sort()) {
    total += s.n; errs += s.err; limited += s.limited;
    const p95 = pct(s.lat, 95);
    if (P95_MS !== null && p95 > P95_MS) slow.push(`${name} p95=${p95.toFixed(0)}ms`);
    console.log(`${name.padEnd(26)} ${String(s.n).padStart(7)} ${String(s.ok).padStart(7)} ${String(s.limited).padStart(6)} ${String(s.err).padStart(6)} ${pct(s.lat, 50).toFixed(0).padStart(7)} ${p95.toFixed(0).padStart(7)} ${pct(s.lat, 99).toFixed(0).padStart(7)} ${Math.max(0, ...s.lat).toFixed(0).padStart(7)}  ${JSON.stringify(s.codes)}`);
  }
  const errPct = total ? (errs / total) * 100 : 0;
  console.log(`\ntổng=${total} lỗi=${errs} (${errPct.toFixed(2)}%) rate_limited=${limited} dropped(quá MAX_INFLIGHT)=${dropped}`);
  if (limited) console.log('CẢNH BÁO: có 429 - kết quả bị rate limit chi phối; nâng RATE_LIMIT_* trên stack test.');
  if (dropped) console.log('CẢNH BÁO: client không theo kịp RPS yêu cầu (hoặc server quá chậm) - tăng MAX_INFLIGHT hoặc giảm RPS.');
  let code = 0;
  if (errPct > MAX_ERR_PCT) { console.log(`FAIL: tỉ lệ lỗi ${errPct.toFixed(2)}% > ${MAX_ERR_PCT}%`); code = 1; }
  if (slow.length) { console.log(`FAIL: vượt P95_MS=${P95_MS}: ${slow.join('; ')}`); code = 1; }
  if (!total) { console.log('FAIL: không có request nào'); code = 1; }
  if (!code) console.log('PASS');
  return code;
}

console.log(`Load: ${API} rps=${RPS} duration=${DURATION}s ramp=${RAMP}s mix=${MIX.map(([n, w]) => `${n}:${w}`).join(',')}`);
if (!MIX.every(([n]) => actions[n])) { console.error(`MIX chứa loại không hợp lệ (cho phép: ${Object.keys(actions).join(', ')})`); process.exit(2); }
try { await setup(); } catch (e) { console.error(`setup thất bại: ${e.message}`); process.exit(1); }

const totalW = MIX.reduce((s, [, w]) => s + w, 0);
const choose = () => { let r = Math.random() * totalW; for (const [n, w] of MIX) { if ((r -= w) < 0) return n; } return MIX[0][0]; };
const start = Date.now(), tick = 20;
let credit = 0, lastReport = start;
const tasks = new Set();
console.log('[run] bắt đầu');
while ((Date.now() - start) / 1000 < DURATION) {
  const el = (Date.now() - start) / 1000;
  const rate = RAMP > 0 && el < RAMP ? RPS * (el / RAMP) : RPS;
  credit += (rate * tick) / 1000;
  while (credit >= 1) { credit -= 1; const p = fire(choose()); tasks.add(p); p.finally(() => tasks.delete(p)); }
  if (Date.now() - lastReport >= 10_000) {
    console.log(`  t=${Math.round(el)}s rate=${rate.toFixed(1)}/s inflight=${inflight} 10s: n=${win.n} lỗi=${win.err} dropped=${dropped}`);
    win.n = 0; win.err = 0; lastReport = Date.now();
  }
  await sleep(tick);
}
await Promise.race([Promise.allSettled([...tasks]), sleep(20000)]);
process.exit(report());
