import type { ReactNode } from 'react';

export interface Column<T> {
  key: string;
  label: string;
  render: (row: T) => ReactNode;
  numeric?: boolean;
}

export function DataTable<T>({ rows, columns, caption }: { rows: T[]; columns: Column<T>[]; caption: string }) {
  if (rows.length === 0) return null;
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left text-sm">
        <caption className="sr-only">{caption}</caption>
        <thead className="text-muted">
          <tr>
            {columns.map((column) => (
              <th key={column.key} scope="col" className={`px-3 py-2 ${column.numeric ? 'text-right' : ''}`}>
                {column.label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, index) => (
            <tr key={index} className="border-t border-line">
              {columns.map((column) => (
                <td key={column.key} className={`px-3 py-2 text-text ${column.numeric ? 'text-right' : ''}`}>
                  {column.render(row)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
