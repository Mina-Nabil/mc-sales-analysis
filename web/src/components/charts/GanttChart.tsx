export interface GanttRow {
  label: string;
  start: number; // 0-100 (% of timeline)
  duration: number; // % width
  color?: string;
}

export function GanttChart({ rows, columns = 6 }: { rows: GanttRow[]; columns?: number }) {
  return (
    <div className="overflow-x-auto">
      <div className="min-w-[560px]">
        <div className="grid border-b border-line pb-2" style={{ gridTemplateColumns: `140px repeat(${columns}, 1fr)` }}>
          <div />
          {Array.from({ length: columns }, (_, i) => (
            <div key={i} className="text-center text-[10.5px] font-semibold text-t2">
              W{i + 1}
            </div>
          ))}
        </div>
        <div className="flex flex-col gap-2 pt-2">
          {rows.map((row) => (
            <div key={row.label} className="grid items-center" style={{ gridTemplateColumns: `140px 1fr` }}>
              <span className="truncate pr-2 text-[12px] font-semibold text-t1">{row.label}</span>
              <div className="relative h-6 rounded-md bg-bg-3">
                <div
                  className="absolute top-0 h-full rounded-md"
                  style={{ left: `${row.start}%`, width: `${row.duration}%`, background: row.color ?? "var(--acc)" }}
                />
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
