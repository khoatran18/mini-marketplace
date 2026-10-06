import { notFound } from 'next/navigation';
import { ProtectedContent } from '../../../components/ProtectedContent';
import { PayClient } from './PayClient';

export default function PayPage({ params }: { params: { id: string } }) {
  const id = Number(params.id);
  if (!Number.isInteger(id) || id <= 0) {
    notFound();
  }
  return (
    <ProtectedContent allowedRoles={['buyer']}>
      <PayClient paymentId={id} />
    </ProtectedContent>
  );
}
