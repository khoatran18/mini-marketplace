'use client';

import type { ChatCard } from '../../lib/ai-chat';
import { formatNumber, formatVND } from '../../lib/format';

const TITLES: Record<string, string> = {
  search_products: 'Kết quả tìm kiếm', get_product: 'Sản phẩm', get_product_stock: 'Tồn kho', list_categories: 'Danh mục', get_cart: 'Giỏ hàng',
  list_my_orders: 'Đơn hàng của bạn', count_my_orders: 'Số đơn theo trạng thái', get_my_order: 'Chi tiết đơn hàng', list_payment_methods: 'Phương thức thanh toán',
  list_my_products: 'Sản phẩm của shop', get_inventory_ledger: 'Sổ kho', list_low_stock: 'Hàng sắp hết', list_store_orders: 'Đơn của shop',
  count_store_orders: 'Số đơn của shop theo trạng thái', get_store_order: 'Chi tiết đơn', analytics_summary: 'Tổng quan', analytics_timeseries: 'Theo thời gian',
  analytics_top_products: 'Sản phẩm bán chạy', analytics_funnel: 'Phễu bán hàng', analytics_low_stock: 'Tồn kho thấp',
  admin_list_orders: 'Đơn hàng toàn sàn', admin_get_order: 'Chi tiết đơn hàng', admin_list_payments: 'Thanh toán', admin_get_payment: 'Chi tiết thanh toán',
  admin_list_categories: 'Danh mục', ops_list_incidents: 'Sự cố', ops_get_incident: 'Chi tiết sự cố', ops_list_audit: 'Nhật ký kiểm toán', ops_outbox_dead: 'Sự kiện outbox lỗi',
  ops_list_proposals: 'Đề xuất vận hành', ops_get_proposal: 'Chi tiết đề xuất vận hành', ops_metrics_query: 'Số liệu hệ thống', ops_system_health: 'Sức khoẻ hệ thống', data_health: 'Luồng dữ liệu'
};

const titleOf = (tool: string) => TITLES[tool] ?? (tool.startsWith('admin_analytics_') ? 'Báo cáo toàn sàn' : 'Dữ liệu');

const PREFERRED = ['id', 'order_id', 'name', 'title', 'product_name', 'status', 'payment_status', 'total', 'total_amount', 'grand_total', 'price', 'revenue', 'quantity', 'available', 'stock', 'created_at'];
const MONEY = /(total|price|amount|revenue|fee|subtotal)/i;
const MAX_ROWS = 8;
const MAX_COLS = 4;

type Row = Record<string, unknown>;
const isObject = (v: unknown): v is Row => typeof v === 'object' && v !== null && !Array.isArray(v);
const isScalar = (v: unknown) => ['string', 'number', 'boolean'].includes(typeof v);

/** The list inside a tool result: the data itself or its first array-of-objects field. */
function rowsOf(data: unknown): Row[] | null {
  if (Array.isArray(data)) return data.filter(isObject);
  if (isObject(data)) {
    for (const value of Object.values(data)) {
      if (Array.isArray(value) && value.length > 0 && value.every(isObject)) return value as Row[];
    }
  }
  return null;
}

function cell(key: string, value: unknown): string {
  if (value === null || value === undefined || value === '') return '–';
  if (typeof value === 'number') return MONEY.test(key) ? formatVND(value) : formatNumber(value);
  if (typeof value === 'string' && /_at$/.test(key)) {
    const d = new Date(value);
    return Number.isNaN(d.getTime()) ? value : d.toLocaleString('vi-VN', { hour12: false });
  }
  return String(value);
}

const label = (key: string) => key.replace(/_/g, ' ');

function columnsOf(rows: Row[]): string[] {
  const keys = Object.keys(rows[0]).filter((k) => isScalar(rows[0][k]));
  const preferred = PREFERRED.filter((k) => keys.includes(k));
  return [...preferred, ...keys.filter((k) => !preferred.includes(k))].slice(0, MAX_COLS);
}

/** Scalars of an object, one level deep (counts, totals, a single order). */
function pairsOf(data: unknown): [string, unknown][] {
  if (!isObject(data)) return [];
  const own = Object.entries(data).filter(([, v]) => isScalar(v));
  const nested = Object.values(data).filter(isObject).flatMap((o) => Object.entries(o).filter(([, v]) => isScalar(v)));
  return [...own, ...nested].slice(0, 10);
}

function Card({ card }: { card: ChatCard }) {
  const rows = rowsOf(card.data);
  const title = titleOf(card.tool);
  return (
    <div className="overflow-hidden rounded-xl border border-line bg-surface text-xs">
      <p className="border-b border-line bg-surface2 px-3 py-1.5 font-semibold text-text">{title}</p>
      {rows && rows.length > 0 ? (
        <div className="overflow-x-auto">
          <table className="w-full text-left">
            <thead className="text-muted">
              <tr>
                {columnsOf(rows).map((k) => (
                  <th key={k} className="px-3 py-1 font-medium capitalize">{label(k)}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.slice(0, MAX_ROWS).map((row, i) => (
                <tr key={i} className="border-t border-line">
                  {columnsOf(rows).map((k) => (
                    <td key={k} className="px-3 py-1 text-text">{cell(k, row[k])}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
          {rows.length > MAX_ROWS ? <p className="border-t border-line px-3 py-1 text-muted">… và {rows.length - MAX_ROWS} dòng nữa</p> : null}
        </div>
      ) : rows ? (
        <p className="px-3 py-2 text-muted">Không có dữ liệu.</p>
      ) : (
        <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 px-3 py-2">
          {pairsOf(card.data).map(([k, v]) => (
            <div key={k} className="contents">
              <dt className="capitalize text-muted">{label(k)}</dt>
              <dd className="text-text">{cell(k, v)}</dd>
            </div>
          ))}
        </dl>
      )}
    </div>
  );
}

/** A search that found nothing (or an empty list) is noise: the answer already says so. */
const isEmpty = (card: ChatCard) => {
  const rows = rowsOf(card.data);
  if (rows) return rows.length === 0;
  if (isObject(card.data)) return Object.values(card.data).some((v) => Array.isArray(v) && v.length === 0) && pairsOf(card.data).every(([k]) => ['success', 'message', 'total', 'page', 'page_size'].includes(k));
  return false;
};

/** The cards worth showing (empty results are noise). The panel uses it for the "(N)" count and to hide the fold when nothing is left. */
export const visibleCards = (cards: ChatCard[]) => cards.filter((card) => !isEmpty(card));

export function ChatCards({ cards }: { cards: ChatCard[] }) {
  const shown = visibleCards(cards);
  if (shown.length === 0) return null;
  return (
    <div className="grid gap-2">
      {shown.map((card, i) => (
        <Card key={i} card={card} />
      ))}
    </div>
  );
}
