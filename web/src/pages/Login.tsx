import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '@/auth'
import { Button } from '@/components/ui'

export default function Login() {
  const { login } = useAuth()
  const nav = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setErr(''); setBusy(true)
    try { await login(email, password); nav('/') }
    catch (e: any) { setErr(e.message || 'Login failed') }
    finally { setBusy(false) }
  }

  return (
    <div className="grid min-h-screen place-items-center bg-bg-0 p-4"
      style={{ backgroundImage: 'radial-gradient(60% 50% at 50% 0%, var(--acc-soft), transparent 70%)' }}>
      <form onSubmit={submit}
        className="w-full max-w-[380px] rounded-[var(--radius-vela-xl)] border border-line bg-bg-2 p-7 shadow-[var(--shadow-vela)]">
        <div className="mb-5 flex items-center gap-2.5">
          <div className="flex h-[38px] w-[38px] items-center justify-center rounded-[11px] bg-gradient-to-br from-acc to-acc-2">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#fff" strokeWidth="2.3" strokeLinecap="round" strokeLinejoin="round">
              <path d="M14 16H9m10 0h3v-3.15a1 1 0 0 0-.84-.99L16 11l-2.7-3.6a1 1 0 0 0-.8-.4H5.24a2 2 0 0 0-1.8 1.1l-.8 1.63A6 6 0 0 0 2 12.42V16h2" /><circle cx="6.5" cy="16.5" r="2.5" /><circle cx="16.5" cy="16.5" r="2.5" />
            </svg>
          </div>
          <span className="text-[18px] font-extrabold">Motorcity<span className="text-acc">.</span></span>
        </div>
        <h1 className="text-xl font-extrabold text-t0">Sign in</h1>
        <p className="mt-0.5 mb-5 text-[13px] text-t1">Egypt car sales analytics</p>

        <label className="mb-3 block text-[12.5px] font-semibold text-t1">
          Email
          <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoFocus
            className="mt-1.5 h-10 w-full rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-3 text-[13px] text-t0 focus:border-acc" />
        </label>
        <label className="mb-4 block text-[12.5px] font-semibold text-t1">
          Password
          <input type="password" value={password} onChange={(e) => setPassword(e.target.value)}
            className="mt-1.5 h-10 w-full rounded-[var(--radius-vela-md)] border border-line bg-bg-inset px-3 text-[13px] text-t0 focus:border-acc" />
        </label>
        {err && <div className="mb-3 rounded-[var(--radius-vela-sm)] bg-bad-soft px-3 py-2 text-[12.5px] font-semibold text-bad">{err}</div>}
        <Button type="submit" fullWidth size="lg" disabled={busy}>{busy ? 'Signing in…' : 'Sign in'}</Button>
      </form>
    </div>
  )
}
