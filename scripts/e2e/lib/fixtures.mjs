// Hàm dựng dữ liệu & thao tác nghiệp vụ dùng chung giữa các kịch bản (chỉ qua REST của gateway).
import { config } from './config.mjs';
import { call, get, post, put, patch, del, jwtClaims, sleep } from './client.mjs';

let seq = 0;
export const uniq = (p) => `${p}${config.runId}${(seq++).toString(36)}`.slice(0, 16); // username ≤ 16 ký tự

export const ADDRESS = {
  label: 'Nhà', receiver_name: 'Nguyen Van E2E', phone: '0901234567', line1: '12 Nguyen Hue',
  ward: 'Ben Nghe', district: 'Quan 1', city: 'TP HCM', is_default: true,
};

// ---------------------------------------------------------------- tài khoản
export async function login(username, role, password = config.password) {
  const r = await post('/auth/login', { username, password, role });
  if (r.status !== 200) return { res: r };
  const claims = jwtClaims(r.body.access_token);
  return { res: r, token: r.body.access_token, refresh: r.body.refresh_token, id: claims.UserID, role, username };
}

export async function registerUser(prefix, role) {
  const username = uniq(prefix);
  const reg = await post('/auth/register', { username, password: config.password, role });
  const lg = await login(username, role);
  return { ...lg, username, role, reg };
}

export const registerBuyer = (p = 'b') => registerUser(p, 'buyer');

// Seller: đăng ký, tạo hồ sơ cửa hàng, chờ auth-service gán store_id (qua Kafka, bất đồng bộ).
export async function registerSeller(p = 's') {
  const u = await registerUser(p, 'seller_admin');
  if (!u.token) return u;
  u.storeRes = await post('/users/sellers', {
    seller: { name: `Shop ${u.username}`, bank_account: '0123456789', tax_code: '0312345678', description: 'Cửa hàng e2e', phone: '0909000111', address: '1 Le Loi, Q1' },
  }, { token: u.token });
  // Có store khi GET /seller/products khác 403
  for (let i = 0; i < 40; i++) {
    const r = await get('/seller/products?page_size=1', { token: u.token });
    if (r.status === 200) { u.hasStore = true; break; }
    await sleep(500);
  }
  return u;
}

export async function loginAdmin() {
  if (!config.adminUser || !config.adminPass) return null;
  const a = await login(config.adminUser, 'admin', config.adminPass);
  return a.token ? a : null;
}

// ---------------------------------------------------------------- catalog
let cachedCategory;
export async function categoryId(admin) {
  if (cachedCategory !== undefined) return cachedCategory;
  const r = await get('/categories');
  let c = r.body?.categories?.find((x) => x.active !== false);
  if (!c && admin) {
    const cr = await post('/admin/categories', { name: `E2E ${config.runId}`, sort: 1, active: true }, { token: admin.token });
    c = cr.body;
    if (c && !c.id) c = null;
  }
  cachedCategory = c?.id ?? 0;
  return cachedCategory;
}

export async function createProduct(seller, { name, price = 100000, inventory = 20, status = 'active', extra = {} } = {}) {
  const cid = await categoryId();
  const body = { name: name ?? `E2E san pham ${uniq('p')}`, price, inventory, description: 'Sản phẩm kiểm thử e2e', status, ...(cid ? { category_id: cid } : {}), ...extra };
  const r = await post('/products', body, { token: seller.token });
  return { id: r.body?.id, res: r, price, name: body.name };
}

export const getProduct = async (token, id) => (await get(`/products/${id}`, { token })).body?.product;
export const inventoryOf = async (seller, id) => (await getProduct(seller.token, id))?.inventory;
// Tồn kho đổi bất đồng bộ (order -> Kafka -> product): chờ tới khi đạt giá trị mong đợi (hoặc hết giờ) rồi trả giá trị cuối.
export async function inventoryEventually(seller, id, want, ms = 15000) {
  let v;
  for (const t0 = Date.now(); Date.now() - t0 < ms; ) {
    v = await inventoryOf(seller, id);
    if (v === want) return v;
    await new Promise((r) => setTimeout(r, 500));
  }
  return v;
}

// ---------------------------------------------------------------- địa chỉ / giỏ / đặt hàng
export async function ensureAddress(buyer) {
  const r = await post('/users/me/addresses', ADDRESS, { token: buyer.token });
  return r;
}

let keySeq = 0;
export const idemKey = (p = 'e2e') => `${p}-${config.runId}-${Date.now().toString(36)}-${keySeq++}`;

export function placeOrder(buyer, { items, fromCart = false, method = 'COD', key = idemKey(), addressId = 0, note } = {}) {
  const body = { payment_method: method, address_id: addressId, ...(fromCart ? { from_cart: true } : { items }), ...(note ? { note } : {}) };
  return post('/orders', body, { token: buyer.token, headers: { 'Idempotency-Key': key } });
}

export const getOrder = async (token, id) => (await get(`/orders/${id}`, { token })).body;
export const sellerOrder = async (seller, id) => (await get(`/seller/orders/${id}`, { token: seller.token }));

export async function waitOrder(t, name, token, orderId, statuses, timeoutMs = 45000) {
  const want = Array.isArray(statuses) ? statuses : [statuses];
  let last;
  const ok = await t.until(name, async () => {
    last = await getOrder(token, orderId);
    return want.includes(last?.status) ? last : null;
  }, { timeoutMs });
  return ok ?? last;
}

// Payment được tạo bất đồng bộ sau khi kho đã giữ: poll GET /checkouts/:id tới khi có `payment`.
export async function waitPayment(t, buyer, checkoutId, timeoutMs = 45000) {
  return t.until('payment được tạo (poll /checkouts/:id)', async () => {
    const r = await get(`/checkouts/${checkoutId}`, { token: buyer.token });
    return r.body?.payment ?? null;
  }, { timeoutMs });
}

export const confirmPayment = (buyer, paymentId, body) => post(`/payments/${paymentId}/confirm`, body, { token: buyer.token });
export const getPayment = async (buyer, id) => (await get(`/payments/${id}`, { token: buyer.token })).body;

export const CARD = { exp: '12/34', cvc: '123' };
export const card = (number, extra = {}) => ({ card_number: number, card_exp: CARD.exp, card_cvc: CARD.cvc, ...extra });

// Đặt 1 đơn (1 sản phẩm) và trả { checkout, order }
export async function quickOrder(buyer, product, { qty = 1, method = 'COD' } = {}) {
  const res = await placeOrder(buyer, { items: [{ product_id: product.id, quantity: qty }], method });
  return { res, checkout: res.body, order: res.body?.orders?.[0] };
}

// Đưa 1 đơn COD tới DELIVERED (ship → deliver)
export async function deliverCod(t, buyer, seller, product) {
  const { res, order } = await quickOrder(buyer, product, { method: 'COD' });
  t.status('đặt đơn COD', res, 201);
  const o = await waitOrder(t, 'COD → CONFIRMED', buyer.token, order.id, 'CONFIRMED');
  const sh = await post(`/seller/orders/${order.id}/ship`, { carrier: 'GHN', tracking_code: 'TRK' + uniq('') }, { token: seller.token });
  t.status('seller ship', sh, 200);
  const dl = await post(`/seller/orders/${order.id}/deliver`, {}, { token: seller.token });
  t.status('seller deliver', dl, 200);
  return { order: dl.body ?? o, id: order.id };
}

// Đưa 1 đơn MOCK_CARD (4242...) tới PAID
export async function payByCard(t, buyer, product, qty = 1) {
  const { res, checkout, order } = await quickOrder(buyer, product, { qty, method: 'MOCK_CARD' });
  t.status('đặt đơn MOCK_CARD', res, 201);
  const pay = await waitPayment(t, buyer, checkout.checkout_id);
  if (!pay) return { order, checkout };
  const c = await confirmPayment(buyer, pay.id, card('4242 4242 4242 4242'));
  t.status('thanh toán thẻ 4242', c, 200);
  const o = await waitOrder(t, 'đơn → PAID', buyer.token, order.id, 'PAID');
  return { order: o, checkout, payment: c.body, id: order.id };
}

export { call, get, post, put, patch, del, sleep };
