import { formatTime } from '../../lib/format';

const STALE_AFTER_MS = 10 * 60 * 1000;

/** "Cập nhật lúc 08:15"; turns into a warning when the data is older than 10 minutes (stale state). */
export function AsOfBadge({ asOf, source }: { asOf?: string; source?: string }) {
  if (!asOf) return <span className="text-xs text-muted">Chưa có thời điểm cập nhật</span>;
  const time = new Date(asOf).getTime();
  const stale = Number.isFinite(time) && Date.now() - time > STALE_AFTER_MS;
  return (
    <span className={`text-xs ${stale ? 'font-semibold text-warning' : 'text-muted'}`}>
      Cập nhật lúc {formatTime(asOf)}
      {source ? ` · nguồn ${source}` : ''}
      {stale ? ' · dữ liệu có thể đã cũ' : ''}
    </span>
  );
}
