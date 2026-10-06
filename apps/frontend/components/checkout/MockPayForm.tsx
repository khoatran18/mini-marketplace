'use client';

import { useState } from 'react';
import type { ConfirmPaymentInput } from '../../lib/api';
import type { Payment, PaymentMethodInfo } from '../../lib/types';
import { failureCodeLabel } from '../../lib/status';
import { Notice, SimulatedBadge } from '../ui/StateBlock';

// Documented test cards (04-orders-payments.md §3.2); the live list comes from GET /payments/methods.
export const defaultTestCards = [
  { number: '4242 4242 4242 4242', result: 'Thành công ngay' },
  { number: '4000 0000 0000 0002', result: 'Bị từ chối (card_declined)' },
  { number: '4000 0000 0000 9995', result: 'Không đủ số dư (insufficient_funds)' },
  { number: '4000 0000 0000 3220', result: 'Cần xác thực 3-D Secure, mã OTP 123456' },
  { number: '4000 0000 0000 0119', result: 'Lỗi nhà cung cấp tạm thời (thử lại được)' },
  { number: '4000 0000 0000 0341', result: 'Không phản hồi, kết quả về sau ~30 giây qua webhook' },
  { number: '4000 0000 0000 0259', result: 'Thành công, webhook gửi 2 lần (kiểm tra idempotency)' },
  { number: '4000 0000 0000 0067', result: 'Thành công nhưng webhook đến sau hạn thanh toán' }
];

interface Props {
  payment: Payment;
  methods: PaymentMethodInfo[];
  busy: boolean;
  onSubmit: (input: ConfirmPaymentInput) => void;
}

/** The fake payment form. Card numbers only choose the simulated outcome and are never stored. */
export function MockPayForm({ payment, methods, busy, onSubmit }: Props) {
  const [number, setNumber] = useState('4242 4242 4242 4242');
  const [exp, setExp] = useState(() => {
    const d = new Date();
    return `12/${String((d.getFullYear() + 3) % 100).padStart(2, '0')}`;
  });
  const [cvc, setCvc] = useState('123');
  const [otp, setOtp] = useState('');
  const [showCards, setShowCards] = useState(false);

  const testCards = methods.find((method) => method.code === 'MOCK_CARD')?.test_cards ?? defaultTestCards;
  const lastFailure = failureCodeLabel(payment.failure_code);

  if (payment.method === 'MOCK_CARD' && payment.next_action === 'otp') {
    return (
      <form
        onSubmit={(event) => {
          event.preventDefault();
          onSubmit({ otp: otp.trim() });
        }}
      >
        <Notice tone="info">Thẻ yêu cầu xác thực 3-D Secure (giả lập). Mã OTP thử nghiệm: <strong>123456</strong>.</Notice>
        {lastFailure ? <Notice tone="error">{lastFailure}</Notice> : null}
        <label>
          Mã OTP
          <input inputMode="numeric" autoComplete="one-time-code" maxLength={6} value={otp} onChange={(event) => setOtp(event.target.value)} required />
        </label>
        <button type="submit" className="btn-primary" disabled={busy}>
          {busy ? 'Đang xác thực…' : 'Xác nhận OTP'}
        </button>
      </form>
    );
  }

  if (payment.method === 'MOCK_CARD') {
    return (
      <div className="grid gap-4">
        <form
          onSubmit={(event) => {
            event.preventDefault();
            onSubmit({ card_number: number, card_exp: exp, card_cvc: cvc });
          }}
        >
          {lastFailure ? <Notice tone="error">{lastFailure} Bạn có thể thử lại với thẻ khác.</Notice> : null}
          <label>
            Số thẻ test
            <input inputMode="numeric" autoComplete="off" value={number} onChange={(event) => setNumber(event.target.value)} required />
          </label>
          <div className="grid grid-cols-2 gap-3">
            <label>
              Hạn (MM/YY)
              <input autoComplete="off" placeholder="MM/YY" value={exp} onChange={(event) => setExp(event.target.value)} required />
            </label>
            <label>
              CVC
              <input inputMode="numeric" autoComplete="off" maxLength={4} value={cvc} onChange={(event) => setCvc(event.target.value)} required />
            </label>
          </div>
          <button type="submit" className="btn-primary" disabled={busy}>
            {busy ? 'Đang xử lý…' : 'Thanh toán'}
          </button>
        </form>
        <div className="grid gap-2">
          <button type="button" className="btn w-fit" aria-expanded={showCards} onClick={() => setShowCards((value) => !value)}>
            {showCards ? 'Ẩn' : 'Xem'} số thẻ kịch bản thử nghiệm
          </button>
          {showCards ? (
            <div className="overflow-x-auto rounded-xl border border-line">
              <table className="w-full text-left text-sm">
                <thead className="bg-surface2 text-muted">
                  <tr>
                    <th className="px-3 py-2">Số thẻ</th>
                    <th className="px-3 py-2">Kết quả</th>
                  </tr>
                </thead>
                <tbody>
                  {testCards.map((card) => (
                    <tr key={card.number} className="border-t border-line">
                      <td className="px-3 py-2 font-mono">
                        <button type="button" className="font-mono text-brand underline" onClick={() => setNumber(card.number)}>
                          {card.number}
                        </button>
                      </td>
                      <td className="px-3 py-2 text-text">{card.result}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : null}
        </div>
      </div>
    );
  }

  if (payment.method === 'MOCK_WALLET') {
    return (
      <div className="grid gap-3">
        <Notice tone="info">Ví điện tử giả lập: bấm đồng ý, kết quả sẽ về sau vài giây qua webhook.</Notice>
        {lastFailure ? <Notice tone="error">{lastFailure}</Notice> : null}
        <div className="flex flex-wrap gap-2">
          <button type="button" className="btn-primary" disabled={busy} onClick={() => onSubmit({ approve: true })}>
            Đồng ý thanh toán
          </button>
          <button type="button" className="btn-danger" disabled={busy} onClick={() => onSubmit({ approve: false })}>
            Từ chối
          </button>
        </div>
      </div>
    );
  }

  if (payment.method === 'MOCK_BANK_TRANSFER') {
    return (
      <div className="grid gap-3">
        <Notice tone="info">Chuyển khoản giả lập: không có giao dịch thật. Bấm xác nhận để mô phỏng tiền đã về.</Notice>
        <dl className="grid gap-1 rounded-xl bg-surface2 p-4 text-sm">
          <div className="flex justify-between gap-3">
            <dt className="text-muted">Nội dung chuyển khoản</dt>
            <dd className="font-mono font-semibold text-text">{payment.provider_ref || `MM${payment.id}`}</dd>
          </div>
        </dl>
        <div>
          <button type="button" className="btn-primary" disabled={busy} onClick={() => onSubmit({ approve: true })}>
            Tôi đã chuyển khoản
          </button>
        </div>
      </div>
    );
  }

  return (
    <p className="text-sm text-muted">
      Phương thức này không cần thanh toán trước <SimulatedBadge>COD</SimulatedBadge>
    </p>
  );
}
