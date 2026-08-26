import { useMemo, useRef, useState, type MouseEvent } from "react";

export interface LineSeries {
  name: string;
  data: number[];
  color: string;
  dashed?: boolean;
}

function buildSmoothPath(points: { x: number; y: number }[]) {
  if (points.length < 2) return "";
  let d = `M ${points[0].x} ${points[0].y}`;
  for (let i = 0; i < points.length - 1; i++) {
    const p0 = points[i === 0 ? i : i - 1];
    const p1 = points[i];
    const p2 = points[i + 1];
    const p3 = points[i + 2 < points.length ? i + 2 : i + 1];
    const cp1x = p1.x + (p2.x - p0.x) / 6;
    const cp1y = p1.y + (p2.y - p0.y) / 6;
    const cp2x = p2.x - (p3.x - p1.x) / 6;
    const cp2y = p2.y - (p3.y - p1.y) / 6;
    d += ` C ${cp1x} ${cp1y}, ${cp2x} ${cp2y}, ${p2.x} ${p2.y}`;
  }
  return d;
}

/**
 * Multi-series line chart with a shared, zero-based Y scale so series are
 * genuinely comparable. Pass `yMax` to force the same scale across sibling
 * charts (e.g. one chart per model side by side).
 */
export function MultiLineChart({ series, labels, height = 260, yMax, formatValue = (v: number) => String(v) }: {
  series: LineSeries[];
  labels: string[];
  height?: number;
  yMax?: number;
  formatValue?: (v: number) => string;
}) {
  const width = 600;
  const padY = 16;
  const wrapRef = useRef<HTMLDivElement>(null);
  const [hoverIdx, setHoverIdx] = useState<number | null>(null);
  const n = labels.length;

  const { paths, max } = useMemo(() => {
    const all = series.flatMap((s) => s.data);
    const max = Math.max(yMax ?? 0, ...all, 1); // zero-based, shared across series
    const toPoints = (data: number[]) =>
      data.map((v, i) => ({
        x: n > 1 ? (i / (n - 1)) * width : width / 2,
        y: padY + (height - padY * 2) * (1 - v / max),
      }));
    return { paths: series.map((s) => ({ s, pts: toPoints(s.data), d: buildSmoothPath(toPoints(s.data)) })), max };
  }, [series, height, yMax, n]);

  function handleMove(e: MouseEvent<HTMLDivElement>) {
    const rect = wrapRef.current?.getBoundingClientRect();
    if (!rect || n === 0) return;
    const rel = (e.clientX - rect.left) / rect.width;
    setHoverIdx(Math.min(n - 1, Math.max(0, Math.round(rel * (n - 1)))));
  }

  const hoverX = hoverIdx !== null && n > 1 ? (hoverIdx / (n - 1)) * width : null;
  const rows = hoverIdx === null ? [] : paths.map(({ s }) => ({ name: s.name, color: s.color, v: s.data[hoverIdx] ?? 0 }));
  const flip = hoverX !== null && hoverX > width / 2;

  return (
    <div>
      <div ref={wrapRef} className="relative w-full" style={{ height }}
        onMouseMove={handleMove} onMouseLeave={() => setHoverIdx(null)}>
        <svg viewBox={`0 0 ${width} ${height}`} className="h-full w-full overflow-visible" preserveAspectRatio="none">
          {hoverX !== null && (
            <line x1={hoverX} y1={0} x2={hoverX} y2={height} stroke="var(--line-2)" strokeDasharray="3 3" />
          )}
          {paths.map(({ s, d }, i) => (
            <path key={i} d={d} fill="none" stroke={s.color} strokeWidth="2.5"
              strokeDasharray={s.dashed ? "6 4" : undefined}
              strokeLinecap="round" vectorEffect="non-scaling-stroke" />
          ))}
          {hoverIdx !== null && paths.map(({ s, pts }, i) => {
            const p = pts[hoverIdx];
            return p ? <circle key={i} cx={p.x} cy={p.y} r="4" fill={s.color} stroke="var(--bg-2)" strokeWidth="2" /> : null;
          })}
        </svg>

        {hoverIdx !== null && rows.length > 0 && (
          <div
            className="pointer-events-none absolute z-50 w-max min-w-[150px] max-w-[280px] rounded-[var(--radius-vela-md)] border border-line bg-bg-2 px-3 py-2 text-[12px] shadow-[var(--shadow-vela)]"
            style={{ left: `${((hoverX ?? 0) / width) * 100}%`, top: 4, transform: flip ? "translateX(calc(-100% - 12px))" : "translateX(12px)" }}
          >
            <div className="mb-1 font-bold text-t0">{labels[hoverIdx]}</div>
            <div className="space-y-0.5">
              {rows.map((r, i) => (
                <div key={i} className="flex items-center gap-2">
                  <span className="h-2 w-2 shrink-0 rounded-full" style={{ background: r.color }} />
                  <span className="flex-1 truncate text-t1">{r.name}</span>
                  <span className="font-bold tabular-nums text-t0">{formatValue(r.v)}</span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>

      <div className="mt-1 flex justify-between px-0.5">
        {labels.map((l, i) => <span key={i} className="text-[10.5px] font-semibold text-t2">{l}</span>)}
      </div>

      <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1.5">
        {series.map((s, i) => (
          <div key={i} className="flex items-center gap-1.5 text-[12px]">
            <span className="h-0.5 w-4 shrink-0 rounded-full"
              style={{ background: s.dashed ? `repeating-linear-gradient(90deg, ${s.color} 0 4px, transparent 4px 7px)` : s.color }} />
            <span className="text-t1">{s.name}</span>
          </div>
        ))}
      </div>
      <span className="sr-only">Peak {formatValue(max)}</span>
    </div>
  );
}
