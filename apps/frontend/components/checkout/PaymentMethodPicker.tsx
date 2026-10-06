'use client';

import type { PaymentMethodInfo } from '../../lib/types';
import { SimulatedBadge } from '../ui/StateBlock';

interface Props {
  methods: PaymentMethodInfo[];
  value: string;
  onChange: (code: string) => void;
}

export function PaymentMethodPicker({ methods, value, onChange }: Props) {
  return (
    <fieldset className="grid gap-2">
      <legend className="sr-only">Phương thức thanh toán</legend>
      {methods.map((method) => (
        <label key={method.code} className={`flex cursor-pointer items-center gap-3 rounded-xl border p-3 font-normal ${value === method.code ? 'border-brand bg-brand-soft' : 'border-line'}`}>
          <input type="radio" name="payment_method" checked={value === method.code} onChange={() => onChange(method.code)} />
          <span className="flex flex-wrap items-center gap-2 text-sm font-semibold text-text">
            {method.label.replace(/\s*\(MÔ PHỎNG\)\s*/, '')}
            {method.simulated ? <SimulatedBadge /> : null}
          </span>
        </label>
      ))}
    </fieldset>
  );
}
