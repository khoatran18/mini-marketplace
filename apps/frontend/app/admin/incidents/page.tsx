import { IncidentsClient } from './IncidentsClient';
import { PageTitle } from '../../../components/layout/PageTitle';

// Admin and operator (guarded by app/admin/layout.tsx; the gateway checks the role again on every call).
export default function AdminIncidentsPage() {
  return (
    <div className="grid gap-5">
      <header className="grid gap-1">
        <PageTitle icon="server">Sự cố và điều tra AI</PageTitle>
        <p className="text-sm text-muted">
          Sự cố sinh từ cảnh báo Prometheus. Trợ lý chỉ ĐỌC và soạn nháp đề xuất; operator xác nhận nháp, một người KHÁC duyệt rồi chạy. Sự cố mức critical được trợ lý tự điều tra, mức warning chỉ khi bạn bấm “Điều tra”.
        </p>
      </header>
      <IncidentsClient />
    </div>
  );
}
