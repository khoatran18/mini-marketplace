// Dựng dữ liệu nền: 2 shop (A, B), 2 buyer, admin (nếu có), sản phẩm, địa chỉ. Các kịch bản sau dùng ctx này.
import { registerBuyer, registerSeller, loginAdmin, createProduct, ensureAddress, categoryId } from '../lib/fixtures.mjs';
import { get } from '../lib/client.mjs';
import { config } from '../lib/config.mjs';

export default {
  id: '00', title: 'Setup: tài khoản, cửa hàng, sản phẩm, địa chỉ', critical: true,
  async run(t, ctx) {
    t.step('gateway sống');
    const h = await get('/healthz');
    t.status('GET /healthz', h, 200);
    if (h.status !== 200) throw new Error(`gateway không truy cập được tại ${config.api} (${h.error ?? h.status})`);

    t.step('admin (bootstrap qua env)');
    ctx.admin = await loginAdmin();
    t.check(ctx.admin ? 'đăng nhập admin' : 'KHÔNG có ADMIN_USERNAME/ADMIN_PASSWORD: bỏ qua các bước admin', true);

    t.step('seller A, seller B (mỗi người một store)');
    ctx.sellerA = await registerSeller('sa');
    ctx.sellerB = await registerSeller('sb');
    for (const [n, s] of [['A', ctx.sellerA], ['B', ctx.sellerB]]) {
      t.check(`seller ${n} đăng nhập`, !!s.token, s.res?.body);
      t.status(`seller ${n} tạo store`, s.storeRes, 200);
      t.check(`seller ${n} đã có store_id (Kafka user.create_seller)`, !!s.hasStore);
    }

    t.step('buyer 1, buyer 2');
    ctx.buyer = await registerBuyer('ba');
    ctx.buyer2 = await registerBuyer('bb');
    t.check('buyer 1 đăng nhập', !!ctx.buyer.token, ctx.buyer.res?.body);
    t.check('buyer 2 đăng nhập', !!ctx.buyer2.token, ctx.buyer2.res?.body);

    t.step('danh mục + sản phẩm');
    const cid = await categoryId(ctx.admin);
    t.check(cid ? `dùng category_id=${cid}` : 'chưa có danh mục (tạo sản phẩm không category)', true);
    ctx.productA = await createProduct(ctx.sellerA, { name: `E2E Ao thun A ${config.runId}`, price: 120000, inventory: 100 });
    ctx.productB = await createProduct(ctx.sellerB, { name: `E2E Ao thun B ${config.runId}`, price: 90000, inventory: 100 });
    t.status('tạo sản phẩm A', ctx.productA.res, 200);
    t.status('tạo sản phẩm B', ctx.productB.res, 200);
    t.check('có id sản phẩm', !!ctx.productA.id && !!ctx.productB.id);

    t.step('địa chỉ giao hàng');
    for (const b of [ctx.buyer, ctx.buyer2]) {
      const a = await ensureAddress(b);
      t.status('POST /users/me/addresses', a, 201);
      b.addressId = a.body?.address?.id;
    }
  },
};
