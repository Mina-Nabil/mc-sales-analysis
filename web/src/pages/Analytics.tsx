import { useEffect, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, fmt } from '@/lib/api'
import { Card, CardHeader, CardTitle, CardSubtitle, Badge } from '@/components/ui'
import { AreaLineChart, BarChart, DonutChart } from '@/components/charts'
import { FilterBar, useFilters } from '@/components/FilterBar'

const MONTHS = ['', 'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
const PALETTE = ['var(--acc)', 'var(--info)', 'var(--ok)', 'var(--warn)', 'var(--bad)', 'var(--acc-2)', '#c084fc', '#22d3ee']
const selCls = 'h-9 rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-2.5 text-[13px] text-t0'

export default function Analytics() {
  const [sp, setSp] = useSearchParams()
  const { filters } = useFilters()
  const [years, setYears] = useState<number[]>([])
  const [brands, setBrands] = useState<any[]>([])
  const [segments, setSegments] = useState<any[]>([])
  const [ts, setTs] = useState<any[]>([])

  const year = sp.get('year') || 'all'
  const setYear = (v: string) => setSp((prev) => { const n = new URLSearchParams(prev); if (v === 'all') n.delete('year'); else n.set('year', v); return n }, { replace: true })

  const qs = useMemo(() => {
    const p = new URLSearchParams()
    for (const d in filters) p.set('filter.' + d, filters[d].join(','))
    if (year !== 'all') p.set('year', year)
    return p.toString()
  }, [JSON.stringify(filters), year]) // eslint-disable-line

  useEffect(() => { api.years().then(setYears).catch(() => {}) }, [])
  useEffect(() => {
    api.agg('brand', qs).then(setBrands).catch(() => setBrands([]))
    api.agg('segment', qs).then(setSegments).catch(() => setSegments([]))
    api.timeseries(qs).then(setTs).catch(() => setTs([]))
  }, [qs])

  const label = year === 'all' ? 'All time' : year

  const series = useMemo(() => {
    if (year === 'all') return { data: ts.map((p) => Number(p.volume)), labels: ts.map((p) => `${MONTHS[p.month]} ${String(p.year).slice(2)}`) }
    const byMonth = new Map<number, number>()
    ts.forEach((p) => byMonth.set(p.month, Number(p.volume)))
    const data: number[] = [], labels: string[] = []
    for (let m = 1; m <= 12; m++) { data.push(byMonth.get(m) || 0); labels.push(MONTHS[m]) }
    return { data, labels }
  }, [ts, year])

  const topBrands = brands.filter((b) => b.volume > 0).slice(0, 8).map((b, i) => ({ label: b.key, value: Number(b.volume), color: PALETTE[i % PALETTE.length] }))
  const segMix = segments.filter((s) => s.volume > 0).slice(0, 7).map((s, i) => ({ label: s.key, value: Number(s.volume), color: PALETTE[i % PALETTE.length] }))
  const latest = series.data[series.data.length - 1] || 0
  const prev = series.data[series.data.length - 2] || 0
  const mom = prev ? ((latest - prev) / prev) * 100 : 0

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-extrabold text-t0 sm:text-[26px]">Analytics</h1>
          <p className="mt-1 text-[13px] text-t1">Registrations trend, brand &amp; segment breakdowns.</p>
        </div>
        <label className="flex items-center gap-2 text-[12px] font-semibold text-t1">
          Year
          <select className={selCls} value={year} onChange={(e) => setYear(e.target.value)}>
            <option value="all">All time</option>
            {years.map((y) => <option key={y} value={y}>{y}</option>)}
          </select>
        </label>
      </div>

      <Card padding="sm"><FilterBar /></Card>

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

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader><div><CardTitle>Top brands</CardTitle><CardSubtitle>By units · {label}</CardSubtitle></div></CardHeader>
          {topBrands.length ? <BarChart data={topBrands} height={240} formatValue={(v) => fmt(v)} /> : <Empty />}
        </Card>
        <Card>
          <CardHeader><div><CardTitle>Segment mix</CardTitle><CardSubtitle>Share of body types · {label}</CardSubtitle></div></CardHeader>
          {segMix.length ? <DonutChart segments={segMix} size={170} /> : <Empty />}
        </Card>
      </div>

      <Card>
        <CardHeader><div><CardTitle>Brand leaderboard</CardTitle><CardSubtitle>Top 12 by volume · {label}</CardSubtitle></div></CardHeader>
        <div className="space-y-2.5">
          {brands.slice(0, 12).map((b, i) => {
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
    </div>
  )
}

function Empty() {
  return <div className="grid h-40 place-items-center text-[13px] text-t2">No data</div>
}
