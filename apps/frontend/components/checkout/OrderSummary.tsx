import type { PreviewOutput } from '../../lib/types';
import { formatVND } from '../../lib/format';
import { StockBadge } from '../shop/StockBadge';

/** Server-priced summary from POST /checkout/preview: the browser never computes prices or shipping. */
export function OrderSummary({ preview }: { preview: PreviewOutput }) {
  const lines = preview.groups.flatMap((group) => group.lines);
  return (
    <div className="grid gap-3">
      <ul className="grid gap-2 text-sm">
        {lines.map((line) => (
          <li key={line.product_id} className="flex items-start justify-between gap-3">
            <span className="grid gap-0.5">
              <span className="font-medium text-text">
                {line.name} × {line.quantity}
              </span>
              {line.issue ? <span className="text-danger">{line.issue}</span> : <StockBadge level={line.stock_level} />}
            </span>
            <span className="whitespace-nowrap text-text">{formatVND(line.line_total)}</span>
          </li>
        ))}
      </ul>
      <dl className="grid gap-1 border-t border-line pt-3 text-sm">
        <div className="flex justify-between">
          <dt className="text-muted">Tạm tính</dt>
          <dd>{formatVND(preview.subtotal)}</dd>
        </div>
        <div className="flex justify-between">
          <dt className="text-muted">Phí giao hàng{preview.groups.length > 1 ? ` (${preview.groups.length} người bán)` : ''}</dt>
          <dd>{formatVND(preview.shipping_fee)}</dd>
        </div>
        <div className="flex justify-between text-base font-bold text-text">
          <dt>Tổng cộng</dt>
          <dd>{formatVND(preview.grand_total)}</dd>
        </div>
      </dl>
    </div>
  );
}
