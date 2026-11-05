import { useEffect, useRef, useState } from 'react'
import { Button, Card, Switch } from '@/components/ui'
import { DIM_GROUPS, ValuePicker, dimLabel } from '@/components/FilterBar'
import { hasCards, type ReportDraft } from '@/lib/reportDraft'
import type { CrossSpec } from '@/lib/views'

const DEFAULT_CROSS: CrossSpec = { dim1: 'model', dim2: 'model_year' }
const selCls = 'h-9 rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-2.5 text-[13px] text-t0'

type CardKind = 'table' | 'cross' | 'bar' | 'pie'

const KINDS: { id: CardKind; label: string; hint: string }[] = [
  { id: 'table', label: 'Table', hint: 'Monthly figures for one dimension' },
  { id: 'cross', label: '2-dimension table', hint: 'Rows grouped by two dimensions' },
  { id: 'bar', label: 'Bar chart', hint: 'Stacked units by month' },
  { id: 'pie', label: 'Pie chart', hint: 'Share of the total' },
]

export function StepCards({
  draft,
  onPatch,
  warnEmpty,
}: {
  draft: ReportDraft
  onPatch: (p: Partial<ReportDraft>) => void
  warnEmpty: boolean
}) {
  const [kind, setKind] = useState<CardKind | null>(null)
  const [cross, setCross] = useState<CrossSpec>({ ...DEFAULT_CROSS })
  const [picking, setPicking] = useState(false)
  const pickRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const h = (e: MouseEvent) => { if (pickRef.current && !pickRef.current.contains(e.target as Node)) setPicking(false) }
    document.addEventListener('mousedown', h)
    return () => document.removeEventListener('mousedown', h)
  }, [])

  const used = (k: CardKind) => (k === 'table' ? draft.tables : k === 'bar' ? draft.bars : k === 'pie' ? draft.pies : [])
  const addDim = (k: CardKind, dim: string) => {
    if (k === 'cross') return
    const key = k === 'table' ? 'tables' : k === 'bar' ? 'bars' : 'pies'
    const cur = draft[key]
    if (cur.includes(dim)) return
    onPatch({ [key]: [...cur, dim] })
  }
  const addCross = () => {
    if (cross.dim1 === cross.dim2) return
    onPatch({ crosses: [...draft.crosses, { ...cross, values2: cross.values2?.length ? cross.values2 : undefined }] })
    setCross({ ...DEFAULT_CROSS })
    setPicking(false)
  }

  const dimSelect = (value: string, onChange: (v: string) => void) => (
    <select className={selCls} value={value} onChange={(e) => onChange(e.target.value)}>
      {DIM_GROUPS.map(([group, dims]) => (
        <optgroup key={group} label={group}>
          {dims.map((k) => <option key={k} value={k}>{dimLabel(k)}</option>)}
        </optgroup>
      ))}
    </select>
  )

  const rows: { key: string; label: string; remove: () => void }[] = [
    ...draft.tables.map((d) => ({
      key: `table:${d}`,
      label: `Table · ${dimLabel(d)}`,
      remove: () => onPatch({ tables: draft.tables.filter((x) => x !== d) }),
    })),
    ...draft.crosses.map((c, i) => ({
      key: `cross:${i}`,
      label: `2-dimension table · ${dimLabel(c.dim1)} × ${dimLabel(c.dim2)}`,
      remove: () => onPatch({ crosses: draft.crosses.filter((_, n) => n !== i) }),
    })),
    ...draft.bars.map((d) => ({
      key: `bar:${d}`,
      label: `Bar chart · ${dimLabel(d)}`,
      remove: () => onPatch({ bars: draft.bars.filter((x) => x !== d) }),
    })),
    ...draft.pies.map((d) => ({
      key: `pie:${d}`,
      label: `Pie chart · ${dimLabel(d)}`,
      remove: () => onPatch({ pies: draft.pies.filter((x) => x !== d) }),
    })),
  ]

  return (
    <div className="space-y-5">
      <Card>
        <Switch
          checked={draft.builtins}
          onChange={(v) => onPatch({ builtins: v })}
          label="Include overview cards"
        />
        <p className="mt-2 text-[12.5px] text-t1">
          Monthly volume, top brands, and a brand leaderboard. Hidden automatically when the report is filtered to a single brand.
        </p>
      </Card>

      <div>
        <p className="mb-2 text-[12.5px] font-bold text-t0">Add a breakdown</p>
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          {KINDS.map((k) => (
            <button
              key={k.id}
              type="button"
              onClick={() => setKind(kind === k.id ? null : k.id)}
              className={'rounded-[var(--radius-vela-md)] border px-3.5 py-3 text-left transition-colors ' +
                (kind === k.id ? 'border-acc bg-acc-soft' : 'border-line bg-bg-2 hover:bg-bg-3')}
            >
              <span className={'block text-[13px] font-bold ' + (kind === k.id ? 'text-acc' : 'text-t0')}>{k.label}</span>
              <span className="mt-0.5 block text-[12px] text-t1">{k.hint}</span>
            </button>
          ))}
        </div>
      </div>

      {kind && kind !== 'cross' && (
        <Card padding="sm">
          <p className="mb-2 text-[12px] font-bold text-t1">Dimension</p>
          {DIM_GROUPS.map(([group, dims]) => (
            <div key={group} className="mb-2 last:mb-0">
              <div className="px-1 py-1 text-[10.5px] font-bold uppercase tracking-wide text-t2">{group}</div>
              <div className="grid grid-cols-2 gap-1">
                {dims.map((d) => {
                  const taken = used(kind).includes(d)
                  return (
                    <button
                      key={d}
                      type="button"
                      disabled={taken}
                      onClick={() => addDim(kind, d)}
                      className={'rounded-[9px] px-2.5 py-1.5 text-left text-[13px] ' +
                        (taken ? 'cursor-not-allowed text-t2/50' : 'text-t1 hover:bg-bg-3 hover:text-t0')}
                    >
                      {dimLabel(d)}{taken ? ' ✓' : ''}
                    </button>
                  )
                })}
              </div>
            </div>
          ))}
        </Card>
      )}

      {kind === 'cross' && (
        <Card padding="sm">
          <p className="mb-3 text-[12px] font-bold text-t1">Row grouping</p>
          <div className="flex flex-wrap items-center gap-2">
            {dimSelect(cross.dim1, (v) => setCross((c) => ({ ...c, dim1: v })))}
            <span className="text-[13px] font-bold text-t2">×</span>
            {dimSelect(cross.dim2, (v) => setCross((c) => ({ ...c, dim2: v, values2: undefined })))}
            <div className="relative" ref={pickRef}>
              <Button type="button" variant="secondary" size="sm" onClick={() => setPicking((o) => !o)}>
                {cross.values2?.length
                  ? `${dimLabel(cross.dim2)}: ${cross.values2.length} pinned`
                  : `${dimLabel(cross.dim2)}: top 4 + Others`}
              </Button>
              {picking && (
                <ValuePicker
                  dim={cross.dim2}
                  selected={cross.values2 || []}
                  onApply={(vals) => { setCross((c) => ({ ...c, values2: vals.length ? vals : undefined })); setPicking(false) }}
                  onClose={() => setPicking(false)}
                />
              )}
            </div>
            <Button type="button" size="sm" disabled={cross.dim1 === cross.dim2} onClick={addCross}>Add table</Button>
          </div>
          {cross.dim1 === cross.dim2 && (
            <p className="mt-2 text-[12px] text-bad">Pick two different dimensions.</p>
          )}
        </Card>
      )}

      <div>
        <p className="mb-2 text-[12.5px] font-bold text-t0">On this report</p>
        {rows.length === 0 && !draft.builtins && (
          <p className={'text-[13px] ' + (warnEmpty ? 'font-semibold text-bad' : 'text-t2')}>
            {warnEmpty
              ? 'This report will have no charts. Add a table or chart, or continue anyway.'
              : 'Nothing yet. Add a table or chart above, or skip and continue.'}
          </p>
        )}
        {draft.builtins && (
          <div className="mb-1.5 rounded-[var(--radius-vela-md)] border border-line bg-bg-2 px-3 py-2 text-[13px] font-semibold text-t0">
            Overview cards · monthly volume, top brands, leaderboard
          </div>
        )}
        {rows.length > 0 && (
          <ul className="space-y-1">
            {rows.map((r) => (
              <li key={r.key} className="flex items-center gap-2 rounded-[var(--radius-vela-md)] border border-line bg-bg-2 px-3 py-2">
                <span className="min-w-0 flex-1 truncate text-[13px] font-semibold text-t0">{r.label}</span>
                <button type="button" onClick={r.remove} aria-label={`Remove ${r.label}`}
                  className="flex h-7 w-7 items-center justify-center rounded-full text-t2 hover:bg-bg-3 hover:text-t0">✕</button>
              </li>
            ))}
          </ul>
        )}
        {hasCards(draft) && rows.length === 0 && draft.builtins && (
          <p className="mt-2 text-[12.5px] text-t2">Add a breakdown if you want more than the overview.</p>
        )}
      </div>
    </div>
  )
}
