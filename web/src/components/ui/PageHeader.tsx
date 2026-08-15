import type { ReactNode } from "react";
import { Breadcrumbs, type Crumb } from "./Breadcrumbs";

export interface PageHeaderProps {
  crumbs?: Crumb[];
  title: string;
  subtitle?: string;
  actions?: ReactNode;
}

export function PageHeader({ crumbs, title, subtitle, actions }: PageHeaderProps) {
  return (
    <div className="mb-6">
      {crumbs && <Breadcrumbs items={crumbs} />}
      <div className="mt-2 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0">
          <h1 className="truncate text-xl font-extrabold text-t0 sm:text-[26px]">{title}</h1>
          {subtitle && <p className="mt-1 text-[13px] text-t1">{subtitle}</p>}
        </div>
        {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
      </div>
    </div>
  );
}
