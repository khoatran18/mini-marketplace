#!/usr/bin/env node
// Runner e2e: chạy các kịch bản trong scenarios/ lần lượt trên stack thật qua gateway.
//   API_URL=http://localhost:8080 ADMIN_USERNAME=admin ADMIN_PASSWORD=... node scripts/e2e/run.mjs [--list] [--only 03,04] [--skip 09]
// Mã thoát: 0 = tất cả đạt; 1 = có lỗi; 2 = tham số sai.
import { readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { config } from './lib/config.mjs';
import { runAll } from './lib/testkit.mjs';

const dir = join(dirname(fileURLToPath(import.meta.url)), 'scenarios');
const args = process.argv.slice(2);
const opt = (name) => { const i = args.indexOf(name); return i >= 0 ? (args[i + 1] ?? '').split(',').filter(Boolean) : null; };
const only = opt('--only'), skip = opt('--skip') ?? [];

const scenarios = [];
for (const f of readdirSync(dir).filter((x) => x.endsWith('.mjs')).sort()) {
  scenarios.push((await import(join(dir, f))).default);
}
if (args.includes('--list')) { scenarios.forEach((s) => console.log(`${s.id}  ${s.title}`)); process.exit(0); }

// --only luôn kèm 00 (setup) vì các kịch bản khác dựa vào ctx
const selected = scenarios.filter((s) => s.id === '00' || ((!only || only.includes(s.id)) && !skip.includes(s.id)));
if (only && selected.length <= 1) { console.error('không có kịch bản nào khớp --only'); process.exit(2); }

console.log(`E2E run ${config.runId} -> ${config.api}`);
console.log(`admin: ${config.adminUser ? config.adminUser : '(không có)'} | dev: ${config.devMode} | expiry TTL: ${config.expiryTtlMin || 'tắt'} | webhook: ${config.paymentWebhookUrl ? 'bật' : 'tắt'}`);
process.exit(await runAll(selected, {}));
