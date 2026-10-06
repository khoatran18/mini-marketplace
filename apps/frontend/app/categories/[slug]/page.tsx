import { Suspense } from 'react';
import { ProductsBrowser } from '../../../components/shop/ProductsBrowser';

interface Props {
  params: { slug: string };
}

export default function CategoryPage({ params }: Props) {
  return (
    <Suspense fallback={<p className="text-sm text-muted">Đang tải…</p>}>
      <ProductsBrowser categorySlug={decodeURIComponent(params.slug)} />
    </Suspense>
  );
}
