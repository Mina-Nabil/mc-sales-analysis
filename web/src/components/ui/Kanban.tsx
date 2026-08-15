import type { ReactNode } from "react";

export function KanbanBoard({ children }: { children: ReactNode }) {
  return <div className="flex gap-4 overflow-x-auto pb-2 snap-x snap-mandatory sm:snap-none">{children}</div>;
}

export function KanbanColumn({ title, count, color = "var(--acc)", children }: { title: string; count: number; color?: string; children: ReactNode }) {
  return (
    <div className="flex w-[270px] shrink-0 snap-start flex-col rounded-[var(--radius-vela-lg)] border border-line bg-bg-2 border-t-[3px]" style={{ borderTopColor: color }}>
      <div className="flex items-center justify-between px-4 py-3.5">
        <span className="text-[13px] font-bold text-t0">{title}</span>
        <span className="rounded-full bg-bg-3 px-2 py-0.5 text-[11px] font-bold text-t1">{count}</span>
      </div>
      <div className="flex flex-col gap-2.5 px-3 pb-3">{children}</div>
    </div>
  );
}

export function KanbanCard({ children }: { children: ReactNode }) {
  return <div className="cursor-pointer rounded-[13px] border border-line bg-bg-1 p-3.5 shadow-[var(--shadow-vela)] hover:border-line-2">{children}</div>;
}
