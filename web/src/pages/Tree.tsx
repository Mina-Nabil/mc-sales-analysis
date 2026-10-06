import { useEffect, useState } from 'react'
import { api, fmt } from '@/lib/api'
import { Card, Button, Badge, Modal } from '@/components/ui'
import { CAR_TYPES, ENGINES, SUPPLIES, ORIGINS, field } from '@/lib/vocab'
import { BrandInherited, distributorLabel, day } from '@/components/BrandInherited'

export default function Tree() {
  const [brands, setBrands] = useState<any[]>([])
  const [segments, setSegments] = useState<any[]>([])
  const [q, setQ] = useState('')
  const [brand, setBrand] = useState<any>(null)
  const [models, setModels] = useState<any[]>([])
  const [model, setModel] = useState<any>(null)
  const [aliases, setAliases] = useState<any[]>([])
  const [assignments, setAssignments] = useState<any[]>([])
  const [msg, setMsg] = useState('')
  const [modal, setModal] = useState<null | 'brand' | 'model' | 'merge'>(null)

  const loadBrands = () => api.brands().then(setBrands)
  useEffect(() => { loadBrands().catch(() => {}); api.segments().then(setSegments).catch(() => {}) }, [])

  const loadAssignments = (id: number) => api.brandAssignments(id).then(setAssignments).catch(() => setAssignments([]))
  const pickBrand = (b: any) => {
    setBrand(b); setModel(null); setAliases([])
    api.brandModels(b.id).then(setModels).catch(() => setModels([]))
    loadAssignments(b.id)
  }
  const reloadModels = () => brand && api.brandModels(brand.id).then(setModels)
  const pickModel = (m: any) => { setModel(m); api.modelAliases(m.id).then(setAliases).catch(() => setAliases([])) }

  // Re-read the brand list after a brand edit and keep the same brand selected.
  const reloadBrand = async () => {
    if (!brand) return
    const list = await api.brands()
    setBrands(list)
    const fresh = list.find((b: any) => b.id === brand.id)
    if (fresh) setBrand(fresh)
    await loadAssignments(brand.id)
  }

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
              <p className="mb-2 px-1 text-[11.5px] text-t2">
                <button className="font-semibold text-acc hover:underline" onClick={() => setModel(null)}>{brand.name}</button>
                {' · '}origin {brand.origin || 'Unknown'}
                {' · '}{distributorLabel(assignments, model.car_type)}
              </p>
              <ModelEditor key={model.id} model={model} brand={brand} segments={segments} onMessage={setMsg}
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
          ) : brand ? (
            <>
              <Head>{brand.name}</Head>
              <BrandEditor key={brand.id} brand={brand} brands={brands} onMessage={setMsg} onSaved={reloadBrand} />
              <BrandDistributors brandId={brand.id} brandName={brand.name} assignments={assignments}
                onMessage={setMsg} onChanged={() => loadAssignments(brand.id)} />
            </>
          ) : <Head>Select a brand or a model</Head>}
        </Card>
      </div>

      <NewBrandModal open={modal === 'brand'} brands={brands} onClose={() => setModal(null)}
        onSubmit={(b) => {
          setModal(null)
          run(() => api.createBrand(b), (r) => r.distributor_error
            ? `Created brand ${r.name}, but the distributor was not set: ${r.distributor_error}`
            : `Created brand ${r.name}.`).then(loadBrands)
        }} />
      <NewModelModal open={modal === 'model'} brand={brand} segments={segments} onClose={() => setModal(null)} onMessage={setMsg}
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

// BrandEditor edits the brand's own attributes. origin reaches every fact of the
// brand by JOIN, so there is nothing to re-derive — the edit is live immediately.
function BrandEditor({ brand, brands, onMessage, onSaved }: any) {
  const [detail, setDetail] = useState<any>(null)
  const [name, setName] = useState(brand.name)
  const [origin, setOrigin] = useState(brand.origin || '')
  const [parent, setParent] = useState(brand.parent_id ? String(brand.parent_id) : '')
  const [notes, setNotes] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    api.brand(brand.id).then((d: any) => {
      setDetail(d); setName(d.name); setOrigin(d.origin || '')
      setParent(d.parent_id ? String(d.parent_id) : ''); setNotes(d.notes || '')
    }).catch(() => {})
  }, [brand.id])

  async function save() {
    setBusy(true)
    try {
      await api.editBrand(brand.id, { name, origin, notes, parent_id: parent ? Number(parent) : null })
      onMessage('Saved — every fact of this brand re-derives from the tree, so the change is already live.')
      await onSaved()
    } catch (e: any) { onMessage('⚠ ' + e.message) } finally { setBusy(false) }
  }

  return (
    <div className="mb-3 rounded-[var(--radius-vela-md)] border border-line bg-bg-inset p-3">
      <F label="Name"><input className={field} value={name} onChange={(e) => setName(e.target.value)} /></F>
      <F label="Origin">
        <select className={field} value={origin} onChange={(e) => setOrigin(e.target.value)}>
          <option value="">— Unknown</option>
          {ORIGINS.map((o) => <option key={o}>{o}</option>)}
        </select>
      </F>
      <F label="Parent brand">
        <select className={field} value={parent} onChange={(e) => setParent(e.target.value)}>
          <option value="">—</option>
          {brands.filter((b: any) => b.id !== brand.id).map((b: any) => <option key={b.id} value={b.id}>{b.name}</option>)}
        </select>
      </F>
      <F label="Notes">
        <textarea rows={2} value={notes} onChange={(e) => setNotes(e.target.value)}
          className="w-full rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-3 py-2 text-[13px] text-t0 focus:border-acc" />
      </F>
      <p className="mb-2 text-[11px] text-t2">
        Origin is brand-level — it reaches every model and every month of this brand through the
        tree, so saving is enough; nothing needs re-importing.
        {detail ? ` ${detail.model_count} models · ${fmt(detail.total_volume)} units.` : ''}
      </p>
      <Button size="sm" disabled={busy || !name} onClick={save}>Save</Button>
    </div>
  )
}

// BrandDistributors manages the effective-dated (brand, car_type) assignments.
// Until now these existed only in the seed, so a brand created in the app had no
// way to get a distributor at all.
function BrandDistributors({ brandId, brandName, assignments, onMessage, onChanged }: any) {
  const [distributors, setDistributors] = useState<any[]>([])
  const [carType, setCarType] = useState('Passenger')
  const [distId, setDistId] = useState('')
  const [from, setFrom] = useState('')
  const [newName, setNewName] = useState('')
  const [adding, setAdding] = useState(false)
  const [busy, setBusy] = useState(false)

  const loadDistributors = () => api.distributors().then(setDistributors).catch(() => {})
  useEffect(() => { loadDistributors() }, [])

  async function run(fn: () => Promise<any>, ok: string) {
    setBusy(true)
    try { await fn(); onMessage(ok); await onChanged() }
    catch (e: any) { onMessage('⚠ ' + e.message) } finally { setBusy(false) }
  }

  async function createDistributor() {
    try {
      const r = await api.createDistributor(newName)
      await loadDistributors()
      setDistId(String(r.id)); setNewName(''); setAdding(false)
      onMessage(`Added distributor ${r.name}.`)
    } catch (e: any) { onMessage('⚠ ' + e.message) }
  }

  return (
    <>
      <Head>Distributors ({assignments.length})</Head>
      {assignments.length === 0 && <p className="mb-2 px-1 text-[12px] text-t2">No distributor assigned — this brand reports as “No distributor”.</p>}
      <ul className="mb-3 space-y-0.5">
        {assignments.map((a: any) => (
          <li key={a.id} className="flex items-center justify-between gap-2 rounded-[9px] px-2.5 py-1.5 hover:bg-bg-3">
            <span className="min-w-0 text-[12.5px] text-t0">
              <Badge variant="neutral">{a.car_type}</Badge> {a.distributor_name}
              <span className="ml-1 text-[11px] text-t2">{day(a.valid_from)} → {a.valid_to ? day(a.valid_to) : 'open'}{a.units ? ` · ${fmt(a.units)} units` : ''}</span>
            </span>
            <button title="remove" className="text-bad hover:opacity-70" disabled={busy}
              onClick={() => run(() => api.deleteBrandAssignment(a.id), 'Assignment removed — those units report as “No distributor”.')}>✕</button>
          </li>
        ))}
      </ul>

      <div className="rounded-[var(--radius-vela-md)] border border-line bg-bg-inset p-3">
        <F label="Car type">
          <select className={field} value={carType} onChange={(e) => setCarType(e.target.value)}>
            {CAR_TYPES.map((t) => <option key={t}>{t}</option>)}
          </select>
        </F>
        <F label="Distributor">
          {adding ? (
            <div className="flex gap-2">
              <input className={field} placeholder="New distributor name" value={newName} autoFocus
                onChange={(e) => setNewName(e.target.value)} />
              <Button size="sm" disabled={!newName} onClick={createDistributor}>Add</Button>
              <Button size="sm" variant="ghost" onClick={() => { setAdding(false); setNewName('') }}>✕</Button>
            </div>
          ) : (
            <select className={field} value={distId}
              onChange={(e) => { if (e.target.value === '__new') { setAdding(true) } else setDistId(e.target.value) }}>
              <option value="">—</option>
              {distributors.map((d: any) => <option key={d.id} value={d.id}>{d.name}</option>)}
              <option value="__new">＋ new distributor…</option>
            </select>
          )}
        </F>
        <F label="Effective from (optional)">
          <input type="date" className={field} value={from} onChange={(e) => setFrom(e.target.value)} />
        </F>
        <p className="mb-2 text-[11px] text-t2">
          Assigned per (brand, car type) and applies to every {carType.toLowerCase()} model of {brandName}.
          Left blank, it takes effect from this brand's first month so existing history is covered;
          give a date instead and the current assignment is closed on that day.
        </p>
        <Button size="sm" disabled={busy || !distId}
          onClick={() => run(() => api.setBrandAssignment(brandId, {
            car_type: carType, distributor_id: Number(distId), valid_from: from,
          }), `Distributor set for ${brandName} / ${carType}.`)}>Set distributor</Button>
      </div>
    </>
  )
}

function ModelEditor({ model, brand, segments, onSave, onMerge, onMessage }: any) {
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
      <p className="mb-2 text-[11px] text-t2">Specs apply to every month's facts for this model. Leaving one blank clears it.</p>
      <div className="mb-3 flex gap-2">
        <Button size="sm" onClick={() => onSave({ name, car_type: carType, engine_type: engine, supply, segment_id: segId || null })}>Save</Button>
        <Button size="sm" variant="secondary" onClick={onMerge}>Merge into…</Button>
      </div>
      <BrandInherited brandId={brand.id} brandName={brand.name} carType={carType} onMessage={onMessage} />
    </div>
  )
}

function F({ label, children }: { label: string; children: React.ReactNode }) {
  return <label className="mb-2 block text-[11.5px] font-semibold text-t1">{label}<div className="mt-1">{children}</div></label>
}

function NewBrandModal({ open, brands, onClose, onSubmit }: any) {
  const [name, setName] = useState(''); const [origin, setOrigin] = useState('China'); const [parent, setParent] = useState('')
  const [carType, setCarType] = useState('Passenger'); const [distId, setDistId] = useState('')
  const [distributors, setDistributors] = useState<any[]>([])
  useEffect(() => {
    if (open) {
      setName(''); setOrigin('China'); setParent(''); setCarType('Passenger'); setDistId('')
      api.distributors().then(setDistributors).catch(() => {})
    }
  }, [open])
  return (
    <Modal open={open} onClose={onClose} title="New brand"
      footer={<><Button variant="ghost" onClick={onClose}>Cancel</Button><Button disabled={!name}
        onClick={() => onSubmit({
          name, origin, parent_id: parent ? Number(parent) : null,
          car_type: distId ? carType : '', distributor_id: distId ? Number(distId) : 0,
        })}>Create</Button></>}>
      <F label="Name"><input className={field} value={name} onChange={(e) => setName(e.target.value)} autoFocus /></F>
      <F label="Origin"><select className={field} value={origin} onChange={(e) => setOrigin(e.target.value)}>{ORIGINS.map((o) => <option key={o}>{o}</option>)}</select></F>
      <F label="Parent brand (optional)"><select className={field} value={parent} onChange={(e) => setParent(e.target.value)}><option value="">—</option>{brands.map((b: any) => <option key={b.id} value={b.id}>{b.name}</option>)}</select></F>
      <F label="Distributor (optional)"><select className={field} value={distId} onChange={(e) => setDistId(e.target.value)}><option value="">— none</option>{distributors.map((d: any) => <option key={d.id} value={d.id}>{d.name}</option>)}</select></F>
      {distId && <F label="…for car type"><select className={field} value={carType} onChange={(e) => setCarType(e.target.value)}>{CAR_TYPES.map((t) => <option key={t}>{t}</option>)}</select></F>}
      <p className="text-[11px] text-t2">Distributor is assigned per (brand, car type); add more car types from the brand's form after creating it.</p>
    </Modal>
  )
}

function NewModelModal({ open, brand, segments, onClose, onSubmit, onMessage }: any) {
  const [name, setName] = useState(''); const [carType, setCarType] = useState('Passenger'); const [seg, setSeg] = useState('')
  const [engine, setEngine] = useState(''); const [supply, setSupply] = useState('')
  useEffect(() => { if (open) { setName(''); setCarType('Passenger'); setSeg(''); setEngine(''); setSupply('') } }, [open])
  if (!brand) return null
  return (
    <Modal open={open} onClose={onClose} title={`New model under ${brand.name}`}
      footer={<><Button variant="ghost" onClick={onClose}>Cancel</Button><Button disabled={!name} onClick={() => onSubmit({ name, car_type: carType, engine_type: engine, supply, segment_id: seg ? Number(seg) : null })}>Create</Button></>}>
      <F label="Name"><input className={field} value={name} onChange={(e) => setName(e.target.value)} autoFocus /></F>
      <F label="Car type"><select className={field} value={carType} onChange={(e) => setCarType(e.target.value)}>{CAR_TYPES.map((t) => <option key={t}>{t}</option>)}</select></F>
      <F label="Segment"><select className={field} value={seg} onChange={(e) => setSeg(e.target.value)}><option value="">—</option>{segments.map((s: any) => <option key={s.id} value={s.id}>{s.name}</option>)}</select></F>
      <F label="Engine"><select className={field} value={engine} onChange={(e) => setEngine(e.target.value)}><option value="">—</option>{ENGINES.map((t) => <option key={t}>{t}</option>)}</select></F>
      <F label="Supply"><select className={field} value={supply} onChange={(e) => setSupply(e.target.value)}><option value="">—</option>{SUPPLIES.map((t) => <option key={t}>{t}</option>)}</select></F>
      <p className="mb-3 text-[11px] text-t2">Engine and supply are model-level specs — the monthly feed carries neither, so they apply to every month of this model's facts.</p>
      <BrandInherited brandId={brand.id} brandName={brand.name} carType={carType} onMessage={onMessage} />
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
