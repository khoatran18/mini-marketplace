// Xác thực: đăng ký/đăng nhập, refresh, đổi mật khẩu, phân quyền cơ bản theo role.
import { post, get } from '../lib/client.mjs';
import { registerBuyer, login, uniq } from '../lib/fixtures.mjs';
import { config } from '../lib/config.mjs';

export default {
  id: '01', title: 'Auth & RBAC',
  async run(t, ctx) {
    t.step('đăng nhập sai / đăng ký sai');
    const bad = await post('/auth/login', { username: ctx.buyer.username, password: 'WrongPass_1', role: 'buyer' });
    t.status('sai mật khẩu → 401', bad, 401);
    t.status('đăng ký role admin → 400', await post('/auth/register', { username: uniq('x'), password: config.password, role: 'admin' }), 400);
    t.status('đăng ký seller_employee → 403', await post('/auth/register', { username: uniq('x'), password: config.password, role: 'seller_employee' }), 403);
    t.status('mật khẩu < 8 ký tự → 400', await post('/auth/register', { username: uniq('x'), password: 'abc', role: 'buyer' }), 400);
    t.status('đăng ký trùng → 409', await post('/auth/register', { username: ctx.buyer.username, password: config.password, role: 'buyer' }), 409);

    t.step('token');
    t.status('không token → 401', await get('/orders'), 401);
    t.status('token rác → 401', await get('/orders', { token: 'abc.def.ghi' }), 401);
    const rf = await post('/auth/refresh-token', { refresh_token: ctx.buyer.refresh });
    t.status('refresh token → 200', rf, 200);
    t.check('refresh trả access_token mới', !!rf.body?.access_token);
    t.status('dùng refresh token làm access → 401', await get('/orders', { token: ctx.buyer.refresh }), 401);
    t.status('dùng access token để refresh → 401', await post('/auth/refresh-token', { refresh_token: ctx.buyer.token }), 401);

    t.step('RBAC');
    t.status('buyer vào /seller/products → 403', await get('/seller/products', { token: ctx.buyer.token }), 403);
    t.status('seller vào /orders (buyer) → 403', await get('/orders', { token: ctx.sellerA.token }), 403);
    t.status('buyer vào /admin/orders → 403', await get('/admin/orders', { token: ctx.buyer.token }), 403);
    t.status('seller vào /admin/system/health → 403', await get('/admin/system/health', { token: ctx.sellerA.token }), 403);
    t.status('buyer POST /products → 403', await post('/products', { name: 'x', price: 1, inventory: 1 }, { token: ctx.buyer.token }), 403);

    t.step('đổi mật khẩu vô hiệu hoá token cũ');
    const u = await registerBuyer('bp');
    const ch = await post('/auth/change-password', { old_password: config.password, new_password: 'NewPass_12' }, { token: u.token });
    t.status('change-password', ch, 200);
    // thu hồi token đi qua outbox -> Kafka -> gateway (bất đồng bộ): chờ tối đa 15 s
    let rev = await get('/orders', { token: u.token });
    for (let i = 0; i < 30 && rev.status !== 401; i++) { await new Promise((r) => setTimeout(r, 500)); rev = await get('/orders', { token: u.token }); }
    t.status('token cũ bị thu hồi → 401', rev, 401);
    const again = await login(u.username, 'buyer', 'NewPass_12');
    t.check('đăng nhập bằng mật khẩu mới', !!again.token);
    t.status('mật khẩu cũ không còn dùng được', await post('/auth/login', { username: u.username, password: config.password, role: 'buyer' }, { retry429: 6 }), 401);
  },
};
