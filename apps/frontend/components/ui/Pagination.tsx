interface Props {
  page: number;
  pageSize: number;
  total: number;
  onChange: (page: number) => void;
}

export function Pagination({ page, pageSize, total, onChange }: Props) {
  const pages = Math.max(1, Math.ceil(total / pageSize));
  if (pages <= 1) return null;
  return (
    <nav aria-label="Phân trang" className="flex flex-wrap items-center justify-center gap-3 text-sm">
      <button type="button" className="btn" disabled={page <= 1} onClick={() => onChange(page - 1)}>
        Trang trước
      </button>
      <span className="font-semibold text-muted">
        Trang {page} / {pages} ({total} kết quả)
      </span>
      <button type="button" className="btn" disabled={page >= pages} onClick={() => onChange(page + 1)}>
        Trang sau
      </button>
    </nav>
  );
}
