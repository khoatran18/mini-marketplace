import { notFound } from 'next/navigation';
import { EditProductClient } from './EditProductClient';

export default function EditProductPage({ params }: { params: { id: string } }) {
  const id = Number(params.id);
  if (!Number.isInteger(id) || id <= 0) {
    notFound();
  }
  return <EditProductClient productId={id} />;
}
