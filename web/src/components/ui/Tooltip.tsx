import { useState, type ReactNode } from "react";
import { cn } from "@/lib/cn";

export function Tooltip({ label, children, side = "top" }: { label: string; children: ReactNode; side?: "top" | "bottom" }) {
  const [show, setShow] = useState(false);

  return (
    <span className="relative inline-flex" onMouseEnter={() => setShow(true)} onMouseLeave={() => setShow(false)}>
      {children}
      {show && (
        <span
          className={cn(
            "pointer-events-none absolute left-1/2 z-50 -translate-x-1/2 whitespace-nowrap rounded-md bg-bg-3 border border-line px-2.5 py-1.5 text-[11px] font-semibold text-t0 shadow-[var(--shadow-vela)] animate-vela-fade",
            side === "top" ? "bottom-full mb-2" : "top-full mt-2",
          )}
        >
          {label}
        </span>
      )}
    </span>
  );
}
