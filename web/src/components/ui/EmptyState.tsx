import type { ReactNode } from "react";

export function EmptyState({ icon = "📭", title, description, action }: { icon?: ReactNode; title: string; description?: string; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center rounded-[var(--radius-vela-lg)] border border-dashed border-line bg-bg-2 px-6 py-14 text-center">
      <div className="mb-4 flex h-14 w-14 items-center justify-center rounded-full bg-bg-3 text-2xl">{icon}</div>
      <p className="text-[14.5px] font-bold text-t0">{title}</p>
      {description && <p className="mt-1.5 max-w-sm text-[13px] text-t1">{description}</p>}
      {action && <div className="mt-5">{action}</div>}
    </div>
  );
}
