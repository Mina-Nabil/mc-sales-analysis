export function Heatmap({ rows, cols, data, color = "124,92,255" }: { rows: string[]; cols: string[]; data: number[][]; color?: string }) {
  const max = Math.max(...data.flat(), 1);

  return (
    <div className="overflow-x-auto">
      <div className="inline-grid gap-1" style={{ gridTemplateColumns: `40px repeat(${cols.length}, minmax(22px, 1fr))` }}>
        <div />
        {cols.map((c) => (
          <div key={c} className="text-center text-[9.5px] font-semibold text-t2">
            {c}
          </div>
        ))}
        {rows.map((row, ri) => (
          <div key={row} className="contents">
            <div className="flex items-center text-[10px] font-semibold text-t2">{row}</div>
            {cols.map((_, ci) => {
              const v = data[ri]?.[ci] ?? 0;
              const alpha = Math.max(0.08, v / max);
              return (
                <div
                  key={ci}
                  title={`${row} / ${cols[ci]}: ${v}`}
                  className="aspect-square rounded-[4px]"
                  style={{ background: `rgba(${color},${alpha})` }}
                />
              );
            })}
          </div>
        ))}
      </div>
    </div>
  );
}
