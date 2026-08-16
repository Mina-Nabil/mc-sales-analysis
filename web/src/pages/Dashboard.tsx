import { useEffect, useMemo, useState } from 'react'
import { api, fmt } from '@/lib/api'
import { Card, CardHeader, CardTitle, CardSubtitle, Button, Badge, Tooltip } from '@/components/ui'
import { BarChart } from '@/components/charts'

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
const YEARS = [2026, 2025, 2024, 2023, 2022, 2021]
const CAR_TYPES = ['', 'Passenger', 'Commercial', 'Bus', 'Construction']
const PALETTE = ['var(--acc)', 'var(--info)', 'var(--ok)', 'var(--warn)', 'var(--bad)', 'var(--acc-2)', '#c084fc', '#22d3ee']
const selCls = 'h-9 rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-2.5 text-[13px] text-t0'

export default function Dashboard() {
  const [dims, setDims] = useState<string[]>([])
  const [dimension, setDimension] = useState('brand')
  const [year, setYear] = useState(2026)
  const [month, setMonth] = useState(0) // 0 = full year
  const [compare, setCompare] = useState(2025)
  const [compareMonth, setCompareMonth] = useState(0)
  const [carType, setCarType] = useState('')
  const [data, setData] = useState<any>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => { api.dimensions().then(setDims).catch(() => {}) }, [])

  const qs = useMemo(() => {
    const p = new URLSearchParams({ dimension, year: String(year), compare_year: String(compare), limit: '100' })
    if (month) p.set('month', String(month))
    if (compareMonth) p.set('compare_month', String(compareMonth))
    if (carType) p.set('filter.car_type', carType)
    return p.toString()
  }, [dimension, year, month, compare, compareMonth, carType])

  useEffect(() => {
    setLoading(true)
    api.matrix(qs).then(setData).catch(() => setData(null)).finally(() => setLoading(false))
  }, [qs])

  const rows = data?.rows || []
  const singleMonth = month > 0
  const primaryLabel = singleMonth ? `${MONTHS[month - 1]} ${year}` : `${year}`
  const compareLabel = compareMonth > 0
    ? `${MONTHS[compareMonth - 1]} ${compare}`
    : singleMonth ? `${MONTHS[month - 1]} ${compare}` : `${compare}`
  const dimLabel = cap(dimension)

  const bars = rows.slice(0, 8).map((r: any, i: number) => ({ label: r.key, value: r.total, color: PALETTE[i % PALETTE.length] }))

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-extrabold text-t0 sm:text-[26px]">Dashboard</h1>
          <p className="mt-1 text-[13px] text-t1">The matrix — any dimension × period, with share, growth &amp; rank.</p>
        </div>
        <a href={api.exportHref(qs)}><Button variant="secondary" icon={<Dl />}>Export .xlsx</Button></a>
      </div>

      <Card padding="sm">
        <div className="flex flex-wrap items-center gap-2.5">
          <Ctl label="Dimension">
            <select className={selCls} value={dimension} onChange={(e) => setDimension(e.target.value)}>
              {(dims.length ? dims : ['brand']).map((d) => <option key={d} value={d}>{cap(d)}</option>)}
            </select>
          </Ctl>
          <Ctl label="Period">
            <select className={selCls} value={year} onChange={(e) => setYear(Number(e.target.value))}>{YEARS.map((y) => <option key={y}>{y}</option>)}</select>
            <select className={selCls} value={month} onChange={(e) => setMonth(Number(e.target.value))}>
              <option value={0}>Full year</option>
              {MONTHS.map((m, i) => <option key={m} value={i + 1}>{m}</option>)}
            </select>
          </Ctl>
          <Ctl label="vs">
            <select className={selCls} value={compare} onChange={(e) => setCompare(Number(e.target.value))}>{YEARS.map((y) => <option key={y}>{y}</option>)}</select>
            <select className={selCls} value={compareMonth} onChange={(e) => setCompareMonth(Number(e.target.value))}>
              <option value={0}>{singleMonth ? 'Same month' : 'Full year'}</option>
              {MONTHS.map((m, i) => <option key={m} value={i + 1}>{m}</option>)}
            </select>
          </Ctl>
          <Ctl label="Car type"><select className={selCls} value={carType} onChange={(e) => setCarType(e.target.value)}>{CAR_TYPES.map((t) => <option key={t} value={t}>{t || 'All'}</option>)}</select></Ctl>
          {data && <span className="ml-auto text-[12px] text-t1">Total <b className="text-t0">{fmt(data.denominator)}</b> units · {rows.length} rows</span>}
        </div>
      </Card>

      <Card>
        <CardHeader><div><CardTitle>Top {dimLabel}s by volume</CardTitle><CardSubtitle>{primaryLabel}</CardSubtitle></div></CardHeader>
        {bars.length ? <BarChart data={bars} height={220} formatValue={(v) => fmt(v)} /> : <div className="grid h-40 place-items-center text-t2">No data</div>}
      </Card>

      <Card padding="none" className="overflow-hidden">
        <div className="flex items-center justify-between border-b border-line px-5 py-4">
          <CardTitle>{dimLabel} matrix · {primaryLabel} vs {compareLabel}</CardTitle>
          {loading && <span className="text-[12px] text-t2">Loading…</span>}
        </div>
        <div className="overflow-x-auto">
          <table className={'w-full border-collapse text-[13px] ' + (singleMonth ? 'min-w-[720px]' : 'min-w-[1100px]')}>
            <thead>
              <tr className="border-b border-line">
                <Th sticky>{dimLabel}</Th>
                {singleMonth ? (
                  <>
                    <Th align="right" hint={`Units registered in ${primaryLabel}`}>{primaryLabel}</Th>
                    <Th align="right" hint={`Units registered in ${compareLabel} (the comparison period)`}>{compareLabel}</Th>
                  </>
                ) : (
                  <>
                    {MONTHS.map((m) => <Th key={m} align="right" hint={`Units in ${m} ${year}`}>{m}</Th>)}
                    <Th align="right" hint={`Total units across ${year} (months with data)`}>Total</Th>
                  </>
                )}
                <Th align="right" hint="Share of the period total shown top-right (all rows sum to 100%)">Share</Th>
                <Th align="right" hint={`% change in volume vs ${compareLabel}. "new" = no volume in the compare period.`}>Growth</Th>
                <Th align="right" hint={`Change in market share, in percentage points, vs ${compareLabel}`}>Δ pts</Th>
                <Th align="right" hint={`Rank by ${primaryLabel} volume. Arrow shows movement vs ${compareLabel}.`}>Rank</Th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r: any) => (
                <tr key={r.key} className="border-b border-line last:border-0 hover:bg-bg-3">
                  <td className="sticky left-0 z-10 bg-bg-2 px-4 py-2 font-semibold text-t0" dir="auto">{r.key}</td>
                  {singleMonth ? (
                    <>
                      <td className="px-3 py-2 text-right font-bold tabular-nums text-t0">{fmt(r.total)}</td>
                      <td className="px-3 py-2 text-right tabular-nums text-t1">{fmt(r.ytd_prior)}</td>
                    </>
                  ) : (
                    <>
                      {r.months.map((m: number, i: number) => <td key={i} className={'px-2.5 py-2 text-right tabular-nums ' + (m ? 'text-t1' : 'text-t2')}>{m ? fmt(m) : '·'}</td>)}
                      <td className="px-3 py-2 text-right font-bold tabular-nums text-t0">{fmt(r.total)}</td>
                    </>
                  )}
                  <td className="px-3 py-2 text-right tabular-nums text-t1">{r.share_pct.toFixed(1)}%</td>
                  <td className="px-3 py-2 text-right">
                    {r.growth_pct === null
                      ? <Badge variant="info">new</Badge>
                      : <span className={r.growth_pct >= 0 ? 'text-ok' : 'text-bad'}>{r.growth_pct >= 0 ? '↗' : '↘'} {Math.abs(r.growth_pct).toFixed(0)}%</span>}
                  </td>
                  <td className={'px-3 py-2 text-right tabular-nums ' + (r.share_point_delta >= 0 ? 'text-ok' : 'text-bad')}>{r.share_point_delta >= 0 ? '+' : ''}{r.share_point_delta.toFixed(2)}</td>
                  <td className="px-3 py-2 text-right tabular-nums text-t1">
                    {r.rank}{r.rank_prior && r.rank_prior !== r.rank ? <span className="ml-1 text-[10px] text-t2">({r.rank < r.rank_prior ? '▲' : '▼'}{Math.abs(r.rank - r.rank_prior)})</span> : ''}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {rows.length === 0 && !loading && <div className="p-10 text-center text-sm text-t1">No data for this selection.</div>}
      </Card>
    </div>
  )
}

function Th({ children, align, sticky, hint }: { children: React.ReactNode; align?: 'right'; sticky?: boolean; hint?: string }) {
  const base = 'px-3 py-3 text-[10.5px] font-bold uppercase tracking-wide text-t2 ' +
    (align === 'right' ? 'text-right' : 'text-left') + (sticky ? ' sticky left-0 z-10 bg-bg-2 px-4' : '')
  if (!hint) return <th className={base}>{children}</th>
  return (
    <th className={base}>
      <Tooltip label={hint}>
        <span className="cursor-help border-b border-dotted border-t2/50">{children}</span>
      </Tooltip>
    </th>
  )
}

function Ctl({ label, children }: { label: string; children: React.ReactNode }) {
  return <label className="flex items-center gap-1.5 text-[12px] font-semibold text-t1">{label}{children}</label>
}
function cap(s: string) { return s ? s[0].toUpperCase() + s.slice(1).replace(/_/g, ' ') : s }
function Dl() {
  return <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3" /></svg>
}
