// Thanh toán MÔ PHỎNG: thẻ test, 3-D Secure, ví, chuyển khoản, COD, hủy payment, webhook ký HMAC.
import { createHmac } from 'node:crypto';
import { get, post, sleep } from '../lib/client.mjs';
import { config } from '../lib/config.mjs';
import { createProduct, quickOrder, waitPayment, waitOrder, confirmPayment, getPayment, card, inventoryOf } from '../lib/fixtures.mjs';

// Tạo đơn online và trả { order, checkout, pay }
async function online(t, buyer, product, method) {
  const { res, checkout, order } = await quickOrder(buyer, product, { method });
  t.status(`đặt đơn ${method}`, res, 201);
  const o = await waitOrder(t, 'đơn → AWAITING_PAYMENT', buyer.token, order.id, 'AWAITING_PAYMENT');
  const pay = await waitPayment(t, buyer, checkout.checkout_id);
  return { order: o, id: order.id, checkout, pay };
}

export default {
  id: '04', title: 'Thanh toán mô phỏng (thẻ, ví, chuyển khoản, COD, webhook)',
  async run(t, ctx) {
    const { sellerA, buyer, buyer2 } = ctx;
    const p = await createProduct(sellerA, { price: 100000, inventory: 60 });

    t.step('thẻ 4242 → thành công');
    {
      const { id, pay } = await online(t, buyer, p, 'MOCK_CARD');
      t.eq('payment REQUIRES_ACTION, next_action=enter_card', `${pay?.status}/${pay?.next_action}`, 'REQUIRES_ACTION/enter_card');
      t.eq('provider mockpay, simulated', `${pay?.provider}/${pay?.simulated}`, 'mockpay/true');
      t.eq('amount (VND) = subtotal + ship', pay?.amount, 100000 + 30000);
      t.status('người khác xem payment → 404', await get(`/payments/${pay.id}`, { token: buyer2.token }), 404);
      t.status('thẻ sai định dạng → 400', await confirmPayment(buyer, pay.id, card('1234')), 400);
      t.status('thẻ hết hạn → 400', await confirmPayment(buyer, pay.id, card('4242 4242 4242 4242', { card_exp: '01/20' })), 400);
      const c = await confirmPayment(buyer, pay.id, card('4242 4242 4242 4242'));
      t.status('confirm thẻ 4242', c, 200);
      t.eq('payment SUCCEEDED', c.body?.status, 'SUCCEEDED');
      t.eq('chỉ lưu 4 số cuối', c.body?.card_last4, '4242');
      const o = await waitOrder(t, 'đơn → PAID', buyer.token, id, 'PAID');
      t.eq('payment_status=PAID', o?.payment_status, 'PAID');
      t.status('confirm lại → 422 (đã thanh toán)', await confirmPayment(buyer, pay.id, card('4242 4242 4242 4242')), 422);
      t.status('hủy payment đã thành công → 422', await post(`/payments/${pay.id}/cancel`, {}, { token: buyer.token }), 422);
    }

    t.step('thẻ bị từ chối rồi thanh toán lại');
    {
      const { id, pay } = await online(t, buyer, p, 'MOCK_CARD');
      const d = await confirmPayment(buyer, pay.id, card('4000 0000 0000 0002'));
      t.check('thẻ 0002 bị từ chối (card_declined)', d.status === 200 || d.status === 422, d.body);
      const st = await getPayment(buyer, pay.id);
      t.check('payment chưa SUCCEEDED, failure_code=card_declined', st?.status !== 'SUCCEEDED' && st?.failure_code === 'card_declined', st);
      const f = await confirmPayment(buyer, pay.id, card('4000 0000 0000 9995'));
      t.check('thẻ 9995 → insufficient_funds', (await getPayment(buyer, pay.id))?.failure_code === 'insufficient_funds', f.body);
      const ok = await confirmPayment(buyer, pay.id, card('4242 4242 4242 4242'));
      t.status('thử lại bằng 4242 → 200', ok, 200);
      await waitOrder(t, 'đơn → PAID sau lần thử lại', buyer.token, id, 'PAID');
    }

    t.step('3-D Secure (OTP)');
    {
      const { id, pay } = await online(t, buyer, p, 'MOCK_CARD');
      const a = await confirmPayment(buyer, pay.id, card('4000 0000 0000 3220'));
      t.status('bước 1 3DS', a, 200);
      t.eq('next_action=otp', a.body?.next_action, 'otp');
      const bad = await confirmPayment(buyer, pay.id, card('4000 0000 0000 3220', { otp: '000000' }));
      t.check('OTP sai → otp_incorrect', (await getPayment(buyer, pay.id))?.failure_code === 'otp_incorrect', bad.body);
      const b = await confirmPayment(buyer, pay.id, card('4000 0000 0000 3220', { otp: '123456' }));
      t.status('OTP đúng', b, 200);
      await waitOrder(t, 'đơn → PAID', buyer.token, id, 'PAID');
    }

    t.step('thẻ webhook gửi 2 lần (0259) chỉ trả tiền một lần');
    {
      const { id, pay } = await online(t, buyer, p, 'MOCK_CARD');
      const c = await confirmPayment(buyer, pay.id, card('4000 0000 0000 0259'));
      t.status('confirm 0259', c, 200);
      await waitOrder(t, 'đơn → PAID', buyer.token, id, 'PAID');
      await sleep(5000); // chờ webhook thứ hai
      const st = await getPayment(buyer, pay.id);
      t.eq('payment vẫn SUCCEEDED, không hoàn tiền', `${st?.status}/${st?.refunded_minor ?? 0}`, 'SUCCEEDED/0');
      t.eq('đơn vẫn PAID', (await get(`/orders/${id}`, { token: buyer.token })).body?.status, 'PAID');
    }

    t.step('ví (MOCK_WALLET) – webhook bất đồng bộ');
    {
      const { id, pay } = await online(t, buyer, p, 'MOCK_WALLET');
      t.eq('next_action=approve_in_wallet', pay?.next_action, 'approve_in_wallet');
      const c = await confirmPayment(buyer, pay.id, { approve: true });
      t.status('approve ví', c, 200);
      t.check('PROCESSING (hoặc SUCCEEDED nếu webhook về rất nhanh)', ['PROCESSING', 'SUCCEEDED'].includes(c.body?.status), c.body);
      await t.until('payment SUCCEEDED sau webhook', async () => (await getPayment(buyer, pay.id))?.status === 'SUCCEEDED', { timeoutMs: config.asyncPayWaitMs });
      await waitOrder(t, 'đơn → PAID', buyer.token, id, 'PAID');
    }
    {
      const { id, pay } = await online(t, buyer, p, 'MOCK_WALLET');
      await confirmPayment(buyer, pay.id, { approve: false });
      t.eq('từ chối trong ví → user_cancelled', (await getPayment(buyer, pay.id))?.failure_code, 'user_cancelled');
      t.check('đơn vẫn AWAITING_PAYMENT', (await get(`/orders/${id}`, { token: buyer.token })).body?.status === 'AWAITING_PAYMENT');
    }

    t.step('chuyển khoản (MOCK_BANK_TRANSFER)');
    {
      const { id, pay } = await online(t, buyer, p, 'MOCK_BANK_TRANSFER');
      t.eq('next_action=transfer_and_confirm', pay?.next_action, 'transfer_and_confirm');
      t.check('provider_ref (nội dung CK) bắt đầu bằng MM', String(pay?.provider_ref ?? '').startsWith('MM'), pay?.provider_ref);
      t.status('thiếu approve → 400', await confirmPayment(buyer, pay.id, {}), 400);
      t.status('approve', await confirmPayment(buyer, pay.id, { approve: true }), 200);
      await t.until('payment SUCCEEDED (≈8s)', async () => (await getPayment(buyer, pay.id))?.status === 'SUCCEEDED', { timeoutMs: config.asyncPayWaitMs });
      await waitOrder(t, 'đơn → PAID', buyer.token, id, 'PAID');
    }

    t.step('hủy payment khi REQUIRES_ACTION');
    {
      const { pay } = await online(t, buyer, p, 'MOCK_CARD');
      const c = await post(`/payments/${pay.id}/cancel`, {}, { token: buyer.token });
      t.status('cancel payment', c, 200);
      t.eq('payment CANCELED', (await getPayment(buyer, pay.id))?.status, 'CANCELED');
    }

    t.step('COD không có payment');
    {
      const { res, checkout, order } = await quickOrder(buyer, p, { method: 'COD' });
      t.status('đặt COD', res, 201);
      await waitOrder(t, 'COD → CONFIRMED', buyer.token, order.id, 'CONFIRMED');
      const co = await get(`/checkouts/${checkout.checkout_id}`, { token: buyer.token });
      t.check('checkout COD không có payment', !co.body?.payment, co.body?.payment);
      t.eq('payment_status=UNPAID cho tới khi giao', (await get(`/orders/${order.id}`, { token: buyer.token })).body?.payment_status, 'UNPAID');
    }

    t.step('webhook ký HMAC (cổng nội bộ :8081)');
    if (!config.paymentWebhookUrl || !config.paymentWebhookSecret) {
      t.check('bỏ qua: đặt PAYMENT_WEBHOOK_URL và PAYMENT_WEBHOOK_SECRET (chạy trong mạng swarm)', true);
    } else {
      const { id, pay } = await online(t, buyer, p, 'MOCK_CARD');
      const send = async (secret, tsOffset = 0) => {
        const body = JSON.stringify({ provider_event_id: `e2e-${config.runId}-${pay.id}-${secret.length}-${tsOffset}`, type: 'payment.succeeded', payment_id: pay.id });
        const ts = Math.floor(Date.now() / 1000) + tsOffset;
        const sig = createHmac('sha256', secret).update(`${ts}.${body}`).digest('hex');
        const r = await fetch(config.paymentWebhookUrl, { method: 'POST', body, headers: { 'X-Signature': `t=${ts},v1=${sig}`, 'Content-Type': 'application/json' } });
        return r.status;
      };
      t.eq('chữ ký sai → 401', await send(config.paymentWebhookSecret + 'x'), 401);
      t.eq('lệch giờ > 5 phút → 401', await send(config.paymentWebhookSecret, -3600), 401);
      t.eq('chữ ký đúng → 200', await send(config.paymentWebhookSecret), 200);
      await waitOrder(t, 'webhook payment.succeeded → đơn PAID', buyer.token, id, 'PAID');
    }

    t.check('tồn kho còn nhất quán (không âm)', (await inventoryOf(sellerA, p.id)) >= 0);
  },
};
