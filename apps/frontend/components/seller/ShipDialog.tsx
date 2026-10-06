'use client';

import { useState } from 'react';
import { SimulatedBadge } from '../ui/StateBlock';

interface Props {
  busy?: boolean;
  onSubmit: (carrier: string, trackingCode: string) => void;
  onCancel: () => void;
}

const carriers = ['GHN (mô phỏng)', 'GHTK (mô phỏng)', 'Viettel Post (mô phỏng)', 'J&T (mô phỏng)'];

/** Inline form that collects the carrier and tracking code before an order is marked as shipped (simulated shipping). */
export function ShipDialog({ busy, onSubmit, onCancel }: Props) {
  const [carrier, setCarrier] = useState(carriers[0]);
  const [code, setCode] = useState('');
  return (
    <form
      className="grid gap-3 rounded-xl border border-line bg-surface2 p-4"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit(carrier.trim(), code.trim());
      }}
    >
      <p className="flex items-center gap-2 text-sm font-semibold text-text">
        Giao hàng <SimulatedBadge>Vận chuyển MÔ PHỎNG</SimulatedBadge>
      </p>
      <label>
        Đơn vị vận chuyển
        <input list="carriers" value={carrier} onChange={(event) => setCarrier(event.target.value)} required />
        <datalist id="carriers">
          {carriers.map((name) => (
            <option key={name} value={name} />
          ))}
        </datalist>
      </label>
      <label>
        Mã vận đơn
        <input value={code} onChange={(event) => setCode(event.target.value)} required placeholder="VD: MM123456789" />
      </label>
      <div className="flex gap-2">
        <button type="submit" className="btn-primary" disabled={busy}>
          {busy ? 'Đang xử lý…' : 'Xác nhận giao hàng'}
        </button>
        <button type="button" className="btn" onClick={onCancel}>
          Đóng
        </button>
      </div>
    </form>
  );
}
