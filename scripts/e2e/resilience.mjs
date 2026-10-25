#!/usr/bin/env node
// Các bước nhỏ do resilience.sh gọi (mỗi lần chạy in ĐÚNG MỘT dòng JSON ra stdout). Trạng thái lưu ở $STATE_FILE.
//   STEP=setup     tạo seller+store+sản phẩm (tồn 50), buyer + địa chỉ; lưu thông tin đăng nhập (token 5 phút nên đăng nhập lại mỗi bước)
//   STEP=order     đặt 1 đơn 1 sản phẩm; METHOD=COD|MOCK_CARD (mặc định COD)  -> {http, order_id, checkout_id}
//   STEP=pay       thanh toán thẻ 4242 cho đơn MOCK_CARD gần nhất               -> {pay_http, order_status}
//   STEP=status    đếm đơn theo trạng thái + tồn kho thật                        -> {counts:{...}, inventory}
//   STEP=wait      chờ tới khi không còn đơn PENDING (WAIT_MS)                   -> như status + timed_out
//   STEP=events    POST /events (1 event)                                        -> {http, accepted}
//   STEP=analytics GET /seller/analytics/summary                                 -> {http}
//   STEP=catalog   GET /search (đọc catalog)                                     -> {http}
//   STEP=health    GET /admin/system/health (cần admin)                          -> {http, status, services:{tên:status}}
//   STEP=probe     GET $PROBE_URL (vd http://order-service:8081/ready)           -> {http}
import { readFileSync, writeFileSync } from 'node:fs';
import { config } from './lib/config.mjs';
import { get, post, sleep } from './lib/client.mjs';
import { registerBuyer, registerSeller, createProduct, ensureAddress, login, loginAdmin, placeOrder } from './lib/fixtures.mjs';

const STATE = process.env.STATE_FILE ?? '/state/state.json';
const out = (o) => console.log(JSON.stringify(o));
const load = () => JSON.parse(readFileSync(STATE, 'utf8'));
const relog = async (u) => (await login(u.username, u.role)).token;
const step = process.env.STEP;

async function counts(s) {
  const tok = await relog(s.buyer);
  const c = {};
  const r = await get('/orders/summary', { token: tok });
  Object.assign(c, r.body?.by_status ?? {});
  const stok = await relog(s.seller);
  const p = await get(`/products/${s.productId}`, { token: stok });
  return { http: r.status, counts: c, inventory: p.body?.product?.inventory };
}

if (step === 'setup') {
  const seller = await registerSeller('rs');
  const buyer = await registerBuyer('rb');
  const prod = await createProduct(seller, { inventory: 50, price: 100000, name: `E2E resilience ${config.runId}` });
  await ensureAddress(buyer);
  writeFileSync(STATE, JSON.stringify({ seller: { username: seller.username, role: 'seller_admin' }, buyer: { username: buyer.username, role: 'buyer' }, productId: prod.id }));
  out({ ok: !!prod.id && !!seller.hasStore && !!buyer.token, product_id: prod.id });
} else if (step === 'order') {
  const s = load();
  const buyer = { token: await relog(s.buyer) };
  const method = process.env.METHOD ?? 'COD';
  const r = await placeOrder(buyer, { items: [{ product_id: s.productId, quantity: 1 }], method });
  s.last = { order_id: r.body?.orders?.[0]?.id, checkout_id: r.body?.checkout_id, method };
  writeFileSync(STATE, JSON.stringify(s));
  out({ http: r.status, order_id: s.last.order_id, checkout_id: s.last.checkout_id, error: r.body?.error });
} else if (step === 'pay') {
  const s = load();
  const buyer = { token: await relog(s.buyer) };
  let pay = null;
  for (let i = 0; i < 40 && !pay; i++) { pay = (await get(`/checkouts/${s.last.checkout_id}`, { token: buyer.token })).body?.payment; if (!pay) await sleep(1000); }
  if (!pay) out({ pay_http: 0, order_status: null, note: 'payment chưa được tạo' });
  else {
    const c = await post(`/payments/${pay.id}/confirm`, { card_number: '4242 4242 4242 4242', card_exp: '12/34', card_cvc: '123' }, { token: buyer.token });
    let st = null;
    for (let i = 0; i < 40 && st !== 'PAID'; i++) { st = (await get(`/orders/${s.last.order_id}`, { token: buyer.token })).body?.status; if (st !== 'PAID') await sleep(1000); }
    out({ pay_http: c.status, order_status: st });
  }
} else if (step === 'status') {
  out(await counts(load()));
} else if (step === 'wait') {
  const s = load();
  const end = Date.now() + Number(process.env.WAIT_MS ?? 90000);
  let st;
  do { st = await counts(s); if (!st.counts.PENDING) break; await sleep(2000); } while (Date.now() < end);
  out({ ...st, timed_out: !!st.counts.PENDING });
} else if (step === 'events') {
  const r = await post('/events', { events: [{ event_id: `${Date.now()}-${Math.random()}`, event_type: 'page_view', ts_client: new Date().toISOString(), anonymous_id: 'a_res', session_id: 's_res', page: { path: '/' }, consent: { analytics: true } }] });
  out({ http: r.status, accepted: r.body?.accepted, dropped: r.body?.dropped });
} else if (step === 'analytics') {
  const s = load();
  const r = await get('/seller/analytics/summary', { token: await relog(s.seller) });
  out({ http: r.status });
} else if (step === 'catalog') {
  out({ http: (await get('/search?q=E2E&page_size=5')).status });
} else if (step === 'health') {
  const a = await loginAdmin();
  if (!a) out({ http: 0, note: 'không có admin' });
  else {
    const r = await get('/admin/system/health', { token: a.token });
    out({ http: r.status, status: r.body?.status, services: Object.fromEntries((r.body?.services ?? []).map((x) => [x.name, x.status])) });
  }
} else if (step === 'probe') {
  try { out({ http: (await fetch(process.env.PROBE_URL, { signal: AbortSignal.timeout(5000) })).status }); } catch { out({ http: 0 }); }
} else {
  console.error(`STEP không hợp lệ: ${step}`);
  process.exit(2);
}
