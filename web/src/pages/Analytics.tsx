import { useEffect, useMemo, useState } from 'react'
import { api, fmt } from '@/lib/api'
import { Card, CardHeader, CardTitle, CardSubtitle, Badge } from '@/components/ui'
import { AreaLineChart, BarChart, DonutChart } from '@/components/charts'

const MONTHS = ['', 'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
const PALETTE = ['var(--acc)', 'var(--info)', 'var(--ok)', 'var(--warn)', 'var(--bad)', 'var(--acc-2)', '#c084fc', '#22d3ee']

export default function Analytics() {
  const [batches, setBatches] = useState<any[]>([])
  const [brands, setBrands] = useState<any[]>([])
  const [segments, setSegments] = useState<any[]>([])

  useEffect(() => {
    api.imports().then(setBatches).catch(() => {})
    api.brands().then(setBrands).catch(() => {})
    api.segments().then(setSegments).catch(() => {})
  }, [])

  // committed batches → monthly time series (oldest → newest)
  const series = useMemo(() => {
    const committed = batches
      .filter((b) => b.state === 'committed')
      .sort((a, b) => a.period_year * 12 + a.period_month - (b.period_year * 12 + b.period_month))
    return {
      data: committed.map((b) => b.volume),
      labels: committed.map((b) => `${MONTHS[b.period_month]} ${String(b.period_year).slice(2)}`),
    }
  }, [batches])

  const topBrands = useMemo(
    () => brands.filter((b) => b.volume > 0).slice(0, 8).map((b, i) => ({ label: b.name, value: b.volume, color: PALETTE[i % PALETTE.length] })),
    [brands],
  )
  const segMix = useMemo(
    () => segments.filter((s) => s.volume > 0).sort((a, b) => b.volume - a.volume).slice(0, 7).map((s, i) => ({ label: s.name, value: s.volume, color: PALETTE[i % PALETTE.length] })),
    [segments],
  )

  const latest = series.data[series.data.length - 1] || 0
  const prev = series.data[series.data.length - 2] || 0
  const mom = prev ? ((latest - prev) / prev) * 100 : 0

  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-xl font-extrabold text-t0 sm:text-[26px]">Analytics</h1>
        <p className="mt-1 text-[13px] text-t1">Five years of monthly registrations, brand & segment breakdowns.</p>
      </div>

      <Card>
        <CardHeader>
          <div>
            <CardTitle>Monthly volume</CardTitle>
            <CardSubtitle>New-car units per committed period</CardSubtitle>
          </div>
          {series.data.length > 1 && (
            <Badge variant={mom >= 0 ? 'success' : 'danger'}>{mom >= 0 ? '↗' : '↘'} {Math.abs(mom).toFixed(1)}% MoM</Badge>
          )}
        </CardHeader>
        {series.data.length > 1
          ? <AreaLineChart data={series.data} labels={series.labels} height={260} formatValue={(v) => fmt(v)} />
          : <Empty />}
      </Card>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader><div><CardTitle>Top brands</CardTitle><CardSubtitle>By total units (all periods)</CardSubtitle></div></CardHeader>
          {topBrands.length ? <BarChart data={topBrands} height={240} formatValue={(v) => fmt(v)} /> : <Empty />}
        </Card>
        <Card>
          <CardHeader><div><CardTitle>Segment mix</CardTitle><CardSubtitle>Share of body types</CardSubtitle></div></CardHeader>
          {segMix.length ? <DonutChart segments={segMix} size={170} /> : <Empty />}
        </Card>
      </div>

      <Card>
        <CardHeader><div><CardTitle>Brand leaderboard</CardTitle><CardSubtitle>Top 12 by volume</CardSubtitle></div></CardHeader>
        <div className="space-y-2.5">
          {brands.slice(0, 12).map((b, i) => {
            const max = brands[0]?.volume || 1
            return (
              <div key={b.id} className="flex items-center gap-3">
                <span className="w-5 text-right text-[12px] font-bold text-t2">{i + 1}</span>
                <span className="w-28 shrink-0 truncate text-[13px] font-semibold text-t0">{b.name}</span>
                <div className="h-2.5 flex-1 overflow-hidden rounded-full bg-bg-3">
                  <div className="h-full rounded-full" style={{ width: `${(b.volume / max) * 100}%`, background: PALETTE[i % PALETTE.length] }} />
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
  return <div className="grid h-40 place-items-center text-[13px] text-t2">No data yet</div>
}
