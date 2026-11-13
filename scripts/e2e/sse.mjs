#!/usr/bin/env node
// Kiểm luồng POST /ai/chat (SSE) đi qua Traefik -> gateway -> upstream giả (ADR-40). Chạy bởi scripts/e2e/sse.sh.
// Đo THỜI ĐIỂM từng khối dữ liệu tới client để chứng minh không có buffering, và luồng dài hơn 60 s không bị cắt.
//   SSE_SECONDS  tổng thời gian luồng (mặc định 70: vượt mốc 60 s hay gặp của proxy)
//   SSE_EVERY    giây giữa hai sự kiện (mặc định 5)
import { config } from './lib/config.mjs';
import { loginAdmin } from './lib/fixtures.mjs';

const base = config.api;
const total = Number(process.env.SSE_SECONDS ?? 70);
const every = Number(process.env.SSE_EVERY ?? 5);
const res = [];
const check = (name, ok, extra = '') => { res.push(ok); console.log(`  ${ok ? 'ok  ' : 'FAIL'} ${name}${extra ? ' — ' + extra : ''}`); };

const feat = await (await fetch(`${base}/config/features`)).json();
check('GET /config/features công khai và báo ai_chat=true', feat?.features?.ai_chat === true, JSON.stringify(feat));

const anon = await fetch(`${base}/ai/chat`, { method: 'POST', body: '{}' });
check('POST /ai/chat không token -> 401', anon.status === 401, String(anon.status));

const a = await loginAdmin();
if (!a) { console.log('  FAIL cần ADMIN_USERNAME/ADMIN_PASSWORD'); process.exit(1); }
const t0 = performance.now();
const r = await fetch(`${base}/ai/chat`, { method: 'POST', headers: { Authorization: `Bearer ${a.token}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ seconds: total, every }) });
check('luồng trả 200 text/event-stream', r.status === 200 && (r.headers.get('content-type') ?? '').startsWith('text/event-stream'), `${r.status} ${r.headers.get('content-type')}`);
const times = [];
let text = '';
const rd = r.body.getReader();
const dec = new TextDecoder();
for (;;) {
  const { value, done } = await rd.read();
  if (done) break;
  times.push((performance.now() - t0) / 1000);
  text += dec.decode(value, { stream: true });
}
const events = (text.match(/^data: /gm) ?? []).length;
const want = Math.floor(total / every);
check(`nhận đủ ${want} sự kiện`, events >= want, `được ${events}`);
check('sự kiện đầu tới trong 3 s (không buffer)', times[0] < 3, `${times[0]?.toFixed(2)} s`);
check('các khối tới rải đều (nhiều lần đọc, không dồn một cục)', times.length >= want, `${times.length} khối`);
// sự kiện thứ k (đếm từ 0) tới ở giây k*every; sự kiện cuối ở (want-1)*every, sau đó upstream đóng luồng sạch
check(`luồng kéo dài >= ${(want - 1) * every - 2} s (vượt mốc 60 s) rồi kết thúc sạch`, times.at(-1) >= (want - 1) * every - 2, `${times.at(-1)?.toFixed(1)} s`);
check('upstream nhận danh tính từ JWT (role admin)', /"role":"admin"/.test(text), text.slice(0, 160).replace(/\n/g, ' '));

// Đóng giữa chừng: upstream phải bị huỷ (sse.sh kiểm log của stub)
const ctl = new AbortController();
const r2 = await fetch(`${base}/ai/chat`, { method: 'POST', headers: { Authorization: `Bearer ${a.token}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ seconds: 60, every: 1 }), signal: ctl.signal });
await r2.body.getReader().read();
ctl.abort();
console.log('  (đã ngắt giữa chừng; sse.sh kiểm upstream bị huỷ)');
process.exit(res.every(Boolean) ? 0 : 1);
