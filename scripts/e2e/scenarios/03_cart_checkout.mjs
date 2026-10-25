// Địa chỉ, giỏ hàng, preview, POST /orders (Idempotency-Key), tách đơn theo store, tồn kho bị giữ đúng 1 lần.
import { get, post, put, del } from '../lib/client.mjs';
import { createProduct, registerBuyer, placeOrder, idemKey, inventoryOf, waitOrder, ADDRESS } from '../lib/fixtures.mjs';

export default {
  id: '03', title: 'Địa chỉ, giỏ hàng, checkout & idempotency',
  async run(t, ctx) {
    const { sellerA, sellerB, buyer, buyer2 } = ctx;
    const pa = await createProduct(sellerA, { price: 100000, inventory: 30 });
    const pb = await createProduct(sellerB, { price: 50000, inventory: 30 });

    t.step('địa chỉ');
    const addrs = await get('/users/me/addresses', { token: buyer.token });
    t.status('GET addresses', addrs, 200);
    t.eq('địa chỉ đầu tiên là mặc định', addrs.body?.addresses?.[0]?.is_default, true);
    const a2 = await post('/users/me/addresses', { ...ADDRESS, label: 'Cơ quan', is_default: false }, { token: buyer.token });
    t.status('thêm địa chỉ thứ 2', a2, 201);
    t.status('địa chỉ sai phone → 400', await post('/users/me/addresses', { ...ADDRESS, phone: 'abc' }, { token: buyer.token }), 400);
    const upd = await put(`/users/me/addresses/${a2.body?.address?.id}`, { ...ADDRESS, label: 'Cơ quan 2', is_default: false }, { token: buyer.token });
    t.status('sửa địa chỉ', upd, 200);
    t.status('buyer 2 sửa địa chỉ của buyer 1 → 404', await put(`/users/me/addresses/${a2.body?.address?.id}`, ADDRESS, { token: buyer2.token }), 404);
    t.status('xoá địa chỉ phụ', await del(`/users/me/addresses/${a2.body?.address?.id}`, { token: buyer.token }), 200);

    t.step('giỏ hàng');
    t.status('xoá giỏ (reset)', await del('/cart', { token: buyer.token }), 200);
    const s1 = await put(`/cart/items/${pa.id}`, { quantity: 2 }, { token: buyer.token });
    t.status('thêm sản phẩm A ×2', s1, 200);
    t.eq('subtotal = giá live × SL', s1.body?.subtotal, 200000);
    t.status('thêm sản phẩm B ×1', await put(`/cart/items/${pb.id}`, { quantity: 1 }, { token: buyer.token }), 200);
    const cart = await get('/cart', { token: buyer.token });
    t.eq('giỏ có 2 dòng', cart.body?.lines?.length, 2);
    t.eq('item_count = 3', Number(cart.body?.item_count), 3);
    t.status('vượt tồn → 422', await put(`/cart/items/${pa.id}`, { quantity: 99 }, { token: buyer.token }), 422);
    t.status('SL > 99 → 400/422', await put(`/cart/items/${pa.id}`, { quantity: 100 }, { token: buyer.token }), [400, 422]);
    t.status('sản phẩm không tồn tại → 422', await put('/cart/items/999999999', { quantity: 1 }, { token: buyer.token }), [404, 422]);
    t.status('merge giỏ local', await post('/cart/merge', { items: [{ product_id: pa.id, quantity: 1 }] }, { token: buyer.token }), 200);
    t.eq('merge cộng dồn (A = 3)', (await get('/cart', { token: buyer.token })).body?.lines?.find((l) => l.product_id === pa.id)?.quantity, 3);
    t.status('đặt lại A = 2', await put(`/cart/items/${pa.id}`, { quantity: 2 }, { token: buyer.token }), 200);
    t.status('buyer khác thấy giỏ riêng', await get('/cart', { token: buyer2.token }), 200);

    t.step('preview');
    const pv = await post('/checkout/preview', { from_cart: true }, { token: buyer.token });
    t.status('POST /checkout/preview', pv, 200);
    t.eq('tách 2 nhóm theo store', pv.body?.groups?.length, 2);
    t.eq('can_order', pv.body?.can_order, true);
    const sumTotals = (pv.body?.groups ?? []).reduce((s, g) => s + g.total, 0);
    t.eq('grand_total = Σ total của các nhóm', sumTotals, pv.body?.grand_total);
    t.eq('grand_total = subtotal + shipping_fee', pv.body?.grand_total, pv.body?.subtotal + pv.body?.shipping_fee);
    t.eq('preview không giữ hàng', await inventoryOf(sellerA, pa.id), 30);

    t.step('POST /orders: kiểm tra đầu vào');
    t.status('thiếu Idempotency-Key → 400', await post('/orders', { from_cart: true, payment_method: 'COD' }, { token: buyer.token }), 400);
    t.status('Idempotency-Key sai ký tự → 400', await placeOrder(buyer, { fromCart: true, key: 'bad key!!' }), 400);
    t.status('payment_method không hỗ trợ → 400', await placeOrder(buyer, { fromCart: true, method: 'PAYPAL' }), 400);
    t.status('giỏ/items rỗng → 400', await placeOrder(buyer2, { items: [] }), 400);
    t.status('địa chỉ không tồn tại → 404', await placeOrder(buyer, { fromCart: true, addressId: 999999999 }), 404);
    t.status('vượt tồn → 422', await placeOrder(buyer, { items: [{ product_id: pa.id, quantity: 31 }] }), 422);
    const noAddr = await registerBuyer('bn');
    t.status('buyer chưa có địa chỉ → 404', await placeOrder(noAddr, { items: [{ product_id: pa.id, quantity: 1 }] }), 404);
    t.eq('đặt lỗi không làm đổi tồn', await inventoryOf(sellerA, pa.id), 30);

    t.step('POST /orders: từ giỏ, tách đơn theo store, idempotent');
    const key = idemKey('co');
    const o1 = await placeOrder(buyer, { fromCart: true, method: 'COD', key });
    t.status('đặt từ giỏ (COD)', o1, 201);
    t.eq('2 đơn (mỗi store một đơn)', o1.body?.orders?.length, 2);
    t.eq('replayed=false', o1.body?.replayed, false);
    const storeIds = new Set((o1.body?.orders ?? []).map((o) => o.store_id));
    t.eq('2 store_id khác nhau', storeIds.size, 2);
    const oA = o1.body?.orders?.find((o) => o.items?.some((i) => i.product_id === pa.id));
    t.eq('giá/tên là snapshot server (unit_price)', oA?.items?.[0]?.unit_price, 100000);
    t.eq('total = subtotal + ship', oA?.total_price, oA?.subtotal + oA?.shipping_fee);
    t.check('shipping_address được sao chụp vào đơn', !!oA?.shipping_address?.receiver_name, oA?.shipping_address);
    const o2 = await placeOrder(buyer, { fromCart: true, method: 'COD', key });
    t.status('gửi lại cùng key → 200', o2, 200);
    t.eq('replayed=true', o2.body?.replayed, true);
    t.eq('cùng checkout_id', o2.body?.checkout_id, o1.body?.checkout_id);
    t.eq('tồn chỉ bị giữ một lần (30 → 28)', await inventoryOf(sellerA, pa.id), 28);
    t.eq('giỏ đã được xoá các dòng đã đặt', (await get('/cart', { token: buyer.token })).body?.lines?.length ?? 0, 0);
    const co = await get(`/checkouts/${o1.body?.checkout_id}`, { token: buyer.token });
    t.status('GET /checkouts/:id', co, 200);
    t.status('buyer khác xem checkout → 404', await get(`/checkouts/${o1.body?.checkout_id}`, { token: buyer2.token }), 404);
    for (const o of o1.body?.orders ?? []) await waitOrder(t, `đơn ${o.id} COD → CONFIRMED`, buyer.token, o.id, 'CONFIRMED');

    t.step('items trùng dòng được gộp');
    const dup = await placeOrder(buyer, { items: [{ product_id: pb.id, quantity: 1 }, { product_id: pb.id, quantity: 2 }], method: 'COD' });
    t.status('đặt với dòng trùng', dup, 201);
    t.eq('gộp thành SL 3', dup.body?.orders?.[0]?.items?.[0]?.quantity, 3);

    t.step('danh sách đơn của buyer');
    const list = await get('/orders?page_size=100', { token: buyer.token });
    t.status('GET /orders', list, 200);
    t.check('có ≥ 3 đơn', (list.body?.orders ?? []).length >= 3);
    t.status('status sai → 400', await get('/orders?status=NOPE', { token: buyer.token }), 400);
    t.status('GET /orders/summary', await get('/orders/summary', { token: buyer.token }), 200);
  },
};
