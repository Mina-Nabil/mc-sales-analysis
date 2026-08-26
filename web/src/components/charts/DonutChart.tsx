export interface DonutSegment {
  label: string;
  value: number;
  color: string;
}

export function DonutChart({ segments, size = 160, thickness = 20, centerLabel, centerValue, formatValue }: {
  segments: DonutSegment[];
  size?: number;
  thickness?: number;
  centerLabel?: string;
  centerValue?: string;
  /** When set, the legend shows "value (pct%)" instead of just the percentage. */
  formatValue?: (n: number) => string;
}) {
  const total = segments.reduce((sum, s) => sum + s.value, 0) || 1;
  const r = (size - thickness) / 2;
  const c = 2 * Math.PI * r;
  let offsetAcc = 0;

  return (
    <div className="flex flex-col items-center gap-4 sm:flex-row sm:items-center">
      <div className="relative shrink-0" style={{ width: size, height: size }}>
        <svg width={size} height={size} className="-rotate-90">
          <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="var(--bg-3)" strokeWidth={thickness} />
          {segments.map((seg, i) => {
            const frac = seg.value / total;
            const dash = frac * c;
            const el = (
              <circle
                key={seg.label}
                cx={size / 2}
                cy={size / 2}
                r={r}
                fill="none"
                stroke={seg.color}
                strokeWidth={thickness}
                strokeDasharray={`${dash} ${c - dash}`}
                strokeDashoffset={-offsetAcc}
                strokeLinecap="butt"
                style={{ animation: `velaFade .5s ease ${0.15 + i * 0.12}s both` }}
              />
            );
            offsetAcc += dash;
            return el;
          })}
        </svg>
        {(centerLabel || centerValue) && (
          <div className="absolute inset-0 flex flex-col items-center justify-center">
            {centerValue && <span className="text-lg font-extrabold text-t0">{centerValue}</span>}
            {centerLabel && <span className="text-[11px] text-t1">{centerLabel}</span>}
          </div>
        )}
      </div>
      <div className="flex flex-col gap-2">
        {segments.map((seg) => (
          <div key={seg.label} className="flex items-center gap-2 text-[12.5px]">
            <span className="h-2.5 w-2.5 shrink-0 rounded-full" style={{ background: seg.color }} />
            <span className="text-t1">{seg.label}</span>
            {formatValue ? (
              <span className="font-bold text-t0">{formatValue(seg.value)} <span className="font-normal text-t2">({Math.round((seg.value / total) * 100)}%)</span></span>
            ) : (
              <span className="font-bold text-t0">{Math.round((seg.value / total) * 100)}%</span>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
