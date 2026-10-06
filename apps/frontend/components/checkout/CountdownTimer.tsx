'use client';

import { useEffect, useState } from 'react';
import { formatCountdown } from '../../lib/format';

/** ⏱ mm:ss until `target` (the payment / reservation deadline). Calls onExpire once when it reaches zero. */
export function CountdownTimer({ target, onExpire, label = 'Giữ hàng đến hết' }: { target?: string | null; onExpire?: () => void; label?: string }) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);
  const remaining = target ? new Date(target).getTime() - now : 0;
  useEffect(() => {
    if (target && remaining <= 0) onExpire?.();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [remaining <= 0, target]);
  if (!target) return null;
  return (
    <span className={`inline-flex items-center gap-2 rounded-full px-3 py-1 text-sm font-semibold ${remaining <= 60_000 ? 'bg-danger-soft text-danger' : 'bg-warning-soft text-warning'}`}>
      <span aria-hidden="true">⏱</span>
      <span className="sr-only">{label}</span>
      <time>{formatCountdown(target, now)}</time>
    </span>
  );
}
