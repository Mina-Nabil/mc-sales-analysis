import { Link } from "react-router-dom";

export interface Crumb {
  label: string;
  to?: string;
}

export function Breadcrumbs({ items }: { items: Crumb[] }) {
  return (
    <nav className="flex items-center gap-1.5 text-[12.5px] text-t1">
      {items.map((item, i) => (
        <span key={i} className="flex items-center gap-1.5">
          {i > 0 && <span className="text-t2">/</span>}
          {item.to ? (
            <Link to={item.to} className="hover:text-t0">
              {item.label}
            </Link>
          ) : (
            <span className="font-semibold text-t0">{item.label}</span>
          )}
        </span>
      ))}
    </nav>
  );
}
