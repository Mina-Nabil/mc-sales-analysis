import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, fmt } from '@/lib/api'
import { Card, CardHeader, CardTitle, CardSubtitle, Badge, Button, Modal, Input } from '@/components/ui'
import { AreaLineChart, BarChart, DonutChart, StackedBarChart } from '@/components/charts'
import { FilterBar, useFilters, FILTER_DIMS, PREFIX } from '@/components/FilterBar'
import { useViews, viewKind, type ViewConfig } from '@/lib/views'

const MONTHS = ['', 'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
const PALETTE = ['var(--acc)', 'var(--info)', 'var(--ok)', 'var(--warn)', 'var(--bad)', 'var(--acc-2)', '#c084fc', '#22d3ee', '#f472b6', '#a3e635', '#fb923c', '#38bdf8', '#818cf8']
const OTHERS_COLOR = 'var(--t2)'
const selCls = 'h-9 rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-2.5 text-[13px] text-t0'

const labelOf = (k: string) => FILTER_DIMS.find((d) => d[0] === k)?.[1] || k

// Dimension picker grouped by the spec's owner (CLAUDE.md §"design rules").
const DIM_GROUPS: [string, string[]][] = [
  ['Brand', ['brand', 'origin', 'distributor']],
  ['Model', ['model', 'segment', 'car_type', 'engine', 'supply']],
  ['Facts', ['governorate', 'traffic_unit', 'region', 'model_year']],
]

const PIES_KEY = 'mc.analytics.pies'
const BARS_KEY = 'mc.analytics.bars'
const TABLES_KEY = 'mc.analytics.tables'
const CROSSES_KEY = 'mc.analytics.crosses'
// A 2-dimension table is stored as "rowDim|colDim" — one string, so it rides
// the same add/remove/persist path as every other card.
const CROSS_SEP = '|'
const splitCross = (c: string): [string, string] => {
  const [a, b] = c.split(CROSS_SEP)
  return [a || 'brand', b || 'region']
}
const loadSaved = (key: string, fallback: string[]): string[] => {
  try {
    const raw = localStorage.getItem(key)
    if (raw) { const p = JSON.parse(raw); if (Array.isArray(p) && p.every((x) => typeof x === 'string')) return p }
  } catch { /* ignore malformed */ }
  return fallback
}

export default function Analytics() {
  const [sp, setSp] = useSearchParams()
  const { filters, excludes, qs: filterQsFn } = useFilters()
  const { id } = useParams()
  const viewId = id ? Number(id) : null
  const navigate = useNavigate()
  const { views, create, rename, saveConfig, remove, loaded: viewsLoaded } = useViews()
  const view = viewId != null ? views.find((v) => v.id === viewId) : undefined

  const [years, setYears] = useState<number[]>([])
  const [brands, setBrands] = useState<any[]>([])
  const [ts, setTs] = useState<any[]>([])

  // Dynamic chart set. In scratch mode (/analytics) it persists to localStorage;
  // in view mode (/analytics/view/:id) it hydrates from + auto-saves to the view.
  const [pies, setPies] = useState<string[]>(() => loadSaved(PIES_KEY, ['segment']))
  const [bars, setBars] = useState<string[]>(() => loadSaved(BARS_KEY, []))
  const [tables, setTables] = useState<string[]>(() => loadSaved(TABLES_KEY, []))
  const [crosses, setCrosses] = useState<string[]>(() => loadSaved(CROSSES_KEY, []))
  // Built-in cards (monthly volume, top brands, leaderboard). An "Empty New
  // Report" starts with them off; every other view keeps them.
  const [builtins, setBuiltins] = useState(true)
  const [name, setName] = useState('')
  const [saveStatus, setSaveStatus] = useState<'idle' | 'saving' | 'saved'>('idle')
  const [nameModal, setNameModal] = useState(false)
  const [newName, setNewName] = useState('')
  const hydratedRef = useRef<number | null>(null)
  const lastSavedRef = useRef<string>('')
  const settledRef = useRef(true) // false while the URL is being forced to a just-loaded view
  const targetRef = useRef<{ f: Record<string, string[]>; y: string; x: Record<string, string[]> } | null>(null)
  // Mirrored as state so gating re-renders: nothing may fetch until a view's
  // filters/year have actually landed in the URL, otherwise an early request
  // built from the previous page's params can resolve last and win.
  const [settled, setSettled] = useState(() => (id ? false : true))

  const year = sp.get('year') || 'all'
  const setYear = (v: string) => setSp((prev) => { const n = new URLSearchParams(prev); if (v === 'all') n.delete('year'); else n.set('year', v); return n }, { replace: true })

  const qs = useMemo(() => {
    const p = new URLSearchParams()
    for (const d in filters) p.set(PREFIX.include + d, filters[d].join(','))
    for (const d in excludes) p.set(PREFIX.exclude + d, excludes[d].join(','))
    if (year !== 'all') p.set('year', year)
    return p.toString()
  }, [JSON.stringify(filters), JSON.stringify(excludes), year]) // eslint-disable-line

  // Bar charts are month-of-year, so they need a concrete year — the selected
  // one, or the latest available when "All time" is chosen.
  const filterQs = filterQsFn()
  const resolvedYear = year !== 'all' ? Number(year) : (years[0] || new Date().getFullYear())

  // Nothing fetches until a saved view's filters/year have landed in the URL.
  const ready = viewId == null || (hydratedRef.current === viewId && settled)

  useEffect(() => { api.years().then(setYears).catch(() => {}) }, [])
  useEffect(() => {
    if (!ready) return
    api.agg('brand', qs).then(setBrands).catch(() => setBrands([]))
    api.timeseries(qs).then(setTs).catch(() => setTs([]))
  }, [qs, ready])

  const addPie = (dim: string) => setPies((p) => (p.includes(dim) ? p : [...p, dim]))
  const removePie = (dim: string) => setPies((p) => p.filter((d) => d !== dim))
  const addBar = (dim: string) => setBars((b) => (b.includes(dim) ? b : [...b, dim]))
  const removeBar = (dim: string) => setBars((b) => b.filter((d) => d !== dim))
  const addTable = (dim: string) => setTables((t) => (t.includes(dim) ? t : [...t, dim]))
  const removeTable = (dim: string) => setTables((t) => t.filter((d) => d !== dim))
  const addCross = () => setCrosses((c) => [...c, `brand${CROSS_SEP}region`])
  const removeCross = (i: number) => setCrosses((c) => c.filter((_, n) => n !== i))
  // Both axes are editable in place, so a table can be re-pointed without
  // removing it — the same dynamic feel as Model Comparison's measure picker.
  const setCrossDim = (i: number, axis: 0 | 1, dim: string) =>
    setCrosses((c) => c.map((v, n) => {
      if (n !== i) return v
      const pair = splitCross(v)
      pair[axis] = dim
      return pair.join(CROSS_SEP)
    }))

  // ── saved-view sync ─────────────────────────────────────────────────────────
  const sanitize = (f: Record<string, string[]>) =>
    Object.fromEntries(Object.entries(f).filter(([, v]) => Array.isArray(v) && v.length))
  const flat = (f: Record<string, string[]>) =>
    Object.keys(f).sort().map((k) => k + '=' + [...f[k]].sort().join(',')).join('&')
  const keyOf = (b: string[], p: string[], t: string[], f: Record<string, string[]>, y: string, bi = true, x: Record<string, string[]> = {}, c: string[] = []) =>
    JSON.stringify({ b, p, t, f: flat(f), y, bi, x: flat(x), c })
  const applyToUrl = (f: Record<string, string[]>, y: string, x: Record<string, string[]> = {}) => {
    const n = new URLSearchParams()
    for (const d in f) if (f[d]?.length) n.set(PREFIX.include + d, f[d].join(','))
    for (const d in x) if (x[d]?.length) n.set(PREFIX.exclude + d, x[d].join(','))
    if (y && y !== 'all') n.set('year', y)
    setSp(n, { replace: true })
  }

  // Hydrate when entering a view; reset to the local working set for scratch.
  useEffect(() => {
    if (viewId == null) {
      hydratedRef.current = null; settledRef.current = true; targetRef.current = null; setSettled(true)
      setBars(loadSaved(BARS_KEY, [])); setPies(loadSaved(PIES_KEY, ['segment'])); setTables(loadSaved(TABLES_KEY, []))
      setCrosses(loadSaved(CROSSES_KEY, []))
      setBuiltins(true)
      return
    }
    if (!view || hydratedRef.current === viewId) return
    const cfg = (view.config || {}) as ViewConfig
    const b = Array.isArray(cfg.bars) ? cfg.bars : []
    const p = Array.isArray(cfg.pies) ? cfg.pies : []
    const t = Array.isArray(cfg.tables) ? cfg.tables : []
    const f = sanitize(cfg.filters && typeof cfg.filters === 'object' ? cfg.filters : {})
    const x = sanitize(cfg.excludes && typeof cfg.excludes === 'object' ? cfg.excludes : {})
    const y = typeof cfg.year === 'string' ? cfg.year : 'all'
    const bi = cfg.builtins !== false
    const c = Array.isArray(cfg.crosses) ? cfg.crosses : []
    setBars(b); setPies(p); setTables(t); setCrosses(c); setBuiltins(bi); setName(view.name)
    lastSavedRef.current = keyOf(b, p, t, f, y, bi, x, c)
    hydratedRef.current = viewId
    targetRef.current = { f, y, x }
    settledRef.current = false // keep forcing the URL to this view until it sticks
    setSettled(false)
    setSaveStatus('saved')
    applyToUrl(f, y, x)
  }, [viewId, view]) // eslint-disable-line

  // Force the URL year/filters to the loaded view until they match — the page
  // remounts on navigation (AppShell keys on pathname) and the incoming URL may
  // briefly carry the previous page's year, so re-assert until settled.
  useLayoutEffect(() => {
    const tgt = targetRef.current
    if (viewId == null || tgt == null || settledRef.current) return
    const cur = keyOf([], [], [], sanitize(filters), year, true, sanitize(excludes))
    if (cur === keyOf([], [], [], tgt.f, tgt.y, true, tgt.x)) { settledRef.current = true; setSettled(true) }
    else applyToUrl(tgt.f, tgt.y, tgt.x)
  }) // runs every render until settled

  // Nothing saves on its own: changes are staged until Save changes / Save as new.
  const nameEdited = viewId != null && !!view && name.trim() !== '' && name !== view.name
  const cfgDirty = viewId != null && settled &&
    keyOf(bars, pies, tables, sanitize(filters), year, builtins, sanitize(excludes), crosses) !== lastSavedRef.current
  const dirty = cfgDirty || nameEdited
  useEffect(() => { if (dirty) setSaveStatus('idle') }, [dirty])

  const saveChanges = async () => {
    if (viewId == null || !view) return
    const f = sanitize(filters)
    const x = sanitize(excludes)
    const k = keyOf(bars, pies, tables, f, year, builtins, x, crosses)
    setSaveStatus('saving')
    try {
      await saveConfig(viewId, { bars, pies, tables, crosses, filters: f, excludes: x, year, builtins })
      if (nameEdited) await rename(viewId, name.trim())
      lastSavedRef.current = k
      setSaveStatus('saved')
    } catch { setSaveStatus('idle') }
  }

  // Scratch mode: keep the local working set across reloads.
  useEffect(() => {
    if (viewId != null) return
    localStorage.setItem(BARS_KEY, JSON.stringify(bars))
    localStorage.setItem(PIES_KEY, JSON.stringify(pies))
    localStorage.setItem(TABLES_KEY, JSON.stringify(tables))
    localStorage.setItem(CROSSES_KEY, JSON.stringify(crosses))
  }, [viewId, bars, pies, tables, crosses])

  const saveAsNew = async () => {
    const nm = newName.trim(); if (!nm) return
    const cfg: ViewConfig = { bars, pies, tables, crosses, filters: sanitize(filters), excludes: sanitize(excludes), year, builtins }
    try { const v = await create(nm, cfg); setNameModal(false); setNewName(''); navigate(`/analytics/view/${v.id}`) }
    catch { /* surfaced by the global loading bar */ }
  }
  const deleteView = async () => { if (viewId != null) { await remove(viewId); navigate('/analytics') } }

  const label = year === 'all' ? 'All time' : year

  const series = useMemo(() => {
    if (year === 'all') return { data: ts.map((p) => Number(p.volume)), labels: ts.map((p) => `${MONTHS[p.month]} ${String(p.year).slice(2)}`) }
    const byMonth = new Map<number, number>()
    ts.forEach((p) => byMonth.set(p.month, Number(p.volume)))
    const data: number[] = [], labels: string[] = []
    for (let m = 1; m <= 12; m++) { data.push(byMonth.get(m) || 0); labels.push(MONTHS[m]) }
    return { data, labels }
  }, [ts, year])

  // With a single brand in scope (e.g. a brand filter), the brand charts add
  // nothing — hide Top brands + the leaderboard.
  const multiBrand = brands.filter((b) => Number(b.volume) > 0).length > 1
  const topBrands = brands.filter((b) => b.volume > 0).slice(0, 8).map((b, i) => ({ label: b.key, value: Number(b.volume), color: PALETTE[i % PALETTE.length] }))
  const latest = series.data[series.data.length - 1] || 0
  const prev = series.data[series.data.length - 2] || 0
  const mom = prev ? ((latest - prev) / prev) * 100 : 0

  if (viewId != null && viewsLoaded && !view) {
    return (
      <Card><div className="grid h-40 place-items-center text-[13px] text-t2">
        View not found.
        <button onClick={() => navigate('/analytics')} className="ml-1 font-semibold text-acc">Go to Main Sales Report</button>
      </div></Card>
    )
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="min-w-0 flex-1 basis-[320px]">
          {viewId != null ? (
            <>
              <input value={name} onChange={(e) => setName(e.target.value)} placeholder="View name" aria-label="View name"
                className="-ml-1 w-full truncate rounded-[var(--radius-vela-sm)] bg-transparent px-1 text-xl font-extrabold text-t0 outline-none hover:bg-bg-3/50 focus:bg-bg-inset sm:text-[26px]" />
              <p className="mt-1 flex items-center gap-2 text-[13px] text-t1">
                Saved view
                <span className="text-t2">·</span>
                <span className="text-[12px] text-t2">
                  {saveStatus === 'saving' ? 'Saving…' : dirty ? 'Unsaved changes' : saveStatus === 'saved' ? 'All changes saved' : ''}
                </span>
              </p>
            </>
          ) : (
            <>
              <h1 className="text-xl font-extrabold text-t0 sm:text-[26px]">Main Sales Report</h1>
              <p className="mt-1 text-[13px] text-t1">Registrations trend, brand &amp; segment breakdowns.</p>
            </>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <AddMenu label="＋ Add table" used={tables} onPick={addTable} />
          <Button variant="secondary" size="sm" onClick={addCross}>＋ Add 2-dimension table</Button>
          <AddMenu label="＋ Add bar chart" used={bars} onPick={addBar} />
          <AddMenu label="＋ Add pie chart" used={pies} onPick={addPie} />
          {viewId != null ? (
            <>
              {dirty && <Button variant="primary" size="sm" onClick={saveChanges}>Save changes</Button>}
              {dirty && <Button variant="secondary" size="sm" onClick={() => { setNewName(name); setNameModal(true) }}>Save as new view</Button>}
              <Button variant="outline" size="sm" onClick={deleteView}>Delete view</Button>
            </>
          ) : (
            <Button variant="primary" size="sm" onClick={() => { setNewName(''); setNameModal(true) }}>Save as new view</Button>
          )}
          <label className="flex items-center gap-2 text-[12px] font-semibold text-t1">
            Year
            <select className={selCls} value={year} onChange={(e) => setYear(e.target.value)}>
              <option value="all">All time</option>
              {years.map((y) => <option key={y} value={y}>{y}</option>)}
            </select>
          </label>
        </div>
      </div>

      <Card padding="sm">
        <div className="space-y-2">
          <FilterBar />
          <div className="border-t border-bad/25 pt-2"><FilterBar mode="exclude" /></div>
        </div>
      </Card>

      {/* 1 — monthly volume, full width */}
      {builtins && (
      <Card>
        <CardHeader>
          <div><CardTitle>{year === 'all' ? 'Monthly volume' : `Monthly volume · ${year}`}</CardTitle>
            <CardSubtitle>New-car units per {year === 'all' ? 'committed period' : 'month'}</CardSubtitle></div>
          {series.data.length > 1 && year === 'all' && (
            <Badge variant={mom >= 0 ? 'success' : 'danger'}>{mom >= 0 ? '↗' : '↘'} {Math.abs(mom).toFixed(1)}% MoM</Badge>
          )}
        </CardHeader>
        {series.data.some((v) => v > 0)
          ? <AreaLineChart data={series.data} labels={series.labels} height={260} formatValue={(v) => fmt(v)} />
          : <Empty />}
      </Card>
      )}

      {/* 2 — pie charts */}
      {pies.length > 0 && (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          {pies.map((dim) => (
            <PieCard key={dim} dimension={dim} label={labelOf(dim)} yearLabel={label} qs={qs} ready={ready} onRemove={() => removePie(dim)} />
          ))}
        </div>
      )}

      {/* 3 — bar charts */}
      {bars.length > 0 && (
        <div className="space-y-4">
          {bars.map((dim) => (
            <BarCard key={dim} dimension={dim} label={labelOf(dim)} year={resolvedYear}
              yearNote={year === 'all' ? ' (latest)' : ''} filterQs={filterQs} ready={ready} onRemove={() => removeBar(dim)} />
          ))}
        </div>
      )}

      {/* 4 — data tables */}
      {tables.length > 0 && (
        <div className="space-y-4">
          {tables.map((dim) => (
            <TableCard key={dim} dimension={dim} label={labelOf(dim)} year={resolvedYear}
              yearNote={year === 'all' ? ' (latest)' : ''} filterQs={filterQs} ready={ready} onRemove={() => removeTable(dim)} />
          ))}
        </div>
      )}

      {/* 5 — two-dimension pivots */}
      {crosses.length > 0 && (
        <div className="space-y-4">
          {crosses.map((c, i) => {
            const [d1, d2] = splitCross(c)
            return (
              <CrossCard key={`${c}-${i}`} dim1={d1} dim2={d2} yearLabel={label} qs={qs} ready={ready}
                onPick={(axis, dim) => setCrossDim(i, axis, dim)} onRemove={() => removeCross(i)} />
            )
          })}
        </div>
      )}

      {pies.length === 0 && bars.length === 0 && tables.length === 0 && crosses.length === 0 && !builtins && (
        <Card><div className="flex min-h-[160px] flex-col items-center justify-center gap-3 px-6 py-8 text-center">
          <p className="text-[13.5px] font-semibold text-t1">This report is empty.</p>
          {/* The option names are chips, not inline words: as buttons were added
              they wrapped one-per-line and left stray commas mid-sentence. */}
          <div className="flex flex-wrap items-center justify-center gap-2">
            {['＋ Add table', '＋ Add 2-dimension table', '＋ Add bar chart', '＋ Add pie chart'].map((t) => (
              <span key={t} className="rounded-full border border-line bg-bg-inset px-2.5 py-1 text-[12px] font-semibold text-t1">{t}</span>
            ))}
          </div>
          <p className="text-[12.5px] text-t2">Pick one above to add a breakdown.</p>
        </div></Card>
      )}

      {builtins && multiBrand && (
        <Card>
          <CardHeader><div><CardTitle>Top brands</CardTitle><CardSubtitle>By units · {label}</CardSubtitle></div></CardHeader>
          {topBrands.length ? <BarChart data={topBrands} height={240} formatValue={(v) => fmt(v)} /> : <Empty />}
        </Card>
      )}

      {builtins && multiBrand && (
        <Card>
          <CardHeader><div><CardTitle>Brand leaderboard</CardTitle><CardSubtitle>Top 30 by volume · {label}</CardSubtitle></div></CardHeader>
          <div className="space-y-2.5">
            {brands.slice(0, 30).map((b, i) => {
              const max = Number(brands[0]?.volume) || 1
              return (
                <div key={b.key} className="flex items-center gap-3">
                  <span className="w-5 text-right text-[12px] font-bold text-t2">{i + 1}</span>
                  <span className="w-28 shrink-0 truncate text-[13px] font-semibold text-t0">{b.key}</span>
                  <div className="h-2.5 flex-1 overflow-hidden rounded-full bg-bg-3">
                    <div className="h-full rounded-full" style={{ width: `${(Number(b.volume) / max) * 100}%`, background: PALETTE[i % PALETTE.length] }} />
                  </div>
                  <span className="w-20 text-right text-[12.5px] font-bold tabular-nums text-t1">{fmt(b.volume)}</span>
                </div>
              )
            })}
          </div>
        </Card>
      )}

      {/* Saved report pages — listed (and deletable) from the main report. */}
      {viewId == null && (() => {
        const mine = views.filter((v) => viewKind(v) === 'brand')
        if (!mine.length) return null
        return (
          <Card>
            <CardHeader><div><CardTitle>Saved reports</CardTitle>
              <CardSubtitle>{mine.length} report page{mine.length === 1 ? '' : 's'} in the sidebar</CardSubtitle></div></CardHeader>
            <div className="space-y-1">
              {mine.map((v) => (
                <div key={v.id} className="flex items-center gap-3 rounded-[var(--radius-vela-md)] px-2 py-1.5 hover:bg-bg-3">
                  <button onClick={() => navigate(`/analytics/view/${v.id}`)}
                    className="min-w-0 flex-1 truncate text-left text-[13px] font-semibold text-t0 hover:text-acc">{v.name}</button>
                  <Button variant="outline" size="sm" onClick={() => remove(v.id)}>Delete</Button>
                </div>
              ))}
            </div>
          </Card>
        )
      })()}

      <Modal open={nameModal} onClose={() => setNameModal(false)} title="Save as new view" size="sm"
        footer={<><Button variant="ghost" onClick={() => setNameModal(false)}>Cancel</Button>
          <Button disabled={!newName.trim()} onClick={saveAsNew}>Create view</Button></>}>
        <p className="mb-3 text-[12.5px] text-t1">Saves the current charts, filters, and year as a named view in the sidebar.</p>
        <Input autoFocus placeholder="e.g. China SUVs — 2026" value={newName}
          onChange={(e) => setNewName(e.target.value)} onKeyDown={(e) => { if (e.key === 'Enter') saveAsNew() }} />
      </Modal>
    </div>
  )
}

// AddMenu — a header button that opens the owner-grouped dimension picker.
function AddMenu({ label, used, onPick }: { label: string; used: string[]; onPick: (dim: string) => void }) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const onClick = (e: MouseEvent) => { if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false) }
    document.addEventListener('mousedown', onClick)
    return () => document.removeEventListener('mousedown', onClick)
  }, [])
  return (
    <div className="relative" ref={ref}>
      <Button variant="secondary" size="sm" onClick={() => setOpen((o) => !o)}>{label}</Button>
      {open && (
        <div className="absolute right-0 top-full z-50 mt-2 w-[300px] rounded-[var(--radius-vela-md)] border border-line bg-bg-2 p-2 shadow-[var(--shadow-vela)]">
          {DIM_GROUPS.map(([group, dims]) => (
            <div key={group} className="mb-1.5 last:mb-0">
              <div className="px-2 py-1 text-[10.5px] font-bold uppercase tracking-wide text-t2">{group}</div>
              <div className="grid grid-cols-2 gap-1">
                {dims.map((k) => {
                  const isUsed = used.includes(k)
                  return (
                    <button key={k} disabled={isUsed} onClick={() => { onPick(k); setOpen(false) }}
                      className={'rounded-[9px] px-2.5 py-1.5 text-left text-[13px] ' + (isUsed ? 'cursor-not-allowed text-t2/50' : 'text-t1 hover:bg-bg-3 hover:text-t0')}>
                      {labelOf(k)}{isUsed && ' ✓'}
                    </button>
                  )
                })}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// PieCard — a donut breakdown of one dimension over the filtered set.
function PieCard({ dimension, label, yearLabel, qs, ready, onRemove }: { dimension: string; label: string; yearLabel: string; qs: string; ready: boolean; onRemove: () => void }) {
  const [buckets, setBuckets] = useState<any[] | null>(null)
  useEffect(() => {
    if (!ready) return
    setBuckets(null)
    api.agg(dimension, qs).then(setBuckets).catch(() => setBuckets([]))
  }, [dimension, qs, ready])

  const segments = useMemo(() => {
    const rows = (buckets || []).filter((b) => Number(b.volume) > 0)
    const top = rows.slice(0, 12).map((b, i) => ({ label: b.key, value: Number(b.volume), color: PALETTE[i % PALETTE.length] }))
    const rest = rows.slice(12).reduce((sum, b) => sum + Number(b.volume), 0)
    if (rest > 0) top.push({ label: 'Others', value: rest, color: OTHERS_COLOR })
    return top
  }, [buckets])

  return (
    <Card>
      <CardHeader>
        <div><CardTitle>{label} mix</CardTitle><CardSubtitle>Share by {label.toLowerCase()} · {yearLabel}</CardSubtitle></div>
        <RemoveBtn label={label} onClick={onRemove} />
      </CardHeader>
      {buckets === null
        ? <Loading />
        : segments.length ? <DonutChart segments={segments} size={170} formatValue={(v) => fmt(v)} /> : <Empty />}
    </Card>
  )
}

// BarCard — a stacked monthly bar chart of one dimension for a single year.
// X = months of the year; each bar is stacked by the dimension's top values.
function BarCard({ dimension, label, year, yearNote, filterQs, ready, onRemove }: { dimension: string; label: string; year: number; yearNote: string; filterQs: string; ready: boolean; onRemove: () => void }) {
  const [res, setRes] = useState<any>(null)
  useEffect(() => {
    if (!ready) return
    setRes(null)
    const p = new URLSearchParams(filterQs)
    p.set('dimension', dimension)
    p.set('year', String(year))
    api.matrix(p.toString()).then(setRes).catch(() => setRes({ rows: [] }))
  }, [dimension, year, filterQs, ready])

  const { data, keys, colors } = useMemo(() => {
    const rows = [...((res?.rows) || [])].sort((a: any, b: any) => Number(b.total) - Number(a.total))
    const top = rows.slice(0, 12)
    const rest = rows.slice(12)
    const hasOthers = rest.some((r: any) => Number(r.total) > 0)
    const data = MONTHS.slice(1).map((mLabel, mi) => {
      const rec: Record<string, number | string> = { label: mLabel }
      top.forEach((r: any) => { rec[r.key] = Number(r.months?.[mi] || 0) })
      if (hasOthers) rec['Others'] = rest.reduce((s: number, r: any) => s + Number(r.months?.[mi] || 0), 0)
      return rec
    })
    const keys = [...top.map((r: any) => r.key), ...(hasOthers ? ['Others'] : [])]
    const colors = keys.map((k, i) => (k === 'Others' ? OTHERS_COLOR : PALETTE[i % PALETTE.length]))
    return { data, keys, colors }
  }, [res])

  const hasData = data.some((d) => keys.some((k) => Number(d[k]) > 0))

  return (
    <Card>
      <CardHeader>
        <div><CardTitle>{label} by month</CardTitle><CardSubtitle>Units per month, stacked by {label.toLowerCase()} · {year}{yearNote}</CardSubtitle></div>
        <RemoveBtn label={label} onClick={onRemove} />
      </CardHeader>
      {res === null ? <Loading /> : hasData ? (
        <>
          <StackedBarChart data={data} keys={keys} colors={colors} height={260} formatValue={(v) => fmt(v)} />
          <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1.5">
            {keys.map((k, i) => (
              <div key={k} className="flex items-center gap-1.5 text-[12px]">
                <span className="h-2.5 w-2.5 shrink-0 rounded-full" style={{ background: colors[i] }} />
                <span className="text-t1">{k}</span>
              </div>
            ))}
          </div>
        </>
      ) : <Empty />}
    </Card>
  )
}

// TableCard — raw monthly numbers for one dimension in a single year.
// Rows = dimension values (sorted by total), columns = Jan–Dec + Total, with a
// column-total footer.
function TableCard({ dimension, label, year, yearNote, filterQs, ready, onRemove }: { dimension: string; label: string; year: number; yearNote: string; filterQs: string; ready: boolean; onRemove: () => void }) {
  const [res, setRes] = useState<any>(null)
  useEffect(() => {
    if (!ready) return
    setRes(null)
    const p = new URLSearchParams(filterQs)
    p.set('dimension', dimension); p.set('year', String(year))
    api.matrix(p.toString()).then(setRes).catch(() => setRes({ rows: [] }))
  }, [dimension, year, filterQs, ready])

  const rows = useMemo(() => [...((res?.rows) || [])].sort((a: any, b: any) => Number(b.total) - Number(a.total)), [res])
  const monthTotals = MONTHS.slice(1).map((_, mi) => rows.reduce((s: number, r: any) => s + Number(r.months?.[mi] || 0), 0))
  const grand = rows.reduce((s: number, r: any) => s + Number(r.total || 0), 0)

  const stickyHead = 'sticky left-0 z-20 bg-bg-2 px-4 py-3 text-left text-[10.5px] font-bold uppercase tracking-wide text-t2'
  const stickyCell = 'sticky left-0 z-10 bg-bg-2 px-4 py-2.5 font-semibold text-t0'

  return (
    <Card padding="none" className="overflow-hidden">
      <div className="flex items-center justify-between border-b border-line px-5 py-4">
        <div><CardTitle>{label} — monthly figures</CardTitle><CardSubtitle>Units by month · {year}{yearNote}</CardSubtitle></div>
        <RemoveBtn label={label} onClick={onRemove} />
      </div>
      {res === null ? <Loading /> : rows.length ? (
        <div className="max-h-[520px] overflow-auto">
          <table className="w-full min-w-[820px] border-collapse text-sm">
            <thead>
              <tr className="border-b border-line">
                <th className={stickyHead + ' top-0 z-30'}>{label}</th>
                {MONTHS.slice(1).map((m) => (
                  <th key={m} className="sticky top-0 bg-bg-2 px-3 py-3 text-right text-[10.5px] font-bold uppercase tracking-wide text-t2">{m}</th>
                ))}
                <th className="sticky top-0 bg-bg-2 px-4 py-3 text-right text-[10.5px] font-bold uppercase tracking-wide text-t2">Total</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r: any) => (
                <tr key={r.key} className="border-b border-line last:border-0 hover:bg-bg-3">
                  <td className={stickyCell} dir="auto">{r.key}</td>
                  {MONTHS.slice(1).map((_, mi) => {
                    const v = Number(r.months?.[mi] || 0)
                    return <td key={mi} className="px-3 py-2.5 text-right tabular-nums text-t1">{v ? fmt(v) : <span className="text-t2">–</span>}</td>
                  })}
                  <td className="px-4 py-2.5 text-right font-bold tabular-nums text-t0">{fmt(r.total)}</td>
                </tr>
              ))}
            </tbody>
            <tfoot>
              <tr className="border-t-2 border-line bg-bg-inset">
                <td className={stickyCell.replace('bg-bg-2', 'bg-bg-inset') + ' font-bold'}>Total</td>
                {monthTotals.map((v, mi) => (
                  <td key={mi} className="px-3 py-2.5 text-right font-bold tabular-nums text-t0">{fmt(v)}</td>
                ))}
                <td className="px-4 py-2.5 text-right font-extrabold tabular-nums text-t0">{fmt(grand)}</td>
              </tr>
            </tfoot>
          </table>
        </div>
      ) : <Empty />}
    </Card>
  )
}

// CrossCard — the 2-dimension pivot: rows are dimension 1, columns dimension 2.
// Both axes are pickers, so the table can be re-pointed in place. Values beyond
// each axis's cut are folded into "Others" server-side, so the grand total still
// equals the filtered period total.
function CrossCard({ dim1, dim2, yearLabel, qs, ready, onPick, onRemove }: {
  dim1: string; dim2: string; yearLabel: string; qs: string; ready: boolean
  onPick: (axis: 0 | 1, dim: string) => void; onRemove: () => void
}) {
  const [res, setRes] = useState<any>(null)
  useEffect(() => {
    if (!ready) return
    setRes(null)
    const p = new URLSearchParams(qs)
    p.set('dimension1', dim1); p.set('dimension2', dim2)
    api.cross(p.toString()).then(setRes).catch(() => setRes({ rows: [], cols: [] }))
  }, [dim1, dim2, qs, ready])

  const cols: string[] = res?.cols || []
  const rows: any[] = res?.rows || []
  const stickyHead = 'sticky left-0 z-20 bg-bg-2 px-4 py-3 text-left text-[10.5px] font-bold uppercase tracking-wide text-t2'
  const stickyCell = 'sticky left-0 z-10 bg-bg-2 px-4 py-2.5 font-semibold text-t0'
  const pick = (axis: 0 | 1, value: string) => (
    <select className={selCls} value={value} onChange={(e) => onPick(axis, e.target.value)}>
      {DIM_GROUPS.map(([group, dims]) => (
        <optgroup key={group} label={group}>
          {dims.map((k) => <option key={k} value={k}>{labelOf(k)}</option>)}
        </optgroup>
      ))}
    </select>
  )

  return (
    <Card padding="none" className="overflow-hidden">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-5 py-4">
        <div className="flex flex-wrap items-center gap-2">
          <CardTitle>{labelOf(dim1)} × {labelOf(dim2)}</CardTitle>
          {pick(0, dim1)}<span className="text-[13px] font-bold text-t2">×</span>{pick(1, dim2)}
        </div>
        <div className="flex items-center gap-2">
          <CardSubtitle>Units · {yearLabel}</CardSubtitle>
          <RemoveBtn label={`${labelOf(dim1)} × ${labelOf(dim2)}`} onClick={onRemove} />
        </div>
      </div>
      {res === null ? <Loading /> : rows.length ? (
        <div className="max-h-[560px] overflow-auto">
          <table className="w-full border-collapse text-sm">
            <thead>
              <tr className="border-b border-line">
                <th className={stickyHead + ' top-0 z-30'}>{labelOf(dim1)}</th>
                {cols.map((c) => (
                  <th key={c} dir="auto" className="sticky top-0 bg-bg-2 px-3 py-3 text-right text-[10.5px] font-bold uppercase tracking-wide text-t2">{c}</th>
                ))}
                <th className="sticky top-0 bg-bg-2 px-4 py-3 text-right text-[10.5px] font-bold uppercase tracking-wide text-t2">Total</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r: any) => (
                <tr key={r.key} className="border-b border-line last:border-0 hover:bg-bg-3">
                  <td className={stickyCell} dir="auto">{r.key}</td>
                  {r.cells.map((v: number, i: number) => (
                    <td key={i} className="px-3 py-2.5 text-right tabular-nums text-t1">{v ? fmt(v) : <span className="text-t2">–</span>}</td>
                  ))}
                  <td className="px-4 py-2.5 text-right font-bold tabular-nums text-t0">{fmt(r.total)}</td>
                </tr>
              ))}
            </tbody>
            <tfoot>
              <tr className="border-t-2 border-line bg-bg-inset">
                <td className={stickyCell.replace('bg-bg-2', 'bg-bg-inset') + ' font-bold'}>Total</td>
                {(res.col_totals || []).map((v: number, i: number) => (
                  <td key={i} className="px-3 py-2.5 text-right font-bold tabular-nums text-t0">{fmt(v)}</td>
                ))}
                <td className="px-4 py-2.5 text-right font-extrabold tabular-nums text-t0">{fmt(res.grand || 0)}</td>
              </tr>
            </tfoot>
          </table>
        </div>
      ) : <Empty />}
    </Card>
  )
}

function RemoveBtn({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <button onClick={onClick} aria-label={`Remove ${label} chart`}
      className="flex h-7 w-7 items-center justify-center rounded-full text-t2 hover:bg-bg-3 hover:text-t0">✕</button>
  )
}

function Loading() {
  return <div className="grid h-40 place-items-center text-[13px] text-t2">Loading…</div>
}

function Empty() {
  return <div className="grid h-40 place-items-center text-[13px] text-t2">No data</div>
}
