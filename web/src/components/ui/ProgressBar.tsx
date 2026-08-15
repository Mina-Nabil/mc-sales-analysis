export interface ProgressBarProps {
  value: number; // 0-100
  color?: string;
  trackColor?: string;
  height?: number;
  label?: string;
}

export function ProgressBar({ value, color = "var(--acc)", trackColor = "var(--bg-3)", height = 8, label }: ProgressBarProps) {
  return (
    <div>
      {label && (
        <div className="mb-1.5 flex items-center justify-between text-[11.5px] font-semibold text-t1">
          <span>{label}</span>
          <span className="text-t0">{Math.round(value)}%</span>
        </div>
      )}
      <div className="w-full overflow-hidden rounded-full" style={{ height, background: trackColor }}>
        <div
          className="h-full rounded-full transition-[width] duration-500"
          style={{ width: `${Math.min(100, Math.max(0, value))}%`, background: color }}
        />
      </div>
    </div>
  );
}

export function RadialProgress({ value, size = 64, stroke = 7, color = "var(--acc)", trackColor = "var(--bg-3)" }: {
  value: number;
  size?: number;
  stroke?: number;
  color?: string;
  trackColor?: string;
}) {
  const r = (size - stroke) / 2;
  const c = 2 * Math.PI * r;
  const offset = c - (Math.min(100, Math.max(0, value)) / 100) * c;

  return (
    <div className="relative inline-flex items-center justify-center" style={{ width: size, height: size }}>
      <svg width={size} height={size} className="-rotate-90">
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke={trackColor} strokeWidth={stroke} />
        <circle
          cx={size / 2}
          cy={size / 2}
          r={r}
          fill="none"
          stroke={color}
          strokeWidth={stroke}
          strokeDasharray={c}
          strokeDashoffset={offset}
          strokeLinecap="round"
          style={{ transition: "stroke-dashoffset .5s ease" }}
        />
      </svg>
      <span className="absolute text-xs font-bold text-t0">{Math.round(value)}%</span>
    </div>
  );
}
