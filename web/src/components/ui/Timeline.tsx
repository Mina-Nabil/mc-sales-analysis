import type { ReactNode } from "react";

export interface TimelineEvent {
  id: string | number;
  title: ReactNode;
  time: string;
  description?: string;
  color?: string;
  icon?: ReactNode;
}

export function Timeline({ events }: { events: TimelineEvent[] }) {
  return (
    <div className="flex flex-col">
      {events.map((event, i) => (
        <div key={event.id} className="flex gap-3.5">
          <div className="flex flex-col items-center">
            <span
              className="mt-1 flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-xs"
              style={{ background: (event.color ?? "var(--acc)") + "22", color: event.color ?? "var(--acc)" }}
            >
              {event.icon ?? "●"}
            </span>
            {i < events.length - 1 && <span className="mt-1 w-px flex-1 bg-line" />}
          </div>
          <div className="min-w-0 pb-6">
            <p className="text-[13px] font-semibold text-t0">{event.title}</p>
            {event.description && <p className="mt-0.5 text-[12.5px] text-t1">{event.description}</p>}
            <p className="mt-1 text-[11px] text-t2">{event.time}</p>
          </div>
        </div>
      ))}
    </div>
  );
}
