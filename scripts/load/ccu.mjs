#!/usr/bin/env node
// Thử tải theo CCU (người dùng đồng thời), vòng kín ("closed loop"): mỗi người dùng ảo (VU) chạy hành trình thật có thời gian "nghĩ", nên số người
// đồng thời = VUS. Khác load.mjs (open loop theo RPS). Node 18+, không cần cài gói.
//
//   ADMIN_USERNAME=admin ADMIN_PASSWORD=... node scripts/load/ccu.mjs --vus=25 --duration=90
//
// Mỗi VU có tài khoản và IP giả riêng (X-Forwarded-For), JWT riêng. Mỗi lần lặp VU chọn NGẪU NHIÊN (có trọng số) một hành trình con, nên tải đa dạng và
// chạm MỌI container nghiệp vụ. Cuối bài in bảng "ĐỘ PHỦ" (số lời gọi theo service) và cảnh báo nếu service nào không được gọi.
//   buyer       : duyệt + tìm + lọc + trang frontend, mua COD/MOCK_CARD/ví/chuyển khoản/thẻ bị từ chối/3-D Secure, quản lý giỏ, hồ sơ + địa chỉ + đổi mật khẩu,
//                 theo dõi đơn (lọc, huỷ, xác nhận đã nhận, trả hàng)
//   seller      : xử lý đơn (giao, hoàn tất, huỷ), duyệt/từ chối trả hàng (hoàn tiền), quản lý sản phẩm (tạo/sửa/ẩn/tồn kho), báo cáo analytics, nhân viên
//   admin       : đơn + thanh toán toàn nền tảng, hoàn tiền, báo cáo analytics toàn nền tảng, tài khoản, danh mục, sức khoẻ hệ thống/ops (cần ADMIN_*)
//   operator    : đọc vận hành: sức khoẻ, sự cố, audit, sổ hành động, truy vấn, playbook, outbox, đề xuất (tạo tự động bằng tài khoản admin)
//   ai_*        : hỏi trợ lý AI (buyer/seller/admin/operator) qua POST /ai/chat (SSE) tới `event: done`. Cần mô hình giả (deploy/services.perf.yml)
// Cần cấu hình thử tải (scripts/deploy-perf.sh): nâng giới hạn tốc độ, gateway tin X-Forwarded-For (docs/design/08, 09).
//
// Tham số (env hoặc --key=value):
//   API_URL      gateway qua Traefik                       (http://api.marketplace.swarm.localhost)
//   FRONTEND_URL frontend qua Traefik                      (API_URL bỏ tiền tố "api.")
//   VUS          số người dùng đồng thời                    (25)
//   DURATION     giây chạy ổn định (không tính ramp)        (60)
//   ARRIVAL_RATE phiên MỚI mỗi giây (chế độ MỞ/nửa mở: khách đến theo nhịp, không chờ hệ thống; mỗi khách làm SESSION_JOURNEYS hành trình rồi rời đi). Đặt thì bỏ qua VUS (0)
//                (mở) khách quay lại dùng lại tài khoản cũ: chỉ đăng nhập, không đăng ký mới mỗi phiên
//   SESSION_JOURNEYS  số hành trình mỗi phiên ở chế độ mở, ngẫu nhiên từ n-1 đến n+1  (3)
//   MAX_ACTIVE   chặn số phiên cùng chạy ở chế độ mở để máy tải không cạn (4000); vượt thì bỏ khách mới và đếm vào 'bỏ'
//   ENTRY_RATE   VU vào mỗi giây (cố định cho mọi mức CCU, để 100 và 500 CCU cùng một cách vào) (5)
//   RAMP         giây tăng dần VU                           (mặc định VUS / ENTRY_RATE)
//   MIX          tỉ lệ VU theo loại                         (buyer:50,seller:12,admin:8,operator:8,ai_buyer:12,ai_seller:4,ai_admin:4,ai_operator:4)
//   THINK_MS / AI_THINK_MS  thời gian nghĩ giữa hai bước / hai câu hỏi AI (1500 / 8000)
//   PRODUCTS     số sản phẩm của shop test                  (30)
//   ADMIN_USERNAME, ADMIN_PASSWORD  tài khoản admin (mặc định ADMIN_BOOTSTRAP_*); thiếu => bỏ admin/operator và báo thiếu độ phủ
//   OUT          ghi kết quả JSON ra file                   (không ghi)
//   MAX_ERR_PCT  ngưỡng lỗi ngoài dự kiến                   (1)
// Mã thoát: 0 đạt, 1 vượt ngưỡng lỗi / setup hỏng / thiếu độ phủ.
import { writeFileSync, readFileSync } from 'node:fs';
import { monitorEventLoopDelay } from 'node:perf_hooks';

const argv = Object.fromEntries(process.argv.slice(2).filter((a) => a.startsWith('--')).map((a) => { const [k, v = '1'] = a.slice(2).split('='); return [k.toUpperCase().replace(/-/g, '_'), v]; }));
const P = (k, d) => argv[k] ?? process.env[k] ?? d;
const API = String(P('API_URL', 'http://api.marketplace.swarm.localhost')).replace(/\/$/, '');
const WEB = String(P('FRONTEND_URL', API.replace('//api.', '//'))).replace(/\/$/, '');
const VUS = Number(P('VUS', 25)), DURATION = Number(P('DURATION', 60)), ARRIVAL = Number(P('ARRIVAL_RATE', 0)), SESSION_N = Number(P('SESSION_JOURNEYS', 3)), MAX_ACTIVE = Number(P('MAX_ACTIVE', 4000)), ENTRY_RATE = Number(P('ENTRY_RATE', 5)), RAMP = Number(P('RAMP', Math.ceil(VUS / ENTRY_RATE)));
const THINK = Number(P('THINK_MS', 1500)), AI_THINK = Number(P('AI_THINK_MS', 8000)), PRODUCTS = Number(P('PRODUCTS', 30));
const MAX_ERR_PCT = Number(P('MAX_ERR_PCT', 1)), OUT = P('OUT', '');
const ADMIN_USER = P('ADMIN_USERNAME', process.env.ADMIN_BOOTSTRAP_USERNAME ?? ''), ADMIN_PASS = P('ADMIN_PASSWORD', process.env.ADMIN_BOOTSTRAP_PASSWORD ?? '');
const MIX = String(P('MIX', 'buyer:50,seller:12,admin:8,operator:8,ai_buyer:12,ai_seller:4,ai_admin:4,ai_operator:4')).split(',').map((x) => { const [n, w] = x.split(':'); return [n, Number(w)]; });
const PASSWORD = 'LoadPass_1';
const run = Date.now().toString(36);
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const think = (m = THINK) => sleep(m * (0.5 + Math.random()));
const pick = (a) => a[Math.floor(Math.random() * a.length)];
const words = ['ao', 'quan', 'giay', 'tui', 'mu', 'dep'];
const chance = (p) => Math.random() < p;
const weighted = (list) => { const t = list.reduce((a, [w]) => a + w, 0); let r = Math.random() * t; for (const [w, f] of list) { if ((r -= w) < 0) return f; } return list[0][1]; };

// ------------------------------------------------------------------ số liệu
const stats = {}, areas = {};
const stat = (n) => (stats[n] ??= { n: 0, ok: 0, expected: 0, limited: 0, err: 0, lat: [], codes: {} });
// Sonde của CHÍNH máy tải trong pha đo: nếu vòng lặp sự kiện của node bị trễ hoặc cả máy gần hết CPU thì số đo bị méo (máy tải và hệ thống dùng chung một máy)
const probe = { eld: null, timer: null, cpu: [], prev: null, memMin: Infinity, active: false };
function cpuTimes() { const l = readFileSync('/proc/stat', 'utf8').split('\n')[0].trim().split(/\s+/).slice(1).map(Number); return { idle: l[3] + (l[4] ?? 0), total: l.reduce((a, b) => a + b, 0) }; }
function startProbe() {
  try { probe.eld = monitorEventLoopDelay({ resolution: 10 }); probe.eld.enable(); } catch { /* không có */ }
  try { probe.prev = cpuTimes(); } catch { probe.prev = null; }
  probe.active = true;
  probe.timer = setInterval(() => {
    try {
      const c = cpuTimes(); if (probe.prev) { const dt = c.total - probe.prev.total; if (dt > 0) probe.cpu.push((100 * (dt - (c.idle - probe.prev.idle))) / dt); } probe.prev = c;
      const m = /MemAvailable:\s+(\d+)/.exec(readFileSync('/proc/meminfo', 'utf8')); if (m) probe.memMin = Math.min(probe.memMin, Number(m[1]) / 1048576);
    } catch { /* không phải Linux */ }
  }, 2000);
}
function stopProbe() {
  if (!probe.active) return null; phaseEnd = Date.now(); probe.active = false; clearInterval(probe.timer); probe.eld?.disable();
  const cpu = probe.cpu, ms = (x) => (x ? +(x / 1e6).toFixed(1) : null);
  const out = {
    eventLoopMs: probe.eld ? { mean: ms(probe.eld.mean), p99: ms(probe.eld.percentile(99)), max: ms(probe.eld.max) } : null,
    hostCpuPct: cpu.length ? { avg: +(cpu.reduce((a, b) => a + b, 0) / cpu.length).toFixed(1), max: +Math.max(...cpu).toFixed(1), samples: cpu.length } : null,
    hostMemAvailableMinGiB: Number.isFinite(probe.memMin) ? +probe.memMin.toFixed(1) : null,
  };
  return out;
}
// (kho riêng cho ai_buyer: tài khoản của họ KHÔNG có địa chỉ mặc định, buyer dùng nhầm thì POST /orders trả 404 'address not found')
// Chế độ MỞ: khách quay lại dùng TÀI KHOẢN CŨ (chỉ đăng nhập) thay vì mỗi phiên đăng ký mới; nếu không, bcrypt của auth-service sẽ là thứ bị đo chứ không phải hệ thống
const POOL = ARRIVAL > 0, pool = { buyer: [], aibuyer: [], sellerai: [], adminstaff: [], opstaff: [] };
const poolKey = { buyer: 'buyer', ai_buyer: 'aibuyer', ai_seller: 'sellerai', ai_admin: 'adminstaff', ai_operator: 'opstaff' };
let active = 0, peakActive = 0, started = 0, dropped = 0;
const counters = { journeys: 0, orders: 0, shipped: 0, returns: 0, refunds: 0, ai_done: 0, ai_err: 0, vu_fail: 0 };
const aiTtfb = [], aiTotal = [], payLag = []; let payTimeouts = 0; // payLag: ms từ lúc đặt đơn tới lúc payment xuất hiện (tạo bất đồng bộ qua Kafka)
let phaseStart = 0, steady = false;
const pct = (arr, p) => { if (!arr.length) return 0; const a = [...arr].sort((x, y) => x - y); return a[Math.min(a.length - 1, Math.floor((p / 100) * a.length))]; };
// container nghiệp vụ mà đường dẫn gateway sẽ chạm tới (ước lượng theo router; để in độ phủ)
const AREAS = [[/^\/auth|^\/admin\/accounts/, 'auth-service'], [/^\/users/, 'user-service'], [/^\/ops|^\/playbooks/, 'ops-service'], [/^\/ai\//, 'ai-agent'],
  [/^\/events|analytics/, 'analytics-service'], [/^\/payments|^\/admin\/payments/, 'payment-service'], [/^\/cart|^\/checkout|^\/orders|^\/seller\/orders|^\/admin\/orders|^\/shop\/rules/, 'order-service'],
  [/^\/products|^\/search|^\/categories|^\/seller\/products|^\/seller\/inventory|^\/admin\/categories|^\/admin\/products/, 'product-service']];
const areaOf = (path) => (AREAS.find(([re]) => re.test(path)) ?? [null, 'api-gateway'])[1];
const hit = (area) => { areas[area] = (areas[area] ?? 0) + 1; };

function tally(name, status, ms, { ok, soft = [] }) {
  const s = stat(steady ? name : `(vào) ${name}`);
  s.n++; s.lat.push(ms); s.codes[status] = (s.codes[status] ?? 0) + 1;
  if (status === 429) s.limited++; else if (ok.includes(status)) s.ok++; else if (soft.includes(status)) s.expected++; else s.err++;
  return s;
}
// ok = mã mong đợi; soft = mã chấp nhận được do tranh chấp bình thường (hai người cùng xử lý một đơn...) nhưng vẫn đếm riêng
async function call(label, method, path, { vu, token, body, headers = {}, ok = [200], soft = [], timeout = 30000 } = {}) {
  const t0 = performance.now();
  let status = 0, json = null;
  try {
    const res = await fetch(API + path, {
      method, signal: AbortSignal.timeout(timeout),
      headers: { ...(body ? { 'Content-Type': 'application/json' } : {}), ...(token ? { Authorization: `Bearer ${token}` } : {}), ...(vu ? { 'X-Forwarded-For': vu.ip } : {}), ...headers },
      body: body ? JSON.stringify(body) : undefined,
    });
    status = res.status;
    const txt = await res.text();
    try { json = JSON.parse(txt); } catch { /* không phải JSON */ }
  } catch { status = 0; }
  const ms = performance.now() - t0;
  tally(label, status, ms, { ok, soft });
  hit(areaOf(path)); hit('api-gateway');
  return { status, json, ms };
}
async function web(vu, path) { // trang HTML của frontend (SSR) qua Traefik
  const t0 = performance.now();
  let status = 0;
  try { const r = await fetch(WEB + path, { signal: AbortSignal.timeout(30000), headers: { 'X-Forwarded-For': vu.ip } }); status = r.status; await r.text(); } catch { status = 0; }
  tally(`WEB ${path.replace(/\d+/g, ':id').split('?')[0]}`, status, performance.now() - t0, { ok: [200] });
  hit('frontend');
}

// ------------------------------------------------------------------ tài khoản
// Vào (đăng ký/đăng nhập) thử lại khi 5xx/timeout: một lượt vào hỏng không nên bỏ cả VU (người thật sẽ bấm lại)
async function entry(label, path, opts) {
  let r;
  for (let i = 0; i < 3; i++) { r = await call(label, 'POST', path, opts); if (r.status !== 0 && r.status < 500) break; await sleep(1000 * (i + 1)); }
  return r;
}
async function login(username, password, role, vu) {
  const l = await entry('POST /auth/login', '/auth/login', { vu, body: { username, password, role } });
  if (l.status !== 200) throw new Error(`login ${role} ${l.status}`);
  return { username, password, role, access: l.json.access_token, refresh: l.json.refresh_token, at: Date.now() };
}
async function account(prefix, role, vu) {
  const username = `${prefix}${run}${Math.random().toString(36).slice(2, 6)}`.slice(0, 16);
  const r = await entry('POST /auth/register', '/auth/register', { vu, body: { username, password: PASSWORD, role } });
  if (r.status !== 200) throw new Error(`register ${r.status}`);
  return login(username, PASSWORD, role, vu);
}
async function tok(u, vu) { // access token sống ~5 phút: làm mới khi tuổi > 3 phút
  if (Date.now() - u.at > 180_000) {
    const r = await call('POST /auth/refresh-token', 'POST', '/auth/refresh-token', { vu, body: { refresh_token: u.refresh } });
    if (r.status === 200) { u.access = r.json.access_token; u.refresh = r.json.refresh_token ?? u.refresh; u.at = Date.now(); }
  }
  return u.access;
}
const made = []; // tài khoản admin/operator tạo thêm cho VU, xoá lúc kết thúc
async function staffAccount(role, vu, W) {
  const name = `l${role[0]}${run}${vu.id}`.slice(0, 16);
  const c = await call('POST /admin/accounts', 'POST', '/admin/accounts', { vu, token: W.admin.access, ok: [200, 201], body: { username: name, password: PASSWORD, role } });
  if (![200, 201].includes(c.status)) throw new Error(`tạo ${role} ${c.status}`);
  made.push(name);
  return login(name, PASSWORD, role, vu);
}
async function sellerAccount(vu) { // seller riêng có store (khoá chat AI: mỗi user một luồng)
  const u = await account('ls', 'seller_admin', vu);
  const st = await call('POST /users/sellers', 'POST', '/users/sellers', { vu, token: u.access, body: { seller: { name: `LOAD ai ${vu.id}`, bank_account: '1', tax_code: '1', description: 'ccu', phone: '0909000111', address: 'x' } } });
  if (st.status !== 200) throw new Error(`tạo store ${st.status}`);
  for (let i = 0; i < 40; i++) { if ((await call('GET /seller/products', 'GET', '/seller/products?page_size=1', { vu, token: u.access, soft: [403] })).status === 200) break; await sleep(500); }
  return login(u.username, PASSWORD, 'seller_admin', vu);
}
const uidOf = (u) => { try { const c = JSON.parse(Buffer.from(u.access.split('.')[1], 'base64url').toString()); return c.UserID ?? c.sub ?? c.user_id ?? c.id; } catch { return null; } };

// ------------------------------------------------------------------ setup: shop dùng chung (+ operator nếu có admin)
async function setup() {
  console.log(`[setup] tạo chủ shop + ${PRODUCTS} sản phẩm ...`);
  const vu = { ip: '198.51.100.250' };
  const owner = await account('lo', 'seller_admin', vu);
  const st = await call('POST /users/sellers', 'POST', '/users/sellers', { vu, token: owner.access, body: { seller: { name: `LOAD shop ${run}`, bank_account: '1', tax_code: '1', description: 'ccu test', phone: '0909000111', address: 'x' } } });
  if (st.status !== 200) throw new Error(`tạo store ${st.status} ${JSON.stringify(st.json)}`);
  for (let i = 0; i < 40; i++) { if ((await call('GET /seller/products', 'GET', '/seller/products?page_size=1', { vu, token: owner.access, soft: [403] })).status === 200) break; await sleep(500); }
  const o2 = await login(owner.username, PASSWORD, 'seller_admin', vu); // đăng nhập lại để token mang store_id
  const cats = await call('GET /categories', 'GET', '/categories', { vu });
  const cid = cats.json?.categories?.[0]?.id;
  const productIds = [];
  for (let i = 0; i < PRODUCTS; i++) {
    const r = await call('POST /products', 'POST', '/products', { vu, token: o2.access, body: { name: `LOAD ${pick(words)} ${run} ${i}`, price: 50000 + i * 1000, inventory: 1_000_000, description: 'ccu', ...(cid ? { category_id: cid } : {}) } });
    if (!r.json?.id) throw new Error(`tạo sản phẩm ${r.status} ${JSON.stringify(r.json)}`);
    productIds.push(r.json.id);
  }
  const W = { owner: o2, productIds, categoryId: cid, admin: null, operator: null };
  if (ADMIN_USER && ADMIN_PASS) {
    try {
      W.admin = await login(ADMIN_USER, ADMIN_PASS, 'admin', vu);
      const name = `lop${run}`.slice(0, 16);
      const c = await call('POST /admin/accounts', 'POST', '/admin/accounts', { vu, token: W.admin.access, ok: [200, 201], body: { username: name, password: PASSWORD, role: 'operator' } });
      if ([200, 201].includes(c.status)) W.operator = { username: name, id: c.json?.id ?? c.json?.account?.id };
      else console.log(`  (không tạo được operator: ${c.status}; bỏ loại operator)`);
    } catch (e) { console.log(`  (đăng nhập admin thất bại: ${e.message}; bỏ admin/operator)`); }
  } else console.log('  (không có ADMIN_USERNAME/ADMIN_PASSWORD: bỏ admin/operator, độ phủ ops-service sẽ thiếu)');
  return W;
}
async function teardown(W) { // xoá các tài khoản admin/operator đã tạo (không xoá tài khoản bootstrap)
  if (!W.admin) return;
  for (const name of [...made, ...(W.operator ? [W.operator.username] : [])]) {
    try {
      const id = (await call('GET /admin/accounts', 'GET', '/admin/accounts?q=' + name, { token: W.admin.access })).json?.accounts?.find((a) => a.username === name)?.id;
      if (id) await call('DELETE /admin/accounts/:id', 'DELETE', `/admin/accounts/${id}`, { token: W.admin.access, ok: [200, 204] });
    } catch { /* bỏ qua */ }
  }
}

// ------------------------------------------------------------------ buyer
const evt = (vu, W, type = 'product_view') => ({ events: [type, 'impression', 'page_view'].map((t, i) => ({ event_id: `${run}-${vu.id}-${vu.seq++}-${i}`, event_type: t, ts_client: new Date().toISOString(), anonymous_id: `a_${run}_${vu.id}`, session_id: `s_${run}_${vu.id}`, surface: 'pdp', page: { path: '/products/1' }, item: { product_id: pick(W.productIds), position: 1 }, device: { type: pick(['desktop', 'mobile']) }, consent: { analytics: true } })) });
const T = (vu) => tok(vu.user, vu);

async function buyerBrowse(vu, W) {
  await web(vu, pick(['/', '/products', '/cart', '/login']));
  await call('GET /categories', 'GET', '/categories', { vu });
  await think();
  await call('GET /products', 'GET', `/products?page=${1 + Math.floor(Math.random() * 3)}&page_size=10`, { vu });
  await web(vu, `/products/${pick(W.productIds)}`);
  await call('POST /events', 'POST', '/events', { vu, body: evt(vu, W), ok: [202] });
  const filters = [`q=${pick(words)}`, `category_id=${W.categoryId ?? ''}`, 'price_min=40000&price_max=90000', 'in_stock=true', `sort=${pick(['relevance', 'price_asc', 'price_desc', 'newest', 'best_selling'])}`];
  for (let i = 0; i < 2; i++) { await call('GET /search', 'GET', `/search?${pick(filters)}&page_size=20`, { vu }); await call('POST /events (search)', 'POST', '/events', { vu, body: evt(vu, W, 'search'), ok: [202] }); await think(THINK / 2); }
  for (const id of [pick(W.productIds), pick(W.productIds)]) { await call('GET /products/:id', 'GET', `/products/${id}`, { vu }); await call('GET /products/:id/stock', 'GET', `/products/${id}/stock`, { vu }); }
  await call('GET /shop/rules', 'GET', '/shop/rules', { vu });
  await call('GET /payments/methods', 'GET', '/payments/methods', { vu });
}

async function fillCart(vu, W, n = 2) {
  for (let i = 0; i < n; i++) await call('PUT /cart/items/:id', 'PUT', `/cart/items/${pick(W.productIds)}`, { vu, token: await T(vu), body: { quantity: 1 + Math.floor(Math.random() * 2) } });
}
async function pollPayment(vu, o) {
  const t0 = performance.now();
  let pay = o.json?.payment;
  for (let i = 0; i < 16 && !pay?.id; i++) { await sleep(500); pay = (await call('GET /checkouts/:id', 'GET', `/checkouts/${o.json.checkout_id}`, { vu, token: await T(vu) })).json?.payment; }
  if (steady) { if (pay?.id) payLag.push(performance.now() - t0); else payTimeouts++; } // quá ~8 s như frontend thì coi là hết thời gian chờ
  return pay;
}
const CARD = { card_exp: '12/34', card_cvc: '123' };
async function buyerPurchase(vu, W) {
  await fillCart(vu, W, 1 + Math.floor(Math.random() * 3));
  await call('GET /cart', 'GET', '/cart', { vu, token: await T(vu) });
  await think();
  const pv = await call('POST /checkout/preview', 'POST', '/checkout/preview', { vu, token: await T(vu), body: { from_cart: true } });
  if (pv.status !== 200 || pv.json?.can_order === false) return;
  const method = weighted([[40, 'COD'], [20, 'CARD'], [10, 'WALLET'], [5, 'BANK'], [10, 'DECLINED'], [10, '3DS'], [5, 'CANCEL_PAY']]);
  const pm = { COD: 'COD', CARD: 'MOCK_CARD', DECLINED: 'MOCK_CARD', '3DS': 'MOCK_CARD', CANCEL_PAY: 'MOCK_CARD', WALLET: 'MOCK_WALLET', BANK: 'MOCK_BANK_TRANSFER' }[method];
  const key = `cu-${run}-${vu.id}-${vu.seq++}`;
  const o = await call(`POST /orders (${pm})`, 'POST', '/orders', { vu, token: await T(vu), ok: [200, 201], headers: { 'Idempotency-Key': key }, body: { from_cart: true, address_id: 0, payment_method: pm, note: 'ccu' } });
  if (![200, 201].includes(o.status)) return;
  counters.orders++;
  if (chance(0.1)) await call('POST /orders (replay)', 'POST', '/orders', { vu, token: await T(vu), ok: [200, 201], headers: { 'Idempotency-Key': key }, body: { from_cart: true, address_id: 0, payment_method: pm } }); // idempotent: trả checkout cũ
  if (method === 'COD') return;
  const pay = await pollPayment(vu, o);
  if (!pay?.id) return;
  const t = await T(vu);
  if (method === 'CARD') await call('POST /payments/:id/confirm', 'POST', `/payments/${pay.id}/confirm`, { vu, token: t, body: { card_number: '4242424242424242', ...CARD } });
  else if (method === 'DECLINED') await call('POST /payments/:id/confirm (declined)', 'POST', `/payments/${pay.id}/confirm`, { vu, token: t, ok: [200], soft: [402, 422, 400], body: { card_number: '4000000000000002', ...CARD } });
  else if (method === '3DS') {
    const c = await call('POST /payments/:id/confirm (3DS)', 'POST', `/payments/${pay.id}/confirm`, { vu, token: t, ok: [200], soft: [402, 422], body: { card_number: '4000000000003220', ...CARD } });
    if (c.status === 200) await call('POST /payments/:id/confirm (otp)', 'POST', `/payments/${pay.id}/confirm`, { vu, token: t, ok: [200], soft: [402, 422], body: { card_number: '4000000000003220', ...CARD, otp: '123456' } });
  } else if (method === 'WALLET' || method === 'BANK') {
    await call('POST /payments/:id/confirm (wallet/bank)', 'POST', `/payments/${pay.id}/confirm`, { vu, token: t, body: { approve: true } });
    await sleep(method === 'WALLET' ? 3500 : 500); // ví: webhook giả sau ~3 s; chuyển khoản ~8 s (không chờ)
  } else if (method === 'CANCEL_PAY') await call('POST /payments/:id/cancel', 'POST', `/payments/${pay.id}/cancel`, { vu, token: t, ok: [200], soft: [422] });
  await call('GET /payments/:id', 'GET', `/payments/${pay.id}`, { vu, token: await T(vu) });
}

async function buyerCart(vu, W) {
  await fillCart(vu, W, 3);
  await call('GET /cart', 'GET', '/cart', { vu, token: await T(vu) });
  await call('POST /cart/merge', 'POST', '/cart/merge', { vu, token: await T(vu), body: { items: [{ product_id: pick(W.productIds), quantity: 1 }, { product_id: pick(W.productIds), quantity: 2 }] } });
  await think();
  await call('PUT /cart/items/:id (qty)', 'PUT', `/cart/items/${pick(W.productIds)}`, { vu, token: await T(vu), body: { quantity: 3 } });
  await call('DELETE /cart/items/:id', 'DELETE', `/cart/items/${pick(W.productIds)}`, { vu, token: await T(vu) });
  await call('POST /checkout/preview (items)', 'POST', '/checkout/preview', { vu, token: await T(vu), body: { items: [{ product_id: pick(W.productIds), quantity: 1 }] } });
  await call('DELETE /cart', 'DELETE', '/cart', { vu, token: await T(vu) });
}

async function buyerProfile(vu) {
  const id = uidOf(vu.user);
  if (!vu.hasProfile) {
    const r = await call('POST /users/buyers', 'POST', '/users/buyers', { vu, token: await T(vu), ok: [200, 201], soft: [409], body: { buyer: { name: `LOAD ${vu.id}`, gender: 'male', date_of_birth: '1995-05-05T00:00:00Z', phone: '0901234567', address: '1 Test' } } });
    vu.hasProfile = r.status < 300;
  }
  if (id) {
    await call('GET /users/buyers/:id', 'GET', `/users/buyers/${id}`, { vu, token: await T(vu), ok: [200], soft: [404] });
    await call('PUT /users/buyers/:id', 'PUT', `/users/buyers/${id}`, { vu, token: await T(vu), ok: [200], soft: [404], body: { buyer: { name: `LOAD ${vu.id} b`, gender: 'male', date_of_birth: '1995-05-05T00:00:00Z', phone: '0901234568', address: '2 Test' } } });
  }
  await call('GET /users/me/addresses', 'GET', '/users/me/addresses', { vu, token: await T(vu) });
  const a = await call('POST /users/me/addresses', 'POST', '/users/me/addresses', { vu, token: await T(vu), ok: [201], soft: [400, 422], body: { label: 'Cơ quan', receiver_name: 'Load Test', phone: '0901234567', line1: '2 Test', city: 'HCM' } });
  const aid = a.json?.address?.id;
  if (aid) {
    await call('PUT /users/me/addresses/:id', 'PUT', `/users/me/addresses/${aid}`, { vu, token: await T(vu), body: { label: 'Cơ quan', receiver_name: 'Load Test', phone: '0901234567', line1: '3 Test', city: 'HCM' } });
    await call('DELETE /users/me/addresses/:id', 'DELETE', `/users/me/addresses/${aid}`, { vu, token: await T(vu), ok: [200, 204] });
  }
  if (chance(0.3)) { // đổi mật khẩu thu hồi token cũ => đăng nhập lại bằng mật khẩu mới
    const np = `${PASSWORD}x`;
    const c = await call('POST /auth/change-password', 'POST', '/auth/change-password', { vu, token: await T(vu), body: { old_password: vu.user.password, new_password: np } });
    if (c.status === 200) vu.user = await login(vu.user.username, np, 'buyer', vu);
  }
}

async function buyerFollowup(vu) {
  await call('GET /orders/summary', 'GET', '/orders/summary', { vu, token: await T(vu) });
  const f = pick(['', 'status=CONFIRMED', 'payment_method=COD', 'payment_status=PAID', 'sort=total_desc', 'min_total=100000']);
  const orders = await call('GET /orders', 'GET', `/orders?${f}&page_size=10`, { vu, token: await T(vu) });
  const list = orders.json?.orders ?? [];
  if (list.length) await call('GET /orders/:id', 'GET', `/orders/${pick(list).id}`, { vu, token: await T(vu) });
  const shipped = (await call('GET /orders (SHIPPED)', 'GET', '/orders?status=SHIPPED&page_size=5', { vu, token: await T(vu) })).json?.orders ?? [];
  if (shipped.length) await call('POST /orders/:id/confirm-received', 'POST', `/orders/${pick(shipped).id}/confirm-received`, { vu, token: await T(vu), soft: [409, 422] });
  const delivered = (await call('GET /orders (DELIVERED)', 'GET', '/orders?status=DELIVERED&page_size=5', { vu, token: await T(vu) })).json?.orders ?? [];
  if (delivered.length && chance(0.5)) { const r = await call('POST /orders/:id/return', 'POST', `/orders/${pick(delivered).id}/return`, { vu, token: await T(vu), body: { reason: 'ccu trả hàng' }, soft: [409, 422] }); if (r.status === 200) counters.returns++; }
  const open = list.filter((o) => ['CONFIRMED', 'AWAITING_PAYMENT', 'PAID'].includes(o.status));
  if (open.length && chance(0.15)) await call('POST /orders/:id/cancel', 'POST', `/orders/${pick(open).id}/cancel`, { vu, token: await T(vu), body: { reason: 'ccu huỷ' }, soft: [409, 422] });
}

// ------------------------------------------------------------------ seller
const ST = (vu) => tok(vu.user, vu);
async function sellerOrders(vu) {
  const inbox = await call('GET /seller/orders', 'GET', '/seller/orders?page_size=20', { vu, token: await ST(vu) });
  // hộp thư mới nhất bị đơn chờ thanh toán lấn át khi tải cao, nên lấy riêng đơn đã sẵn sàng giao (CONFIRMED/PAID)
  const ready = (await call('GET /seller/orders (ready)', 'GET', `/seller/orders?status=${pick(['CONFIRMED', 'PAID'])}&page_size=10`, { vu, token: await ST(vu) })).json?.orders ?? [];
  await call('GET /seller/orders/summary', 'GET', '/seller/orders/summary', { vu, token: await ST(vu) });
  const orders = inbox.json?.orders ?? [];
  if (orders.length) await call('GET /seller/orders/:id', 'GET', `/seller/orders/${pick(orders).id}`, { vu, token: await ST(vu) });
  const todo = [...orders.filter((o) => ['PAID', 'CONFIRMED'].includes(o.status)), ...ready];
  if (todo.length) {
    const o = pick(todo);
    if (chance(0.1)) { await call('POST /seller/orders/:id/cancel', 'POST', `/seller/orders/${o.id}/cancel`, { vu, token: await ST(vu), body: { reason: 'ccu hết hàng' }, soft: [404, 409, 422] }); return; }
    const s = await call('POST /seller/orders/:id/ship', 'POST', `/seller/orders/${o.id}/ship`, { vu, token: await ST(vu), body: { carrier: 'GHN', tracking_code: `LT${Date.now().toString(36)}` }, soft: [404, 409, 422] });
    if (s.status === 200) { counters.shipped++; await think(); if (chance(0.7)) await call('POST /seller/orders/:id/deliver', 'POST', `/seller/orders/${o.id}/deliver`, { vu, token: await ST(vu), soft: [404, 409, 422] }); }
  }
}
async function sellerReturns(vu) {
  const rr = (await call('GET /seller/orders (REFUND_REQUESTED)', 'GET', '/seller/orders?status=REFUND_REQUESTED&page_size=10', { vu, token: await ST(vu) })).json?.orders ?? [];
  if (!rr.length) return;
  const o = pick(rr);
  if (chance(0.8)) { const a = await call('POST /seller/orders/:id/return/approve', 'POST', `/seller/orders/${o.id}/return/approve`, { vu, token: await ST(vu), soft: [404, 409, 422] }); if (a.status === 200) counters.refunds++; }
  else await call('POST /seller/orders/:id/return/reject', 'POST', `/seller/orders/${o.id}/return/reject`, { vu, token: await ST(vu), body: { reason: 'ccu từ chối' }, soft: [404, 409, 422] });
}
async function sellerCatalog(vu, W) {
  await call('GET /seller/products', 'GET', '/seller/products?page_size=20', { vu, token: await ST(vu) });
  const r = await call('POST /products', 'POST', '/products', { vu, token: await ST(vu), body: { name: `LOAD moi ${run} ${vu.id}-${vu.seq++}`, price: 70000, inventory: 500, description: 'ccu', status: 'active', ...(W.categoryId ? { category_id: W.categoryId } : {}) } });
  const id = r.json?.id;
  if (id) {
    await call('PUT /products/:id', 'PUT', `/products/${id}`, { vu, token: await ST(vu), body: { product: { name: `LOAD sua ${run} ${id}`, price: 75000, inventory: 450, description: 'ccu sửa', status: '', ...(W.categoryId ? { category_id: W.categoryId } : {}) } } });
    await call('PATCH /products/:id/status', 'PATCH', `/products/${id}/status`, { vu, token: await ST(vu), body: { status: 'hidden' } });
    await call('PATCH /products/:id/status', 'PATCH', `/products/${id}/status`, { vu, token: await ST(vu), body: { status: 'active' } });
  }
  const pid = pick(W.productIds);
  await call('POST /seller/products/:id/inventory/adjust', 'POST', `/seller/products/${pid}/inventory/adjust`, { vu, token: await ST(vu), body: { delta: 10, reason: 'ccu restock' } });
  await call('GET /seller/products/:id/inventory/ledger', 'GET', `/seller/products/${pid}/inventory/ledger?page_size=10`, { vu, token: await ST(vu) });
  await call('GET /seller/inventory/low-stock', 'GET', '/seller/inventory/low-stock', { vu, token: await ST(vu) });
}
async function sellerAnalytics(vu) {
  for (const p of ['summary', 'timeseries?granularity=day', 'top-products?sort=revenue', 'funnel', 'low-stock']) await call(`GET /seller/analytics/${p.split('?')[0]}`, 'GET', `/seller/analytics/${p}`, { vu, token: await ST(vu) });
}
async function sellerStaff(vu) { // một lần: tạo nhân viên rồi đăng nhập bằng nhân viên
  if (vu.staffDone) return sellerOrders(vu);
  vu.staffDone = true;
  const name = `ls${run}${vu.id}`.slice(0, 16);
  const c = await call('POST /auth/register-seller-roles', 'POST', '/auth/register-seller-roles', { vu, token: await ST(vu), ok: [200, 201], soft: [403, 409], body: { username: name, password: PASSWORD } });
  if ([200, 201].includes(c.status)) { const e = await login(name, PASSWORD, 'seller_employee', vu); await call('GET /seller/orders (staff)', 'GET', '/seller/orders?page_size=10', { vu, token: e.access, soft: [403] }); }
}

// ------------------------------------------------------------------ admin / operator
const AT = (vu) => tok(vu.user, vu);
async function adminOrdersPayments(vu) {
  const orders = await call('GET /admin/orders', 'GET', `/admin/orders?${pick(['', 'status=CONFIRMED', 'payment_method=MOCK_CARD', 'sort=newest'])}&page_size=10`, { vu, token: await AT(vu) });
  const o = (orders.json?.orders ?? [])[0];
  if (o) await call('GET /admin/orders/:id', 'GET', `/admin/orders/${o.id}`, { vu, token: await AT(vu) });
  const pays = await call('GET /admin/payments', 'GET', `/admin/payments?${pick(['', 'status=SUCCEEDED', 'method=MOCK_CARD', 'sort=amount_desc'])}&page_size=10`, { vu, token: await AT(vu) });
  const p = (pays.json?.payments ?? [])[0];
  if (p) await call('GET /admin/payments/:id', 'GET', `/admin/payments/${p.id}`, { vu, token: await AT(vu) });
  const ok = (await call('GET /admin/payments (SUCCEEDED)', 'GET', '/admin/payments?status=SUCCEEDED&page_size=5', { vu, token: await AT(vu) })).json?.payments ?? [];
  if (ok.length && chance(0.3)) { const r = await call('POST /admin/payments/:id/refund', 'POST', `/admin/payments/${pick(ok).id}/refund`, { vu, token: await AT(vu), soft: [422, 409], body: { amount_minor: 100, reason: 'ccu hoàn tiền thử' } }); if (r.status === 200) counters.refunds++; }
}
async function adminAnalytics(vu) {
  for (const p of ['summary', 'timeseries?granularity=day', 'top-products?sort=revenue', 'funnel', 'low-stock', 'traffic', 'payments', 'search-terms', 'data-health']) await call(`GET /admin/analytics/${p.split('?')[0]}`, 'GET', `/admin/analytics/${p}`, { vu, token: await AT(vu) });
}
async function adminManage(vu, W) {
  await call('GET /admin/accounts', 'GET', '/admin/accounts?limit=20', { vu, token: await AT(vu) });
  await call('GET /admin/categories', 'GET', '/admin/categories', { vu, token: await AT(vu) });
  if (chance(0.3)) { const slug = `ccu-${run}-${vu.id}-${vu.seq++}`; await call('POST /admin/categories', 'POST', '/admin/categories', { vu, token: await AT(vu), ok: [200, 201], soft: [400], body: { name: `CCU ${slug}`, slug, sort: 99, active: false } }); }
  await call('PATCH /admin/products/:id/status', 'PATCH', `/admin/products/${pick(W.productIds)}/status`, { vu, token: await AT(vu), soft: [404], body: { status: 'active' } });
}
async function opsReads(vu) {
  const t = await AT(vu);
  await call('GET /ops/system/health', 'GET', '/ops/system/health', { vu, token: t });
  const inc = await call('GET /ops/incidents', 'GET', '/ops/incidents?limit=20', { vu, token: t, soft: [503] });
  const i = (inc.json?.incidents ?? [])[0];
  if (i) await call('GET /ops/incidents/:id', 'GET', `/ops/incidents/${i.id}`, { vu, token: t, soft: [404, 503] });
  await call('GET /ops/audit', 'GET', '/ops/audit?limit=20', { vu, token: t, soft: [503] });
  await call('GET /ops/actions', 'GET', '/ops/actions', { vu, token: t, soft: [503] });
  const cat = await call('GET /ops/queries', 'GET', '/ops/queries', { vu, token: t, soft: [503] });
  const q = pick((cat.json?.queries ?? []).filter((x) => (x.params ?? []).every((y) => ['service', 'window'].includes(y.name)))); // bỏ truy vấn cần tham số khác (vd request_id của Loki)
  if (q) { // tham số lấy từ danh mục thật (enum hoặc mặc định), chỉ service/window được chuyển tiếp
    const qs = (q.params ?? []).filter((x) => ['service', 'window'].includes(x.name)).map((x) => `${x.name}=${encodeURIComponent(pick(x.enum ?? [x.default ?? '']))}`).join('&');
    await call('GET /ops/queries/:name', 'GET', `/ops/queries/${q.name}${qs ? '?' + qs : ''}`, { vu, token: t, soft: [502, 503, 504] });
  }
  await call('GET /ops/playbooks', 'GET', '/ops/playbooks', { vu, token: t, soft: [503] });
  await call('GET /ops/outbox/:svc/dead', 'GET', `/ops/outbox/${pick(['order-service', 'product-service', 'payment-service'])}/dead?limit=10`, { vu, token: t, soft: [503] });
  await call('GET /ops/proposals', 'GET', '/ops/proposals?limit=20', { vu, token: t, soft: [503] });
  await call('GET /ops/ai/kill', 'GET', '/ops/ai/kill', { vu, token: t });
  await call('GET /ops/rca/runs', 'GET', '/ops/rca/runs?limit=10', { vu, token: t, soft: [503, 404] });
}

// ------------------------------------------------------------------ AI
const aiQ = { buyer: ['Các đơn gần đây của mình thế nào?', 'Mình có đơn nào đang giao không?'], seller: ['Shop mình có bao nhiêu đơn theo trạng thái?'], admin: ['Các đơn mới nhất của nền tảng?'], operator: ['Hệ thống đang chạy thế nào?'] };
async function aiChat(vu, persona) {
  const t0 = performance.now();
  let status = 0, ttfb = 0, done = false;
  try {
    const res = await fetch(API + '/ai/chat', { method: 'POST', signal: AbortSignal.timeout(130000), headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${await AT(vu)}`, 'X-Forwarded-For': vu.ip }, body: JSON.stringify({ message: pick(aiQ[persona]) }) });
    status = res.status;
    if (status === 200) {
      const rd = res.body.getReader(), dec = new TextDecoder();
      let text = '';
      for (;;) { const { value, done: d } = await rd.read(); if (!ttfb && value) ttfb = performance.now() - t0; if (d) break; text += dec.decode(value, { stream: true }); }
      done = /^event: done/m.test(text);
    }
  } catch { status = 0; }
  const ms = performance.now() - t0;
  const okDone = status === 200 && done;
  tally(`POST /ai/chat (${persona})`, okDone ? 200 : status === 200 ? 599 : status, ms, { ok: [200] }); // 200 mà không có `event: done` tính là lỗi
  if (okDone) { counters.ai_done++; if (steady) { aiTtfb.push(ttfb); aiTotal.push(ms); } } else if (status !== 429) counters.ai_err++;
  hit('ai-agent'); hit(persona === 'operator' ? 'ops-service' : 'order-service'); // tool của mô hình giả gọi ngược gateway -> service
}

// ------------------------------------------------------------------ vòng lặp VU
const KINDS = {
  buyer: { login: async (vu) => { const p = POOL && pool.buyer.pop(); if (p) { vu.user = await login(p.username, p.password, 'buyer', vu); vu.hasProfile = p.hasProfile; return; } vu.user = await account('lb', 'buyer', vu); const a = await call('POST /users/me/addresses', 'POST', '/users/me/addresses', { vu, token: vu.user.access, ok: [201], body: { label: 'Nhà', receiver_name: 'Load Test', phone: '0901234567', line1: '1 Test', city: 'HCM', is_default: true } }); if (a.status !== 201) throw new Error(`địa chỉ ${a.status}`); },
    step: (vu, W) => weighted([[22, () => buyerBrowse(vu, W)], [32, () => buyerPurchase(vu, W)], [10, () => buyerCart(vu, W)], [12, () => buyerProfile(vu)], [24, () => buyerFollowup(vu)]])() },
  seller: { login: async (vu, W) => { vu.user = await login(W.owner.username, PASSWORD, 'seller_admin', vu); },
    step: (vu, W) => weighted([[35, () => sellerOrders(vu)], [15, () => sellerReturns(vu)], [20, () => sellerCatalog(vu, W)], [20, () => sellerAnalytics(vu)], [10, () => sellerStaff(vu)]])() },
  admin: { need: 'admin', login: async (vu) => { vu.user = await login(ADMIN_USER, ADMIN_PASS, 'admin', vu); },
    step: (vu, W) => weighted([[30, () => adminOrdersPayments(vu)], [30, () => adminAnalytics(vu)], [15, () => adminManage(vu, W)], [25, () => opsReads(vu)]])() },
  operator: { need: 'operator', login: async (vu, W) => { vu.user = await login(W.operator.username, PASSWORD, 'operator', vu); }, step: (vu) => opsReads(vu) },
  ai_buyer: { ai: 'buyer', login: async (vu) => { const p = POOL && pool.aibuyer.pop(); if (p) { vu.user = await login(p.username, p.password, 'buyer', vu); vu.hasProfile = p.hasProfile; return; } vu.user = await account('lb', 'buyer', vu); } },
  ai_seller: { ai: 'seller', login: async (vu) => { const p = POOL && pool.sellerai.pop(); vu.user = p ? await login(p.username, p.password, 'seller_admin', vu) : await sellerAccount(vu); } },
  ai_admin: { ai: 'admin', need: 'admin', login: async (vu, W) => { const p = POOL && pool.adminstaff.pop(); vu.user = p ? await login(p.username, p.password, 'admin', vu) : await staffAccount('admin', vu, W); } },
  ai_operator: { ai: 'operator', need: 'operator', login: async (vu, W) => { const p = POOL && pool.opstaff.pop(); vu.user = p ? await login(p.username, p.password, 'operator', vu) : await staffAccount('operator', vu, W); } },
};
async function vuLoop(id, kind, W, endAt, startDelay) {
  await sleep(startDelay);
  const vu = { id, kind, ip: `198.51.${(id >> 8) & 255}.${(id & 255) || 1}`, seq: 0 };
  const K = KINDS[kind];
  try { await K.login(vu, W); } catch (e) { counters.vu_fail++; console.log(`  VU ${id} (${kind}) vào thất bại: ${e.message}`); return; }
  while (Date.now() < endAt) {
    try { if (K.ai) { await aiChat(vu, K.ai); await think(AI_THINK); } else { await K.step(vu, W); await think(THINK * 2); } counters.journeys++; }
    catch (e) { counters.vu_fail++; console.log(`  VU ${id} (${kind}) lỗi: ${e.message}`); await sleep(1000); }
  }
}
// Chế độ MỞ (nửa mở): một khách = một phiên vài hành trình rồi rời đi; khách đến theo nhịp bất kể hệ thống đang chậm
async function session(id, kind, W) {
  active++; peakActive = Math.max(peakActive, active);
  const vu = { id, kind, ip: `198.51.${(id >> 8) & 255}.${(id & 255) || 1}`, seq: 0 };
  const K = KINDS[kind];
  try {
    await K.login(vu, W);
    const n = Math.max(1, SESSION_N + Math.floor(Math.random() * 3) - 1);
    for (let i = 0; i < n; i++) {
      try { if (K.ai) { await aiChat(vu, K.ai); await think(AI_THINK); } else { await K.step(vu, W); await think(THINK * 2); } counters.journeys++; }
      catch (e) { counters.vu_fail++; console.log(`  phiên ${id} (${kind}) lỗi: ${e.message}`); }
    }
  } catch (e) { counters.vu_fail++; console.log(`  phiên ${id} (${kind}) vào thất bại: ${e.message}`); }
  finally { active--; const k = poolKey[kind]; if (k && vu.user?.username) pool[k].push({ username: vu.user.username, password: vu.user.password, hasProfile: vu.hasProfile }); }
}
async function runOpen(W) {
  const mix = MIX.filter(([n]) => !(KINDS[n]?.need === 'admin' && !W.admin) && !(KINDS[n]?.need === 'operator' && !W.operator));
  const sum = mix.reduce((a, [, w]) => a + w, 0);
  const pickKind = () => { let r = Math.random() * sum; for (const [n, w] of mix) { if ((r -= w) < 0) return n; } return mix[0][0]; };
  const t0 = Date.now(), steadyAt = t0 + 30000, endAt = steadyAt + DURATION * 1000; // 30 s khởi động: đủ để số phiên cùng lúc và kho tài khoản tới mức ổn định
  setTimeout(() => { steady = true; phaseStart = Date.now(); startProbe(); console.log('[ổn định] bắt đầu đo'); }, steadyAt - t0);
  const ticker = setInterval(() => { const t = Object.values(stats); console.log(`  t=${((Date.now() - t0) / 1000).toFixed(0)}s lời gọi=${t.reduce((a, s) => a + s.n, 0)} lỗi=${t.reduce((a, s) => a + s.err, 0)} phiên đang chạy=${active} đơn=${counters.orders}`); }, 10000);
  const running = [];
  for (let k = 0; ; k++) { // phiên thứ k đến đúng lúc t0 + k/ARRIVAL (không dồn lại khi hệ thống chậm)
    const at = t0 + (k * 1000) / ARRIVAL; if (at >= endAt) break;
    const wait = at - Date.now(); if (wait > 0) await sleep(wait);
    if (active >= MAX_ACTIVE) { dropped++; continue; }
    started++; running.push(session(k + 1, pickKind(), W));
  }
  const measured = (Date.now() - phaseStart) / 1000; steady = false; PROBE = stopProbe(); // lời gọi của phiên còn dở sau khi hết giờ không tính vào số đo
  await Promise.race([Promise.all(running), sleep(90000)]);
  clearInterval(ticker);
  return measured;
}
function assign(W) { // chia VU theo MIX (bỏ loại thiếu điều kiện), phần dư dồn cho nhóm có phần thập phân lớn nhất
  const mix = MIX.filter(([n]) => !(KINDS[n]?.need === 'admin' && !W.admin) && !(KINDS[n]?.need === 'operator' && !W.operator));
  const sum = mix.reduce((a, [, w]) => a + w, 0);
  const parts = mix.map(([n, w]) => ({ n, exact: (VUS * w) / sum, c: Math.floor((VUS * w) / sum) }));
  let left = VUS - parts.reduce((a, p) => a + p.c, 0);
  for (const p of [...parts].sort((a, b) => (b.exact - b.c) - (a.exact - a.c))) { if (left-- > 0) p.c++; }
  return parts.flatMap((p) => Array(p.c).fill(p.n));
}

// ------------------------------------------------------------------ báo cáo
const WANT = ['api-gateway', 'auth-service', 'user-service', 'product-service', 'order-service', 'payment-service', 'analytics-service', 'ops-service', 'ai-agent', 'frontend'];
let PROBE = null, phaseEnd = 0;
function report(elapsed) {
  PROBE ??= stopProbe();
  console.log('\n=============== KẾT QUẢ ===============');
  console.log('nhãn'.padEnd(46), 'n'.padStart(6), 'ok'.padStart(6), 'dự kiến'.padStart(8), '429'.padStart(5), 'lỗi'.padStart(5), 'p50'.padStart(7), 'p95'.padStart(7), 'p99'.padStart(7), 'max'.padStart(7), ' mã');
  let n = 0, err = 0, limited = 0;
  for (const [name, s] of Object.entries(stats).sort(([a], [b]) => a.localeCompare(b))) {
    if (!name.startsWith('(vào)')) { n += s.n; err += s.err; limited += s.limited; }
    console.log(name.padEnd(46), String(s.n).padStart(6), String(s.ok).padStart(6), String(s.expected).padStart(8), String(s.limited).padStart(5), String(s.err).padStart(5),
      `${pct(s.lat, 50).toFixed(0)}`.padStart(7), `${pct(s.lat, 95).toFixed(0)}`.padStart(7), `${pct(s.lat, 99).toFixed(0)}`.padStart(7), `${Math.max(0, ...s.lat).toFixed(0)}`.padStart(7), ' ' + JSON.stringify(s.codes));
  }
  const errPct = n ? (100 * err) / n : 0;
  console.log(`\nổn định ${elapsed.toFixed(0)} s: ${n} lời gọi (${(n / elapsed).toFixed(1)}/s), lỗi ngoài dự kiến ${err} (${errPct.toFixed(2)}%), 429: ${limited}`);
  console.log(`hành trình xong ${counters.journeys}, đơn tạo ${counters.orders}, giao đi ${counters.shipped}, trả hàng ${counters.returns}, hoàn tiền ${counters.refunds}, VU hỏng ${counters.vu_fail}`);
  if (aiTotal.length || counters.ai_err) console.log(`AI: xong ${counters.ai_done}, lỗi ${counters.ai_err}; byte đầu p50/p95 ${pct(aiTtfb, 50).toFixed(0)}/${pct(aiTtfb, 95).toFixed(0)} ms; cả lượt p50/p95 ${pct(aiTotal, 50).toFixed(0)}/${pct(aiTotal, 95).toFixed(0)} ms`);
  if (payLag.length || payTimeouts) console.log(`Thanh toán tạo bất đồng bộ (đặt đơn -> payment xuất hiện): ${payLag.length} đơn, p50/p95/max ${pct(payLag, 50).toFixed(0)}/${pct(payLag, 95).toFixed(0)}/${Math.max(0, ...payLag).toFixed(0)} ms; quá 8 s chưa có: ${payTimeouts}`);
  if (PROBE) {
    const e = PROBE.eventLoopMs, h = PROBE.hostCpuPct;
    if (e) console.log(`MÁY TẢI: độ trễ vòng lặp sự kiện node mean/p99/max ${e.mean}/${e.p99}/${e.max} ms${e.p99 > 100 ? '  <-- CẢNH BÁO: máy tải bị nghẽn, độ trễ đo bị cộng thêm' : ''}`);
    if (h) console.log(`TOÀN MÁY: CPU trung bình ${h.avg}%, đỉnh ${h.max}% (${h.samples} mẫu)${h.max > 75 ? '  <-- CẢNH BÁO: trên 75%, máy tải và hệ thống tranh nhau CPU, số đo kém tin cậy' : ''}; RAM trống thấp nhất ${PROBE.hostMemAvailableMinGiB} GiB`);
  }
  console.log('\nĐỘ PHỦ (lời gọi tới từng container nghiệp vụ, gồm cả giai đoạn vào):');
  const missing = WANT.filter((a) => !areas[a]);
  for (const a of WANT) console.log(`  ${a.padEnd(20)} ${String(areas[a] ?? 0).padStart(6)}${areas[a] ? '' : '   <-- KHÔNG ĐƯỢC GỌI'}`);
  const valid = errPct <= MAX_ERR_PCT && limited === 0 && counters.vu_fail === 0;
  console.log(valid ? `\nPHÉP ĐO HỢP LỆ: lỗi ${errPct.toFixed(2)}% <= ${MAX_ERR_PCT}%, không có 429, mọi VU chạy được.` : `\nPHÉP ĐO CHƯA HỢP LỆ: lỗi ${errPct.toFixed(2)}% (ngưỡng ${MAX_ERR_PCT}%), 429=${limited}, VU hỏng=${counters.vu_fail}. Xem cột "mã".`);
  if (missing.length) console.log(`THIẾU ĐỘ PHỦ: ${missing.join(', ')}`);
  if (OUT) {
    // Chỉ ghi số tổng hợp (không ghi từng mẫu độ trễ): đủ để tính lại % và so sánh giữa các lần đo
    const lab = {}, codes = {}; let calls = 0, errs = 0;
    for (const [name, st] of Object.entries(stats)) {
      lab[name] = { n: st.n, ok: st.ok, expected: st.expected, limited: st.limited, err: st.err, p50: +pct(st.lat, 50).toFixed(1), p95: +pct(st.lat, 95).toFixed(1), p99: +pct(st.lat, 99).toFixed(1), max: +Math.max(0, ...st.lat).toFixed(1), codes: st.codes };
      if (!name.startsWith('(vào)')) { calls += st.n; errs += st.err; for (const [c, k] of Object.entries(st.codes)) codes[c] = (codes[c] ?? 0) + k; }
    }
    const q = (a) => ({ n: a.length, p50: +pct(a, 50).toFixed(1), p95: +pct(a, 95).toFixed(1), max: +Math.max(0, ...a).toFixed(1) });
    writeFileSync(OUT, JSON.stringify({
      steadyStartMs: phaseStart, steadyEndMs: phaseEnd || Date.now(), mode: ARRIVAL > 0 ? 'open' : 'closed', vus: VUS, arrivalRate: ARRIVAL || null, duration: DURATION, elapsed,
      steady: { calls, callsPerSec: +(calls / elapsed).toFixed(1), unexpectedErrors: errs, unexpectedErrorPct: calls ? +((100 * errs) / calls).toFixed(3) : 0, codes },
      open: ARRIVAL > 0 ? { started, dropped, peakActive } : null,
      labels: lab, areas, counters, ai: { ttfb: q(aiTtfb), total: q(aiTotal) }, payment: { lag: q(payLag), timeoutsOver8s: payTimeouts }, loadgen: PROBE, valid, missing,
    }, null, 1));
  }
  return valid && !missing.length;
}

(async () => {
  if (ARRIVAL > 0) {
    console.log(`MỞ: ${ARRIVAL} phiên/s trong ${DURATION}s ổn định (+30 s khởi động), mỗi phiên ~${SESSION_N} hành trình, API ${API}`);
    let W; try { W = await setup(); } catch (e) { console.log('setup hỏng:', e.message); process.exit(1); }
    const measured = await runOpen(W);
    await teardown(W);
    console.log(`\nCHẾ ĐỘ MỞ: nhịp ${ARRIVAL} phiên/s, đã bắt đầu ${started} phiên, bỏ ${dropped} (vượt MAX_ACTIVE=${MAX_ACTIVE}), đỉnh ${peakActive} phiên cùng lúc`);
    process.exit(report(measured) ? 0 : 1);
  }
  console.log(`CCU ${VUS} VU, ${DURATION}s ổn định + ramp ${RAMP}s, API ${API}, frontend ${WEB}`);
  let W;
  try { W = await setup(); } catch (e) { console.log('setup hỏng:', e.message); process.exit(1); }
  const kinds = assign(W);
  const counts = {}; for (const k of kinds) counts[k] = (counts[k] ?? 0) + 1;
  console.log('[chạy]', JSON.stringify(counts));
  const t0 = Date.now(), steadyAt = t0 + RAMP * 1000 + 15000, endAt = steadyAt + DURATION * 1000;
  setTimeout(() => { steady = true; phaseStart = Date.now(); startProbe(); console.log('[ổn định] bắt đầu đo'); }, steadyAt - t0);
  const ticker = setInterval(() => { const t = Object.values(stats); console.log(`  t=${((Date.now() - t0) / 1000).toFixed(0)}s lời gọi=${t.reduce((a, s) => a + s.n, 0)} lỗi=${t.reduce((a, s) => a + s.err, 0)} 429=${t.reduce((a, s) => a + s.limited, 0)} đơn=${counters.orders}`); }, 10000);
  await Promise.all(kinds.map((k, i) => vuLoop(i + 1, k, W, endAt, (RAMP * 1000 * i) / VUS)));
  clearInterval(ticker); PROBE = stopProbe();
  await teardown(W);
  process.exit(report((Date.now() - phaseStart) / 1000) ? 0 : 1);
})();
