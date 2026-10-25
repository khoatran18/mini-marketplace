// Trả hàng / hoàn tiền: yêu cầu → seller duyệt (REFUNDED) hoặc từ chối (DELIVERED, chỉ một lần), admin hoàn tiền thủ công.
import { get, post } from '../lib/client.mjs';
import { createProduct, deliverCod, payByCard, uniq } from '../lib/fixtures.mjs';

async function deliverOnline(t, buyer, seller, p) {
  const r = await payByCard(t, buyer, p, 1);
  t.status('ship', await post(`/seller/orders/${r.id}/ship`, { carrier: 'GHN', tracking_code: 'E2E' + uniq('') }, { token: seller.token }), 200);
  t.status('buyer xác nhận đã nhận', await post(`/orders/${r.id}/confirm-received`, {}, { token: buyer.token }), 200);
  return r;
}

export default {
  id: '06', title: 'Trả hàng & hoàn tiền',
  async run(t, ctx) {
    const { sellerA, sellerB, buyer, buyer2, admin } = ctx;
    const p = await createProduct(sellerA, { price: 100000, inventory: 40 });

    t.step('đơn online: yêu cầu trả → seller duyệt → hoàn tiền');
    {
      const r = await deliverOnline(t, buyer, sellerA, p);
      t.status('return thiếu reason → 400', await post(`/orders/${r.id}/return`, {}, { token: buyer.token }), 400);
      t.status('buyer khác yêu cầu trả → 404', await post(`/orders/${r.id}/return`, { reason: 'x' }, { token: buyer2.token }), 404);
      const rq = await post(`/orders/${r.id}/return`, { reason: 'Sản phẩm lỗi' }, { token: buyer.token });
      t.status('buyer yêu cầu trả', rq, 200);
      t.eq('REFUND_REQUESTED', rq.body?.status, 'REFUND_REQUESTED');
      t.eq('return_reason được lưu', rq.body?.return_reason, 'Sản phẩm lỗi');
      t.status('seller B duyệt đơn shop A → 404', await post(`/seller/orders/${r.id}/return/approve`, {}, { token: sellerB.token }), 404);
      const ap = await post(`/seller/orders/${r.id}/return/approve`, {}, { token: sellerA.token });
      t.status('seller duyệt', ap, 200);
      t.eq('REFUNDED', ap.body?.status, 'REFUNDED');
      await t.until('payment được hoàn tiền', async () => {
        const pay = (await get(`/payments/${r.payment.id}`, { token: buyer.token })).body;
        return pay?.status === 'REFUNDED' && pay?.refunded === pay?.amount;
      }, { timeoutMs: 40000 });
      t.status('return lần 2 trên đơn đã hoàn → 422', await post(`/orders/${r.id}/return`, { reason: 'x' }, { token: buyer.token }), 422);
    }

    t.step('đơn COD: yêu cầu trả → seller từ chối → không trả lại được nữa');
    {
      const d = await deliverCod(t, buyer, sellerA, p);
      t.status('buyer yêu cầu trả', await post(`/orders/${d.id}/return`, { reason: 'Không vừa' }, { token: buyer.token }), 200);
      t.status('từ chối thiếu reason → 400', await post(`/seller/orders/${d.id}/return/reject`, {}, { token: sellerA.token }), 400);
      const rj = await post(`/seller/orders/${d.id}/return/reject`, { reason: 'Đã qua sử dụng' }, { token: sellerA.token });
      t.status('seller từ chối', rj, 200);
      t.eq('quay lại DELIVERED', rj.body?.status, 'DELIVERED');
      t.status('yêu cầu trả lần 2 → 422', await post(`/orders/${d.id}/return`, { reason: 'lại' }, { token: buyer.token }), 422);
    }

    t.step('đơn COD: duyệt trả hàng (không có payment online để hoàn)');
    {
      const d = await deliverCod(t, buyer, sellerA, p);
      await post(`/orders/${d.id}/return`, { reason: 'Giao sai màu' }, { token: buyer.token });
      const ap = await post(`/seller/orders/${d.id}/return/approve`, {}, { token: sellerA.token });
      t.status('seller duyệt', ap, 200);
      t.eq('REFUNDED', ap.body?.status, 'REFUNDED');
    }

    t.step('admin: duyệt trả hàng, hoàn tiền thủ công');
    if (!admin) { t.check('bỏ qua: không có admin', true); return; }
    {
      const r = await deliverOnline(t, buyer, sellerA, p);
      await post(`/orders/${r.id}/return`, { reason: 'Lỗi' }, { token: buyer.token });
      t.status('admin duyệt trả', await post(`/admin/orders/${r.id}/return/approve`, {}, { token: admin.token }), 200);
    }
    {
      const r = await payByCard(t, buyer, p, 1);
      const pay = (await get(`/admin/payments/${r.payment.id}`, { token: admin.token })).body;
      t.check('admin thấy attempts[]', Array.isArray(pay?.attempts) && pay.attempts.length > 0, pay);
      t.status('hoàn vượt số đã thu → 422', await post(`/admin/payments/${r.payment.id}/refund`, { amount_minor: pay.amount_minor + 1, reason: 'x' }, { token: admin.token }), 422);
      const part = await post(`/admin/payments/${r.payment.id}/refund`, { amount_minor: 1000, reason: 'e2e hoàn một phần' }, { token: admin.token });
      t.status('hoàn một phần', part, 200);
      t.eq('PARTIALLY_REFUNDED', part.body?.status, 'PARTIALLY_REFUNDED');
      t.status('buyer không gọi được refund admin → 403', await post(`/admin/payments/${r.payment.id}/refund`, { amount_minor: 1, reason: 'x' }, { token: buyer.token }), 403);
      t.status('admin liệt kê payments', await get('/admin/payments?status=PARTIALLY_REFUNDED', { token: admin.token }), 200);
    }
  },
};
