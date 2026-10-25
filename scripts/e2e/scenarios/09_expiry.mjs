// (Tuỳ chọn) Hết hạn giữ hàng: đơn online không thanh toán → EXPIRED và trả kho. Cần ORDER_PAYMENT_TTL_MIN nhỏ (vd 1)
// và E2E_EXPIRY_TTL_MIN=<số phút đó>. Cũng kiểm tra công cụ force payment khi E2E_DEV_MODE=1 (ENV=dev).
import { get, post } from '../lib/client.mjs';
import { config } from '../lib/config.mjs';
import { createProduct, quickOrder, waitOrder, waitPayment, inventoryOf, getPayment } from '../lib/fixtures.mjs';

export default {
  id: '09', title: 'Hết hạn giữ hàng & force payment (dev)',
  async run(t, ctx) {
    const { sellerA, buyer, admin } = ctx;
    if (!config.expiryTtlMin && !config.devMode) t.skip('đặt E2E_EXPIRY_TTL_MIN (TTL phút của ORDER_PAYMENT_TTL_MIN) và/hoặc E2E_DEV_MODE=1');
    const p = await createProduct(sellerA, { price: 100000, inventory: 10 });

    if (config.devMode) {
      t.step('force payment (ENV=dev)');
      if (!admin) t.check('bỏ qua: không có admin', true);
      else {
        const { res, checkout, order } = await quickOrder(buyer, p, { method: 'MOCK_CARD' });
        t.status('đặt MOCK_CARD', res, 201);
        const pay = await waitPayment(t, buyer, checkout.checkout_id);
        const f = await post(`/admin/dev/payments/${pay.id}/force`, { status: 'SUCCEEDED' }, { token: admin.token });
        t.status('force SUCCEEDED', f, 200);
        await waitOrder(t, 'đơn → PAID', buyer.token, order.id, 'PAID');
        const q = await quickOrder(buyer, p, { method: 'MOCK_CARD' });
        const pay2 = await waitPayment(t, buyer, q.checkout.checkout_id);
        t.status('force FAILED', await post(`/admin/dev/payments/${pay2.id}/force`, { status: 'FAILED' }, { token: admin.token }), 200);
        t.eq('payment FAILED (kết thúc)', (await getPayment(buyer, pay2.id))?.status, 'FAILED');
      }
    }

    if (config.expiryTtlMin) {
      t.step(`đơn không thanh toán hết hạn sau ~${config.expiryTtlMin} phút`);
      const before = await inventoryOf(sellerA, p.id);
      const { res, order } = await quickOrder(buyer, p, { qty: 4, method: 'MOCK_CARD' });
      t.status('đặt MOCK_CARD ×4', res, 201);
      await waitOrder(t, 'AWAITING_PAYMENT', buyer.token, order.id, 'AWAITING_PAYMENT');
      t.eq('tồn đang bị giữ', await inventoryOf(sellerA, p.id), before - 4);
      t.check('expires_at được đặt', !!(await get(`/orders/${order.id}`, { token: buyer.token })).body?.expires_at);
      const wait = config.expiryTtlMin * 60_000 + 90_000; // TTL + biên cho worker quét
      console.log(`    (chờ tối đa ${Math.round(wait / 1000)}s)`);
      await waitOrder(t, 'đơn → EXPIRED', buyer.token, order.id, 'EXPIRED', wait);
      await t.until('tồn được trả lại', async () => (await inventoryOf(sellerA, p.id)) === before, { timeoutMs: 30000 });
    }
  },
};
