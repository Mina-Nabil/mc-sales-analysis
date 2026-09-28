import { useCallback, useEffect, useRef, useState } from 'react'
import { api, fmt } from '@/lib/api'
import { Card, Button, Badge, Modal } from '@/components/ui'

const CAR_TYPES = ['Passenger', 'Commercial', 'Bus', 'Construction']
const ENGINES = ['ICE', 'HYBRID', 'BEV', 'REEV', 'Other']
const SUPPLIES = ['CKD', 'SUP']
const ORIGINS = ['China', 'Europe', 'Japan', 'Korea', 'USA', 'India', 'Russia', 'UAE', 'Egypt', 'Others']
const field = 'h-9 w-full rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-3 text-[13px] text-t0 focus:border-acc'

export default function Review() {
  const [tab, setTab] = useState<'models' | 'brands'>('models')
  const [items, setItems] = useState<any[]>([])
  const [brandItems, setBrandItems] = useState<any[]>([])
  const [brands, setBrands] = useState<any[]>([])
  const [segments, setSegments] = useState<any[]>([])
  const [sel, setSel] = useState(0)
  const [busy, setBusy] = useState(false)
  const [msg, setMsg] = useState('')
  const [minConf, setMinConf] = useState('0.95')
  const [newModelFor, setNewModelFor] = useState<any>(null)
  const [resolveFor, setResolveFor] = useState<any>(null)
  const [mergeFor, setMergeFor] = useState<any>(null)
  const [excludeFor, setExcludeFor] = useState<any>(null)
  const rowsRef = useRef<(HTMLTableRowElement | null)[]>([])

  const load = useCallback(async () => {
    if (tab === 'models') setItems(await api.review(100))
    else { setBrandItems(await api.brandQueue(100)); if (!brands.length) setBrands(await api.brands()) }
  }, [tab]) // eslint-disable-line
  useEffect(() => { load().catch((e) => setMsg(e.message)) }, [load])
  useEffect(() => { api.segments().then(setSegments).catch(() => {}) }, [])

  const act = useCallback(async (fn: () => Promise<any>, ok: string | ((r: any) => string)) => {
    setBusy(true); setMsg('')
    try { const r = await fn(); setMsg(typeof ok === 'function' ? ok(r) : ok); await load() }
    catch (e: any) { setMsg('⚠ ' + e.message) } finally { setBusy(false) }
  }, [load])

  // A decision may also unlock sibling spellings already stored under an
  // equivalent raw string — the server replays the deterministic ladder and
  // reports what else it settled, so the reviewer sees the queue shrink.
  const withReplay = (label: string) => (r: any) => {
    const rep = r?.replay
    const extra = rep && (rep.model_rows || rep.brand_rows)
      ? ` Also linked ${fmt(rep.model_rows + rep.brand_rows)} more rows (${fmt(rep.model_volume + rep.brand_volume)} units) from equivalent spellings.`
      : ''
    return `${label} — ${fmt(r?.facts_rederived)} facts re-derived.${extra}`
  }

  const confirm = (it: any) => {
    if (!it) return
    if (!it.proposal_id) { setMergeFor(it); return }
    act(() => api.confirm(it.alias_id), withReplay('Merged'))
  }
  const mergeInto = (it: any, modelID: number) =>
    act(() => api.reassign(it.alias_id, modelID), withReplay('Merged'))
  const reject = (it: any) => it && act(() => api.reject(it.alias_id), 'Marked wrong — volume returned to unresolved.')
  const exclude = (it: any, reason: string) =>
    act(() => api.exclude(it.alias_id, reason), (r) => `Excluded ${fmt(r.excluded_volume)} units — kept on record, out of the measures.`)

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.target as HTMLElement).tagName === 'INPUT' || newModelFor || resolveFor || mergeFor || excludeFor || tab !== 'models') return
      if (e.key === 'j') setSel((s) => Math.min(s + 1, items.length - 1))
      else if (e.key === 'k') setSel((s) => Math.max(s - 1, 0))
      else if (e.key === 'Enter') confirm(items[sel])
      else if (e.key === 'm') items[sel] && setMergeFor(items[sel])
      else if (e.key === 'r') reject(items[sel])
      else if (e.key === 'n') items[sel] && setNewModelFor(items[sel])
      else if (e.key === 'x') items[sel] && setExcludeFor(items[sel])
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [items, sel, newModelFor, resolveFor, mergeFor, excludeFor, tab]) // eslint-disable-line
  useEffect(() => { rowsRef.current[sel]?.scrollIntoView({ block: 'nearest' }) }, [sel])

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-extrabold text-t0 sm:text-[26px]">Review queue</h1>
          <p className="mt-1 text-[13px] text-t1">Volume-ranked inbox. Keys: <Kbd>J</Kbd>/<Kbd>K</Kbd> move · <Kbd>Enter</Kbd> accept proposal · <Kbd>M</Kbd> merge into… · <Kbd>N</Kbd> new model · <Kbd>R</Kbd> wrong match · <Kbd>X</Kbd> not needed.</p>
        </div>
        {tab === 'models' && (
          <div className="flex items-center gap-2">
            <Button variant="secondary" size="sm" disabled={busy}
              onClick={() => act(() => api.resolve(), (r) => `Fuzzy pass: ${r.auto_resolved} auto, ${r.proposed} proposed.`)}>Run fuzzy pass</Button>
            <input value={minConf} onChange={(e) => setMinConf(e.target.value)} className="h-8 w-16 rounded-[var(--radius-vela-sm)] border border-line bg-bg-inset px-2 text-[12px] text-t0" />
            <Button size="sm" disabled={busy} onClick={() => act(() => api.bulkConfirm(Number(minConf)), (r) => `Bulk-confirmed ${r.confirmed} items.`)}>Bulk confirm ≥</Button>
          </div>
        )}
      </div>

      <div className="flex gap-1 border-b border-line">
        {(['models', 'brands'] as const).map((t) => (
          <button key={t} onClick={() => setTab(t)}
            className={'border-b-2 px-3.5 py-2.5 text-[13px] font-semibold transition-colors ' + (tab === t ? 'border-acc text-t0' : 'border-transparent text-t1 hover:text-t0')}>
            {t === 'models' ? 'Models' : 'New brands'}
          </button>
        ))}
      </div>

      {msg && <div className="rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-3.5 py-2.5 text-[13px] text-t0">{msg}</div>}

      {tab === 'models' ? (
        <Card padding="none" className="overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full min-w-[920px] border-collapse text-sm">
              <thead><tr className="border-b border-line">
                {['Brand', 'Raw model', 'Proposal', 'Conf', 'Volume', ''].map((h, i) => (
                  <th key={i} className={'px-4 py-3 text-[10.5px] font-bold uppercase tracking-wide text-t2 ' + (h === 'Volume' ? 'text-right' : 'text-left')}>{h}</th>
                ))}
              </tr></thead>
              <tbody>
                {items.map((it, i) => (
                  <tr key={it.alias_id} ref={(el) => { rowsRef.current[i] = el }} onClick={() => setSel(i)}
                    className={'border-b border-line last:border-0 cursor-pointer ' + (i === sel ? 'bg-acc-soft' : 'hover:bg-bg-3')}>
                    <td className="px-4 py-2.5 font-semibold text-t0">{it.brand}</td>
                    <td className="px-4 py-2.5" dir="rtl">{it.raw_model || <span className="text-t2">(blank)</span>}</td>
                    <td className="px-4 py-2.5 text-t1">{it.proposal || <span className="text-t2">— new model</span>}</td>
                    <td className="px-4 py-2.5 text-t1">{it.confidence ? it.confidence.toFixed(2) : ''}</td>
                    <td className="px-4 py-2.5 text-right font-bold tabular-nums text-t0">{fmt(it.volume)}</td>
                    <td className="px-4 py-2.5">
                      <div className="flex justify-end gap-1.5">
                        {it.proposal_id && (
                          <Button size="sm" disabled={busy} title={`Merge into ${it.proposal}`}
                            onClick={(e) => { e.stopPropagation(); confirm(it) }}>✓</Button>
                        )}
                        <Button size="sm" variant="secondary" disabled={busy} title="Merge into a model you pick"
                          onClick={(e) => { e.stopPropagation(); setMergeFor(it) }}>Merge…</Button>
                        <Button size="sm" variant="secondary" disabled={busy} title="Create a new model for this string"
                          onClick={(e) => { e.stopPropagation(); setNewModelFor(it) }}>＋ model</Button>
                        {it.proposal_id && (
                          <Button size="sm" variant="ghost" disabled={busy} title="Proposal is wrong — volume stays unresolved"
                            onClick={(e) => { e.stopPropagation(); reject(it) }}>Wrong</Button>
                        )}
                        <Button size="sm" variant="danger" disabled={busy} title="Volume is out of scope — exclude from the measures"
                          onClick={(e) => { e.stopPropagation(); setExcludeFor(it) }}>Not needed</Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {items.length === 0 && <div className="p-10 text-center text-sm text-t1">Model queue is empty. 🎉</div>}
        </Card>
      ) : (
        <Card padding="none" className="overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full min-w-[520px] border-collapse text-sm">
              <thead><tr className="border-b border-line">
                {['Raw brand', 'Volume', 'Rows', ''].map((h, i) => (
                  <th key={i} className={'px-4 py-3 text-[10.5px] font-bold uppercase tracking-wide text-t2 ' + (h === 'Volume' || h === 'Rows' ? 'text-right' : 'text-left')}>{h}</th>
                ))}
              </tr></thead>
              <tbody>
                {brandItems.map((it) => (
                  <tr key={it.raw_brand} className="border-b border-line last:border-0 hover:bg-bg-3">
                    <td className="px-4 py-2.5 font-semibold text-t0" dir="rtl">{it.raw_brand || <span className="text-t2">(blank)</span>}</td>
                    <td className="px-4 py-2.5 text-right font-bold tabular-nums text-t0">{fmt(it.volume)}</td>
                    <td className="px-4 py-2.5 text-right tabular-nums text-t1">{fmt(it.rows)}</td>
                    <td className="px-4 py-2.5 text-right"><Button size="sm" variant="secondary" disabled={busy} onClick={() => setResolveFor(it)}>Resolve…</Button></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {brandItems.length === 0 && <div className="p-10 text-center text-sm text-t1">No unresolved brands. 🎉</div>}
        </Card>
      )}

      <NewModelModal item={newModelFor} segments={segments} onClose={() => setNewModelFor(null)}
        onSubmit={(body) => { const it = newModelFor; setNewModelFor(null); act(() => api.newModelForReview(it.alias_id, body), (r) => `Created model, re-derived ${fmt(r.facts_rederived)} facts.`) }} />
      <MergeModal item={mergeFor} onClose={() => setMergeFor(null)}
        onSubmit={(modelID: number) => { const it = mergeFor; setMergeFor(null); mergeInto(it, modelID) }} />
      <ExcludeModal item={excludeFor} onClose={() => setExcludeFor(null)}
        onSubmit={(reason: string) => { const it = excludeFor; setExcludeFor(null); exclude(it, reason) }} />
      <BrandResolveModal item={resolveFor} brands={brands} onClose={() => setResolveFor(null)}
        onSubmit={(body) => { const it = resolveFor; setResolveFor(null); act(() => api.resolveBrand({ raw_brand: it.raw_brand, ...body }), (r) => `Resolved — ${fmt(r.facts_rederived)} facts now carry the brand.`) }} />
    </div>
  )
}

function Kbd({ children }: { children: React.ReactNode }) {
  return <kbd className="rounded border border-line-2 bg-bg-3 px-1.5 text-[10px] font-bold text-t1">{children}</kbd>
}

// MergeModal — pick which existing model this raw spelling belongs to. The
// model list is scoped to the row's brand (aliases are per-brand, TECH §2.2);
// the fuzzy proposal, when there is one, starts selected.
function MergeModal({ item, onClose, onSubmit }: any) {
  const [models, setModels] = useState<any[]>([])
  const [q, setQ] = useState('')
  const [picked, setPicked] = useState<number | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!item) return
    setQ(''); setPicked(item.proposal_id ?? null); setModels([]); setLoading(true)
    api.brandModels(item.brand_id).then((m: any[]) => setModels(m || [])).catch(() => setModels([])).finally(() => setLoading(false))
  }, [item])

  if (!item) return null
  const needle = q.trim().toLowerCase()
  const shown = needle ? models.filter((m) => m.name.toLowerCase().includes(needle)) : models

  return (
    <Modal open={!!item} onClose={onClose} title={`Merge into an existing ${item.brand} model`}
      footer={<><Button variant="ghost" onClick={onClose}>Cancel</Button>
        <Button disabled={!picked} onClick={() => picked && onSubmit(picked)}>Merge volume</Button></>}>
      <p className="mb-3 text-[12.5px] text-t1">
        Raw string: <span dir="rtl" className="font-semibold text-t0">{item.raw_model || '(blank)'}</span> · {fmt(item.volume)} units
      </p>
      <input className={field + ' mb-2'} placeholder="Search models…" value={q} onChange={(e) => setQ(e.target.value)} autoFocus />
      <div className="max-h-[280px] overflow-y-auto rounded-[var(--radius-vela-md)] border border-line">
        {loading && <div className="p-4 text-center text-[13px] text-t2">Loading models…</div>}
        {!loading && shown.length === 0 && <div className="p-4 text-center text-[13px] text-t2">No models match.</div>}
        {shown.map((m) => (
          <button key={m.id} onClick={() => setPicked(m.id)}
            className={'flex w-full items-center justify-between border-b border-line px-3 py-2 text-left text-[13px] last:border-0 ' +
              (picked === m.id ? 'bg-acc-soft font-semibold text-acc' : 'text-t1 hover:bg-bg-3')}>
            <span>{m.name}{m.id === item.proposal_id && <span className="ml-2 text-[11px] font-semibold text-t2">proposed</span>}</span>
            <span className="tabular-nums text-t2">{fmt(m.volume)}</span>
          </button>
        ))}
      </div>
    </Modal>
  )
}

// ExcludeModal — "this volume is not needed". Confirmed explicitly because it
// takes units out of every measure; the facts are marked, never deleted, and
// stay reported so period totals still reconcile.
function ExcludeModal({ item, onClose, onSubmit }: any) {
  const [reason, setReason] = useState('')
  useEffect(() => { if (item) setReason('') }, [item])
  if (!item) return null
  return (
    <Modal open={!!item} onClose={onClose} title="Exclude this volume"
      footer={<><Button variant="ghost" onClick={onClose}>Cancel</Button>
        <Button variant="danger" onClick={() => onSubmit(reason)}>Exclude {fmt(item.volume)} units</Button></>}>
      <p className="mb-3 text-[12.5px] text-t1">
        <span dir="rtl" className="font-semibold text-t0">{item.brand} · {item.raw_model || '(blank)'}</span>
      </p>
      <p className="mb-3 text-[12.5px] text-t1">
        These {fmt(item.volume)} units leave every share, growth and rank figure. Nothing is deleted — the rows stay
        on record and the excluded total is reported with the period, so totals still reconcile against the authority.
      </p>
      <Field label="Reason (optional)">
        <input className={field} value={reason} onChange={(e) => setReason(e.target.value)}
          placeholder="e.g. agricultural vehicle, not a car" autoFocus />
      </Field>
    </Modal>
  )
}

function NewModelModal({ item, segments, onClose, onSubmit }: any) {
  const [name, setName] = useState('')
  const [carType, setCarType] = useState('Passenger')
  const [segmentId, setSegmentId] = useState('')
  const [engine, setEngine] = useState('')
  const [supply, setSupply] = useState('')
  useEffect(() => { if (item) { setName(item.raw_model || ''); setCarType('Passenger'); setSegmentId(''); setEngine(''); setSupply('') } }, [item])
  if (!item) return null
  return (
    <Modal open={!!item} onClose={onClose} title={`New model under ${item.brand}`}
      footer={<><Button variant="ghost" onClick={onClose}>Cancel</Button><Button disabled={!name} onClick={() => onSubmit({ name, car_type: carType, engine_type: engine, supply, segment_id: segmentId ? Number(segmentId) : null })}>Create &amp; confirm</Button></>}>
      <p className="mb-3 text-[12.5px] text-t1">Raw string: <span dir="rtl" className="font-semibold text-t0">{item.raw_model || '(blank)'}</span> · {fmt(item.volume)} units</p>
      <Field label="Model name"><input className={field} value={name} onChange={(e) => setName(e.target.value)} autoFocus /></Field>
      <Field label="Car type"><select className={field} value={carType} onChange={(e) => setCarType(e.target.value)}>{CAR_TYPES.map((t) => <option key={t}>{t}</option>)}</select></Field>
      <Field label="Segment"><select className={field} value={segmentId} onChange={(e) => setSegmentId(e.target.value)}><option value="">—</option>{segments.map((s: any) => <option key={s.id} value={s.id}>{s.name}</option>)}</select></Field>
      <Field label="Engine"><select className={field} value={engine} onChange={(e) => setEngine(e.target.value)}><option value="">—</option>{ENGINES.map((t) => <option key={t}>{t}</option>)}</select></Field>
      <Field label="Supply"><select className={field} value={supply} onChange={(e) => setSupply(e.target.value)}><option value="">—</option>{SUPPLIES.map((t) => <option key={t}>{t}</option>)}</select></Field>
      <p className="mt-1 text-[11px] text-t2">Engine and supply are model-level specs — the monthly feed carries neither, so they apply to every month of this model's facts.</p>
    </Modal>
  )
}

function BrandResolveModal({ item, brands, onClose, onSubmit }: any) {
  const [mode, setMode] = useState<'existing' | 'new'>('existing')
  const [brandId, setBrandId] = useState('')
  const [name, setName] = useState('')
  const [origin, setOrigin] = useState('China')
  useEffect(() => { if (item) { setMode('existing'); setBrandId(''); setName(''); setOrigin('China') } }, [item])
  if (!item) return null
  return (
    <Modal open={!!item} onClose={onClose} title="Resolve raw brand"
      footer={<><Button variant="ghost" onClick={onClose}>Cancel</Button><Button disabled={mode === 'existing' ? !brandId : !name}
        onClick={() => onSubmit(mode === 'existing' ? { brand_id: Number(brandId) } : { new_brand: { name, origin } })}>Resolve</Button></>}>
      <p className="mb-3 text-[12.5px] text-t1">Raw: <span dir="rtl" className="font-semibold text-t0">{item.raw_brand}</span> · {fmt(item.volume)} units</p>
      <div className="mb-3 flex gap-1">
        {(['existing', 'new'] as const).map((m) => (
          <button key={m} onClick={() => setMode(m)}
            className={'rounded-[var(--radius-vela-sm)] px-3 py-1.5 text-[12.5px] font-semibold ' + (mode === m ? 'bg-acc-soft text-acc' : 'text-t1 hover:bg-bg-3')}>
            {m === 'existing' ? 'Existing brand' : 'New brand'}
          </button>
        ))}
      </div>
      {mode === 'existing'
        ? <Field label="Brand"><select className={field} value={brandId} onChange={(e) => setBrandId(e.target.value)}><option value="">—</option>{brands.map((b: any) => <option key={b.id} value={b.id}>{b.name}</option>)}</select></Field>
        : <><Field label="Name"><input className={field} value={name} onChange={(e) => setName(e.target.value)} autoFocus /></Field>
          <Field label="Origin"><select className={field} value={origin} onChange={(e) => setOrigin(e.target.value)}>{ORIGINS.map((o) => <option key={o}>{o}</option>)}</select></Field></>}
    </Modal>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return <label className="mb-3 block text-[12px] font-semibold text-t1">{label}<div className="mt-1">{children}</div></label>
}
