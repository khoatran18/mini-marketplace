// Steps used by resilience.sh (run inside the swarm network). State is kept in /state/state.json.
//   STEP=setup   create seller+store+product (stock 10) and a buyer
//   STEP=order   place one order for 1 unit and print its HTTP status
//   STEP=status  print order counts per status and the product inventory
//   STEP=wait    poll until no PENDING orders remain (or time out) and print the final state
import { readFileSync, writeFileSync } from 'node:fs';
const API = process.env.API_URL;
const STATE = '/state/state.json';
const run = Date.now().toString(36);
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function call(method, path, token, body) {
  const res = await fetch(API + path, { method, headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) }, body: body && JSON.stringify(body) });
  let json = null; try { json = await res.json(); } catch {}
  return { status: res.status, body: json };
}
async function user(prefix, role) {
  const username = `${prefix}_${run}`;
  await call('POST', '/auth/register', null, { username, password: 'ResPass_1', role });
  return (await call('POST', '/auth/login', null, { username, password: 'ResPass_1', role })).body.access_token;
}
const load = () => JSON.parse(readFileSync(STATE, 'utf8'));
const count = async (token, status) => ((await call('GET', `/orders?status=${status}`, token)).body?.orders ?? []).length;

async function status(s) {
  const out = {};
  for (const st of ['PENDING', 'SUCCESS', 'FAILED', 'CANCELED']) out[st] = await count(s.buyer, st);
  out.inventory = (await call('GET', `/products/${s.productId}`, s.buyer)).body?.product?.inventory;
  return out;
}

const step = process.env.STEP;
if (step === 'setup') {
  const seller = await user('rsel', 'seller_admin');
  const buyer = await user('rbuy', 'buyer');
  await call('POST', '/users/sellers', seller, { seller: { name: `RShop ${run}`, bank_account: `RA-${run}`, tax_code: 'T', description: 'd', phone: '1', address: 'a', date_of_birth: '1990-01-01T00:00:00Z' } });
  let product;
  for (let i = 0; i < 40 && !product; i++) {
    const r = await call('POST', '/products', seller, { name: `RWidget ${run}`, price: 10, inventory: 10, attributes: {} });
    if (r.status === 200) break;
    await sleep(1000);
  }
  for (let id = 1; id <= 300 && !product; id++) {
    const l = (await call('GET', `/products/seller/${id}`, buyer)).body?.products ?? [];
    product = l.find((p) => p.name === `RWidget ${run}`);
  }
  writeFileSync(STATE, JSON.stringify({ buyer, productId: product.id }));
  console.log(`setup ok product=${product.id}`);
} else if (step === 'order') {
  const s = load();
  const r = await call('POST', '/orders', s.buyer, { order: { order_items: [{ product_id: s.productId, quantity: 1 }] } });
  console.log(`order http=${r.status} ${r.body?.message ?? r.body?.error}`);
} else if (step === 'status') {
  console.log(JSON.stringify(await status(load())));
} else if (step === 'wait') {
  const s = load();
  const end = Date.now() + Number(process.env.WAIT_MS ?? 60000);
  let st;
  while (Date.now() < end) { st = await status(s); if (st.PENDING === 0) break; await sleep(2000); }
  console.log(JSON.stringify(st));
} else { console.error('unknown STEP'); process.exit(2); }
