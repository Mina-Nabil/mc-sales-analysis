export function Gauge({ value, max = 100, size = 140, color = "var(--acc)", label }: { value: number; max?: number; size?: number; color?: string; label?: string }) {
  const stroke = 14;
  const r = (size - stroke) / 2;
  const circumference = Math.PI * r; // half circle
  const pct = Math.min(1, Math.max(0, value / max));
  const offset = circumference * (1 - pct);

  return (
    <div className="flex flex-col items-center">
      <svg width={size} height={size / 2 + stroke} viewBox={`0 0 ${size} ${size / 2 + stroke}`}>
        <path
          d={`M ${stroke / 2} ${size / 2} A ${r} ${r} 0 0 1 ${size - stroke / 2} ${size / 2}`}
          fill="none"
          stroke="var(--bg-3)"
          strokeWidth={stroke}
          strokeLinecap="round"
        />
        <path
          d={`M ${stroke / 2} ${size / 2} A ${r} ${r} 0 0 1 ${size - stroke / 2} ${size / 2}`}
          fill="none"
          stroke={color}
          strokeWidth={stroke}
          strokeLinecap="round"
          strokeDasharray={circumference}
          strokeDashoffset={offset}
          style={{ transition: "stroke-dashoffset .5s ease" }}
        />
      </svg>
      <span className="-mt-2 text-xl font-extrabold text-t0">{value}</span>
      {label && <span className="text-[11px] text-t1">{label}</span>}
    </div>
  );
}
