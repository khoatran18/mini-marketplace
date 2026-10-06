'use client';

import Link from 'next/link';
import { useState } from 'react';
import { AnalyticsPanel } from '../../../components/analytics/AnalyticsPanel';
import { KpiRow } from '../../../components/analytics/KpiRow';
import { parsePayments, rangeParams } from '../../../lib/analytics';
import { EmptyBlock, ErrorBlock, LoadingBlock, Notice, SimulatedBadge } from '../../../components/ui/StateBlock';
import { Pagination } from '../../../components/ui/Pagination';
import { StatusBadge } from '../../../components/ui/StatusBadge';
import { forcePaymentStatusRequest, getAdminPaymentRequest, listAdminPaymentsRequest, refundPaymentRequest } from '../../../lib/api';
import { formatDateTime, formatNumber, formatPercent, formatVND } from '../../../lib/format';
import { useApiData, useAuthedAction } from '../../../lib/hooks';
import { paymentMethodLabel, paymentMethods } from '../../../lib/status';
import type { Payment } from '../../../lib/types';

const PAGE_SIZE = 20;
const isDev = process.env.NODE_ENV !== 'production' || process.env.NEXT_PUBLIC_APP_ENV === 'dev';
const statusOptions = ['REQUIRES_ACTION', 'PROCESSING', 'SUCCEEDED', 'FAILED', 'CANCELED', 'EXPIRED', 'PARTIALLY_REFUNDED', 'REFUNDED'];

function PaymentDetail({ id, onChanged }: { id: number; onChanged: () => void }) {
  const run = useAuthedAction();
  const detail = useApiData((token) => getAdminPaymentRequest(id, token as string), [id]);
  const [amount, setAmount] = useState('');
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null);
  const payment: Payment | null = detail.data;

  const after = (text: string) => {
    setMessage({ tone: 'success', text });
    detail.reload();
    onChanged();
  };
  const fail = (err: unknown) => setMessage({ tone: 'error', text: (err as Error).message });

  if (detail.loading) return <LoadingBlock />;
  if (detail.error) return <ErrorBlock message={detail.error} onRetry={detail.reload} />;
  if (!payment) return null;

  const refundable = Math.max(0, payment.amount - payment.refunded);
  const canRefund = ['SUCCEEDED', 'PARTIALLY_REFUNDED'].includes(payment.status) && refundable > 0;

  return (
    <div className="grid gap-4 rounded-xl border border-line bg-surface2 p-4 text-sm">
      <div className="flex flex-wrap items-center gap-3">
        <h3 className="text-base font-semibold text-text">Thanh toán #{payment.id}</h3>
        <StatusBadge kind="payment" value={payment.status} />
        <SimulatedBadge />
      </div>
      <dl className="grid gap-1 sm:grid-cols-2">
        <div className="flex gap-2"><dt className="text-muted">Checkout:</dt><dd className="font-mono text-text">{payment.checkout_id}</dd></div>
        <div className="flex gap-2"><dt className="text-muted">Đơn hàng:</dt><dd>{(payment.order_ids ?? []).map((orderId) => (<Link key={orderId} href={`/admin/orders/${orderId}`} className="mr-2 font-semibold text-brand">#{orderId}</Link>))}</dd></div>
        <div className="flex gap-2"><dt className="text-muted">Số tiền:</dt><dd className="text-text">{formatVND(payment.amount)} (đã hoàn {formatVND(payment.refunded)})</dd></div>
        <div className="flex gap-2"><dt className="text-muted">Thẻ:</dt><dd className="text-text">{payment.card_last4 ? `•••• ${payment.card_last4}` : '–'}</dd></div>
        <div className="flex gap-2"><dt className="text-muted">Mã lỗi gần nhất:</dt><dd className="text-text">{payment.failure_code || '–'}</dd></div>
        <div className="flex gap-2"><dt className="text-muted">Hết hạn:</dt><dd className="text-text">{formatDateTime(payment.expires_at)}</dd></div>
      </dl>

      <div className="grid gap-4 md:grid-cols-2">
        <div>
          <h4 className="mb-1 font-semibold text-text">Lần thử ({payment.attempts?.length ?? 0})</h4>
          {payment.attempts?.length ? (
            <ul className="grid gap-1">
              {payment.attempts.map((attempt) => (
                <li key={attempt.id} className="text-xs text-text">
                  {formatDateTime(attempt.at)} · {attempt.scenario} · {attempt.status}
                  {attempt.failure_code ? ` · ${attempt.failure_code}` : ''}
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-xs text-muted">Chưa có.</p>
          )}
        </div>
        <div>
          <h4 className="mb-1 font-semibold text-text">Hoàn tiền ({payment.refunds?.length ?? 0})</h4>
          {payment.refunds?.length ? (
            <ul className="grid gap-1">
              {payment.refunds.map((refund) => (
                <li key={refund.id} className="text-xs text-text">
                  {formatDateTime(refund.at)} · {formatVND(refund.amount_minor / 100)} · {refund.status}
                  {refund.reason ? ` · ${refund.reason}` : ''}
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-xs text-muted">Chưa có.</p>
          )}
        </div>
      </div>

      {message ? <Notice tone={message.tone}>{message.text}</Notice> : null}

      {canRefund ? (
        <form
          className="grid gap-2 sm:grid-cols-[160px_1fr_auto]"
          onSubmit={(event) => {
            event.preventDefault();
            const vnd = Number(amount);
            if (!Number.isFinite(vnd) || vnd <= 0 || vnd > refundable) return setMessage({ tone: 'error', text: `Số tiền hoàn phải từ 1 đến ${formatVND(refundable)}.` });
            setBusy(true);
            // the API takes minor units (1 VND = 100)
            void run((token) => refundPaymentRequest(payment.id, Math.round(vnd * 100), reason.trim(), token))
              .then(() => after('Đã hoàn tiền (mô phỏng).'))
              .catch(fail)
              .finally(() => setBusy(false));
          }}
        >
          <input type="number" min={1} max={refundable} step={1} placeholder={`Tối đa ${refundable}`} aria-label="Số tiền hoàn (VND)" value={amount} onChange={(event) => setAmount(event.target.value)} required />
          <input placeholder="Lý do hoàn tiền" aria-label="Lý do" value={reason} onChange={(event) => setReason(event.target.value)} required />
          <button type="submit" className="btn-primary" disabled={busy}>
            Hoàn tiền
          </button>
        </form>
      ) : null}

      {isDev ? (
        <div className="flex flex-wrap items-center gap-2 border-t border-line pt-3">
          <span className="text-xs font-semibold text-warning">DEV: ép trạng thái</span>
          {['SUCCEEDED', 'FAILED', 'EXPIRED'].map((status) => (
            <button
              key={status}
              type="button"
              className="btn py-1 text-xs"
              disabled={busy}
              onClick={() => {
                setBusy(true);
                void run((token) => forcePaymentStatusRequest(payment.id, status, token))
                  .then(() => after(`Đã ép trạng thái ${status}.`))
                  .catch(fail)
                  .finally(() => setBusy(false));
              }}
            >
              {status}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}

export function AdminPaymentsClient() {
  const [status, setStatus] = useState('');
  const [method, setMethod] = useState('');
  const [page, setPage] = useState(1);
  const [open, setOpen] = useState<number | null>(null);
  const list = useApiData((token) => listAdminPaymentsRequest({ status: status || undefined, method: method || undefined, page, page_size: PAGE_SIZE }, token as string), [status, method, page]);
  const payments = list.data?.payments ?? [];

  return (
    <div className="grid gap-6">
      <header className="grid gap-1">
        <h1 className="flex items-center gap-2 text-2xl font-bold text-text">
          Thanh toán <SimulatedBadge />
        </h1>
        <p className="text-sm text-muted">Giao dịch do nhà cung cấp giả lập tạo ra: không có tiền thật.</p>
      </header>

      <AnalyticsPanel
        title="Tỉ lệ thành công & lỗi"
        scope="admin"
        report="payments"
        params={rangeParams({ period: '7d' })}
        definition="Tỉ lệ thành công = SUCCEEDED / (SUCCEEDED + FAILED), 7 ngày gần nhất."
        parse={parsePayments}
        render={(p) => (
          <div className="grid gap-4">
            <KpiRow
              kpis={[
                { key: 'succeeded', label: 'Thành công', value: formatNumber(p.succeeded) },
                { key: 'failed', label: 'Thất bại', value: formatNumber(p.failed), goodWhenDown: true },
                { key: 'rate', label: 'Tỉ lệ thành công', value: formatPercent(p.success_rate) },
                { key: 'awaiting', label: 'Đơn chờ thanh toán', value: formatNumber(p.orders_awaiting_payment) }
              ]}
            />
            {p.failure_codes.length > 0 ? (
              <p className="text-sm text-muted">
                Mã lỗi hàng đầu: {p.failure_codes.slice(0, 5).map((item) => `${item.key} (${item.count})`).join(', ')}
              </p>
            ) : null}
          </div>
        )}
      />

      <section className="card grid gap-4">
        <div className="flex flex-wrap items-center gap-3">
          <label className="flex items-center gap-2">
            Trạng thái
            <select className="w-auto py-1 text-sm" value={status} onChange={(event) => { setStatus(event.target.value); setPage(1); }}>
              <option value="">Tất cả</option>
              {statusOptions.map((value) => (
                <option key={value} value={value}>{value}</option>
              ))}
            </select>
          </label>
          <label className="flex items-center gap-2">
            Phương thức
            <select className="w-auto py-1 text-sm" value={method} onChange={(event) => { setMethod(event.target.value); setPage(1); }}>
              <option value="">Tất cả</option>
              {Object.keys(paymentMethods).filter((code) => code !== 'COD').map((code) => (
                <option key={code} value={code}>{code}</option>
              ))}
            </select>
          </label>
        </div>

        {list.loading ? <LoadingBlock /> : null}
        {list.error ? <ErrorBlock message={list.error} onRetry={list.reload} /> : null}
        {!list.loading && !list.error && payments.length === 0 ? <EmptyBlock>Không có giao dịch.</EmptyBlock> : null}

        {payments.length > 0 ? (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <caption className="sr-only">Giao dịch thanh toán</caption>
              <thead className="text-muted">
                <tr>
                  <th className="px-3 py-2">Mã</th>
                  <th className="px-3 py-2">Thời gian</th>
                  <th className="px-3 py-2">Phương thức</th>
                  <th className="px-3 py-2 text-right">Số tiền</th>
                  <th className="px-3 py-2">Trạng thái</th>
                  <th className="px-3 py-2">Mã lỗi</th>
                  <th className="px-3 py-2" />
                </tr>
              </thead>
              <tbody>
                {payments.map((payment) => (
                  <tr key={payment.id} className="border-t border-line">
                    <td className="px-3 py-2 font-semibold text-text">#{payment.id}</td>
                    <td className="whitespace-nowrap px-3 py-2 text-muted">{formatDateTime(payment.created_at)}</td>
                    <td className="px-3 py-2 text-text">{paymentMethodLabel(payment.method)}</td>
                    <td className="whitespace-nowrap px-3 py-2 text-right text-text">{formatVND(payment.amount)}</td>
                    <td className="px-3 py-2"><StatusBadge kind="payment" value={payment.status} /></td>
                    <td className="px-3 py-2 text-muted">{payment.failure_code || '–'}</td>
                    <td className="px-3 py-2">
                      <button type="button" className="btn py-1 text-xs" aria-expanded={open === payment.id} onClick={() => setOpen(open === payment.id ? null : payment.id)}>
                        {open === payment.id ? 'Đóng' : 'Chi tiết'}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : null}
        {open ? <PaymentDetail id={open} onChanged={list.reload} /> : null}
        <Pagination page={page} pageSize={PAGE_SIZE} total={list.data?.total ?? 0} onChange={setPage} />
      </section>
    </div>
  );
}
