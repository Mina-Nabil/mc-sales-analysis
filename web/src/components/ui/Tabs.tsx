import { useState, type ReactNode } from "react";
import { cn } from "@/lib/cn";

export interface TabItem {
  key: string;
  label: string;
  content: ReactNode;
}

/** Self-contained (state-based) tab widget, for in-page tab switches that don't need a URL. */
export function Tabs({ items, defaultKey }: { items: TabItem[]; defaultKey?: string }) {
  const [active, setActive] = useState(defaultKey ?? items[0]?.key);
  const activeItem = items.find((i) => i.key === active);

  return (
    <div>
      <div className="flex gap-1 overflow-x-auto rounded-[var(--radius-vela-md)] bg-bg-3 p-1">
        {items.map((item) => (
          <button
            key={item.key}
            onClick={() => setActive(item.key)}
            className={cn(
              "shrink-0 whitespace-nowrap rounded-[10px] px-3.5 py-1.5 text-[12.5px] font-semibold transition-colors",
              active === item.key ? "bg-bg-1 text-t0 shadow-[var(--shadow-vela)]" : "text-t1 hover:text-t0",
            )}
          >
            {item.label}
          </button>
        ))}
      </div>
      <div className="mt-4">{activeItem?.content}</div>
    </div>
  );
}
