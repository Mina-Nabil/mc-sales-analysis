import { cn } from "@/lib/cn";

export interface PaginationProps {
  page: number;
  totalPages: number;
  onChange: (page: number) => void;
}

export function Pagination({ page, totalPages, onChange }: PaginationProps) {
  const pages = Array.from({ length: totalPages }, (_, i) => i + 1).filter(
    (p) => p === 1 || p === totalPages || Math.abs(p - page) <= 1,
  );

  return (
    <div className="flex flex-wrap items-center justify-center gap-1.5 sm:justify-between">
      <button
        disabled={page <= 1}
        onClick={() => onChange(page - 1)}
        className="h-9 rounded-[10px] border border-line px-3 text-[12.5px] font-semibold text-t1 hover:bg-bg-3 disabled:opacity-40"
      >
        Prev
      </button>
      <div className="flex items-center gap-1">
        {pages.map((p, i) => (
          <span key={p} className="flex items-center gap-1">
            {i > 0 && pages[i - 1] !== p - 1 && <span className="px-1 text-t2">…</span>}
            <button
              onClick={() => onChange(p)}
              className={cn(
                "h-9 w-9 rounded-[10px] text-[12.5px] font-semibold",
                p === page ? "bg-acc text-white" : "text-t1 hover:bg-bg-3",
              )}
            >
              {p}
            </button>
          </span>
        ))}
      </div>
      <button
        disabled={page >= totalPages}
        onClick={() => onChange(page + 1)}
        className="h-9 rounded-[10px] border border-line px-3 text-[12.5px] font-semibold text-t1 hover:bg-bg-3 disabled:opacity-40"
      >
        Next
      </button>
    </div>
  );
}
