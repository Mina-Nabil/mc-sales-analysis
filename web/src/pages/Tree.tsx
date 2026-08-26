import { useEffect, useState } from 'react'
import { api, fmt } from '@/lib/api'
import { Card, Button, Badge, Modal } from '@/components/ui'

const CAR_TYPES = ['Passenger', 'Commercial', 'Bus', 'Construction']
const ENGINES = ['ICE', 'HYBRID', 'BEV', 'REEV', 'Other']
const SUPPLIES = ['CKD', 'SUP']
const ORIGINS = ['China', 'Europe', 'Japan', 'Korea', 'USA', 'India', 'Russia', 'UAE', 'Egypt', 'Others']
const field = 'h-9 w-full rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-3 text-[13px] text-t0 focus:border-acc'

export default function Tree() {
  const [brands, setBrands] = useState<any[]>([])
  const [segments, setSegments] = useState<any[]>([])
  const [q, setQ] = useState('')
  const [brand, setBrand] = useState<any>(null)
  const [models, setModels] = useState<any[]>([])
  const [model, setModel] = useState<any>(null)
  const [aliases, setAliases] = useState<any[]>([])
  const [msg, setMsg] = useState('')
  const [modal, setModal] = useState<null | 'brand' | 'model' | 'merge'>(null)

  const loadBrands = () => api.brands().then(setBrands)
  useEffect(() => { loadBrands().catch(() => {}); api.segments().then(setSegments).catch(() => {}) }, [])

  const pickBrand = (b: any) => { setBrand(b); setModel(null); setAliases([]); api.brandModels(b.id).then(setModels).catch(() => setModels([])) }
  const reloadModels = () => brand && api.brandModels(brand.id).then(setModels)
  const pickModel = (m: any) => { setModel(m); api.modelAliases(m.id).then(setAliases).catch(() => setAliases([])) }

  async function run(fn: () => Promise<any>, ok: (r: any) => string) {
    setMsg('')
    try { const r = await fn(); setMsg(ok(r)); return r } catch (e: any) { setMsg('⚠ ' + e.message) }
  }
  const shown = brands.filter((b) => b.name.toLowerCase().includes(q.toLowerCase()))

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div><h1 className="text-xl font-extrabold text-t0 sm:text-[26px]">Car tree</h1>
          <p className="mt-1 text-[13px] text-t1">The classification asset — edits re-derive all history.</p></div>
        <div className="flex gap-2">
          <Button variant="secondary" onClick={() => setModal('brand')}>＋ New brand</Button>
          <Button disabled={!brand} onClick={() => setModal('model')}>＋ New model</Button>
        </div>
      </div>
      {msg && <div className="rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-3.5 py-2.5 text-[13px] text-t0">{msg}</div>}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Card padding="sm" className="max-h-[72vh] overflow-auto">
          <input placeholder="Search brands…" value={q} onChange={(e) => setQ(e.target.value)} className={field + ' mb-2'} />
          <List items={shown} active={brand?.id} onPick={pickBrand} render={(b) => (
            <><span className="font-semibold text-t0">{b.name}</span><span className="text-[12px] text-t1">{fmt(b.volume)}</span></>
          )} />
        </Card>

        <Card padding="sm" className="max-h-[72vh] overflow-auto">
          <Head>{brand ? `${brand.name} — models` : 'Select a brand'}</Head>
          <List items={models} active={model?.id} onPick={pickModel} render={(m) => (
            <><span className="min-w-0 truncate text-[13px] text-t0">{m.name} {m.segment && <Badge variant="neutral">{m.segment}</Badge>}</span>
              <span className="text-[12px] text-t1">{fmt(m.volume)}</span></>
          )} />
        </Card>

        <Card padding="sm" className="max-h-[72vh] overflow-auto">
          {model ? (
            <>
              <Head>{model.name}</Head>
              <ModelEditor key={model.id} model={model} segments={segments}
                onSave={(fields) => run(() => api.editModel(model.id, fields), () => 'Saved — history re-derived.').then(reloadModels)}
                onMerge={() => setModal('merge')} />
              <Head>Aliases ({aliases.length})</Head>
              <ul className="space-y-0.5">
                {aliases.map((a) => (
                  <li key={a.id} className="flex items-center justify-between gap-2 rounded-[9px] px-2.5 py-1.5 hover:bg-bg-3">
                    <span dir="rtl" className="min-w-0 truncate text-[13px] text-t0">{a.raw}</span>
                    <span className="flex items-center gap-2">
                      <span className="text-[11px] text-t2">{a.status}</span>
                      <button title="detach" className="text-bad hover:opacity-70"
                        onClick={() => run(() => api.deleteAlias(a.id), (r) => `Detached — ${fmt(r.facts_rederived)} facts back to unresolved.`).then(() => pickModel(model))}>✕</button>
                    </span>
                  </li>
                ))}
              </ul>
            </>
          ) : <Head>Select a model</Head>}
        </Card>
      </div>

      <NewBrandModal open={modal === 'brand'} brands={brands} onClose={() => setModal(null)}
        onSubmit={(b) => { setModal(null); run(() => api.createBrand(b), (r) => `Created brand ${r.name}.`).then(loadBrands) }} />
      <NewModelModal open={modal === 'model'} brand={brand} segments={segments} onClose={() => setModal(null)}
        onSubmit={(m) => { setModal(null); run(() => api.createModel({ ...m, brand_id: brand.id }), (r) => `Created model ${r.name}.`).then(reloadModels) }} />
      <MergeModal open={modal === 'merge'} model={model} candidates={models.filter((m) => m.id !== model?.id)} onClose={() => setModal(null)}
        onDone={(text) => { setModal(null); setModel(null); setMsg(text); reloadModels() }} />
    </div>
  )
}

function Head({ children }: { children: React.ReactNode }) {
  return <div className="mb-2 mt-1 px-1 text-[12px] font-bold uppercase tracking-wide text-t2">{children}</div>
}
function List({ items, active, onPick, render }: any) {
  return (
    <ul className="space-y-0.5">
      {items.map((it: any) => (
        <li key={it.id} onClick={() => onPick(it)}
          className={'flex cursor-pointer items-center justify-between gap-2 rounded-[9px] px-2.5 py-2 ' + (active === it.id ? 'bg-acc-soft' : 'hover:bg-bg-3')}>
          {render(it)}
        </li>
      ))}
    </ul>
  )
}

function ModelEditor({ model, segments, onSave, onMerge }: any) {
  const [name, setName] = useState(model.name)
  const [carType, setCarType] = useState(model.car_type || '')
  const [engine, setEngine] = useState(model.engine_type || '')
  const [supply, setSupply] = useState(model.supply || '')
  const [segName, setSegName] = useState(model.segment || '')
  const segId = segments.find((s: any) => s.name === segName)?.id
  return (
    <div className="mb-3 rounded-[var(--radius-vela-md)] border border-line bg-bg-inset p-3">
      <F label="Name"><input className={field} value={name} onChange={(e) => setName(e.target.value)} /></F>
      <F label="Car type"><select className={field} value={carType} onChange={(e) => setCarType(e.target.value)}><option value="">—</option>{CAR_TYPES.map((t) => <option key={t}>{t}</option>)}</select></F>
      <F label="Segment"><select className={field} value={segName} onChange={(e) => setSegName(e.target.value)}><option value="">—</option>{segments.map((s: any) => <option key={s.id}>{s.name}</option>)}</select></F>
      <F label="Engine"><select className={field} value={engine} onChange={(e) => setEngine(e.target.value)}><option value="">—</option>{ENGINES.map((t) => <option key={t}>{t}</option>)}</select></F>
      <F label="Supply"><select className={field} value={supply} onChange={(e) => setSupply(e.target.value)}><option value="">—</option>{SUPPLIES.map((t) => <option key={t}>{t}</option>)}</select></F>
      <p className="mb-2 text-[11px] text-t2">Specs apply to every month's facts for this model.</p>
      <div className="mt-1 flex gap-2">
        <Button size="sm" onClick={() => onSave({ name, car_type: carType, engine_type: engine, supply, segment_id: segId || null })}>Save</Button>
        <Button size="sm" variant="secondary" onClick={onMerge}>Merge into…</Button>
      </div>
    </div>
  )
}

function F({ label, children }: { label: string; children: React.ReactNode }) {
  return <label className="mb-2 block text-[11.5px] font-semibold text-t1">{label}<div className="mt-1">{children}</div></label>
}

function NewBrandModal({ open, brands, onClose, onSubmit }: any) {
  const [name, setName] = useState(''); const [origin, setOrigin] = useState('China'); const [parent, setParent] = useState('')
  useEffect(() => { if (open) { setName(''); setOrigin('China'); setParent('') } }, [open])
  return (
    <Modal open={open} onClose={onClose} title="New brand"
      footer={<><Button variant="ghost" onClick={onClose}>Cancel</Button><Button disabled={!name} onClick={() => onSubmit({ name, origin, parent_id: parent ? Number(parent) : null })}>Create</Button></>}>
      <F label="Name"><input className={field} value={name} onChange={(e) => setName(e.target.value)} autoFocus /></F>
      <F label="Origin"><select className={field} value={origin} onChange={(e) => setOrigin(e.target.value)}>{ORIGINS.map((o) => <option key={o}>{o}</option>)}</select></F>
      <F label="Parent brand (optional)"><select className={field} value={parent} onChange={(e) => setParent(e.target.value)}><option value="">—</option>{brands.map((b: any) => <option key={b.id} value={b.id}>{b.name}</option>)}</select></F>
    </Modal>
  )
}

function NewModelModal({ open, brand, segments, onClose, onSubmit }: any) {
  const [name, setName] = useState(''); const [carType, setCarType] = useState('Passenger'); const [seg, setSeg] = useState('')
  useEffect(() => { if (open) { setName(''); setCarType('Passenger'); setSeg('') } }, [open])
  if (!brand) return null
  return (
    <Modal open={open} onClose={onClose} title={`New model under ${brand.name}`}
      footer={<><Button variant="ghost" onClick={onClose}>Cancel</Button><Button disabled={!name} onClick={() => onSubmit({ name, car_type: carType, segment_id: seg ? Number(seg) : null })}>Create</Button></>}>
      <F label="Name"><input className={field} value={name} onChange={(e) => setName(e.target.value)} autoFocus /></F>
      <F label="Car type"><select className={field} value={carType} onChange={(e) => setCarType(e.target.value)}>{CAR_TYPES.map((t) => <option key={t}>{t}</option>)}</select></F>
      <F label="Segment"><select className={field} value={seg} onChange={(e) => setSeg(e.target.value)}><option value="">—</option>{segments.map((s: any) => <option key={s.id} value={s.id}>{s.name}</option>)}</select></F>
    </Modal>
  )
}

function MergeModal({ open, model, candidates, onClose, onDone }: any) {
  const [intoId, setIntoId] = useState(''); const [preview, setPreview] = useState<any>(null)
  useEffect(() => { if (open) { setIntoId(''); setPreview(null) } }, [open])
  async function pick(id: string) { setIntoId(id); setPreview(null); if (id && model) setPreview(await api.mergePreview(model.id, Number(id)).catch(() => null)) }
  async function doMerge() {
    try { const r = await api.merge(model.id, Number(intoId)); onDone(`Merged ${r.source_model} → ${r.into_model}: moved ${fmt(r.units)} units, ${r.aliases} aliases.`) }
    catch (e: any) { onDone('⚠ ' + e.message) }
  }
  if (!model) return null
  return (
    <Modal open={open} onClose={onClose} title={`Merge "${model.name}" into…`}
      footer={<><Button variant="ghost" onClick={onClose}>Cancel</Button><Button variant="danger" disabled={!intoId} onClick={doMerge}>Merge</Button></>}>
      <F label="Target model"><select className={field} value={intoId} onChange={(e) => pick(e.target.value)}><option value="">—</option>{candidates.map((m: any) => <option key={m.id} value={m.id}>{m.name}</option>)}</select></F>
      {preview && (
        <div className="rounded-[var(--radius-vela-md)] bg-warn-soft px-3 py-2.5 text-[12.5px] text-warn">
          Moves <b>{fmt(preview.units)}</b> units across {fmt(preview.facts)} facts and {preview.aliases} aliases from <b>{preview.source_model}</b> into <b>{preview.into_model}</b>. This rewrites history.
        </div>
      )}
    </Modal>
  )
}
