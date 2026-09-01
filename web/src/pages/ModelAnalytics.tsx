import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, fmt } from '@/lib/api'
import { Card, CardHeader, CardTitle, CardSubtitle, Button, Badge } from '@/components/ui'
import { MultiLineChart, type LineSeries } from '@/components/charts'
import { ValuePicker } from '@/components/FilterBar'
import { useViews, type ModelViewConfig } from '@/lib/views'

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
const PALETTE = ['var(--acc)', 'var(--info)', 'var(--ok)', 'var(--warn)', 'var(--bad)', 'var(--acc-2)', '#c084fc', '#22d3ee', '#f472b6', '#a3e635', '#fb923c', '#38bdf8']
const MODEL_COLORS = ['var(--acc)', 'var(--info)'] // A, B
const selCls = 'h-9 rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-2.5 text-[13px] text-t0'
const fieldCls = 'h-9 w-full rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-3 text-[13px] text-t0 focus:border-acc'

type Measure = 'total' | 'model_year' | 'engine' | 'region' | 'governorate' | 'traffic_unit'
const MEASURES: [Measure, string][] = [
  ['total', 'Total sales'],
  ['model_year', 'By model year'],
  ['engine', 'By engine'],
  ['region', 'By region'],
  ['governorate', 'By governorate'],
  ['traffic_unit', 'By traffic unit'],
]
// Model year reads as a sequence, not a ranking — order those keys oldest-first
// (Unknown last) instead of by volume, so both the chart legend and the figures
// table below it follow the calendar.
const CHRONOLOGICAL: Partial<Record<Measure, boolean>> = { model_year: true }
const cmpChronological = (a: string, b: string) => {
  const na = Number(a), nb = Number(b)
  if (isNaN(na) !== isNaN(nb)) return isNaN(na) ? 1 : -1 // 'Unknown' last
  return isNaN(na) ? a.localeCompare(b) : na - nb
}
const TOP_N = 8

// The specs compared side by side. Each reads one field off the model detail.
const SPECS: [string, (m: any) => string][] = [
  ['Brand', (m) => m.brand],
  ['Origin', (m) => m.origin],
  ['Car type', (m) => m.car_type],
  ['Segment', (m) => m.segment],
  ['Engine type', (m) => m.engine_type],
  ['Supply', (m) => m.supply],
  ['Distributor', (m) => m.distributor],
]
const val = (v: any) => (v == null || v === '' ? '—' : String(v))

/** "KIA Sportage vs Nissan Sunny (2026)" — or one model, or neither. */
function autoName(a: any, b: any, year: string | number) {
  const n = (m: any) => `${m.brand} ${m.name}`
  const y = year ? ` (${year})` : ''
  if (a && b) return `${n(a)} vs ${n(b)}${y}`
  const one = a || b
  return one ? `${n(one)}${y}` : `Model comparison${y}`
}

export default function ModelAnalytics() {
  const [sp, setSp] = useSearchParams()
  const [years, setYears] = useState<number[]>([])
  const { id } = useParams()
  const viewId = id ? Number(id) : null
  const navigate = useNavigate()
  const { views, loaded: viewsLoaded, create, saveConfig, remove } = useViews()
  const view = viewId != null ? views.find((v) => v.id === viewId) : undefined

  const idA = sp.get('a') ? Number(sp.get('a')) : null
  const idB = sp.get('b') ? Number(sp.get('b')) : null
  const measure = (sp.get('measure') || 'total') as Measure
  const scope = sp.get('vals') ? sp.get('vals')!.split(',').filter(Boolean) : []
  const year = sp.get('year') || ''

  const setParam = (k: string, v: string | null) =>
    setSp((prev) => { const n = new URLSearchParams(prev); if (!v) n.delete(k); else n.set(k, v); return n }, { replace: true })

  useEffect(() => { api.years().then(setYears).catch(() => {}) }, [])
  // Default the year to the newest available once years load.
  useEffect(() => { if (!year && years.length) setParam('year', String(years[0])) }, [years]) // eslint-disable-line
  const resolvedYear = year ? Number(year) : years[0]

  // Saved-view sync state (declared before `ready`, which reads it).
  const [saveStatus, setSaveStatus] = useState<'idle' | 'saving' | 'saved'>('idle')
  const hydratedRef = useRef<number | null>(null)
  const lastSavedRef = useRef('')
  const settledRef = useRef(true)
  // Mirrored as state so gating re-renders: nothing fetches until a saved view's
  // params have actually landed in the URL, otherwise an early request built
  // from the previous page's params can resolve last and win.
  const [settled, setSettled] = useState(() => (id ? false : true))
  const targetRef = useRef<ModelViewConfig | null>(null)

  const [mA, setMA] = useState<any>(null)
  const [mB, setMB] = useState<any>(null)
  const ready = viewId == null || (hydratedRef.current === viewId && settled)
  useEffect(() => { if (!ready) return; if (idA) api.model(idA).then(setMA).catch(() => setMA(null)); else setMA(null) }, [idA, ready])
  useEffect(() => { if (!ready) return; if (idB) api.model(idB).then(setMB).catch(() => setMB(null)); else setMB(null) }, [idB, ready])

  const models = [mA, mB].filter(Boolean)
  // Counts every highlighted row (specs + the active span) so the badge matches
  // what the table actually flags. Total units is excluded — it nearly always
  // differs, so it is not treated as a meaningful difference.
  const spanOf = (m: any) => `${m.first_period || '—'} → ${m.last_period || '—'}`
  const diffCount = mA && mB
    ? SPECS.filter(([, get]) => val(get(mA)) !== val(get(mB))).length +
      (spanOf(mA) !== spanOf(mB) ? 1 : 0)
    : 0

  // ── saved-view sync ─────────────────────────────────────────────────────────

  const cfgKey = (c: ModelViewConfig) => JSON.stringify([c.a, c.b, c.year, c.measure, [...c.vals].sort()])
  const current: ModelViewConfig = { kind: 'model', a: idA, b: idB, year, measure, vals: scope }
  const applyCfg = (c: ModelViewConfig) => {
    const n = new URLSearchParams()
    if (c.a) n.set('a', String(c.a))
    if (c.b) n.set('b', String(c.b))
    if (c.year) n.set('year', String(c.year))
    if (c.measure && c.measure !== 'total') n.set('measure', c.measure)
    if (c.vals?.length) n.set('vals', c.vals.join(','))
    setSp(n, { replace: true })
  }

  // Hydrate on entering a view.
  useEffect(() => {
    if (viewId == null) { hydratedRef.current = null; settledRef.current = true; targetRef.current = null; setSettled(true); return }
    if (!view || hydratedRef.current === viewId) return
    const c = (view.config || {}) as ModelViewConfig
    const t: ModelViewConfig = {
      kind: 'model',
      a: c.a ?? null, b: c.b ?? null,
      year: typeof c.year === 'string' ? c.year : String(c.year ?? ''),
      measure: c.measure || 'total',
      vals: Array.isArray(c.vals) ? c.vals : [],
      custom: c.custom,
    }
    targetRef.current = t
    lastSavedRef.current = cfgKey(t)
    hydratedRef.current = viewId
    settledRef.current = false
    setSettled(false)
    setSaveStatus('saved')
    applyCfg(t)
  }, [viewId, view]) // eslint-disable-line

  // The page remounts on navigation (AppShell keys on pathname) and the incoming
  // URL can still carry the previous page's params — re-assert until it sticks.
  useLayoutEffect(() => {
    const t = targetRef.current
    if (viewId == null || !t || settledRef.current) return
    if (cfgKey(current) === cfgKey(t)) { settledRef.current = true; setSettled(true) }
    else applyCfg(t)
  })

  // Name draft — edited by hand, or regenerated from the selection.
  const [nameDraft, setNameDraft] = useState('')
  useEffect(() => { setNameDraft(view?.name ?? '') }, [view?.name])

  // Nothing saves on its own: changes are staged until Save changes / Save as new.
  const nameEdited = viewId != null && !!view && nameDraft.trim() !== '' && nameDraft !== view.name
  const cfgDirty = viewId != null && settled && cfgKey(current) !== lastSavedRef.current
  const dirty = cfgDirty || nameEdited
  useEffect(() => { if (dirty) setSaveStatus('idle') }, [dirty])

  const saveChanges = async () => {
    if (viewId == null || !view) return
    const wasCustom = (view.config as ModelViewConfig)?.custom
    // A hand-typed name sticks; otherwise keep regenerating it from the selection.
    const custom = wasCustom || nameEdited
    const name = custom ? nameDraft.trim() : autoName(mA, mB, year)
    setSaveStatus('saving')
    try {
      await saveConfig(viewId, { ...current, custom }, name)
      lastSavedRef.current = cfgKey(current)
      setSaveStatus('saved')
    } catch { setSaveStatus('idle') }
  }

  const saveAsView = async () => {
    const name = nameEdited ? nameDraft.trim() : autoName(mA, mB, year)
    try {
      const v = await create(name, { ...current, custom: nameEdited || undefined })
      navigate(`/models/view/${v.id}`)
    } catch { /* surfaced by the global loading bar */ }
  }
  const deleteView = async () => { if (viewId != null) { await remove(viewId); navigate('/models') } }

  if (viewId != null && viewsLoaded && !view) {
    return (
      <Card><div className="grid h-40 place-items-center text-[13px] text-t2">
        View not found.
        <button onClick={() => navigate('/models')} className="ml-1 font-semibold text-acc">Go to Model Comparison</button>
      </div></Card>
    )
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="min-w-0 flex-1 basis-[320px]">
          {viewId != null ? (
            <>
              <input value={nameDraft} aria-label="View name" onChange={(e) => setNameDraft(e.target.value)}
                className="-ml-1 w-full truncate rounded-[var(--radius-vela-sm)] bg-transparent px-1 text-xl font-extrabold text-t0 outline-none hover:bg-bg-3/50 focus:bg-bg-inset sm:text-[26px]" />
              <p className="mt-1 text-[13px] text-t1">Saved comparison
                <span className="ml-2 text-[12px] text-t2">
                  {saveStatus === 'saving' ? 'Saving…' : dirty ? 'Unsaved changes' : saveStatus === 'saved' ? 'All changes saved' : ''}
                </span>
              </p>
            </>
          ) : (
            <>
              <h1 className="text-xl font-extrabold text-t0 sm:text-[26px]">Model Comparison</h1>
              <p className="mt-1 text-[13px] text-t1">Compare two models — specs and monthly sales.</p>
            </>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {viewId != null ? (
            <>
              {dirty && <Button variant="primary" size="sm" onClick={saveChanges}>Save changes</Button>}
              {dirty && <Button variant="secondary" size="sm" onClick={saveAsView}>Save as new view</Button>}
              <Button variant="outline" size="sm" onClick={deleteView}>Delete view</Button>
            </>
          ) : (
            <Button variant="primary" size="sm" disabled={!mA && !mB} onClick={saveAsView}>Save view</Button>
          )}
          <label className="flex items-center gap-2 text-[12px] font-semibold text-t1">
            Year
            <select className={selCls} value={year} onChange={(e) => setParam('year', e.target.value)}>
              {years.map((y) => <option key={y} value={y}>{y}</option>)}
            </select>
          </label>
        </div>
      </div>

      {/* ── Selectors ─────────────────────────────────────────────────────── */}
      <Card>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <ModelPicker label="Main model" color={MODEL_COLORS[0]} model={mA}
            onPick={(m) => setParam('a', String(m.id))} onClear={() => setParam('a', null)} />
          <ModelPicker label="Compare to" color={MODEL_COLORS[1]} model={mB} optional
            onPick={(m) => setParam('b', String(m.id))} onClear={() => setParam('b', null)} />
        </div>
      </Card>

      {models.length === 0 ? (
        <Card><div className="grid h-40 place-items-center text-center text-[13px] text-t2">
          Pick a model above to see its specs and sales.
        </div></Card>
      ) : (
        <>
          {/* ── Specs ─────────────────────────────────────────────────────── */}
          <Card>
            <CardHeader>
              <div><CardTitle>Model details</CardTitle>
                <CardSubtitle>{mA && mB ? 'Side-by-side comparison' : 'Specifications'}</CardSubtitle></div>
              {mA && mB && (
                <Badge variant={diffCount ? 'warning' : 'success'}>
                  {diffCount ? `${diffCount} difference${diffCount > 1 ? 's' : ''}` : 'Identical specs'}
                </Badge>
              )}
            </CardHeader>

            {mA && mB ? (
              <div className="overflow-x-auto">
                <table className="w-full min-w-[520px] border-collapse text-sm">
                  <thead><tr className="border-b border-line">
                    <th className="px-3 py-2.5 text-left text-[10.5px] font-bold uppercase tracking-wide text-t2">Spec</th>
                    {[mA, mB].map((m, i) => (
                      <th key={i} className="px-3 py-2.5 text-left text-[12.5px] font-bold text-t0">
                        <span className="inline-flex items-center gap-1.5">
                          <span className="h-2.5 w-2.5 rounded-full" style={{ background: MODEL_COLORS[i] }} />
                          {m.brand} {m.name}
                        </span>
                      </th>
                    ))}
                  </tr></thead>
                  <tbody>
                    {SPECS.map(([label, get]) => {
                      const a = val(get(mA)), b = val(get(mB))
                      const differs = a !== b
                      return (
                        <tr key={label} className={'border-b border-line last:border-0 ' + (differs ? 'bg-warn-soft' : '')}>
                          <td className="px-3 py-2.5 text-[12.5px] text-t2">{label}</td>
                          <td className={'px-3 py-2.5 text-[13px] ' + (differs ? 'font-bold text-warn' : 'text-t1')}>{a}</td>
                          <td className={'px-3 py-2.5 text-[13px] ' + (differs ? 'font-bold text-warn' : 'text-t1')}>{b}</td>
                        </tr>
                      )
                    })}
                    {(() => {
                      const span = (m: any) => `${m.first_period || '—'} → ${m.last_period || '—'}`
                      const differs = span(mA) !== span(mB)
                      return (
                        <tr className={'border-t-2 border-line ' + (differs ? 'bg-warn-soft' : '')}>
                          <td className="px-3 py-2.5 text-[12.5px] text-t2">Active</td>
                          {[mA, mB].map((m, i) => (
                            <td key={i} className={'whitespace-nowrap px-3 py-2.5 text-[13px] font-bold ' + (differs ? 'text-warn' : 'text-t1')}>
                              {span(m)}
                            </td>
                          ))}
                        </tr>
                      )
                    })()}
                    {/* Total units is never highlighted — two models almost always
                        differ in volume, so flagging it would be noise. */}
                    <tr className="border-t border-line">
                      <td className="px-3 py-2.5 text-[12.5px] text-t2">Total units (all time)</td>
                      {[mA, mB].map((m, i) => (
                        <td key={i} className="px-3 py-2.5 text-[13px] font-bold tabular-nums text-t0">{fmt(m.total_volume)}</td>
                      ))}
                    </tr>
                  </tbody>
                </table>
              </div>
            ) : (
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
                {SPECS.map(([label, get]) => (
                  <div key={label} className="rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-4 py-3">
                    <div className="text-[11px] text-t2">{label}</div>
                    <div className="truncate text-[15px] font-extrabold text-t0">{val(get(models[0]))}</div>
                  </div>
                ))}
                <div className="rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-4 py-3">
                  <div className="text-[11px] text-t2">Total units</div>
                  <div className="text-[15px] font-extrabold text-t0">{fmt(models[0].total_volume)}</div>
                </div>
                <div className="rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-4 py-3">
                  <div className="text-[11px] text-t2">Active</div>
                  <div className="text-[15px] font-extrabold text-t0">{models[0].first_period || '—'} → {models[0].last_period || '—'}</div>
                </div>
              </div>
            )}
          </Card>

          {/* ── Sales graph ───────────────────────────────────────────────── */}
          <SalesGraph models={[mA, mB]} year={resolvedYear} measure={measure} scope={scope} ready={ready}
            onMeasure={(m) => { setSp((prev) => { const n = new URLSearchParams(prev); n.set('measure', m); n.delete('vals'); return n }, { replace: true }) }}
            onScope={(v) => setParam('vals', v.length ? v.join(',') : null)} />
        </>
      )}
    </div>
  )
}

// ── Model picker: searchable brand list → model list ──────────────────────────
function ModelPicker({ label, color, model, optional, onPick, onClear }: {
  label: string; color: string; model: any; optional?: boolean
  onPick: (m: any) => void; onClear: () => void
}) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const h = (e: MouseEvent) => { if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false) }
    document.addEventListener('mousedown', h)
    return () => document.removeEventListener('mousedown', h)
  }, [])

  return (
    <div className="relative" ref={ref}>
      <div className="mb-1.5 flex items-center justify-between">
        <span className="text-[11.5px] font-bold text-t1">
          <span className="mr-1.5 inline-block h-2.5 w-2.5 rounded-full align-middle" style={{ background: color }} />
          {label}{optional && <span className="ml-1 font-normal text-t2">(optional)</span>}
        </span>
        {model && <button onClick={onClear} className="text-[11.5px] font-semibold text-t2 hover:text-t0">Clear</button>}
      </div>
      <button onClick={() => setOpen((o) => !o)}
        className="flex h-[42px] w-full items-center justify-between rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-3.5 text-left text-[13px] text-t0 hover:border-line-2">
        <span className={model ? 'truncate font-semibold' : 'text-t2'}>
          {model ? `${model.brand} ${model.name}` : 'Select a model…'}
        </span>
        <span className="ml-2 shrink-0 text-t2">▾</span>
      </button>
      {open && <BrandModelPanel onPick={(m) => { onPick(m); setOpen(false) }} />}
    </div>
  )
}

function BrandModelPanel({ onPick }: { onPick: (m: any) => void }) {
  const [brands, setBrands] = useState<any[] | null>(null)
  const [brand, setBrand] = useState<any>(null)
  const [models, setModels] = useState<any[] | null>(null)
  const [qb, setQb] = useState('')
  const [qm, setQm] = useState('')

  useEffect(() => { api.brands().then(setBrands).catch(() => setBrands([])) }, [])
  const pickBrand = (b: any) => { setBrand(b); setModels(null); setQm(''); api.brandModels(b.id).then(setModels).catch(() => setModels([])) }

  const shownBrands = (brands || []).filter((b) => b.name.toLowerCase().includes(qb.toLowerCase())).slice(0, 200)
  const shownModels = (models || []).filter((m) => m.name.toLowerCase().includes(qm.toLowerCase())).slice(0, 300)

  return (
    <div className="absolute left-0 right-0 top-full z-50 mt-2 grid grid-cols-2 gap-2 rounded-[var(--radius-vela-md)] border border-line bg-bg-2 p-2.5 shadow-[var(--shadow-vela)]">
      <div>
        <input autoFocus placeholder="Search brand…" value={qb} onChange={(e) => setQb(e.target.value)} className={fieldCls} />
        <div className="mt-1.5 max-h-[260px] overflow-y-auto">
          {brands === null && <div className="p-3 text-center text-[12px] text-t2">Loading…</div>}
          {shownBrands.map((b) => (
            <button key={b.id} onClick={() => pickBrand(b)}
              className={'block w-full truncate rounded-[8px] px-2 py-1.5 text-left text-[13px] hover:bg-bg-3 ' + (brand?.id === b.id ? 'bg-acc-soft text-acc' : 'text-t1')}>
              {b.name}
            </button>
          ))}
          {brands && shownBrands.length === 0 && <div className="p-3 text-center text-[12px] text-t2">No matches</div>}
        </div>
      </div>
      <div>
        <input placeholder="Search model…" value={qm} onChange={(e) => setQm(e.target.value)} className={fieldCls} disabled={!brand} />
        <div className="mt-1.5 max-h-[260px] overflow-y-auto">
          {!brand && <div className="p-3 text-center text-[12px] text-t2">Pick a brand first</div>}
          {brand && models === null && <div className="p-3 text-center text-[12px] text-t2">Loading…</div>}
          {shownModels.map((m) => (
            <button key={m.id} onClick={() => onPick({ ...m, brand: brand.name })}
              className="flex w-full items-center justify-between gap-2 rounded-[8px] px-2 py-1.5 text-left text-[13px] text-t1 hover:bg-bg-3 hover:text-t0">
              <span className="truncate">{m.name}</span>
              <span className="shrink-0 text-[11px] tabular-nums text-t2">{fmt(m.volume)}</span>
            </button>
          ))}
          {brand && models && shownModels.length === 0 && <div className="p-3 text-center text-[12px] text-t2">No models</div>}
        </div>
      </div>
    </div>
  )
}

// ── Sales graph ──────────────────────────────────────────────────────────────
function SalesGraph({ models, year, measure, scope, ready, onMeasure, onScope }: {
  models: (any | null)[]; year: number; measure: Measure; scope: string[]; ready: boolean
  onMeasure: (m: Measure) => void; onScope: (v: string[]) => void
}) {
  const [pickerOpen, setPickerOpen] = useState(false)
  const pickRef = useRef<HTMLDivElement>(null)
  const [data, setData] = useState<Record<number, any[]> | null>(null)
  const present = models.filter(Boolean) as any[]
  const key = present.map((m) => m.id).join(',') + '|' + year + '|' + measure + '|' + scope.join(',')

  useEffect(() => {
    const h = (e: MouseEvent) => { if (pickRef.current && !pickRef.current.contains(e.target as Node)) setPickerOpen(false) }
    document.addEventListener('mousedown', h)
    return () => document.removeEventListener('mousedown', h)
  }, [])

  useEffect(() => {
    if (!ready || !present.length || !year) return
    setData(null)
    const load = async (m: any) => {
      if (measure === 'total') {
        const p = new URLSearchParams({ year: String(year), 'filter.model_id': String(m.id) })
        const pts = await api.timeseries(p.toString())
        const arr = Array(12).fill(0)
        pts.forEach((pt: any) => { if (pt.year === year) arr[pt.month - 1] = Number(pt.volume) })
        return [{ key: 'Total', months: arr }]
      }
      const p = new URLSearchParams({ dimension: measure, year: String(year), 'filter.model_id': String(m.id) })
      if (scope.length) p.set('filter.' + measure, scope.join(','))
      // Fetch the full set (not each model's own top-N) so the two models can be
      // ranked on their COMBINED total and a missing value is a genuine zero.
      p.set('limit', '200')
      const res = await api.matrix(p.toString())
      return (res.rows || []).map((r: any) => ({ key: r.key, months: (r.months || []).map(Number) }))
    }
    Promise.all(present.map((m) => load(m).then((rows) => [m.id, rows] as const)))
      .then((pairs) => {
        let obj: Record<number, any[]> = Object.fromEntries(pairs)
        if (measure !== 'total') {
          // Rank values by combined volume, trim to Top-N unless the user picked
          // explicit values, then give every model the same ordered key set.
          const totals = new Map<string, number>()
          Object.values(obj).forEach((rows) => rows.forEach((r: any) =>
            totals.set(r.key, (totals.get(r.key) || 0) + r.months.reduce((s: number, v: number) => s + v, 0))))
          let keys = [...totals.entries()].sort((a, b) => b[1] - a[1]).map(([k]) => k)
          if (!scope.length) keys = keys.slice(0, TOP_N)
          if (CHRONOLOGICAL[measure]) keys = keys.sort(cmpChronological)
          obj = Object.fromEntries(Object.entries(obj).map(([id, rows]) => {
            const byKey = new Map((rows as any[]).map((r) => [r.key, r]))
            return [Number(id), keys.map((k) => byKey.get(k) || { key: k, months: Array(12).fill(0) })]
          }))
        }
        setData(obj)
      })
      .catch(() => setData({}))
  }, [key, ready]) // eslint-disable-line

  // Per-model graphs (Top-N breakdown) share one Y scale so the two models are
  // visually comparable rather than each auto-scaled to its own peak.
  const globalMax = useMemo(() => {
    if (!data) return undefined
    let mx = 0
    Object.values(data).forEach((rows: any) => rows.forEach((r: any) => r.months.forEach((v: number) => { if (v > mx) mx = v })))
    return mx || undefined
  }, [data])

  const perModelMode = measure !== 'total' && scope.length === 0

  // One colour per dimension VALUE, shared across both models — otherwise each
  // model's rank decides its colours and e.g. Alexandria and Qalioub both come
  // out green in the two per-model graphs.
  const valueColor = useMemo(() => {
    const map = new Map<string, string>()
    if (!data) return map
    const totals = new Map<string, number>()
    Object.values(data).forEach((rows: any) =>
      rows.forEach((r: any) => totals.set(r.key, (totals.get(r.key) || 0) + (r.months || []).reduce((s: number, v: any) => s + Number(v), 0))))
    ;[...totals.entries()].sort((a, b) => b[1] - a[1]).forEach(([k], i) => map.set(k, PALETTE[i % PALETTE.length]))
    return map
  }, [data])
  const colorOf = (k: string) => valueColor.get(k) || PALETTE[0]

  let single: LineSeries[] = []
  if (data && !perModelMode) {
    present.forEach((m, mi) => {
      const rows = data[m.id] || []
      if (measure === 'total') {
        single.push({ name: `${m.brand} ${m.name}`, data: rows[0]?.months || Array(12).fill(0), color: MODEL_COLORS[mi], dashed: mi === 1 })
      } else {
        rows.forEach((r: any) => {
          single.push({ name: `${m.brand} ${m.name} — ${r.key}`, data: r.months, color: colorOf(r.key), dashed: mi === 1 })
        })
      }
    })
  }

  const label = MEASURES.find(([k]) => k === measure)?.[1] || ''
  const dimLabel = label.replace(/^By /, '')

  // Raw figures behind the graph — same fetched data, same filters, no refetch.
  // One row per plotted series so the table mirrors the chart exactly.
  const tableRows = useMemo(() => {
    if (!data) return []
    const out: { model: string; mi: number; key: string; months: number[]; total: number; color: string }[] = []
    present.forEach((m, mi) => {
      (data[m.id] || []).forEach((r: any) => {
        const months: number[] = (r.months || []).map(Number)
        out.push({
          model: `${m.brand} ${m.name}`,
          mi,
          key: r.key,
          months,
          total: months.reduce((s, v) => s + v, 0),
          color: measure === 'total' ? MODEL_COLORS[mi] : colorOf(r.key),
        })
      })
    })
    return out
  }, [data, measure, key]) // eslint-disable-line

  const colTotals = MONTHS.map((_, mi) => tableRows.reduce((s, r) => s + (r.months[mi] || 0), 0))
  const grand = tableRows.reduce((s, r) => s + r.total, 0)
  const showModelCol = present.length > 1
  const showDimCol = measure !== 'total'

  // Any dimension breakdown groups the rows BY VALUE with each model nested
  // underneath, so the two models sit side by side for every value.
  const groupByValue = showDimCol
  const groups = useMemo(() => {
    if (!groupByValue) return []
    const map = new Map<string, typeof tableRows>()
    tableRows.forEach((r) => { const a = map.get(r.key) || []; a.push(r); map.set(r.key, a) })
    return [...map.entries()]
      .map(([k, rows]) => ({
        key: k,
        rows: [...rows].sort((a, b) => a.mi - b.mi), // keep model A above model B
        total: rows.reduce((s, r) => s + r.total, 0),
        months: MONTHS.map((_, mi) => rows.reduce((s, r) => s + (r.months[mi] || 0), 0)),
      }))
      .sort((a, b) => (CHRONOLOGICAL[measure] ? cmpChronological(a.key, b.key) : b.total - a.total))
  }, [tableRows, groupByValue, measure])

  return (
    <Card>
      <CardHeader>
        <div><CardTitle>Monthly sales</CardTitle>
          <CardSubtitle>{label} · {year}{scope.length ? ` · ${scope.length} selected` : measure !== 'total' ? ` · top ${TOP_N}` : ''}</CardSubtitle></div>
        <div className="flex flex-wrap items-center gap-2">
          <select className={selCls} value={measure} onChange={(e) => onMeasure(e.target.value as Measure)}>
            {MEASURES.map(([k, l]) => <option key={k} value={k}>{l}</option>)}
          </select>
          {measure !== 'total' && (
            <div className="relative" ref={pickRef}>
              <Button variant="secondary" size="sm" onClick={() => setPickerOpen((o) => !o)}>
                {scope.length ? `${scope.length} selected` : `Top ${TOP_N}`} ▾
              </Button>
              {pickerOpen && (
                <ValuePicker dim={measure} selected={scope}
                  onApply={(v) => { onScope(v); setPickerOpen(false) }} onClose={() => setPickerOpen(false)} />
              )}
            </div>
          )}
        </div>
      </CardHeader>

      {single.length > 12 && (
        <div className="mb-3 rounded-[var(--radius-vela-md)] border border-warn/40 bg-warn-soft px-3 py-2 text-[12px] text-warn">
          {single.length} lines — narrow the selection for a clearer read.
        </div>
      )}

      {data === null ? <Loading /> : perModelMode ? (
        <div className="space-y-5">
          {present.map((m, mi) => {
            const rows = data[m.id] || []
            const series: LineSeries[] = rows.map((r: any) => ({ name: r.key, data: r.months, color: colorOf(r.key) }))
            return (
              <div key={m.id}>
                <div className="mb-1.5 flex items-center gap-1.5 text-[12.5px] font-bold text-t0">
                  <span className="h-2.5 w-2.5 rounded-full" style={{ background: MODEL_COLORS[mi] }} />
                  {m.brand} {m.name}
                </div>
                {series.length ? <MultiLineChart series={series} labels={MONTHS} height={240} yMax={globalMax} formatValue={(v) => fmt(v)} /> : <Empty />}
              </div>
            )
          })}
        </div>
      ) : single.length ? (
        <MultiLineChart series={single} labels={MONTHS} height={280} formatValue={(v) => fmt(v)} />
      ) : <Empty />}

      {tableRows.length > 0 && (
        <div className="mt-6 border-t border-line pt-4">
          <div className="mb-2.5 text-[12px] font-bold uppercase tracking-wide text-t2">Figures</div>
          <div className="max-h-[840px] overflow-auto">
            <table className="w-full min-w-[820px] border-collapse text-sm">
              <thead>
                <tr className="border-b border-line">
                  {groupByValue && <th className="sticky top-0 z-20 bg-bg-2 px-3 py-2.5 text-left text-[10.5px] font-bold uppercase tracking-wide text-t2">{dimLabel}</th>}
                  {showModelCol && <th className="sticky top-0 z-20 bg-bg-2 px-3 py-2.5 text-left text-[10.5px] font-bold uppercase tracking-wide text-t2">Model</th>}
                  {showDimCol && !groupByValue && <th className="sticky top-0 z-20 bg-bg-2 px-3 py-2.5 text-left text-[10.5px] font-bold uppercase tracking-wide text-t2">{dimLabel}</th>}
                  {!showModelCol && !showDimCol && <th className="sticky top-0 z-20 bg-bg-2 px-3 py-2.5 text-left text-[10.5px] font-bold uppercase tracking-wide text-t2">Series</th>}
                  {MONTHS.map((m) => (
                    <th key={m} className="sticky top-0 z-20 bg-bg-2 px-2.5 py-2.5 text-right text-[10.5px] font-bold uppercase tracking-wide text-t2">{m}</th>
                  ))}
                  <th className="sticky top-0 z-20 bg-bg-2 px-3 py-2.5 text-right text-[10.5px] font-bold uppercase tracking-wide text-t2">Total</th>
                </tr>
              </thead>
              <tbody>
                {groupByValue && groups.map((g) => g.rows.map((r, ri) => (
                  <tr key={g.key + '|' + r.model}
                    className={'hover:bg-bg-3 ' + (ri === g.rows.length - 1 ? 'border-b border-line' : '')}>
                    {ri === 0 && (
                      <td rowSpan={g.rows.length} className="border-r border-line px-3 py-2 align-middle" dir="auto">
                        <div className="flex items-center gap-1.5 font-semibold text-t0">
                          <span className="h-2.5 w-2.5 shrink-0 rounded-full" style={{ background: colorOf(g.key) }} />
                          {g.key}
                        </div>
                        {g.rows.length > 1 && <div className="mt-0.5 pl-4 text-[11px] tabular-nums text-t2">{fmt(g.total)} total</div>}
                      </td>
                    )}
                    {showModelCol && (
                      <td className="whitespace-nowrap px-3 py-2 text-[12.5px] font-semibold text-t1">
                        <span className="mr-1.5 inline-block h-0.5 w-3.5 align-middle"
                          style={{ background: r.mi === 1 ? `repeating-linear-gradient(90deg, ${r.color} 0 3px, transparent 3px 6px)` : r.color }} />
                        {r.model}
                      </td>
                    )}
                    {MONTHS.map((_, mi) => {
                      const v = r.months[mi] || 0
                      return <td key={mi} className="px-2.5 py-2 text-right tabular-nums text-t1">{v ? fmt(v) : <span className="text-t2">–</span>}</td>
                    })}
                    <td className="px-3 py-2 text-right font-bold tabular-nums text-t0">{fmt(r.total)}</td>
                  </tr>
                )))}
                {!groupByValue && tableRows.map((r, i) => (
                  <tr key={i} className="border-b border-line last:border-0 hover:bg-bg-3">
                    {showModelCol && (
                      <td className="whitespace-nowrap px-3 py-2 font-semibold text-t0">
                        {!showDimCol && <span className="mr-1.5 inline-block h-2 w-2 rounded-full align-middle" style={{ background: r.color }} />}
                        {r.model}
                      </td>
                    )}
                    {showDimCol && (
                      <td className="px-3 py-2 text-t1" dir="auto">
                        <span className="mr-1.5 inline-block h-2 w-2 rounded-full align-middle" style={{ background: r.color }} />
                        {r.key}
                      </td>
                    )}
                    {!showModelCol && !showDimCol && (
                      <td className="px-3 py-2 font-semibold text-t0">
                        <span className="mr-1.5 inline-block h-2 w-2 rounded-full align-middle" style={{ background: r.color }} />
                        {r.model}
                      </td>
                    )}
                    {MONTHS.map((_, mi) => {
                      const v = r.months[mi] || 0
                      return <td key={mi} className="px-2.5 py-2 text-right tabular-nums text-t1">{v ? fmt(v) : <span className="text-t2">–</span>}</td>
                    })}
                    <td className="px-3 py-2 text-right font-bold tabular-nums text-t0">{fmt(r.total)}</td>
                  </tr>
                ))}
              </tbody>
              <tfoot>
                <tr className="border-t-2 border-line bg-bg-inset">
                  <td className="px-3 py-2 font-bold text-t0" colSpan={(groupByValue ? 1 : 0) + (showModelCol ? 1 : 0) + (showDimCol && !groupByValue ? 1 : 0) || 1}>Total</td>
                  {colTotals.map((v, mi) => (
                    <td key={mi} className="px-2.5 py-2 text-right font-bold tabular-nums text-t0">{fmt(v)}</td>
                  ))}
                  <td className="px-3 py-2 text-right font-extrabold tabular-nums text-t0">{fmt(grand)}</td>
                </tr>
              </tfoot>
            </table>
          </div>
        </div>
      )}
    </Card>
  )
}

function Loading() { return <div className="grid h-40 place-items-center text-[13px] text-t2">Loading…</div> }
function Empty() { return <div className="grid h-40 place-items-center text-[13px] text-t2">No data</div> }
