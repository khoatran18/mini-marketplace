import { Suspense } from 'react';
import { ProductsBrowser } from '../../components/shop/ProductsBrowser';

export default function ProductsPage() {
  return (
    <Suspense fallback={<p className="text-sm text-muted">Đang tải…</p>}>
      <ProductsBrowser />
    </Suspense>
  );
}
