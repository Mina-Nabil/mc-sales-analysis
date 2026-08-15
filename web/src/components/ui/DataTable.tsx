import type { ReactNode } from "react";
import { cn } from "@/lib/cn";

export interface DataTableColumn<T> {
  key: string;
  header: string;
  render: (row: T) => ReactNode;
  /** Hide this column below the given breakpoint to keep mobile tables scannable. */
  hideBelow?: "sm" | "md" | "lg";
  align?: "left" | "right" | "center";
  width?: string;
}

export interface DataTableProps<T> {
  columns: DataTableColumn<T>[];
  data: T[];
  rowKey: (row: T) => string | number;
  onRowClick?: (row: T) => void;
  selectable?: boolean;
  emptyMessage?: string;
  className?: string;
}

const hideBelowClasses: Record<NonNullable<DataTableColumn<unknown>["hideBelow"]>, string> = {
  sm: "hidden sm:table-cell",
  md: "hidden md:table-cell",
  lg: "hidden lg:table-cell",
};

const alignClasses = {
  left: "text-left",
  right: "text-right",
  center: "text-center",
};

export function DataTable<T>({ columns, data, rowKey, onRowClick, emptyMessage = "No records found.", className }: DataTableProps<T>) {
  if (data.length === 0) {
    return <div className="rounded-[var(--radius-vela-lg)] border border-line bg-bg-2 p-10 text-center text-sm text-t1">{emptyMessage}</div>;
  }

  return (
    <div className={cn("overflow-x-auto rounded-[var(--radius-vela-lg)] border border-line bg-bg-2", className)}>
      <table className="w-full min-w-[640px] border-collapse text-sm">
        <thead>
          <tr className="border-b border-line">
            {columns.map((col) => (
              <th
                key={col.key}
                style={{ width: col.width }}
                className={cn(
                  "px-4 py-3 text-[10.5px] font-bold uppercase tracking-wide text-t2",
                  alignClasses[col.align ?? "left"],
                  col.hideBelow && hideBelowClasses[col.hideBelow],
                )}
              >
                {col.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {data.map((row) => (
            <tr
              key={rowKey(row)}
              onClick={onRowClick ? () => onRowClick(row) : undefined}
              className={cn("border-b border-line last:border-b-0 transition-colors", onRowClick && "cursor-pointer hover:bg-bg-3")}
            >
              {columns.map((col) => (
                <td
                  key={col.key}
                  className={cn("px-4 py-3.5 text-t0 align-middle", alignClasses[col.align ?? "left"], col.hideBelow && hideBelowClasses[col.hideBelow])}
                >
                  {col.render(row)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
