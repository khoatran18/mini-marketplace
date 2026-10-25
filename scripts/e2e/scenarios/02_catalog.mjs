// Catalog: tạo/sửa sản phẩm, tìm kiếm, che số tồn, quyền sở hữu, điều chỉnh tồn kho + ledger.
import { get, post, put, patch, sleep } from '../lib/client.mjs';
import { createProduct, getProduct } from '../lib/fixtures.mjs';

export default {
  id: '02', title: 'Catalog, search, tồn kho',
  async run(t, ctx) {
    const { sellerA, sellerB, buyer } = ctx;

    t.step('công khai');
    const cats = await get('/categories');
    t.status('GET /categories', cats, 200);
    t.check('categories là mảng', Array.isArray(cats.body?.categories));
    const pm = await get('/payments/methods');
    t.status('GET /payments/methods', pm, 200);
    t.check('có 4 phương thức (COD + 3 MOCK)', (pm.body?.methods ?? []).length === 4, pm.body);

    t.step('xem sản phẩm');
    const p = await createProduct(sellerA, { name: `E2E Giay the thao ${ctx.productA.id}`, price: 250000, inventory: 50 });
    t.status('seller A tạo sản phẩm tồn 50', p.res, 200);
    const pub = await getProduct(undefined, p.id);
    t.eq('khách xem được sản phẩm active', pub?.status, 'active');
    t.eq('giá', pub?.price, 250000);
    t.eq('tồn bị che ở mức 20 với khách', pub?.inventory, 20);
    t.eq('stock_level ok', pub?.stock_level, 'ok');
    const own = await getProduct(sellerA.token, p.id);
    t.eq('chủ shop thấy tồn thật (50)', own?.inventory, 50);
    const st = await get(`/products/${p.id}/stock`);
    t.eq('GET /products/:id/stock level', st.body?.level, 'ok');
    t.status('sản phẩm không tồn tại → 404', await get('/products/999999999'), 404);

    t.step('tìm kiếm');
    let found = null;
    for (let i = 0; i < 10 && !found; i++) { // chỉ mục tìm kiếm cập nhật gần như tức thì, vẫn cho thử lại vài lần
      const s = await get(`/search?q=${encodeURIComponent('E2E Giay the thao')}&page_size=20`);
      found = s.body?.products?.find((x) => x.id === p.id);
      if (!found) await sleep(700);
    }
    t.check('tìm thấy sản phẩm theo tên', !!found);
    const sf = await get(`/search?seller_id=${pub?.seller_id}&sort=price_desc&in_stock=true`);
    t.status('search theo seller_id + sort + in_stock', sf, 200);
    t.status('search page_size lớn bị kẹp (không lỗi)', await get('/search?page_size=9999'), 200);
    t.status('GET /products phân trang', await get('/products?page=1&page_size=5'), 200);

    t.step('trạng thái sản phẩm');
    const d = await createProduct(sellerA, { name: 'E2E ban nhap', inventory: 5, status: 'draft' });
    t.status('khách không xem được draft → 404', await get(`/products/${d.id}`), 404);
    t.status('chủ shop xem được draft', await get(`/products/${d.id}`, { token: sellerA.token }), 200);
    t.status('PATCH status → active', await patch(`/products/${d.id}/status`, { status: 'active' }, { token: sellerA.token }), 200);
    t.status('khách thấy sau khi active', await get(`/products/${d.id}`), 200);
    t.status('seller không được đặt banned → 403', await patch(`/products/${d.id}/status`, { status: 'banned' }, { token: sellerA.token }), [400, 403]);

    t.step('quyền sở hữu');
    t.status('seller B sửa sản phẩm của A → 403', await put(`/products/${p.id}`, { product: { name: 'hack', price: 1, inventory: 1 } }, { token: sellerB.token }), [403, 404]);
    t.status('seller B đổi trạng thái sản phẩm A → 403', await patch(`/products/${p.id}/status`, { status: 'hidden' }, { token: sellerB.token }), [403, 404]);
    t.status('seller B chỉnh tồn sản phẩm A → 403', await post(`/seller/products/${p.id}/inventory/adjust`, { delta: 5, reason: 'hack' }, { token: sellerB.token }), [403, 404]);
    t.status('tạo sản phẩm giá âm → 400', await post('/products', { name: 'x', price: -1, inventory: 1 }, { token: sellerA.token }), 400);
    t.status('buyer sửa sản phẩm → 403', await put(`/products/${p.id}`, { product: { name: 'hack' } }, { token: buyer.token }), 403);

    t.step('điều chỉnh tồn kho + ledger');
    const adj = await post(`/seller/products/${p.id}/inventory/adjust`, { delta: 10, reason: 'e2e restock' }, { token: sellerA.token });
    t.status('adjust +10', adj, 200);
    t.eq('tồn sau adjust', (await getProduct(sellerA.token, p.id))?.inventory, 60);
    t.status('adjust làm tồn âm → 422', await post(`/seller/products/${p.id}/inventory/adjust`, { delta: -1000, reason: 'x' }, { token: sellerA.token }), 422);
    t.status('adjust thiếu reason → 400', await post(`/seller/products/${p.id}/inventory/adjust`, { delta: 1 }, { token: sellerA.token }), 400);
    const led = await get(`/seller/products/${p.id}/inventory/ledger`, { token: sellerA.token });
    t.status('ledger', led, 200);
    const reasons = (led.body?.entries ?? []).map((e) => e.reason);
    t.check('ledger có initial + restock', reasons.includes('initial') && reasons.includes('restock'), reasons);
    t.status('low-stock (Postgres)', await get('/seller/inventory/low-stock?threshold=5', { token: sellerA.token }), 200);
    const mine = await get('/seller/products', { token: sellerA.token });
    t.check('GET /seller/products chỉ chứa sản phẩm của shop mình', mine.status === 200 && (mine.body?.products ?? []).every((x) => x.id !== ctx.productB.id));
  },
};
