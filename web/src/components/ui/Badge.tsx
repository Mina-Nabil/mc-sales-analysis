import type { ReactNode } from "react";
import { cn } from "@/lib/cn";
import { statusVariant, type StatusVariant } from "@/lib/status";

const variantClasses: Record<StatusVariant, string> = {
  success: "text-ok bg-ok-soft",
  warning: "text-warn bg-warn-soft",
  danger: "text-bad bg-bad-soft",
  info: "text-info bg-info-soft",
  accent: "text-acc bg-acc-soft",
  neutral: "text-t1 bg-bg-3",
};

export interface BadgeProps {
  children: ReactNode;
  variant?: StatusVariant;
  /** If provided (and `variant` isn't), the variant is inferred from this status text. */
  status?: string;
  dot?: boolean;
  className?: string;
}

export function Badge({ children, variant, status, dot, className }: BadgeProps) {
  const resolved = variant ?? (status ? statusVariant(status) : "neutral");
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] font-bold leading-none",
        variantClasses[resolved],
        className,
      )}
    >
      {dot && <span className="h-1.5 w-1.5 rounded-full" style={{ background: "currentColor" }} />}
      {children}
    </span>
  );
}
