import { InventoryTable } from '../../../components/seller/InventoryTable';
import { PageTitle } from '../../../components/layout/PageTitle';

export default function SellerInventoryPage() {
  return (
    <div className="grid gap-5">
      <header className="grid gap-1">
        <PageTitle icon="box">Tồn kho</PageTitle>
        <p className="text-sm text-muted">Khả dụng = tồn vật lý − hàng đang giữ cho đơn chưa kết thúc. Mọi điều chỉnh đều được ghi sổ kèm lý do.</p>
      </header>
      <InventoryTable />
    </div>
  );
}
