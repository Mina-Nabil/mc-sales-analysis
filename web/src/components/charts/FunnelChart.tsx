export interface FunnelStage {
  label: string;
  value: number;
  color?: string;
}

export function FunnelChart({ stages }: { stages: FunnelStage[] }) {
  const max = Math.max(...stages.map((s) => s.value), 1);

  return (
    <div className="flex flex-col gap-2.5">
      {stages.map((s) => {
        const pct = (s.value / max) * 100;
        // Show the value inside the bar only when it's wide enough to hold the
        // text; otherwise render it just after the bar so it never overflows.
        const inside = pct >= 26;
        const value = s.value.toLocaleString();
        return (
          <div key={s.label} className="flex items-center gap-3">
            <span className="w-24 shrink-0 truncate text-[12px] font-semibold text-t1 sm:w-32">{s.label}</span>
            <div className="flex h-8 flex-1 items-center rounded-[8px] bg-bg-3">
              <div
                className="flex h-full items-center justify-end overflow-hidden rounded-[8px] px-2.5 text-[11px] font-bold text-white"
                style={{ width: `${Math.max(pct, 6)}%`, background: s.color ?? "var(--acc)" }}
              >
                {inside && value}
              </div>
              {!inside && <span className="whitespace-nowrap px-2.5 text-[11px] font-bold text-t0">{value}</span>}
            </div>
          </div>
        );
      })}
    </div>
  );
}
