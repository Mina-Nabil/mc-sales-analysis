import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, fmt } from '@/lib/api'
import { Card, CardHeader, CardTitle, StatCard, Button } from '@/components/ui'
import { DonutChart } from '@/components/charts'

const ic = (d: string) => (
  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round"><path d={d} /></svg>
)
const COLORS: Record<string, string> = {
  confirmed: 'var(--ok)', auto_resolved: 'var(--info)', needs_review: 'var(--warn)', unresolved: 'var(--bad)',
}

export default function Overview() {
  const [stats, setStats] = useState<any>(null)
  const [conf, setConf] = useState<any[]>([])

  useEffect(() => {
    api.stats().then(setStats).catch(() => {})
    api.confidence().then(setConf).catch(() => {})
  }, [])

  const total = conf.reduce((a, c) => a + c.volume, 0) || 1
  const order = ['confirmed', 'auto_resolved', 'needs_review', 'unresolved']
  const segments = order.filter((s) => conf.find((c) => c.status === s)).map((s) => ({
    label: s.replace('_', ' '), value: conf.find((c) => c.status === s)?.volume || 0, color: COLORS[s],
  }))
  const confirmedPct = Math.round(((conf.find((c) => c.status === 'confirmed')?.volume || 0) / total) * 100)

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-extrabold text-t0 sm:text-[26px]">Overview</h1>
          <p className="mt-1 text-[13px] text-t1">Egypt new-car registrations · cars only</p>
        </div>
        <Link to="/analytics"><Button variant="secondary">View analytics →</Button></Link>
      </div>

      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard label="Total units" value={fmt(stats?.volume)} icon={ic('M3 3v18h18M7 14l3-3 3 3 5-6')} />
        <StatCard label="Fact rows" value={fmt(stats?.facts)} iconColor="var(--info)" iconBg="var(--info-soft)" icon={ic('M3 3h18v18H3zM3 9h18M9 21V9')} />
        <StatCard label="Periods" value={fmt(stats?.periods)} iconColor="var(--ok)" iconBg="var(--ok-soft)" icon={ic('M8 2v4M16 2v4M3 10h18M5 4h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z')} />
        <StatCard label="Review queue" value={fmt(stats?.queue_depth)} iconColor="var(--warn)" iconBg="var(--warn-soft)"
          delta={stats ? { value: `${fmt(stats.queue_volume)} units`, positive: false } : undefined}
          icon={ic('M22 12h-6l-2 3h-4l-2-3H2M5 5h14l3 7v6a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1v-6z')} />
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-1">
          <CardHeader><CardTitle>Data confidence</CardTitle></CardHeader>
          <DonutChart segments={segments} centerValue={`${confirmedPct}%`} centerLabel="confirmed" />
        </Card>
        <Card className="lg:col-span-2">
          <CardHeader><CardTitle>Classification is the asset</CardTitle></CardHeader>
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
            <Mini label="Brands" value={fmt(stats?.brands)} />
            <Mini label="Models" value={fmt(stats?.models)} />
            <Mini label="Confirmed" value={`${confirmedPct}%`} />
            <Mini label="Unresolved" value={`${(((conf.find((c) => c.status === 'unresolved')?.volume || 0) / total) * 100).toFixed(1)}%`} />
          </div>
          <p className="mt-4 text-[13px] leading-relaxed text-t1">
            Every dashboard is derived from a human-owned car tree. Fixing one node re-derives all history —
            work the <Link to="/review" className="font-semibold text-acc">review queue</Link> to raise the confirmed share.
          </p>
        </Card>
      </div>
    </div>
  )
}

function Mini({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-4 py-3">
      <div className="text-xl font-extrabold text-t0">{value}</div>
      <div className="text-[11px] text-t1">{label}</div>
    </div>
  )
}
