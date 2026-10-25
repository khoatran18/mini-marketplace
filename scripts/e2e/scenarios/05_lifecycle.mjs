// Vòng đời đơn: COD CONFIRMED→SHIPPED→DELIVERED, online PAID→SHIPPED→DELIVERED (buyer xác nhận), hủy, chuyển trạng thái sai.
import { get, post, put, del } from '../lib/client.mjs';
import { createProduct, quickOrder, waitOrder, inventoryOf, payByCard, uniq } from '../lib/fixtures.mjs';

export default {
  id: '05', title: 'Vòng đời đơn hàng & hủy đơn',
  async run(t, ctx) {
    const { sellerA, sellerB, buyer, buyer2 } = ctx;
    const p = await createProduct(sellerA, { price: 100000, inventory: 40 });

    t.step('COD: CONFIRMED → SHIPPED → DELIVERED');
    {
      const { res, order } = await quickOrder(buyer, p, { qty: 2, method: 'COD' });
      t.status('đặt COD ×2', res, 201);
      t.eq('PENDING/CONFIRMED sau khi giữ hàng', (await waitOrder(t, 'COD → CONFIRMED', buyer.token, order.id, 'CONFIRMED'))?.status, 'CONFIRMED');
      t.eq('tồn 40 → 38', await inventoryOf(sellerA, p.id), 38);
      t.status('ship thiếu carrier → 400', await post(`/seller/orders/${order.id}/ship`, {}, { token: sellerA.token }), 400);
      t.status('deliver khi chưa ship → 422', await post(`/seller/orders/${order.id}/deliver`, {}, { token: sellerA.token }), 422);
      t.status('buyer confirm-received khi chưa ship → 422', await post(`/orders/${order.id}/confirm-received`, {}, { token: buyer.token }), 422);
      const sh = await post(`/seller/orders/${order.id}/ship`, { carrier: 'GHN', tracking_code: 'E2E' + uniq('') }, { token: sellerA.token });
      t.status('ship', sh, 200);
      t.eq('SHIPPED', sh.body?.status, 'SHIPPED');
      t.check('có carrier/tracking_code/shipped_at', !!sh.body?.carrier && !!sh.body?.tracking_code && !!sh.body?.shipped_at, sh.body);
      t.status('hủy đơn đã SHIPPED → 422', await post(`/orders/${order.id}/cancel`, { reason: 'x' }, { token: buyer.token }), 422);
      const dl = await post(`/seller/orders/${order.id}/deliver`, {}, { token: sellerA.token });
      t.status('deliver', dl, 200);
      t.eq('DELIVERED', dl.body?.status, 'DELIVERED');
      t.eq('COD: payment_status=PAID khi giao', dl.body?.payment_status, 'PAID');
      const hist = (await get(`/orders/${order.id}`, { token: buyer.token })).body?.history ?? [];
      const path = hist.map((h) => h.to_status);
      t.check('history ghi đủ chuyển trạng thái', ['CONFIRMED', 'SHIPPED', 'DELIVERED'].every((s) => path.includes(s)), path);
      t.eq('tồn sau bán vẫn 38 (đã bán, không trả kho)', await inventoryOf(sellerA, p.id), 38);
    }

    t.step('online: PAID → SHIPPED, buyer xác nhận đã nhận');
    {
      const r = await payByCard(t, buyer, p, 1);
      t.status('seller ship đơn PAID', await post(`/seller/orders/${r.id}/ship`, { carrier: 'VTP', tracking_code: 'E2E' + uniq('') }, { token: sellerA.token }), 200);
      const c = await post(`/orders/${r.id}/confirm-received`, {}, { token: buyer.token });
      t.status('buyer confirm-received', c, 200);
      t.eq('DELIVERED', c.body?.status, 'DELIVERED');
    }

    t.step('hủy khi AWAITING_PAYMENT → trả kho');
    {
      const before = await inventoryOf(sellerA, p.id);
      const { res, order } = await quickOrder(buyer, p, { qty: 3, method: 'MOCK_CARD' });
      t.status('đặt MOCK_CARD ×3', res, 201);
      await waitOrder(t, 'AWAITING_PAYMENT', buyer.token, order.id, 'AWAITING_PAYMENT');
      t.eq('tồn bị giữ', await inventoryOf(sellerA, p.id), before - 3);
      t.status('buyer khác hủy → 404', await post(`/orders/${order.id}/cancel`, {}, { token: buyer2.token }), 404);
      t.status('seller khác hủy → 404', await post(`/seller/orders/${order.id}/cancel`, { reason: 'x' }, { token: sellerB.token }), 404);
      const c = await post(`/orders/${order.id}/cancel`, { reason: 'đổi ý' }, { token: buyer.token });
      t.status('buyer hủy', c, 200);
      t.eq('CANCELED', c.body?.status, 'CANCELED');
      t.eq('canceled_by=buyer', c.body?.canceled_by, 'buyer');
      await t.until('tồn được trả lại', async () => (await inventoryOf(sellerA, p.id)) === before);
      t.status('hủy lại đơn đã hủy → 422', await post(`/orders/${order.id}/cancel`, {}, { token: buyer.token }), 422);
    }

    t.step('hủy COD đã CONFIRMED (DELETE alias cũ) & seller hủy');
    {
      const before = await inventoryOf(sellerA, p.id);
      const a = await quickOrder(buyer, p, { method: 'COD' });
      await waitOrder(t, 'CONFIRMED', buyer.token, a.order.id, 'CONFIRMED');
      t.status('DELETE /orders/:id', await del(`/orders/${a.order.id}`, { token: buyer.token }), 200);
      const b = await quickOrder(buyer, p, { method: 'COD' });
      await waitOrder(t, 'CONFIRMED', buyer.token, b.order.id, 'CONFIRMED');
      t.status('seller hủy thiếu reason → 400', await post(`/seller/orders/${b.order.id}/cancel`, {}, { token: sellerA.token }), 400);
      const sc = await post(`/seller/orders/${b.order.id}/cancel`, { reason: 'hết hàng thực tế' }, { token: sellerA.token });
      t.status('seller hủy', sc, 200);
      t.eq('canceled_by=seller', sc.body?.canceled_by, 'seller');
      await t.until('tồn được trả lại sau 2 lần hủy', async () => (await inventoryOf(sellerA, p.id)) === before);
    }

    t.step('hủy đơn đã PAID → hoàn tiền');
    {
      const r = await payByCard(t, buyer, p, 1);
      const c = await post(`/orders/${r.id}/cancel`, { reason: 'không cần nữa' }, { token: buyer.token });
      t.status('hủy đơn PAID', c, 200);
      await t.until('payment được hoàn (REFUNDED)', async () => {
        const pay = (await get(`/payments/${r.payment.id}`, { token: buyer.token })).body;
        return pay?.status === 'REFUNDED' || pay?.refunded_minor > 0;
      }, { timeoutMs: 40000 });
      await t.until('đơn → payment_status REFUNDED', async () => (await get(`/orders/${r.id}`, { token: buyer.token })).body?.payment_status === 'REFUNDED', { timeoutMs: 20000 });
    }

    t.step('chuyển trạng thái / quyền sai');
    {
      const { order } = await quickOrder(buyer, p, { method: 'COD' });
      await waitOrder(t, 'CONFIRMED', buyer.token, order.id, 'CONFIRMED');
      t.status('seller B ship đơn của shop A → 404', await post(`/seller/orders/${order.id}/ship`, { carrier: 'x', tracking_code: 'y' }, { token: sellerB.token }), 404);
      t.status('buyer ship → 403', await post(`/seller/orders/${order.id}/ship`, { carrier: 'x', tracking_code: 'y' }, { token: buyer.token }), 403);
      t.status('return khi chưa DELIVERED → 422', await post(`/orders/${order.id}/return`, { reason: 'x' }, { token: buyer.token }), 422);
      t.status('không có PUT /orders/:id', await put(`/orders/${order.id}`, { status: 'DELIVERED' }, { token: buyer.token }), [404, 405]);
      await post(`/orders/${order.id}/cancel`, {}, { token: buyer.token });
    }
  },
};
