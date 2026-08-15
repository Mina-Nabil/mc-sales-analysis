import { useEffect, useState } from 'react'
import { api } from '@/lib/api'
import { Card, CardHeader, CardTitle, Button } from '@/components/ui'

const LABELS: Record<string, string> = {
  fuzzy_threshold: 'Fuzzy auto-link threshold',
  fuzzy_review_floor: 'Fuzzy review floor',
  ai_threshold: 'AI auto-link threshold',
  ai_model: 'AI model (Bedrock inference profile)',
  'ingest.exclude_motorcycles': 'Exclude motorcycles',
}
const field = 'h-10 w-full rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-3 text-[13px] text-t0 focus:border-acc'

export default function Settings() {
  const [settings, setSettings] = useState<Record<string, string>>({})
  const [draft, setDraft] = useState<Record<string, string>>({})
  const [msg, setMsg] = useState('')

  useEffect(() => { api.settings().then((s) => { setSettings(s); setDraft(s) }).catch((e) => setMsg(e.message)) }, [])

  async function save() {
    setMsg('')
    const changed: Record<string, string> = {}
    for (const k of Object.keys(draft)) if (draft[k] !== settings[k]) changed[k] = draft[k]
    if (!Object.keys(changed).length) { setMsg('No changes.'); return }
    try { const s = await api.saveSettings(changed); setSettings(s); setDraft(s); setMsg('Saved.') }
    catch (e: any) { setMsg('⚠ ' + e.message) }
  }

  return (
    <div className="space-y-5">
      <div><h1 className="text-xl font-extrabold text-t0 sm:text-[26px]">Settings</h1>
        <p className="mt-1 text-[13px] text-t1">Resolution thresholds & ingestion. Every change is audited.</p></div>
      {msg && <div className="rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-3.5 py-2.5 text-[13px] text-t0">{msg}</div>}
      <Card className="max-w-2xl">
        <CardHeader><CardTitle>Configuration</CardTitle></CardHeader>
        <div className="space-y-4">
          {Object.keys(settings).sort().map((k) => (
            <label key={k} className="block text-[12.5px] font-semibold text-t1">
              {LABELS[k] || k}
              <input className={field + ' mt-1.5'} value={draft[k] ?? ''} onChange={(e) => setDraft({ ...draft, [k]: e.target.value })} />
            </label>
          ))}
          <Button onClick={save}>Save changes</Button>
        </div>
      </Card>
    </div>
  )
}
