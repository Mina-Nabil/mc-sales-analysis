import { useEffect, useState } from 'react'
import { api, fmt } from '@/lib/api'
import { buildFilterQs } from '@/components/FilterBar'

/** Live unit count for the draft's include/exclude/year so the user can see the scope. */
export function ScopeUnits({
  filters,
  excludes,
  year,
}: {
  filters: Record<string, string[]>
  excludes: Record<string, string[]>
  year: string
}) {
  const [n, setN] = useState<number | null>(null)
  const [err, setErr] = useState(false)
  const qs = buildFilterQs(filters, excludes, year)

  useEffect(() => {
    let live = true
    setN(null)
    setErr(false)
    api.timeseries(qs).then((rows: any[]) => {
      if (!live) return
      setN((rows || []).reduce((s, r) => s + Number(r.volume || 0), 0))
    }).catch(() => { if (live) setErr(true) })
    return () => { live = false }
  }, [qs])

  return (
    <p className="text-[13.5px] text-t1">
      {err
        ? 'Could not count units in this scope.'
        : n == null
          ? 'Counting units in scope…'
          : <><span className="font-extrabold tabular-nums text-t0">{fmt(n)}</span> units in scope{year !== 'all' ? ` · ${year}` : ' · all years'}</>}
    </p>
  )
}
