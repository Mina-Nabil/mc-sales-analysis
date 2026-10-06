import { useEffect, useState } from 'react'
import { api, fmt } from '@/lib/api'
import { Button } from '@/components/ui'
import { ORIGINS, field } from '@/lib/vocab'

// Origin and distributor are brand-level, never model-level:
//   • origin  is brands.origin           → analytics dimension COALESCE(b.origin,'Unknown')
//   • distributor is an effective-dated distributor_assignments row on
//     (brand_id, car_type) → resolved per fact period by a LATERAL join
// Nothing is stored on a fact, so editing either is visible across all history
// the moment it saves — there is nothing to propagate.
//
// These helpers let a model form show and edit the values a model inherits, while
// being explicit that the write lands on the brand and reaches every sibling.

/** The assignment in force for a car type: latest range that has not ended. */
export function inForce(assignments: any[], carType: string) {
  const today = new Date().toISOString().slice(0, 10)
  return assignments
    .filter((a) => a.car_type === carType)
    .filter((a) => !a.valid_to || a.valid_to.slice(0, 10) > today)
    .sort((a, b) => (a.valid_from < b.valid_from ? 1 : -1))[0]
}

export function distributorLabel(assignments: any[], carType: string) {
  return inForce(assignments, carType)?.distributor_name || 'No distributor'
}

/** Formats a yyyy-mm-dd (or ISO timestamp) for display; '' → 'open'. */
export const day = (s?: string | null) => (s ? s.slice(0, 10) : '—')

/**
 * Inherited-from-brand block for a model form. Loads the brand's own record and
 * its assignments, and writes changes straight back to the brand.
 */
export function BrandInherited({ brandId, brandName, carType, onMessage }: {
  brandId: number
  brandName: string
  carType: string
  onMessage?: (m: string) => void
}) {
  const [brand, setBrand] = useState<any>(null)
  const [assignments, setAssignments] = useState<any[]>([])
  const [distributors, setDistributors] = useState<any[]>([])
  const [origin, setOrigin] = useState('')
  const [distId, setDistId] = useState('')
  const [busy, setBusy] = useState(false)

  const reload = () => Promise.all([
    api.brand(brandId).then((b: any) => { setBrand(b); setOrigin(b.origin || '') }),
    api.brandAssignments(brandId).then(setAssignments),
  ])
  useEffect(() => { reload().catch(() => {}); api.distributors().then(setDistributors).catch(() => {}) }, [brandId])
  useEffect(() => { setDistId(String(inForce(assignments, carType)?.distributor_id || '')) }, [assignments, carType])

  async function run(fn: () => Promise<any>, ok: string) {
    setBusy(true)
    try { await fn(); await reload(); onMessage?.(ok) }
    catch (e: any) { onMessage?.('⚠ ' + e.message) }
    finally { setBusy(false) }
  }

  const current = inForce(assignments, carType)
  const originDirty = (brand?.origin || '') !== origin
  const distDirty = String(current?.distributor_id || '') !== distId

  return (
    <div className="mb-3 rounded-[var(--radius-vela-md)] border border-dashed border-line px-3 py-2.5">
      <div className="mb-2 text-[11px] font-bold uppercase tracking-wide text-t2">
        Inherited from {brandName}
      </div>

      <div className="mb-2 flex items-end gap-2">
        <div className="min-w-0 flex-1">
          <label className="block text-[11.5px] font-semibold text-t1">Origin
            <select className={field + ' mt-1'} value={origin} onChange={(e) => setOrigin(e.target.value)}>
              <option value="">— Unknown</option>
              {ORIGINS.map((o) => <option key={o}>{o}</option>)}
            </select>
          </label>
        </div>
        <Button size="sm" variant="secondary" disabled={busy || !originDirty}
          onClick={() => run(() => api.editBrand(brandId, {
            name: brand.name, origin, notes: brand.notes || '', parent_id: brand.parent_id ?? null,
          }), `Origin of ${brandName} set to ${origin || 'Unknown'}.`)}>Save</Button>
      </div>

      <div className="flex items-end gap-2">
        <div className="min-w-0 flex-1">
          <label className="block text-[11.5px] font-semibold text-t1">
            Distributor {carType ? `(${carType})` : ''}
            <select className={field + ' mt-1'} value={distId} disabled={!carType}
              onChange={(e) => setDistId(e.target.value)}>
              <option value="">— No distributor</option>
              {distributors.map((d: any) => <option key={d.id} value={d.id}>{d.name}</option>)}
            </select>
          </label>
        </div>
        <Button size="sm" variant="secondary" disabled={busy || !carType || !distDirty || !distId}
          onClick={() => run(() => api.setBrandAssignment(brandId, {
            car_type: carType, distributor_id: Number(distId),
          }), `Distributor for ${brandName} / ${carType} updated.`)}>Save</Button>
      </div>

      <p className="mt-2 text-[11px] text-t2">
        Origin is brand-level and distributor is per (brand, car type) — neither is stored on a
        model, so saving here affects <b>every {carType || 'matching'} model of {brandName}</b> across
        all history{current ? `. In force since ${day(current.valid_from)}` : ''}
        {current?.units ? ` · ${fmt(current.units)} units` : ''}.
      </p>
    </div>
  )
}
