import { useState, type ReactNode } from "react";
import { cn } from "@/lib/cn";

export interface AccordionItemData {
  key: string;
  title: string;
  content: ReactNode;
}

export function Accordion({ items, defaultOpen }: { items: AccordionItemData[]; defaultOpen?: string }) {
  const [openKey, setOpenKey] = useState<string | undefined>(defaultOpen);

  return (
    <div className="divide-y divide-line rounded-[var(--radius-vela-lg)] border border-line bg-bg-2">
      {items.map((item) => {
        const isOpen = openKey === item.key;
        return (
          <div key={item.key}>
            <button
              onClick={() => setOpenKey(isOpen ? undefined : item.key)}
              className="flex w-full items-center justify-between gap-3 px-5 py-4 text-left"
            >
              <span className="text-[13.5px] font-semibold text-t0">{item.title}</span>
              <svg
                width="15"
                height="15"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
                className={cn("shrink-0 text-t2 transition-transform", isOpen && "rotate-180")}
              >
                <path d="m6 9 6 6 6-6" />
              </svg>
            </button>
            {isOpen && <div className="px-5 pb-4 text-[13px] leading-relaxed text-t1 animate-vela-fade">{item.content}</div>}
          </div>
        );
      })}
    </div>
  );
}
