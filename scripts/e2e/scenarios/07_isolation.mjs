// Cô lập dữ liệu: seller/buyer này không thấy hay thao tác được dữ liệu của người khác.
import { get, post } from '../lib/client.mjs';
import { createProduct, quickOrder, waitOrder } from '../lib/fixtures.mjs';

export default {
  id: '07', title: 'Cô lập chéo shop / buyer',
  async run(t, ctx) {
    const { sellerA, sellerB, buyer, buyer2, admin } = ctx;
    const pa = await createProduct(sellerA, { price: 100000, inventory: 20 });
    const pb = await createProduct(sellerB, { price: 100000, inventory: 20 });
    const a = await quickOrder(buyer, pa, { method: 'COD' });
    const b = await quickOrder(buyer, pb, { method: 'COD' });
    t.status('đơn shop A', a.res, 201);
    t.status('đơn shop B', b.res, 201);
    await waitOrder(t, 'đơn A CONFIRMED', buyer.token, a.order.id, 'CONFIRMED');
    await waitOrder(t, 'đơn B CONFIRMED', buyer.token, b.order.id, 'CONFIRMED');

    t.step('seller chỉ thấy đơn của store mình');
    const la = await get('/seller/orders?page_size=100', { token: sellerA.token });
    const lb = await get('/seller/orders?page_size=100', { token: sellerB.token });
    t.status('GET /seller/orders (A)', la, 200);
    const idsA = (la.body?.orders ?? []).map((o) => o.id), idsB = (lb.body?.orders ?? []).map((o) => o.id);
    t.check('A thấy đơn A, không thấy đơn B', idsA.includes(a.order.id) && !idsA.includes(b.order.id), idsA);
    t.check('B thấy đơn B, không thấy đơn A', idsB.includes(b.order.id) && !idsB.includes(a.order.id), idsB);
    t.check('mọi đơn trong list A thuộc store của A', (la.body?.orders ?? []).every((o) => o.store_id === a.order.store_id));
    t.status('B đọc đơn A → 404', await get(`/seller/orders/${a.order.id}`, { token: sellerB.token }), 404);
    t.status('A đọc đơn A → 200', await get(`/seller/orders/${a.order.id}`, { token: sellerA.token }), 200);
    t.status('B deliver đơn A → 404', await post(`/seller/orders/${a.order.id}/deliver`, {}, { token: sellerB.token }), 404);
    t.status('seller xem summary riêng', await get('/seller/orders/summary', { token: sellerB.token }), 200);

    t.step('buyer chỉ thấy đơn / payment của mình');
    t.status('buyer 2 đọc đơn buyer 1 → 404', await get(`/orders/${a.order.id}`, { token: buyer2.token }), 404);
    const l2 = await get(`/orders?page_size=100&buyer_id=${buyer.id}`, { token: buyer2.token });
    t.check('buyer_id trong query bị bỏ qua', (l2.body?.orders ?? []).every((o) => o.id !== a.order.id));
    t.status('buyer 2 hủy đơn buyer 1 → 404', await post(`/orders/${a.order.id}/cancel`, {}, { token: buyer2.token }), 404);
    t.status('buyer 2 xem checkout → 404', await get(`/checkouts/${a.checkout.checkout_id}`, { token: buyer2.token }), 404);
    t.status('buyer 2 đọc hồ sơ buyer 1 → 403', await get(`/users/buyers/${buyer.id}`, { token: buyer2.token }), 403);

    t.step('bỏ qua định danh/giá/trạng thái do client gửi');
    const spoof = await post('/orders', {
      items: [{ product_id: pa.id, quantity: 1, unit_price: 1 }], payment_method: 'COD', buyer_id: buyer2.id, status: 'PAID', total_price: 1, store_id: 999,
    }, { token: buyer.token, headers: { 'Idempotency-Key': `spoof-${Date.now()}` } });
    t.status('đặt với trường giả mạo', spoof, 201);
    const so = spoof.body?.orders?.[0];
    t.check('buyer_id lấy từ JWT', so?.buyer_id === buyer.id, so);
    t.check('giá do server tính (≥ 100000)', so?.subtotal === 100000, so?.subtotal);
    t.check('status không phải PAID', so?.status !== 'PAID');
    t.check('store_id lấy từ sản phẩm', so?.store_id === a.order.store_id);

    t.step('admin thấy tất cả');
    if (!admin) { t.check('bỏ qua: không có admin', true); }
    else {
      const all = await get('/admin/orders?page_size=100', { token: admin.token });
      t.status('GET /admin/orders', all, 200);
      t.status('admin đọc đơn bất kỳ', await get(`/admin/orders/${b.order.id}`, { token: admin.token }), 200);
      t.status('seller vào /admin/orders → 403', await get('/admin/orders', { token: sellerA.token }), 403);
    }
  },
};
