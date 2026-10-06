import { formatByKind, isObject, isScalar, kindFor, labelFor } from '../../lib/analytics';

/** Generic table over rows of unknown shape: columns come from the data, formats from the column name. */
export function DataTable({ rows, maxColumns = 8, caption }: { rows: Record<string, unknown>[]; maxColumns?: number; caption: string }) {
  if (rows.length === 0) return null;
  const displayable = (value: unknown) => isScalar(value) || (isObject(value) && 'amount' in value);
  const columns = Array.from(new Set(rows.slice(0, 20).flatMap((row) => Object.keys(row))))
    .filter((key) => rows.some((row) => displayable(row[key])))
    .slice(0, maxColumns);
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left text-sm">
        <caption className="sr-only">{caption}</caption>
        <thead className="text-muted">
          <tr>
            {columns.map((column) => {
              const numericColumn = typeof rows[0][column] === 'number';
              return (
                <th key={column} scope="col" className={`px-3 py-2 ${numericColumn ? 'text-right' : ''}`}>
                  {labelFor(column)}
                </th>
              );
            })}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, index) => (
            <tr key={index} className="border-t border-line">
              {columns.map((column) => {
                const value = row[column];
                const numericCell = typeof value === 'number';
                return (
                  <td key={column} className={`px-3 py-2 text-text ${numericCell ? 'text-right' : ''}`}>
                    {formatByKind(value, kindFor(column, value))}
                  </td>
                );
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
