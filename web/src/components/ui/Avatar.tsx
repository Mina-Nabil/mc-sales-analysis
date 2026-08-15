import { cn } from "@/lib/cn";

const GRADIENTS = [
  "linear-gradient(135deg,#7c5cff,#56a8ff)",
  "linear-gradient(135deg,#33d493,#56a8ff)",
  "linear-gradient(135deg,#f7b84e,#f76d7d)",
  "linear-gradient(135deg,#9d86ff,#7c5cff)",
  "linear-gradient(135deg,#56a8ff,#33d493)",
  "linear-gradient(135deg,#f76d7d,#9d86ff)",
];

function hashString(s: string) {
  let h = 0;
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0;
  return h;
}

function initials(name: string) {
  const parts = name.trim().split(/\s+/);
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
}

const sizeClasses = {
  xs: "h-6 w-6 text-[10px]",
  sm: "h-8 w-8 text-[11px]",
  md: "h-10 w-10 text-[13px]",
  lg: "h-12 w-12 text-[15px]",
  xl: "h-16 w-16 text-xl",
};

export interface AvatarProps {
  name: string;
  size?: keyof typeof sizeClasses;
  status?: "online" | "offline" | "away" | "busy";
  className?: string;
  ring?: boolean;
}

const statusColors: Record<NonNullable<AvatarProps["status"]>, string> = {
  online: "var(--ok)",
  offline: "var(--t2)",
  away: "var(--warn)",
  busy: "var(--bad)",
};

export function Avatar({ name, size = "md", status, className, ring }: AvatarProps) {
  const gradient = GRADIENTS[hashString(name) % GRADIENTS.length];
  return (
    <span className={cn("relative inline-flex shrink-0", className)}>
      <span
        className={cn(
          "flex items-center justify-center rounded-full font-bold text-white select-none",
          sizeClasses[size],
          ring && "ring-2 ring-bg-2",
        )}
        style={{ background: gradient }}
      >
        {initials(name)}
      </span>
      {status && (
        <span
          className="absolute -bottom-0.5 -right-0.5 h-2.5 w-2.5 rounded-full border-2"
          style={{ background: statusColors[status], borderColor: "var(--bg-2)" }}
        />
      )}
    </span>
  );
}

export function AvatarGroup({ names, max = 4 }: { names: string[]; max?: number }) {
  const shown = names.slice(0, max);
  const rest = names.length - shown.length;
  return (
    <div className="flex items-center">
      {shown.map((n, i) => (
        <Avatar key={n + i} name={n} size="sm" ring className={i > 0 ? "-ml-2" : ""} />
      ))}
      {rest > 0 && (
        <span className="-ml-2 flex h-8 w-8 items-center justify-center rounded-full bg-bg-3 text-[11px] font-bold text-t1 ring-2 ring-bg-2">
          +{rest}
        </span>
      )}
    </div>
  );
}
