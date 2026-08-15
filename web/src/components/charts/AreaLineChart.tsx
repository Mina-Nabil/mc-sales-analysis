import { useId, useMemo, useState } from "react";

export interface AreaLineChartProps {
  data: number[];
  labels?: string[];
  color?: string;
  height?: number;
  showArea?: boolean;
  formatValue?: (v: number) => string;
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

export function AreaLineChart({ data, labels, color = "var(--acc)", height = 240, showArea = true, formatValue = (v) => String(v) }: AreaLineChartProps) {
  const gradientId = useId();
  const [hoverIdx, setHoverIdx] = useState<number | null>(null);
  const width = 600;
  const padY = 16;

  const { points, min, max } = useMemo(() => {
    const min = Math.min(...data);
    const max = Math.max(...data);
    const range = max - min || 1;
    const points = data.map((v, i) => ({
      x: (i / (data.length - 1)) * width,
      y: padY + (height - padY * 2) * (1 - (v - min) / range),
    }));
    return { points, min, max };
  }, [data, height]);

  const linePath = buildSmoothPath(points);
  const areaPath = `${linePath} L ${width} ${height} L 0 ${height} Z`;
  const active = hoverIdx !== null ? points[hoverIdx] : null;

  function handleMove(e: React.MouseEvent<SVGSVGElement>) {
    const rect = e.currentTarget.getBoundingClientRect();
    const relX = ((e.clientX - rect.left) / rect.width) * width;
    const idx = Math.round((relX / width) * (data.length - 1));
    setHoverIdx(Math.min(data.length - 1, Math.max(0, idx)));
  }

  return (
    <div className="relative w-full" style={{ height }}>
      <svg
        viewBox={`0 0 ${width} ${height}`}
        className="vela-reveal h-full w-full overflow-visible"
        preserveAspectRatio="none"
        onMouseMove={handleMove}
        onMouseLeave={() => setHoverIdx(null)}
      >
        <defs>
          <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={color} stopOpacity="0.35" />
            <stop offset="100%" stopColor={color} stopOpacity="0" />
          </linearGradient>
        </defs>
        {showArea && <path d={areaPath} fill={`url(#${gradientId})`} />}
        <path d={linePath} fill="none" stroke={color} strokeWidth="2.5" strokeLinecap="round" vectorEffect="non-scaling-stroke" />
        {active && (
          <g>
            <line x1={active.x} y1={0} x2={active.x} y2={height} stroke="var(--line-2)" strokeDasharray="3 3" />
            <circle cx={active.x} cy={active.y} r="5" fill={color} stroke="var(--bg-2)" strokeWidth="2" />
          </g>
        )}
      </svg>
      {active && hoverIdx !== null && (
        <div
          className="pointer-events-none absolute -translate-x-1/2 -translate-y-full rounded-lg border border-line bg-bg-3 px-2.5 py-1.5 text-[11px] font-bold text-t0 shadow-[var(--shadow-vela)]"
          style={{ left: `${(active.x / width) * 100}%`, top: `${(active.y / height) * 100}%`, marginTop: -8 }}
        >
          {labels?.[hoverIdx] ? `${labels[hoverIdx]}: ` : ""}
          {formatValue(data[hoverIdx])}
        </div>
      )}
      <span className="sr-only">
        Range {formatValue(min)} to {formatValue(max)}
      </span>
    </div>
  );
}
