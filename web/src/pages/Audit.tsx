import { useEffect, useState } from 'react'
import { api, fmt } from '@/lib/api'
import { Card, CardTitle, Badge } from '@/components/ui'

export default function Audit() {
  const [rows, setRows] = useState<any[]>([])
  useEffect(() => { api.changes().then(setRows).catch(() => {}) }, [])
  return (
    <div className="space-y-5">
      <div><h1 className="text-xl font-extrabold text-t0 sm:text-[26px]">Audit log</h1>
        <p className="mt-1 text-[13px] text-t1">Every mutation — human, agent, migration — is recorded.</p></div>
      <Card padding="none" className="overflow-hidden">
        <div className="border-b border-line px-5 py-4"><CardTitle>Recent changes</CardTitle></div>
        <div className="overflow-x-auto">
          <table className="w-full min-w-[640px] border-collapse text-sm">
            <thead><tr className="border-b border-line">
              {['When', 'Entity', 'Action', 'Actor', 'Units'].map((h, i) => (
                <th key={i} className={'px-4 py-3 text-[10.5px] font-bold uppercase tracking-wide text-t2 ' + (h === 'Units' ? 'text-right' : 'text-left')}>{h}</th>
              ))}
            </tr></thead>
            <tbody>
              {rows.map((c) => (
                <tr key={c.id} className="border-b border-line last:border-0 hover:bg-bg-3">
                  <td className="px-4 py-2.5 text-[12px] text-t1">{new Date(c.created_at).toLocaleString()}</td>
                  <td className="px-4 py-2.5 text-t0">{c.entity_type}{c.entity_id ? ` #${c.entity_id}` : ''}</td>
                  <td className="px-4 py-2.5 text-t1">{c.action}</td>
                  <td className="px-4 py-2.5">
                    <Badge variant={c.actor_kind === 'human' ? 'info' : c.actor_kind === 'agent' ? 'accent' : 'neutral'}>{c.actor_kind}</Badge>
                  </td>
                  <td className="px-4 py-2.5 text-right tabular-nums text-t1">{c.volume_impact ? fmt(c.volume_impact) : ''}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  )
}
