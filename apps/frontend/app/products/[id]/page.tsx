import { notFound } from 'next/navigation';
import { ProductDetailsClient } from './ProductDetailsClient';

interface Props {
  params: { id: string };
}

export default function ProductDetailPage({ params }: Props) {
  const id = Number(params.id);

  if (Number.isNaN(id)) {
    notFound();
  }

  return <ProductDetailsClient productId={id} />;
}
