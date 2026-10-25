// (Tuỳ chọn, RATE_LIMIT_TEST=1) Rate limit /auth/* theo IP. Chạy CUỐI CÙNG vì làm IP bị chặn tạm thời.
import { call } from '../lib/client.mjs';
import { config } from '../lib/config.mjs';

export default {
  id: '10', title: 'Rate limit /auth (429)',
  async run(t) {
    if (!config.rateLimitTest) t.skip('đặt RATE_LIMIT_TEST=1 (AUTH_RATE_LIMIT_PER_MINUTE=20)');
    let n429 = 0;
    for (let i = 0; i < 40; i++) {
      const r = await call('POST', '/auth/login', { body: { username: 'nobody_x', password: 'WrongPass_1', role: 'buyer' }, retry429: 0 });
      if (r.status === 429) { n429++; t.check('body 429 đúng định dạng', r.body?.error === 'Too many requests', r.body); break; }
    }
    t.check('bị chặn 429 sau vượt hạn mức', n429 > 0);
  },
};
