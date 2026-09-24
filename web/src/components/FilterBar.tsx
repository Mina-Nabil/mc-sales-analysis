import { useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api } from '@/lib/api'

export const FILTER_DIMS: [string, string][] = [
  ['brand', 'Brand'], ['model', 'Model'], ['segment', 'Segment'],
  ['car_type', 'Car type'], ['engine', 'Engine'], ['origin', 'Origin'], ['supply', 'Supply'],
  ['distributor', 'Distributor'], ['region', 'Region'], ['governorate', 'Governorate'], ['traffic_unit', 'Traffic unit'],
  ['model_year', 'Model year'],
]
const labelOf = (k: string) => FILTER_DIMS.find((d) => d[0] === k)?.[1] || k

/** Filter rows: what to keep, and what to leave out. */
export type FilterMode = 'include' | 'exclude'
/** URL key prefix per row — `filter.brand=…` keeps, `exclude.brand=…` drops. */
export const PREFIX: Record<FilterMode, string> = { include: 'filter.', exclude: 'exclude.' }

/** Global filter state, stored in the URL so any view is shareable. */
export function useFilters() {
  const [sp, setSp] = useSearchParams()
  const filters: Record<string, string[]> = {}
  const excludes: Record<string, string[]> = {}
  for (const [k, v] of sp.entries()) {
    if (!v) continue
    if (k.startsWith(PREFIX.include)) filters[k.slice(PREFIX.include.length)] = v.split(',')
    else if (k.startsWith(PREFIX.exclude)) excludes[k.slice(PREFIX.exclude.length)] = v.split(',')
  }

  const setFilter = (dim: string, values: string[], mode: FilterMode = 'include') =>
    setSp((prev) => {
      const n = new URLSearchParams(prev)
      if (values.length) n.set(PREFIX[mode] + dim, values.join(',')); else n.delete(PREFIX[mode] + dim)
      return n
    }, { replace: true })

  const clearAll = (mode: FilterMode = 'include') =>
    setSp((prev) => {
      const n = new URLSearchParams(prev)
      for (const k of [...n.keys()]) if (k.startsWith(PREFIX[mode])) n.delete(k)
      return n
    }, { replace: true })

  // Both rows travel together: every card query must keep the same scope.
  const qs = () => {
    const p = new URLSearchParams()
    for (const d in filters) p.set(PREFIX.include + d, filters[d].join(','))
    for (const d in excludes) p.set(PREFIX.exclude + d, excludes[d].join(','))
    return p.toString()
  }
  return { filters, excludes, setFilter, clearAll, qs }
}

export function FilterBar({ mode = 'include' }: { mode?: FilterMode }) {
  const { filters, excludes, setFilter, clearAll } = useFilters()
  const [open, setOpen] = useState<string | null>(null) // null | '__add__' | dim
  const ref = useRef<HTMLDivElement>(null)
  const isExclude = mode === 'exclude'
  const current = isExclude ? excludes : filters
  const active = Object.keys(current)
  const set = (dim: string, vals: string[]) => setFilter(dim, vals, mode)
  // Exclusions read as a warning, so they wear the danger token in both themes.
  const chip = isExclude
    ? 'border-bad/40 bg-bad-soft text-bad'
    : 'border-acc/40 bg-acc-soft text-acc'
  const xCls = isExclude ? 'text-bad/70 hover:text-bad' : 'text-acc/70 hover:text-acc'

  useEffect(() => {
    const onClick = (e: MouseEvent) => { if (ref.current && !ref.current.contains(e.target as Node)) setOpen(null) }
    document.addEventListener('mousedown', onClick)
    return () => document.removeEventListener('mousedown', onClick)
  }, [])

  return (
    <div className="relative flex flex-wrap items-center gap-2" ref={ref}>
      <span className={'inline-flex items-center gap-1.5 text-[12px] font-bold ' + (isExclude ? 'text-bad' : 'text-t1')}>
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M22 3H2l8 9.46V19l4 2v-8.54z" /></svg>
        {isExclude ? 'Exclude data from:' : 'Include data from:'}
      </span>
      {active.map((dim) => (
        <button key={dim} onClick={() => setOpen(open === dim ? null : dim)}
          className={'inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[12px] font-semibold ' + chip}>
          {labelOf(dim)}: {current[dim].length > 2 ? `${current[dim].length} selected` : current[dim].join(', ')}
          <span onClick={(e) => { e.stopPropagation(); set(dim, []) }} className={xCls}>✕</span>
        </button>
      ))}
      <button onClick={() => setOpen(open === '__add__' ? null : '__add__')}
        className={'inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-[12px] font-semibold hover:bg-bg-3 ' +
          (isExclude ? 'border-bad/40 bg-bad-soft text-bad' : 'border-line bg-bg-inset text-t1')}>
        ＋ {isExclude ? 'Exclusion' : 'Filter'}
      </button>
      {active.length > 0 && <button onClick={() => clearAll(mode)} className="text-[12px] font-semibold text-t2 hover:text-t0">Clear all</button>}

      {open === '__add__' && (
        <div className="absolute left-0 top-full z-50 mt-2 grid w-[320px] grid-cols-2 gap-1 rounded-[var(--radius-vela-md)] border border-line bg-bg-2 p-2 shadow-[var(--shadow-vela)]">
          {FILTER_DIMS.map(([k, label]) => (
            <button key={k} onClick={() => setOpen(k)}
              className="rounded-[9px] px-2.5 py-1.5 text-left text-[13px] text-t1 hover:bg-bg-3 hover:text-t0">{label}</button>
          ))}
        </div>
      )}
      {open && open !== '__add__' && (
        <ValuePicker dim={open} selected={current[open] || []} onApply={(vals) => { set(open, vals); setOpen(null) }} onClose={() => setOpen(null)} />
      )}
    </div>
  )
}

export function ValuePicker({ dim, selected, onApply, onClose }: { dim: string; selected: string[]; onApply: (v: string[]) => void; onClose: () => void }) {
  const [opts, setOpts] = useState<string[] | null>(null)
  const [q, setQ] = useState('')
  const [sel, setSel] = useState<string[]>(selected)
  useEffect(() => { api.values(dim).then(setOpts).catch(() => setOpts([])) }, [dim])
  const shown = (opts || []).filter((o) => o.toLowerCase().includes(q.toLowerCase())).slice(0, 200)
  const toggle = (v: string) => setSel((s) => s.includes(v) ? s.filter((x) => x !== v) : [...s, v])
  return (
    <div className="absolute left-0 top-full z-50 mt-2 w-[280px] rounded-[var(--radius-vela-md)] border border-line bg-bg-2 p-2.5 shadow-[var(--shadow-vela)]">
      <input autoFocus placeholder={`Search ${labelOf(dim).toLowerCase()}…`} value={q} onChange={(e) => setQ(e.target.value)}
        className="mb-2 h-8 w-full rounded-[var(--radius-vela-sm)] border border-line bg-bg-inset px-2.5 text-[13px] text-t0" />
      <div className="max-h-[240px] overflow-y-auto">
        {opts === null && <div className="p-3 text-center text-[12px] text-t2">Loading…</div>}
        {shown.map((o) => (
          <label key={o} className="flex cursor-pointer items-center gap-2 rounded-[8px] px-2 py-1.5 text-[13px] text-t0 hover:bg-bg-3">
            <input type="checkbox" checked={sel.includes(o)} onChange={() => toggle(o)} className="accent-[var(--acc)]" />
            <span dir="auto" className="truncate">{o}</span>
          </label>
        ))}
        {opts && shown.length === 0 && <div className="p-3 text-center text-[12px] text-t2">No matches</div>}
      </div>
      <div className="mt-2 flex items-center justify-between border-t border-line pt-2">
        <button onClick={() => setSel([])} className="text-[12px] font-semibold text-t2 hover:text-t0">Clear</button>
        <div className="flex gap-2">
          <button onClick={onClose} className="rounded-[8px] px-2.5 py-1 text-[12px] font-semibold text-t1 hover:bg-bg-3">Cancel</button>
          <button onClick={() => onApply(sel)} className="rounded-[8px] bg-acc px-3 py-1 text-[12px] font-bold text-white">Apply</button>
        </div>
      </div>
    </div>
  )
}
