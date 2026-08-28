import { useCallback, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";

/**
 * Hover tooltip rendered in a portal with fixed positioning.
 *
 * It must escape its ancestors: table headers live inside `overflow-auto`
 * scroll containers, which clip an absolutely-positioned tooltip. Fixed
 * coordinates measured from the trigger avoid that, and the result is clamped
 * to the viewport so edge columns (e.g. the last one) stay fully readable.
 */
export function Tooltip({ label, children, side = "top" }: { label: string; children: ReactNode; side?: "top" | "bottom" }) {
  const ref = useRef<HTMLSpanElement>(null);
  const [pos, setPos] = useState<{ left: number; top: number; below: boolean } | null>(null);

  const show = useCallback(() => {
    const el = ref.current;
    if (!el) return;
    const r = el.getBoundingClientRect();
    const margin = 8;
    const width = Math.min(280, Math.max(120, label.length * 6.2 + 20)); // estimate; clamped below
    let left = r.left + r.width / 2 - width / 2;
    left = Math.max(margin, Math.min(left, window.innerWidth - width - margin));
    // Flip above/below when there isn't room on the preferred side.
    const wantBelow = side === "bottom";
    const roomBelow = window.innerHeight - r.bottom > 60;
    const below = wantBelow ? roomBelow : r.top < 60;
    setPos({ left, top: below ? r.bottom + margin : r.top - margin, below });
  }, [label, side]);

  return (
    <>
      <span ref={ref} className="relative inline-flex" onMouseEnter={show} onMouseLeave={() => setPos(null)}>
        {children}
      </span>
      {pos && createPortal(
        <span
          role="tooltip"
          className="pointer-events-none fixed z-[300] max-w-[280px] rounded-md border border-line bg-bg-3 px-2.5 py-1.5 text-[11px] font-semibold leading-snug text-t0 shadow-[var(--shadow-vela)] animate-vela-fade"
          style={{ left: pos.left, top: pos.top, transform: pos.below ? undefined : "translateY(-100%)" }}
        >
          {label}
        </span>,
        document.body,
      )}
    </>
  );
}
