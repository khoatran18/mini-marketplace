// Bộ khung kiểm thử tối giản: check/step/until + báo cáo pass/fail.
import { sleep } from './client.mjs';

export class Skip extends Error {}

export class T {
  constructor(scenario) {
    this.scenario = scenario;
    this.passed = 0;
    this.failed = [];
    this.skipped = [];
  }
  step(title) { console.log(`\n  - ${title}`); }
  check(name, ok, detail = '') {
    if (ok) { this.passed++; console.log(`    ok   ${name}`); }
    else {
      const d = typeof detail === 'string' ? detail : JSON.stringify(detail);
      this.failed.push(name);
      console.log(`    FAIL ${name}${d ? ` :: ${d.slice(0, 400)}` : ''}`);
    }
    return !!ok;
  }
  // So sánh mã HTTP; in kèm body khi sai để dễ chẩn đoán
  status(name, res, expected) {
    const list = Array.isArray(expected) ? expected : [expected];
    return this.check(name, list.includes(res.status), `http=${res.status} mong đợi ${list.join('|')} body=${JSON.stringify(res.body)}${res.error ? ' err=' + res.error : ''}`);
  }
  eq(name, actual, expected) {
    return this.check(name, actual === expected, `thực tế=${JSON.stringify(actual)} mong đợi=${JSON.stringify(expected)}`);
  }
  skip(reason) { throw new Skip(reason); }
  // Thăm dò định kỳ tới khi fn() trả giá trị truthy; hết giờ => ghi FAIL và trả null
  async until(name, fn, { timeoutMs = 45000, everyMs = 1000, quiet = false } = {}) {
    const end = Date.now() + timeoutMs;
    let last;
    while (Date.now() < end) {
      last = await fn();
      if (last) { if (!quiet) this.check(name, true); return last; }
      await sleep(everyMs);
    }
    this.check(name, false, `hết ${timeoutMs}ms`);
    return null;
  }
}

// Chạy danh sách kịch bản tuần tự; trả về mã thoát (0 = tất cả pass).
export async function runAll(scenarios, ctx) {
  const results = [];
  for (const sc of scenarios) {
    const t = new T(sc);
    const t0 = Date.now();
    console.log(`\n[${sc.id}] ${sc.title}`);
    let state = 'pass';
    try {
      await sc.run(t, ctx);
      if (t.failed.length) state = 'fail';
    } catch (e) {
      if (e instanceof Skip) { state = 'skip'; t.skipped.push(e.message); console.log(`    SKIP ${e.message}`); }
      else { state = 'fail'; t.failed.push(`ngoại lệ: ${e.message}`); console.log(`    FAIL ngoại lệ: ${e.stack ?? e}`); }
    }
    results.push({ sc, t, state, ms: Date.now() - t0 });
    // Kịch bản nền tảng (setup) hỏng thì các kịch bản sau không có dữ liệu để chạy
    if (sc.critical && state === 'fail') { console.log('\n  Kịch bản nền tảng thất bại, dừng chạy.'); break; }
  }
  console.log('\n================ TỔNG KẾT ================');
  let pass = 0, fail = 0;
  for (const r of results) {
    const tag = r.state === 'pass' ? 'PASS' : r.state === 'skip' ? 'SKIP' : 'FAIL';
    console.log(`${tag}  [${r.sc.id}] ${r.sc.title}  (${r.t.passed} ok, ${r.t.failed.length} lỗi, ${(r.ms / 1000).toFixed(1)}s)`);
    r.t.failed.forEach((f) => console.log(`        - ${f}`));
    pass += r.t.passed; fail += r.t.failed.length;
  }
  console.log(`Tổng: ${pass} kiểm tra đạt, ${fail} thất bại.`);
  return fail === 0 && results.every((r) => r.state !== 'fail') ? 0 : 1;
}
