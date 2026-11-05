import { Card } from '@/components/ui'
import { FilterEditor, dimLabel } from '@/components/FilterBar'
import { hasCards, setFilterMap, type ReportDraft } from '@/lib/reportDraft'
import { ScopeUnits } from './ScopeUnits'

function countMap(m: Record<string, string[]>) {
  return Object.values(m).reduce((s, v) => s + v.length, 0)
}

export function StepExclude({
  draft,
  onPatch,
}: {
  draft: ReportDraft
  onPatch: (p: Partial<ReportDraft>) => void
}) {
  const filterN = countMap(draft.filters)
  const excludeN = countMap(draft.excludes)
  const cardN =
    (draft.builtins ? 1 : 0) + draft.tables.length + draft.crosses.length + draft.bars.length + draft.pies.length

  return (
    <div className="space-y-5">
      <Card padding="sm">
        <FilterEditor
          mode="exclude"
          values={draft.excludes}
          onChange={(dim, vals) => onPatch({ excludes: setFilterMap(draft.excludes, dim, vals) })}
          onClearAll={() => onPatch({ excludes: {} })}
        />
      </Card>

      <ScopeUnits filters={draft.filters} excludes={draft.excludes} year={draft.year} />

      <Card padding="sm">
        <p className="text-[12.5px] font-bold text-t0">Summary</p>
        <dl className="mt-2 space-y-1.5 text-[13px] text-t1">
          <div className="flex justify-between gap-4"><dt>Name</dt><dd className="font-semibold text-t0">{draft.name.trim() || 'Untitled'}</dd></div>
          <div className="flex justify-between gap-4"><dt>Year</dt><dd className="font-semibold text-t0">{draft.year === 'all' ? 'All time' : draft.year}</dd></div>
          <div className="flex justify-between gap-4">
            <dt>Included</dt>
            <dd className="text-right font-semibold text-t0">
              {filterN === 0 ? 'Everything' : Object.entries(draft.filters).map(([k, v]) => `${dimLabel(k)} (${v.length})`).join(', ')}
            </dd>
          </div>
          <div className="flex justify-between gap-4">
            <dt>Excluded</dt>
            <dd className="text-right font-semibold text-t0">
              {excludeN === 0 ? 'Nothing' : Object.entries(draft.excludes).map(([k, v]) => `${dimLabel(k)} (${v.length})`).join(', ')}
            </dd>
          </div>
          <div className="flex justify-between gap-4">
            <dt>Cards</dt>
            <dd className="font-semibold text-t0">{hasCards(draft) ? cardN : 'None'}</dd>
          </div>
        </dl>
      </Card>
    </div>
  )
}
