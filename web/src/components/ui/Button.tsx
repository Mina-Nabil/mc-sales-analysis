import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from "react";
import { cn } from "@/lib/cn";

type Variant = "primary" | "secondary" | "outline" | "ghost" | "danger";
type Size = "sm" | "md" | "lg";

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: Size;
  icon?: ReactNode;
  iconRight?: ReactNode;
  fullWidth?: boolean;
}

const variantClasses: Record<Variant, string> = {
  primary: "bg-acc text-white shadow-[0_10px_24px_-10px_var(--acc-soft)] hover:bg-acc-2",
  secondary: "bg-bg-3 text-t0 border border-line hover:border-line-2",
  outline: "bg-transparent text-t0 border border-line hover:bg-bg-3",
  ghost: "bg-transparent text-t1 hover:bg-bg-3 hover:text-t0",
  danger: "bg-bad text-white hover:opacity-90",
};

const sizeClasses: Record<Size, string> = {
  sm: "h-8 px-3 text-xs gap-1.5 rounded-[var(--radius-vela-sm)]",
  md: "h-10 px-4 text-[13px] gap-2 rounded-[var(--radius-vela-md)]",
  lg: "h-11 px-5 text-sm gap-2 rounded-[var(--radius-vela-md)]",
};

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = "primary", size = "md", icon, iconRight, fullWidth, children, ...props }, ref) => {
    return (
      <button
        ref={ref}
        className={cn(
          "inline-flex items-center justify-center font-bold font-sans transition-colors cursor-pointer whitespace-nowrap disabled:opacity-50 disabled:cursor-not-allowed",
          variantClasses[variant],
          sizeClasses[size],
          fullWidth && "w-full",
          className,
        )}
        {...props}
      >
        {icon}
        {children}
        {iconRight}
      </button>
    );
  },
);
Button.displayName = "Button";
