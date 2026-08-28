import { useEffect, useRef, useState } from 'react'
import { api, fmt } from '@/lib/api'
import { Card, CardHeader, CardTitle, Button, Badge, Modal, Select, Input, Checkbox, FormField } from '@/components/ui'

const MONTHS = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December']

// The traffic-authority feeds should be named YYYY-MM.xlsx (e.g. 2026-07.xlsx).
// deriveFromName pulls that period out of a file name, or returns null.
function deriveFromName(name: string): { year: number; month: number } | null {
  const m = name.match(/(20\d{2})[-_.]?(0[1-9]|1[0-2])/)
  if (!m) return null
  return { year: Number(m[1]), month: Number(m[2]) }
}

export default function Import() {
  const now = new Date()
  const [upload, setUpload] = useState<any>(null)
  const [report, setReport] = useState<any>(null)
  const [busy, setBusy] = useState(false)
  const [msg, setMsg] = useState('')
  const [reason, setReason] = useState('')
  const [batches, setBatches] = useState<any[]>([])
  const fileRef = useRef<HTMLInputElement>(null)

  // Period modal state
  const [pending, setPending] = useState<File | null>(null)
  const [year, setYear] = useState(now.getFullYear())
  const [month, setMonth] = useState(now.getMonth() + 1)
  const [fromName, setFromName] = useState(false)
  const nameDerived = pending ? deriveFromName(pending.name) : null

  const loadBatches = () => api.imports().then(setBatches).catch(() => {})
  useEffect(() => { loadBatches() }, [])

  function onFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = '' // allow re-picking the same file
    if (!file) return
    setMsg(''); setReport(null); setUpload(null)
    const d = deriveFromName(file.name)
    if (d) { setYear(d.year); setMonth(d.month); setFromName(true) }
    else { setFromName(false) }
    setPending(file)
  }

  async function startImport() {
    if (!pending) return
    const yr = fromName && nameDerived ? nameDerived.year : year
    const mo = fromName && nameDerived ? nameDerived.month : month
    setYear(yr); setMonth(mo)
    const file = pending
    setPending(null)
    setBusy(true); setMsg(''); setReport(null); setUpload(null)
    try {
      const up = await api.upload(file, yr, mo)
      setUpload(up)
      setReport(await api.dryRun(up.upload_id, yr, mo))
    } catch (err: any) { setMsg('⚠ ' + err.message) } finally { setBusy(false) }
  }

  async function commit() {
    if (!upload) return
    setBusy(true); setMsg('')
    try {
      const r = await api.commit(upload.upload_id, reason, year, month)
      setMsg(`${r.revised ? 'Revised' : 'Committed'} batch #${r.batch_id}: ${fmt(r.car_volume)} car units (${fmt(r.dropped_moto_volume)} motorcycle units dropped).`)
      setReport(null); setUpload(null); setReason(''); loadBatches()
    } catch (err: any) { setMsg('⚠ ' + err.message) } finally { setBusy(false) }
  }
  const swingWarn = report && (report.swing_pct > 30 || report.swing_pct < -30)

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div><h1 className="text-xl font-extrabold text-t0 sm:text-[26px]">Import a month</h1>
          <p className="mt-1 text-[13px] text-t1">Upload the traffic-authority primary feed → dry-run → commit.</p></div>
        <Button onClick={() => fileRef.current?.click()} disabled={busy}>{busy ? 'Working…' : 'Choose .xlsx'}</Button>
        <input ref={fileRef} type="file" accept=".xlsx" onChange={onFile} hidden />
      </div>
      {msg && <div className="rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-3.5 py-2.5 text-[13px] text-t0">{msg}</div>}

      <Modal
        open={!!pending}
        onClose={() => setPending(null)}
        title="Set the period"
        size="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setPending(null)}>Cancel</Button>
            <Button onClick={startImport} disabled={fromName && !nameDerived}>Import</Button>
          </>
        }
      >
        <div className="space-y-4">
          <p className="truncate text-[12.5px] text-t1">📄 {pending?.name}</p>
          <div className="grid grid-cols-2 gap-3">
            <FormField label="Month">
              <Select value={month} onChange={(e) => setMonth(Number(e.target.value))} disabled={fromName}>
                {MONTHS.map((m, i) => <option key={i} value={i + 1}>{m}</option>)}
              </Select>
            </FormField>
            <FormField label="Year">
              <Input type="number" min={2000} max={2100} value={year}
                onChange={(e) => setYear(Number(e.target.value))} disabled={fromName} />
            </FormField>
          </div>
          <div>
            <Checkbox
              checked={fromName}
              onChange={(e) => setFromName(e.target.checked)}
              label="Derive month & year from the file name"
            />
            <p className="mt-1.5 pl-6 text-[11.5px] text-t2">
              Name the file <code className="rounded bg-bg-3 px-1 py-0.5 text-t1">YYYY-MM.xlsx</code> — e.g. <code className="rounded bg-bg-3 px-1 py-0.5 text-t1">2026-07.xlsx</code>.
              {fromName && (nameDerived
                ? <span className="font-semibold text-t0"> Detected {MONTHS[nameDerived.month - 1]} {nameDerived.year}.</span>
                : <span className="font-semibold text-bad"> Could not find a date in “{pending?.name}”.</span>)}
            </p>
          </div>
        </div>
      </Modal>

      {report && (
        <Card>
          <CardHeader>
            <CardTitle>Dry run — {report.period_year}-{String(report.period_month).padStart(2, '0')}</CardTitle>
            {report.existing_batch && <Badge variant="warning">Will revise existing batch</Badge>}
          </CardHeader>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <KV label="Parsed" value={`${fmt(report.parsed_rows)} rows`} sub={`${fmt(report.parsed_volume)} units`} />
            <KV label="Motorcycles dropped" value={fmt(report.dropped_moto_rows)} sub={`${fmt(report.dropped_moto_volume)} units`} />
            <KV label="Car facts" value={fmt(report.car_rows)} sub={`${fmt(report.car_volume)} units`} />
            <KV label={`vs ${report.prev_period || '—'}`} value={report.swing_pct ? `${report.swing_pct.toFixed(1)}%` : '—'}
              sub={report.prev_period_volume ? `${fmt(report.prev_period_volume)} units` : undefined} warn={swingWarn} />
          </div>

          <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
            <KV label="Brand resolved" value={fmt(report.brand_resolved)} />
            <KV label="Brand + model resolved" value={fmt(report.brand_model_resolved)} />
            <KV label="Unresolved" value={fmt(report.tier_rows?.unresolved || 0)}
              sub={`${fmt(report.tier_volume?.unresolved || 0)} units`} warn={(report.tier_rows?.unresolved || 0) > 0} />
            <KV label="Motorcycle filter" value={report.exclude_motorcycles ? 'On' : 'Off'} />
          </div>

          <div className="mt-4">
            <div className="mb-2 text-[12px] font-bold uppercase tracking-wide text-t2">Match ladder</div>
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
              {['exact', 'normalized', 'nospace', 'unresolved'].map((t) => (
                <KV key={t} label={t} value={fmt(report.tier_rows?.[t] || 0)} sub={`${fmt(report.tier_volume?.[t] || 0)} units`} />
              ))}
            </div>
          </div>

          <div className="mt-4 grid grid-cols-1 gap-3 text-[13px] sm:grid-cols-3">
            <Meta label="File" value={upload?.filename || '—'} />
            <Meta label="Feed" value={report.signature || '—'} />
            <Meta label="Period" value={`${report.period_year}-${String(report.period_month).padStart(2, '0')}`} />
          </div>
          {report.new_brands?.length > 0 && (
            <div className="mt-4">
              <div className="mb-2 text-[12px] font-bold uppercase tracking-wide text-t2">Top new brands → review</div>
              <div className="flex flex-wrap gap-2">
                {report.new_brands.slice(0, 8).map((b: any, i: number) => (
                  <span key={i} className="rounded-full border border-warn/40 bg-warn-soft px-2.5 py-1 text-[12px] text-warn"><span dir="rtl">{b.raw}</span> · {fmt(b.volume)}</span>
                ))}
              </div>
            </div>
          )}
          {report.new_models?.length > 0 && (
            <div className="mt-4">
              <div className="mb-2 text-[12px] font-bold uppercase tracking-wide text-t2">Top new models → review</div>
              <div className="flex flex-wrap gap-2">
                {report.new_models.slice(0, 8).map((m: any, i: number) => (
                  <span key={i} className="rounded-full border border-line bg-bg-inset px-2.5 py-1 text-[12px] text-t1"><span dir="rtl">{m.raw}</span> · {fmt(m.volume)}</span>
                ))}
              </div>
            </div>
          )}
          <div className="mt-5 flex flex-wrap items-center gap-2">
            <input placeholder="revision reason (optional)" value={reason} onChange={(e) => setReason(e.target.value)}
              className="h-10 min-w-[220px] flex-1 rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-3 text-[13px] text-t0" />
            <Button onClick={commit} disabled={busy}>Commit</Button>
          </div>
        </Card>
      )}

      {report && upload && <RowsTable token={upload.upload_id} year={year} month={month} />}

      {!report && (
      <Card padding="none" className="overflow-hidden">
        <div className="border-b border-line px-5 py-4"><CardTitle>Recent batches</CardTitle></div>
        <div className="overflow-x-auto">
          <table className="w-full min-w-[640px] border-collapse text-sm">
            <thead><tr className="border-b border-line">
              {['#', 'Period', 'State', 'Rows', 'Units', 'Moto dropped'].map((h, i) => (
                <th key={i} className={'px-4 py-3 text-[10.5px] font-bold uppercase tracking-wide text-t2 ' + (i >= 3 ? 'text-right' : 'text-left')}>{h}</th>
              ))}
            </tr></thead>
            <tbody>
              {batches.map((b) => (
                <tr key={b.id} className="border-b border-line last:border-0 hover:bg-bg-3">
                  <td className="px-4 py-2.5 text-t1">{b.id}</td>
                  <td className="px-4 py-2.5 font-semibold text-t0">{b.period_year}-{String(b.period_month).padStart(2, '0')}</td>
                  <td className="px-4 py-2.5"><Badge status={b.state === 'committed' ? 'confirmed' : b.state}>{b.state}</Badge></td>
                  <td className="px-4 py-2.5 text-right tabular-nums text-t1">{fmt(b.rows)}</td>
                  <td className="px-4 py-2.5 text-right font-bold tabular-nums text-t0">{fmt(b.volume)}</td>
                  <td className="px-4 py-2.5 text-right tabular-nums text-t2">{fmt(b.dropped_volume)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>
      )}
    </div>
  )
}

function Meta({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center gap-2 rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-3.5 py-2">
      <span className="shrink-0 text-[11.5px] text-t2">{label}</span>
      <span className="truncate font-semibold text-t0">{value}</span>
    </div>
  )
}

function KV({ label, value, sub, warn }: { label: string; value: string; sub?: string; warn?: boolean }) {
  return (
    <div className={'rounded-[var(--radius-vela-md)] border px-4 py-3 ' + (warn ? 'border-warn/40 bg-warn-soft' : 'border-line bg-bg-inset')}>
      <div className="text-[11px] text-t2">{label}</div>
      <div className={'text-lg font-extrabold ' + (warn ? 'text-warn' : 'text-t0')}>{value}{warn ? ' ⚠' : ''}</div>
      {sub && <div className="text-[11px] text-t1">{sub}</div>}
    </div>
  )
}

// ── Loaded rows, laid out like the source sheet ──────────────────────────────
// Governorate → Traffic unit → Brand are merged (rowspan) exactly as they are
// in the feed, with one row per model + its volume. RTL, since the raw values
// are Arabic. Server-paginated: a month is ~64k rows.
const STATUS_LABEL: Record<string, string> = {
  all: 'All rows', resolved: 'Resolved', new_model: 'New model', new_brand: 'New brand', motorcycle: 'Motorcycle (dropped)',
}
const STATUS_STYLE: Record<string, string> = {
  resolved: 'text-ok',
  new_model: 'text-warn',
  new_brand: 'text-bad',
  motorcycle: 'text-t2',
}

function RowsTable({ token, year, month }: { token: string; year: number; month: number }) {
  const [data, setData] = useState<any>(null)
  const [offset, setOffset] = useState(0)
  const [status, setStatus] = useState('all')
  const [q, setQ] = useState('')
  const [term, setTerm] = useState('')
  const limit = 200

  // debounce the search box
  useEffect(() => { const t = setTimeout(() => { setTerm(q); setOffset(0) }, 350); return () => clearTimeout(t) }, [q])

  useEffect(() => {
    const p = new URLSearchParams({ offset: String(offset), limit: String(limit), status, year: String(year), month: String(month) })
    if (term) p.set('q', term)
    let alive = true
    api.importRows(token, p.toString()).then((d) => { if (alive) setData(d) }).catch(() => { if (alive) setData({ rows: [], total: 0, counts: {} }) })
    return () => { alive = false }
  }, [token, offset, status, term, year, month])

  const rows: any[] = data?.rows || []
  // Merge repeated governorate / unit / brand cells within this page.
  const span = (key: 'gov' | 'unit' | 'brand', i: number) => {
    const same = (a: any, b: any) =>
      a.gov === b.gov && (key === 'gov' || a.unit === b.unit) && (key !== 'brand' || a.brand === b.brand)
    if (i > 0 && same(rows[i - 1], rows[i])) return 0 // covered by the cell above
    let n = 1
    while (i + n < rows.length && same(rows[i], rows[i + n])) n++
    return n
  }
  const total = data?.total ?? 0
  const pageEnd = Math.min(offset + limit, total)

  return (
    <Card padding="none" className="overflow-hidden">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-5 py-4">
        <div>
          <CardTitle>Rows to be loaded</CardTitle>
          <p className="mt-0.5 text-[12.5px] text-t1">
            {data ? <>{fmt(total)} rows{term || status !== 'all' ? ` of ${fmt(data.grand_total)}` : ''} · {fmt(data.shown_volume)} units</> : 'Loading…'}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search brand, model, unit…" dir="auto"
            className="h-9 w-[220px] rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-3 text-[13px] text-t0" />
          <select value={status} onChange={(e) => { setStatus(e.target.value); setOffset(0) }}
            className="h-9 rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-2.5 text-[13px] text-t0">
            {Object.keys(STATUS_LABEL).map((k) => (
              <option key={k} value={k}>{STATUS_LABEL[k]}{data?.counts?.[k] != null ? ` (${fmt(data.counts[k])})` : ''}</option>
            ))}
          </select>
        </div>
      </div>

      <div className="max-h-[720px] overflow-auto">
        <table className="w-full min-w-[720px] border-collapse text-sm">
          <thead>
            <tr className="border-b border-line">
              {['Governorate', 'Traffic unit', 'Brand', 'Model', 'Volume', 'Status'].map((h, i) => (
                <th key={h} className={'sticky top-0 z-10 bg-bg-2 px-4 py-3 text-[10.5px] font-bold uppercase tracking-wide text-t2 ' + (i === 4 ? 'text-right' : 'text-left')}>{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {data === null && <tr><td colSpan={6} className="px-4 py-10 text-center text-[13px] text-t2">Loading…</td></tr>}
            {rows.map((r, i) => {
              const gs = span('gov', i), us = span('unit', i), bs = span('brand', i)
              return (
                <tr key={i} className="border-b border-line last:border-0 hover:bg-bg-3">
                  {gs > 0 && <td rowSpan={gs} className="border-l border-line px-4 py-2 align-top font-semibold text-t0" dir="rtl">{r.gov}</td>}
                  {us > 0 && <td rowSpan={us} className="border-l border-line px-4 py-2 align-top text-t1" dir="rtl">{r.unit}</td>}
                  {bs > 0 && (
                    <td rowSpan={bs} className="border-l border-line px-4 py-2 align-top text-t1" dir="rtl">
                      {r.brand}
                      {r.canon_brand && <span className="mt-0.5 block text-[11px] text-t2" dir="ltr">→ {r.canon_brand}</span>}
                    </td>
                  )}
                  <td className="border-l border-line px-4 py-2 text-t0" dir="rtl">
                    {r.model || <span className="text-t2">(blank)</span>}
                    {r.canon_model && <span className="mt-0.5 block text-[11px] text-t2" dir="ltr">→ {r.canon_model}</span>}
                  </td>
                  <td className="px-4 py-2 text-right font-bold tabular-nums text-t0">{fmt(r.volume)}</td>
                  <td className={'px-4 py-2 text-[12px] font-semibold ' + (STATUS_STYLE[r.status] || 'text-t1')}>
                    {STATUS_LABEL[r.status] || r.status}
                  </td>
                </tr>
              )
            })}
            {data && rows.length === 0 && <tr><td colSpan={6} className="px-4 py-10 text-center text-[13px] text-t2">No matching rows.</td></tr>}
          </tbody>
        </table>
      </div>

      {total > limit && (
        <div className="flex items-center justify-between gap-3 border-t border-line px-5 py-3">
          <span className="text-[12.5px] text-t2">{fmt(offset + 1)}–{fmt(pageEnd)} of {fmt(total)}</span>
          <div className="flex gap-2">
            <Button size="sm" variant="secondary" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - limit))}>← Prev</Button>
            <Button size="sm" variant="secondary" disabled={pageEnd >= total} onClick={() => setOffset(offset + limit)}>Next →</Button>
          </div>
        </div>
      )}
    </Card>
  )
}
