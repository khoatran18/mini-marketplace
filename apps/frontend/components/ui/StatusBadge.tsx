import { orderStatusMeta, paymentStatusMeta, productStatusMeta, stockLevelMeta, toneClass, type StatusMeta } from '../../lib/status';

type Kind = 'order' | 'payment' | 'stock' | 'product';

const resolvers: Record<Kind, (value?: string | null) => StatusMeta> = {
  order: orderStatusMeta,
  payment: paymentStatusMeta,
  stock: stockLevelMeta,
  product: productStatusMeta
};

export function StatusBadge({ kind, value, className = '' }: { kind: Kind; value?: string | null; className?: string }) {
  const meta = resolvers[kind](value);
  return (
    <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold ${toneClass[meta.tone]} ${className}`}>
      {meta.label}
    </span>
  );
}
