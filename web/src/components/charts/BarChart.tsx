import { useRef, useState, type MouseEvent } from "react";

export interface BarDatum {
  label: string;
  value: number;
  color?: string;
}

export function BarChart({ data, height = 220, color = "var(--acc)", formatValue = (v: number) => String(v) }: {
  data: BarDatum[];
  height?: number;
  color?: string;
  formatValue?: (v: number) => string;
}) {
  const max = Math.max(...data.map((d) => d.value), 1);

  return (
    <div className="flex items-stretch gap-2.5 sm:gap-3" style={{ height }}>
      {data.map((d, i) => (
        <div key={d.label} className="flex flex-1 flex-col items-center gap-2">
          <span className="text-[10.5px] font-bold text-t1">{formatValue(d.value)}</span>
          <div className="flex w-full flex-1 items-end">
            <div
              className="w-full rounded-t-[8px] transition-all"
              style={{
                height: `${(d.value / max) * 100}%`,
                background: d.color ?? color,
                minHeight: 4,
                transformOrigin: "bottom",
                animation: `velaGrowY .55s cubic-bezier(.22,.61,.36,1) ${i * 0.05}s both`,
              }}
            />
          </div>
          <span className="truncate text-[10.5px] font-semibold text-t2">{d.label}</span>
        </div>
      ))}
    </div>
  );
}

export function StackedBarChart({ data, keys, colors, height = 220, formatValue = (v: number) => String(v) }: {
  data: Array<Record<string, number | string>>;
  keys: string[];
  colors: string[];
  height?: number;
  formatValue?: (v: number) => string;
}) {
  const max = Math.max(...data.map((d) => keys.reduce((sum, k) => sum + (Number(d[k]) || 0), 0)), 1);
  const ref = useRef<HTMLDivElement>(null);
  const [hover, setHover] = useState<{ i: number; x: number; y: number } | null>(null);

  const track = (i: number) => (e: MouseEvent) => {
    const rect = ref.current?.getBoundingClientRect();
    if (!rect) return;
    setHover({ i, x: e.clientX - rect.left, y: e.clientY - rect.top });
  };

  return (
    <div ref={ref} className="relative">
      <div className="flex items-stretch gap-2.5 sm:gap-3" style={{ height }}>
        {data.map((d, i) => (
          <div key={i} className="flex flex-1 flex-col items-center gap-2"
            onMouseEnter={track(i)} onMouseMove={track(i)} onMouseLeave={() => setHover(null)}>
            <div
              className={"flex w-full flex-1 flex-col-reverse items-stretch overflow-hidden rounded-t-[8px] transition-opacity " + (hover && hover.i !== i ? "opacity-60" : "")}
              style={{ transformOrigin: "bottom", animation: `velaGrowY .55s cubic-bezier(.22,.61,.36,1) ${i * 0.05}s both` }}
            >
              {keys.map((k, ki) => {
                const v = Number(d[k]) || 0;
                return <div key={k} style={{ height: `${(v / max) * 100}%`, background: colors[ki] }} />;
              })}
            </div>
            <span className="truncate text-[10.5px] font-semibold text-t2">{String(d.label ?? i)}</span>
          </div>
        ))}
      </div>

      {hover && (() => {
        const d = data[hover.i];
        const total = keys.reduce((s, k) => s + (Number(d[k]) || 0), 0);
        const rows = keys.map((k, ki) => ({ k, v: Number(d[k]) || 0, c: colors[ki] })).filter((r) => r.v > 0);
        const w = ref.current?.clientWidth ?? 0;
        const flip = hover.x > w / 2;
        return (
          <div
            className="pointer-events-none absolute z-50 w-max min-w-[160px] max-w-[260px] rounded-[var(--radius-vela-md)] border border-line bg-bg-2 px-3 py-2 text-[12px] shadow-[var(--shadow-vela)]"
            style={{ left: hover.x + (flip ? -12 : 12), top: Math.max(hover.y - 12, 0), transform: flip ? "translateX(-100%)" : undefined }}
          >
            <div className="mb-1 font-bold text-t0">{String(d.label ?? hover.i)}</div>
            <div className="space-y-0.5">
              {rows.map((r) => (
                <div key={r.k} className="flex items-center gap-2">
                  <span className="h-2 w-2 shrink-0 rounded-full" style={{ background: r.c }} />
                  <span className="flex-1 truncate text-t1">{r.k}</span>
                  <span className="font-bold tabular-nums text-t0">{formatValue(r.v)}</span>
                </div>
              ))}
            </div>
            <div className="mt-1 flex items-center justify-between gap-4 border-t border-line pt-1">
              <span className="text-t2">Total</span>
              <span className="font-bold tabular-nums text-t0">{formatValue(total)}</span>
            </div>
          </div>
        );
      })()}
    </div>
  );
}
