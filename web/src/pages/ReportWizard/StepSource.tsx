import { useEffect, useState } from 'react'
import { Card } from '@/components/ui'
import { FilterEditor } from '@/components/FilterBar'
import { setFilterMap, type DraftPatch, type ReportDraft } from '@/lib/reportDraft'
import { api } from '@/lib/api'
import { ScopeUnits } from './ScopeUnits'

const selCls = 'h-10 rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-3 text-[13px] text-t0'

export function StepSource({
  draft,
  onPatch,
}: {
  draft: ReportDraft
  onPatch: (p: DraftPatch) => void
}) {
  const [years, setYears] = useState<number[]>([])
  useEffect(() => { api.years().then(setYears).catch(() => {}) }, [])

  return (
    <div className="space-y-5">
      <div>
        <label className="mb-1.5 block text-[12.5px] font-bold text-t0">Registration year</label>
        <select className={selCls} value={draft.year} onChange={(e) => onPatch({ year: e.target.value })}>
          <option value="all">All time</option>
          {years.map((y) => <option key={y} value={y}>{y}</option>)}
        </select>
        <p className="mt-1.5 text-[11.5px] text-t2">The year vehicles were registered, not the model year.</p>
      </div>

      <Card padding="sm">
        <FilterEditor
          mode="include"
          values={draft.filters}
          onChange={(dim, vals) => onPatch((d) => ({ filters: setFilterMap(d.filters, dim, vals) }))}
          onClearAll={() => onPatch({ filters: {} })}
        />
      </Card>

      <ScopeUnits filters={draft.filters} excludes={{}} year={draft.year} />
    </div>
  )
}
