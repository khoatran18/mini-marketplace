// Tracking → analytics-service → ClickHouse → báo cáo seller/admin (có chờ flush); phạm vi theo store; health admin.
import { randomUUID } from 'node:crypto';
import { get, post } from '../lib/client.mjs';
import { config } from '../lib/config.mjs';

const ev = (type, extra = {}) => ({
  event_id: randomUUID(), event_type: type, ts_client: new Date().toISOString(),
  anonymous_id: `a_${config.runId}`, session_id: `s_${config.runId}`, surface: 'pdp',
  page: { path: '/products/1' }, device: { type: 'desktop', os: 'Linux' }, consent: { analytics: true }, app_version: 'e2e-0.1', ...extra,
});

export default {
  id: '08', title: 'Tracking /events & báo cáo analytics (+ system health)',
  async run(t, ctx) {
    const { sellerA, sellerB, buyer, admin, productA, productB } = ctx;

    t.step('POST /events');
    const views = Array.from({ length: 6 }, (_, i) => ev('product_view', { session_id: `s_${config.runId}_${i}`, item: { product_id: productA.id, position: i }, page: { path: `/products/${productA.id}` } }));
    const r = await post('/events', { events: views });
    t.status('ẩn danh, 6 event', r, 202);
    t.eq('accepted=6 (hoặc dropped nếu analytics down)', r.body?.accepted ?? -1, 6);
    const mixed = await post('/events', { events: [ev('order_placed'), ev('search', { props: { q: 'áo thun', result_count: 0 } }), ev('add_to_cart', { consent: { analytics: false } })] }, { token: buyer.token });
    t.status('có token buyer + event server-only', mixed, 202);
    t.eq('rejected=2 (server-only + no_consent)', mixed.body?.rejected, 2);
    t.check('lý do từ chối đúng', JSON.stringify(mixed.body?.reasons ?? []).includes('unknown_or_server_only_type') && JSON.stringify(mixed.body?.reasons ?? []).includes('no_consent'), mixed.body?.reasons);
    t.status('events rỗng → 400', await post('/events', { events: [] }), 400);
    t.status('> 50 event → 400', await post('/events', { events: Array.from({ length: 51 }, () => ev('page_view')) }), 400);
    t.status('token sai trên /events → 401', await post('/events', { events: [ev('page_view')] }, { token: 'bad.token.x' }), 401);
    t.status('identify cần đăng nhập → 401', await post('/events/identify', { anonymous_id: 'a_x' }), 401);
    t.status('identify thiếu anonymous_id → 400', await post('/events/identify', {}, { token: buyer.token }), 400);
    t.status('identify OK', await post('/events/identify', { anonymous_id: `a_${config.runId}` }, { token: buyer.token }), [200, 202]);

    t.step('phân quyền báo cáo');
    t.status('buyer vào /seller/analytics/summary → 403', await get('/seller/analytics/summary', { token: buyer.token }), 403);
    t.status('không token → 401', await get('/seller/analytics/summary'), 401);
    t.status('seller vào /admin/analytics/traffic → 403', await get('/admin/analytics/traffic', { token: sellerA.token }), 403);
    t.status('from sai định dạng → 400', await get('/seller/analytics/summary?from=hom-qua', { token: sellerA.token }), 400);
    t.status('khoảng > 400 ngày → 400', await get('/seller/analytics/summary?from=2020-01-01&to=2026-01-01', { token: sellerA.token }), 400);

    t.step(`chờ analytics ghi nhận (tối đa ${config.analyticsWaitMs / 1000}s)`);
    const topOf = async (s) => (await get('/seller/analytics/top-products?limit=100', { token: s.token }));
    const gotViews = await t.until('seller A thấy ≥ 6 lượt xem sản phẩm A', async () => {
      const x = await topOf(sellerA);
      if (x.status === 503) return null;
      return x.body?.data?.find((p) => p.product_id === productA.id && p.views >= 6);
    }, { timeoutMs: config.analyticsWaitMs, everyMs: 3000 });
    if (gotViews) t.check('top-products có name & status', !!gotViews.name);

    t.step('đơn hàng → doanh thu seller (đơn PAID/DELIVERED ở các kịch bản trước)');
    const sumA = await t.until('summary A: orders_placed > 0 & net_revenue > 0', async () => {
      const x = await get('/seller/analytics/summary', { token: sellerA.token });
      const c = x.body?.data?.current;
      return c && c.orders_placed > 0 && c.net_revenue > 0 ? x : null;
    }, { timeoutMs: config.analyticsWaitMs, everyMs: 3000 });
    if (sumA) {
      t.eq('source=clickhouse', sumA.body?.source, 'clickhouse');
      t.eq('timezone', sumA.body?.timezone, 'Asia/Ho_Chi_Minh');
      const c = sumA.body.data.current;
      t.check('net_revenue = revenue − refunds', Math.abs(c.net_revenue - (c.revenue - c.refunds)) < 0.01, c);
    }
    const ts = await get('/seller/analytics/timeseries?granularity=day', { token: sellerA.token });
    t.status('timeseries', ts, 200);
    t.status('funnel', await get('/seller/analytics/funnel', { token: sellerA.token }), 200);
    t.status('low-stock (ClickHouse)', await get('/seller/analytics/low-stock', { token: sellerA.token }), 200);
    t.status('top-products sort=viewed_unsold', await get('/seller/analytics/top-products?sort=viewed_unsold', { token: sellerA.token }), 200);

    t.step('cô lập analytics giữa 2 shop');
    const tb = await topOf(sellerB);
    t.status('top-products của B', tb, 200);
    t.check('B không thấy sản phẩm của A', !(tb.body?.data ?? []).some((p) => p.product_id === productA.id), tb.body?.data);
    const sb = await get('/seller/analytics/summary', { token: sellerB.token });
    t.eq('B chưa có đơn PAID/DELIVERED → doanh thu 0', sb.body?.data?.current?.revenue, 0);
    t.status('seller_id/store_id từ client bị bỏ qua', await get(`/seller/analytics/summary?store_id=${productA.id}`, { token: sellerB.token }), 200);

    t.step('báo cáo admin + system health');
    if (!admin) { t.check('bỏ qua: không có admin', true); return; }
    for (const rep of ['summary', 'timeseries', 'top-products', 'funnel', 'low-stock', 'traffic', 'payments', 'search-terms', 'data-health']) {
      t.status(`/admin/analytics/${rep}`, await get(`/admin/analytics/${rep}`, { token: admin.token }), 200);
    }
    const tr = await t.until('admin traffic: page_views > 0', async () => {
      const x = await get('/admin/analytics/traffic', { token: admin.token });
      return x.body?.data?.page_views > 0 || x.body?.data?.sessions > 0 ? x : null;
    }, { timeoutMs: config.analyticsWaitMs, everyMs: 3000 });
    const dh = await get('/admin/analytics/data-health', { token: admin.token });
    t.check('data-health liệt kê bảng', Array.isArray(dh.body?.data?.tables), dh.body);
    const pay = await get('/admin/analytics/payments', { token: admin.token });
    t.check('payments report có succeeded', typeof pay.body?.data?.succeeded === 'number', pay.body);

    const h = await get('/admin/system/health', { token: admin.token });
    t.status('GET /admin/system/health', h, 200);
    t.check('status hợp lệ', ['ready', 'degraded', 'not_ready', 'unreachable'].includes(h.body?.status), h.body?.status);
    const names = (h.body?.services ?? []).map((s) => s.name);
    t.check('liệt kê ≥ 7 service', names.length >= 7, names);
    t.eq('toàn bộ service ready', h.body?.status, 'ready');
    void tr; void productB;
  },
};
