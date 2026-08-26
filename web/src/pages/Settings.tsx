import { useEffect, useState } from 'react'
import { api } from '@/lib/api'
import { useAuth } from '@/auth'
import { Card, CardHeader, CardTitle, CardSubtitle, Button, Badge, Modal, Input, FormField } from '@/components/ui'

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
      <div>
        <h1 className="text-xl font-extrabold text-t0 sm:text-[26px]">Settings</h1>
        <p className="mt-1 text-[13px] text-t1">Resolution thresholds &amp; ingestion. Every change is audited.</p>
      </div>
      {msg && <div className="rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-3.5 py-2.5 text-[13px] text-t0">{msg}</div>}

      <div className="grid grid-cols-1 items-start gap-5 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <div>
              <CardTitle>Configuration</CardTitle>
              <CardSubtitle>Resolution thresholds &amp; ingestion.</CardSubtitle>
            </div>
          </CardHeader>
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

        <Users />
      </div>
    </div>
  )
}

// ── User management ──────────────────────────────────────────────────────────
// Every user is an admin; there are no roles. The seed admin cannot be
// deactivated or deleted, and you cannot lock yourself out of your own account.
function Users() {
  const { user: me } = useAuth()
  const [users, setUsers] = useState<any[] | null>(null)
  const [msg, setMsg] = useState('')
  const [editing, setEditing] = useState<any>(null)   // user row, or {} for "new"
  const [confirmDel, setConfirmDel] = useState<any>(null)
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)

  const load = () => api.users().then(setUsers).catch((e) => { setUsers([]); setMsg('⚠ ' + e.message) })
  useEffect(() => { load() }, [])

  const openNew = () => { setEmail(''); setPassword(''); setEditing({}) }
  const openEdit = (u: any) => { setEmail(u.email); setPassword(''); setEditing(u) }

  const act = async (fn: () => Promise<any>, ok: string) => {
    setBusy(true); setMsg('')
    try { await fn(); setMsg(ok); await load() }
    catch (e: any) { setMsg('⚠ ' + e.message) }
    finally { setBusy(false) }
  }

  const submit = async () => {
    const isNew = !editing?.id
    if (isNew) {
      await act(() => api.createUser(email.trim(), password), 'User created.')
    } else {
      const patch: any = {}
      if (email.trim() && email.trim() !== editing.email) patch.email = email.trim()
      if (password) patch.password = password
      if (!Object.keys(patch).length) { setEditing(null); setMsg('No changes.'); return }
      await act(() => api.updateUser(editing.id, patch), 'User updated.')
    }
    setEditing(null)
  }

  const canSubmit = editing?.id ? (email.trim() !== '' || password !== '') : (email.trim() !== '' && password.length >= 8)

  return (
    <Card padding="none" className="overflow-hidden">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-5 py-4">
        <div><CardTitle>Users</CardTitle>
          <CardSubtitle>Everyone is an admin. Deactivated users are blocked from every request.</CardSubtitle></div>
        <Button size="sm" onClick={openNew}>＋ Add user</Button>
      </div>

      {msg && <div className="border-b border-line bg-bg-inset px-5 py-2.5 text-[13px] text-t0">{msg}</div>}

      <div className="overflow-x-auto">
        <table className="w-full min-w-[520px] border-collapse text-sm">
          <thead><tr className="border-b border-line">
            {['Email', 'Status', 'Added', ''].map((h, i) => (
              <th key={i} className={'px-4 py-3 text-[10.5px] font-bold uppercase tracking-wide text-t2 ' + (i === 3 ? 'text-right' : 'text-left')}>{h}</th>
            ))}
          </tr></thead>
          <tbody>
            {users === null && <tr><td colSpan={4} className="px-4 py-8 text-center text-[13px] text-t2">Loading…</td></tr>}
            {users?.map((u) => {
              const isMe = me?.id === u.id
              const locked = u.is_seed || isMe // cannot deactivate/delete
              return (
                <tr key={u.id} className="border-b border-line last:border-0 hover:bg-bg-3">
                  <td className="px-4 py-2.5 font-semibold text-t0">
                    {u.email}
                    {u.is_seed && <span className="ml-2 text-[11px] font-normal text-t2">seed admin</span>}
                    {isMe && <span className="ml-2 text-[11px] font-normal text-acc">you</span>}
                  </td>
                  <td className="px-4 py-2.5">
                    <Badge variant={u.is_active ? 'success' : 'danger'}>{u.is_active ? 'Active' : 'Deactivated'}</Badge>
                  </td>
                  <td className="px-4 py-2.5 text-[12.5px] text-t2">{u.created_at ? new Date(u.created_at).toLocaleDateString() : '—'}</td>
                  <td className="px-4 py-2.5">
                    <div className="flex justify-end gap-1.5">
                      <Button size="sm" variant="secondary" disabled={busy} onClick={() => openEdit(u)}>Edit</Button>
                      <Button size="sm" variant="outline" disabled={busy || locked}
                        title={locked ? (u.is_seed ? 'The seed admin cannot be deactivated' : 'You cannot deactivate your own account') : ''}
                        onClick={() => act(() => api.setUserActive(u.id, !u.is_active), u.is_active ? 'User deactivated.' : 'User activated.')}>
                        {u.is_active ? 'Deactivate' : 'Activate'}
                      </Button>
                      <Button size="sm" variant="danger" disabled={busy || locked}
                        title={locked ? (u.is_seed ? 'The seed admin cannot be deleted' : 'You cannot delete your own account') : ''}
                        onClick={() => setConfirmDel(u)}>Delete</Button>
                    </div>
                  </td>
                </tr>
              )
            })}
            {users?.length === 0 && <tr><td colSpan={4} className="px-4 py-8 text-center text-[13px] text-t2">No users.</td></tr>}
          </tbody>
        </table>
      </div>

      <Modal open={!!editing} onClose={() => setEditing(null)} size="sm"
        title={editing?.id ? `Edit ${editing.email}` : 'Add user'}
        footer={<><Button variant="ghost" onClick={() => setEditing(null)}>Cancel</Button>
          <Button disabled={!canSubmit || busy} onClick={submit}>{editing?.id ? 'Save' : 'Create user'}</Button></>}>
        <div className="space-y-4">
          <FormField label="Email">
            <Input value={email} onChange={(e) => setEmail(e.target.value)} placeholder="name@motorcity.local" autoFocus />
          </FormField>
          <FormField label={editing?.id ? 'New password' : 'Password'}
            hint={editing?.id ? 'Leave blank to keep the current password. Changing it signs them out everywhere.' : 'At least 8 characters.'}>
            <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder={editing?.id ? '••••••••' : ''} />
          </FormField>
        </div>
      </Modal>

      <Modal open={!!confirmDel} onClose={() => setConfirmDel(null)} size="sm" title="Delete user"
        footer={<><Button variant="ghost" onClick={() => setConfirmDel(null)}>Cancel</Button>
          <Button variant="danger" disabled={busy}
            onClick={() => { const u = confirmDel; setConfirmDel(null); act(() => api.deleteUser(u.id), 'User deleted.') }}>Delete</Button></>}>
        <p className="text-[13px] text-t1">
          Delete <span className="font-bold text-t0">{confirmDel?.email}</span>? This cannot be undone.
        </p>
        <p className="mt-2 text-[12.5px] text-t2">
          Their history (imports, review decisions, audit entries) is kept — the actor reference is set to nobody.
        </p>
      </Modal>
    </Card>
  )
}
