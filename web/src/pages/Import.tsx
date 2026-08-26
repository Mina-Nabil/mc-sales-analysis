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
            <KV label={`vs ${report.prev_period || '—'}`} value={report.swing_pct ? `${report.swing_pct.toFixed(1)}%` : '—'} warn={swingWarn} />
          </div>
          <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
            {['exact', 'normalized', 'nospace', 'unresolved'].map((t) => (
              <KV key={t} label={t} value={fmt(report.tier_rows?.[t] || 0)} />
            ))}
          </div>
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
