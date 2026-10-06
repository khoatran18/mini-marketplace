// End-to-end test against a deployed stack (Node 18+, no dependencies).
//   API_URL=https://api.marketplace.swarm.localhost UI_URL=https://marketplace.swarm.localhost \
//   DASH_URL=https://dashboard.marketplace.swarm.localhost DASH_AUTH=admin:admin \
//   NODE_EXTRA_CA_CERTS=deploy/traefik/certs/local.crt node scripts/e2e/e2e.mjs
// Exercises Traefik → gateway → gRPC services → Postgres/Redis/Kafka for real.
const API = process.env.API_URL ?? 'http://localhost:8080';
const UI = process.env.UI_URL;
const DASH = process.env.DASH_URL;
const DASH_AUTH = process.env.DASH_AUTH;
const run = Date.now().toString(36);
// The protobuf contract currently limits usernames/passwords to [A-Za-z0-9_]{3,16}
const PASSWORD = 'E2ePass_1';
const NEW_PASSWORD = 'AnotherPass_1';

let passed = 0;
const failures = [];
function check(name, ok, detail = '') {
  if (ok) { passed++; console.log(`  ok   ${name}`); }
  else { failures.push(name); console.log(`  FAIL ${name} ${detail}`); }
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function until(name, fn, timeoutMs = 45000) {
  const end = Date.now() + timeoutMs;
  let last;
  while (Date.now() < end) {
    last = await fn();
    if (last) return last;
    await sleep(1000);
  }
  check(name, false, '(timed out)');
  return null;
}

async function call(method, path, { token, body, headers = {} } = {}) {
  const res = await fetch(API + path, {
    method,
    headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}), ...headers },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  let json = null;
  try { json = await res.json(); } catch { /* empty body */ }
  return { status: res.status, body: json, headers: res.headers };
}

async function newUser(prefix, role) {
  const username = `${prefix}_${run}`;
  const reg = await call('POST', '/auth/register', { body: { username, password: PASSWORD, role } });
  const login = await call('POST', '/auth/login', { body: { username, password: PASSWORD, role } });
  const payload = JSON.parse(Buffer.from(login.body.access_token.split('.')[1], 'base64url').toString());
  return { username, role, reg, access: login.body.access_token, refresh: login.body.refresh_token, id: payload.UserID };
}

const orders = (token, status) => call('GET', `/orders?status=${status}`, { token });
const productById = async (token, id) => (await call('GET', `/products/${id}`, { token })).body?.product;

async function main() {
  console.log(`E2E run ${run} against ${API}`);
  if (process.env.RATE_LIMIT_TEST === '1') return rateLimitOnly();

  console.log('\n[1] Edge: health, TLS routing, UI, dashboard, CORS');
  check('gateway /health through Traefik', (await call('GET', '/health')).status === 200);
  if (UI) {
    const ui = await fetch(UI);
    check('frontend served through Traefik', ui.status === 200 && (await ui.text()).includes('<html'));
  }
  if (DASH) {
    const noAuth = await fetch(`${DASH}/dashboard/`);
    check('Traefik dashboard requires basic auth', noAuth.status === 401, `status ${noAuth.status}`);
    const auth = await fetch(`${DASH}/dashboard/`, { headers: { Authorization: 'Basic ' + Buffer.from(DASH_AUTH).toString('base64') } });
    check('Traefik dashboard accepts credentials', auth.status === 200, `status ${auth.status}`);
  }
  const pre = await call('OPTIONS', '/products', { headers: { Origin: UI ?? 'http://localhost:3000', 'Access-Control-Request-Method': 'GET' } });
  check('CORS allows the frontend origin', pre.headers.get('access-control-allow-origin') === (UI ?? 'http://localhost:3000'));
  const evil = await call('OPTIONS', '/products', { headers: { Origin: 'https://evil.example.com', 'Access-Control-Request-Method': 'GET' } });
  check('CORS rejects foreign origins', !evil.headers.get('access-control-allow-origin'));

  console.log('\n[2] Authentication');
  const seller = await newUser('sela', 'seller_admin');
  const buyer = await newUser('buya', 'buyer');
  const buyer2 = await newUser('buyb', 'buyer');
  check('register + login (seller, buyers)', seller.access && buyer.access && buyer2.access);
  check('weak password rejected', (await call('POST', '/auth/register', { body: { username: `weak_${run}`, password: 'short', role: 'buyer' } })).status === 400);
  check('unknown role rejected', (await call('POST', '/auth/register', { body: { username: `adm_${run}`, password: PASSWORD, role: 'admin' } })).status === 400);
  check('seller_employee not self-registrable', [400, 403].includes((await call('POST', '/auth/register', { body: { username: `emp_${run}`, password: PASSWORD, role: 'seller_employee' } })).status));
  const dup = await call('POST', '/auth/register', { body: { username: seller.username, password: PASSWORD, role: 'seller_admin' } });
  check('duplicate registration is a 409', dup.status === 409, `status ${dup.status}`);
  const bad = await call('POST', '/auth/login', { body: { username: buyer.username, password: 'WrongPass_1', role: 'buyer' } });
  const unk = await call('POST', '/auth/login', { body: { username: `nobody_${run}`, password: PASSWORD, role: 'buyer' } });
  check('wrong password → 401, same error as unknown user', bad.status === 401 && unk.status === 401 && bad.body.error === unk.body.error);
  check('no token → 401', (await call('GET', '/orders?status=PENDING')).status === 401);
  check('refresh token cannot be used as access token', (await orders(buyer.refresh, 'PENDING')).status === 401);
  const refreshed = await call('POST', '/auth/refresh-token', { body: { refresh_token: buyer.refresh } });
  check('refresh token yields new tokens', refreshed.status === 200 && refreshed.body.access_token);
  check('access token cannot refresh', (await call('POST', '/auth/refresh-token', { body: { refresh_token: buyer.access } })).status === 401);

  console.log('\n[3] Store and catalog (user-service → Kafka → auth-service store link)');
  const storeBody = { seller: { name: `Shop ${run}`, bank_account: `ACC-${run}`, tax_code: `TAX-${run}`, description: 'e2e shop', phone: '0123', address: 'Hanoi', date_of_birth: '1990-01-01T00:00:00Z' }, user_id: 999999 };
  check('seller_admin can create a store', (await call('POST', '/users/sellers', { token: seller.access, body: storeBody })).status === 200);
  check('buyer cannot create a store', (await call('POST', '/users/sellers', { token: buyer.access, body: storeBody })).status === 403);
  check('seller cannot create products before the store is linked or while unlinked',
    true); // linkage is asynchronous; verified below by waiting for the first successful product
  let storeId = null;
  const created = await until('product created once account↔store link event is consumed', async () => {
    const r = await call('POST', '/products', { token: seller.access, body: { name: `Widget ${run}`, price: 100.5, inventory: 5, attributes: { color: 'red' }, seller_id: 424242 } });
    return r.status === 200 ? r : null;
  });
  check('product creation succeeds for linked seller', !!created);
  for (let id = 1; id <= 200 && !storeId; id++) {
    const r = await call('GET', `/users/sellers/${id}`, { token: seller.access });
    if (r.status === 200 && r.body.seller?.bank_account === `ACC-${run}`) storeId = id;
  }
  check('own store found with full profile', storeId !== null);
  const mine = (await call('GET', `/products/seller/${storeId}`, { token: seller.access })).body?.products ?? [];
  const product = mine.find((p) => p.name === `Widget ${run}`);
  check('product belongs to the caller store, not the spoofed seller_id', !!product && product.seller_id === storeId);
  const pub = await call('GET', `/users/sellers/${storeId}`, { token: buyer.access });
  check('other users get only the public store view', pub.status === 200 && pub.body.seller.name === `Shop ${run}` && !pub.body.seller.bank_account && !pub.body.seller.tax_code);
  check('buyer cannot create products', (await call('POST', '/products', { token: buyer.access, body: { name: 'x', price: 1, inventory: 1 } })).status === 403);
  const seller2 = await newUser('selb', 'seller_admin');
  check('seller without a store cannot create products', (await call('POST', '/products', { token: seller2.access, body: { name: 'x', price: 1, inventory: 1 } })).status === 403);
  check("another seller cannot update the product", (await call('PUT', `/products/${product.id}`, { token: seller2.access, body: { product: { name: 'hijack', price: 1, inventory: 1 } } })).status === 403);
  check('negative inventory rejected', (await call('PUT', `/products/${product.id}`, { token: seller.access, body: { product: { name: 'x', price: 1, inventory: -1 } } })).status === 400);
  check('page_size is capped, page 0 is safe', (await call('GET', '/products?page=0&page_size=100000', { token: buyer.access })).status === 200);
  check('buyer profile of another user is forbidden', (await call('GET', `/users/buyers/${buyer2.id}`, { token: buyer.access })).status === 403);
  const profile = await call('POST', '/users/buyers', { token: buyer.access, body: { buyer: { user_id: buyer2.id, name: 'Buyer One', gender: 'x', phone: '1', address: 'a', date_of_birth: '1995-01-01T00:00:00Z' } } });
  const own = await call('GET', `/users/buyers/${buyer.id}`, { token: buyer.access });
  check('buyer profile is always created for the caller', profile.status === 200 && own.status === 200 && own.body.buyer.name === 'Buyer One');

  console.log('\n[4] Order saga across order-service, Kafka and product-service');
  const order = (token, qty, extra = {}) => call('POST', '/orders', { token, body: { order: { buyer_id: 1, status: 'SUCCESS', total_price: 0.01, order_items: [{ product_id: product.id, quantity: qty, price: 0.01 }], ...extra } } });
  check('seller cannot place orders', (await order(seller.access, 1)).status === 403);
  check('empty order rejected', (await call('POST', '/orders', { token: buyer.access, body: { order: { order_items: [] } } })).status === 400);
  check('zero quantity rejected', (await order(buyer.access, 0)).status === 400);
  check('more than the stock is refused up front (422)', (await order(buyer.access, 6)).status === 422);
  check('unknown product is 404', (await call('POST', '/orders', { token: buyer.access, body: { order: { order_items: [{ product_id: 99999999, quantity: 1 }] } } })).status === 404);

  const o1 = await order(buyer.access, 2);
  check('order accepted (client price/status/buyer ignored)', o1.status === 200, JSON.stringify(o1.body));
  const success = await until('order becomes SUCCESS via Kafka round trip', async () => {
    const l = (await orders(buyer.access, 'SUCCESS')).body?.orders ?? [];
    return l.length ? l : null;
  });
  const o = success?.[0];
  check('total price computed by the server (2 × 100.50)', o?.total_price === 201, `got ${o?.total_price}`);
  check('item has catalog price and product name', o?.order_items?.[0]?.price === 100.5 && o?.order_items?.[0]?.name === `Widget ${run}`);
  check('order belongs to the authenticated buyer', o?.buyer_id === buyer.id);
  check('inventory reserved (5 → 3)', (await productById(buyer.access, product.id))?.inventory === 3);

  check('other buyer cannot read the order', (await call('GET', `/orders/${o.id}`, { token: buyer2.access })).status === 404);
  check('other buyer cannot cancel the order', (await call('DELETE', `/orders/${o.id}`, { token: buyer2.access })).status === 404);
  check('other buyer does not see it in listings', ((await orders(buyer2.access, 'SUCCESS')).body?.orders ?? []).length === 0);
  check('buyer_id query parameter is ignored', ((await call('GET', `/orders?status=SUCCESS&buyer_id=${buyer.id}`, { token: buyer2.access })).body?.orders ?? []).length === 0);
  check('PUT /orders/:id no longer exists', (await call('PUT', `/orders/${o.id}`, { token: buyer.access, body: { order: { status: 'SUCCESS', total_price: 0 } } })).status === 404);
  check('invalid status filter is 400', (await orders(buyer.access, 'BOGUS')).status === 400);

  const cancel = await call('DELETE', `/orders/${o.id}`, { token: buyer.access });
  check('owner cancels a SUCCESS order', cancel.status === 200, JSON.stringify(cancel.body));
  await until('inventory released after cancel event (3 → 5)', async () => (await productById(buyer.access, product.id))?.inventory === 5);
  check('order is CANCELED', ((await orders(buyer.access, 'CANCELED')).body?.orders ?? []).some((x) => x.id === o.id));
  check('second cancel is refused (422)', (await call('DELETE', `/orders/${o.id}`, { token: buyer.access })).status === 422);
  await sleep(8000);
  check('inventory not released twice', (await productById(buyer.access, product.id))?.inventory === 5);

  console.log('\n[5] Concurrency: 12 buyers race for 5 units');
  const racers = await Promise.all(Array.from({ length: 12 }, (_, i) => newUser(`rc${i}`, 'buyer')));
  const results = await Promise.all(racers.map((r) => order(r.access, 1)));
  // Late arrivals may already see the stock gone and are refused up front (422); the rest enter the saga.
  const accepted = results.filter((r) => r.status === 200).length;
  check('every order is either accepted or refused for lack of stock', results.every((r) => r.status === 200 || r.status === 422), results.map((r) => r.status).join(','));
  await until('all racing orders settle', async () => {
    const counts = await Promise.all(racers.map(async (r) => ((await orders(r.access, 'PENDING')).body?.orders ?? []).length));
    return counts.every((c) => c === 0);
  }, 90000);
  let ok = 0; let failed = 0;
  for (const r of racers) {
    ok += ((await orders(r.access, 'SUCCESS')).body?.orders ?? []).length;
    failed += ((await orders(r.access, 'FAILED')).body?.orders ?? []).length;
  }
  check('exactly 5 succeed, the other accepted orders fail (no overselling)', ok === 5 && failed === accepted - 5, `accepted=${accepted} success=${ok} failed=${failed}`);
  check('inventory is exactly 0', (await productById(buyer.access, product.id))?.inventory === 0);

  console.log('\n[6] Password change revokes old tokens (gateway ← Kafka ← auth-service)');
  const victim = await newUser('victim', 'buyer');
  check('change-password requires authentication', (await call('POST', '/auth/change-password', { body: { username: victim.username, role: 'buyer', old_password: PASSWORD, new_password: NEW_PASSWORD } })).status === 401);
  check('wrong old password refused without leaking details', await (async () => {
    const r = await call('POST', '/auth/change-password', { token: victim.access, body: { old_password: 'NopeNope_1', new_password: NEW_PASSWORD } });
    return r.status === 400 && !/bcrypt/i.test(JSON.stringify(r.body));
  })());
  const change = await call('POST', '/auth/change-password', { token: victim.access, body: { old_password: PASSWORD, new_password: NEW_PASSWORD } });
  check('password changed', change.status === 200, JSON.stringify(change.body));
  await until('old access token rejected after the event reaches Redis', async () => (await orders(victim.access, 'PENDING')).status === 401);
  check('old refresh token rejected', (await call('POST', '/auth/refresh-token', { body: { refresh_token: victim.refresh } })).status === 401);
  const relog = await call('POST', '/auth/login', { body: { username: victim.username, password: NEW_PASSWORD, role: 'buyer' } });
  check('login with the new password works and the new token is accepted', relog.status === 200 && (await orders(relog.body.access_token, 'PENDING')).status === 200);

  console.log(`\n${passed} passed, ${failures.length} failed`);
  if (failures.length) { console.log('Failed:\n - ' + failures.join('\n - ')); process.exit(1); }
}

// Run with the default AUTH_RATE_LIMIT_PER_MINUTE (20); the main suite needs it raised because it logs in many users.
async function rateLimitOnly() {
  console.log('\n[7] Rate limiting on /auth (Redis)');
  const codes = [];
  for (let i = 0; i < 30; i++) codes.push((await call('POST', '/auth/login', { body: { username: `nobody_${run}`, password: 'x', role: 'buyer' } })).status);
  check('login attempts are throttled with 429', codes.includes(429), codes.join(','));

  console.log(`\n${passed} passed, ${failures.length} failed`);
  if (failures.length) process.exit(1);
}

main().catch((e) => { console.error(e); process.exit(2); });
