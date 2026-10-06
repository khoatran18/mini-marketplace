import { InventoryTable } from '../../../components/seller/InventoryTable';

export default function SellerInventoryPage() {
  return (
    <div className="grid gap-5">
      <header className="grid gap-1">
        <h1 className="text-2xl font-bold text-text">Tồn kho</h1>
        <p className="text-sm text-muted">Khả dụng = tồn vật lý − hàng đang giữ cho đơn chưa kết thúc. Mọi điều chỉnh đều được ghi sổ kèm lý do.</p>
      </header>
      <InventoryTable />
    </div>
  );
}
