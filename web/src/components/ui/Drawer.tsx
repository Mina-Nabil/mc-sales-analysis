import type { ReactNode } from "react";
import { createPortal } from "react-dom";
import { cn } from "@/lib/cn";

export interface DrawerProps {
  open: boolean;
  onClose: () => void;
  children: ReactNode;
  side?: "left" | "right";
  width?: string;
}

export function Drawer({ open, onClose, children, side = "left", width = "280px" }: DrawerProps) {
  if (!open) return null;

  return createPortal(
    <div className="fixed inset-0 z-[90] lg:hidden">
      <div className="absolute inset-0 bg-black/60 animate-vela-fade" onClick={onClose} />
      <div
        className={cn(
          "absolute top-0 h-full bg-bg-1 shadow-[var(--shadow-vela)] animate-vela-pop",
          side === "left" ? "left-0" : "right-0",
        )}
        style={{ width, maxWidth: "85vw" }}
      >
        {children}
      </div>
    </div>,
    document.body,
  );
}
