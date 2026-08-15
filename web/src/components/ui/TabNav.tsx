import { NavLink } from "react-router-dom";
import { cn } from "@/lib/cn";

export interface TabNavItem {
  label: string;
  to: string;
  end?: boolean;
}

/** Route-driven tab strip used by tabbed module layouts (Account, Settings, Marketing, Reports, Components). */
export function TabNav({ items }: { items: TabNavItem[] }) {
  return (
    <div className="flex gap-1 overflow-x-auto border-b border-line pb-0">
      {items.map((item) => (
        <NavLink
          key={item.to}
          to={item.to}
          end={item.end}
          className={({ isActive }) =>
            cn(
              "shrink-0 whitespace-nowrap border-b-2 px-3.5 py-2.5 text-[13px] font-semibold transition-colors",
              isActive ? "border-acc text-t0" : "border-transparent text-t1 hover:text-t0",
            )
          }
        >
          {item.label}
        </NavLink>
      ))}
    </div>
  );
}
