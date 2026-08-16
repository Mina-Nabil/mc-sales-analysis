import { NavLink, useLocation } from 'react-router-dom'
import { cn } from '@/lib/cn'

const NAV: [string, string, string, boolean?][] = [
  ['/', 'Overview', 'M3 3h7v7H3zM14 3h7v7h-7zM14 14h7v7h-7zM3 14h7v7H3z', true],
  ['/analytics', 'Analytics', 'M3 3v18h18M7 14l3-3 3 3 5-6'],
  ['/dashboard', 'Dashboard', 'M3 3h7v9H3zM14 3h7v5h-7zM14 12h7v9h-7zM3 16h7v5H3z'],
  ['/review', 'Review queue', 'M22 12h-6l-2 3h-4l-2-3H2M5 5h14l3 7v6a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1v-6z'],
  ['/tree', 'Car tree', 'M12 2 2 7l10 5 10-5zM2 17l10 5 10-5M2 12l10 5 10-5'],
  ['/import', 'Import', 'M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M17 8l-5-5-5 5M12 3v12'],
  ['/settings', 'Settings', 'M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z'],
  ['/audit', 'Audit', 'M3 5h13M3 12h9M3 19h9m5-3 2 2 4-4'],
]

const linkBase =
  'flex items-center gap-3 rounded-[11px] px-3 py-2.5 text-[13px] font-semibold text-t1 transition-colors hover:bg-bg-3 hover:text-t0'
const linkActive = 'bg-acc-soft text-acc!'

export function SidebarContent({ onNavigate }: { onNavigate?: () => void }) {
  // Carry the active filter query across pages so filters stay global (§5.3).
  const { search } = useLocation()
  const carried = new URLSearchParams(search)
  for (const k of [...carried.keys()]) if (!k.startsWith('filter.')) carried.delete(k)
  const filterSearch = carried.toString()
  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-2.5 px-5 py-[18px]" style={{ minHeight: 74 }}>
        <div className="flex h-[34px] w-[34px] shrink-0 items-center justify-center rounded-[10px] bg-gradient-to-br from-acc to-acc-2 shadow-[0_6px_16px_-4px_var(--acc-soft)]">
          <svg width="19" height="19" viewBox="0 0 24 24" fill="none" stroke="#fff" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round">
            <path d="M14 16H9m10 0h3v-3.15a1 1 0 0 0-.84-.99L16 11l-2.7-3.6a1 1 0 0 0-.8-.4H5.24a2 2 0 0 0-1.8 1.1l-.8 1.63A6 6 0 0 0 2 12.42V16h2" />
            <circle cx="6.5" cy="16.5" r="2.5" /><circle cx="16.5" cy="16.5" r="2.5" />
          </svg>
        </div>
        <span className="text-[16px] font-extrabold text-t0">
          Motorcity<span className="text-acc">.</span>
        </span>
      </div>

      <nav className="flex-1 overflow-y-auto overflow-x-hidden px-3.5 pb-3.5">
        <p className="px-2.5 pb-1.5 pt-1 text-[10.5px] font-bold uppercase tracking-wider text-t2">Menu</p>
        {NAV.map(([to, label, d, end]) => (
          <NavLink key={to} to={{ pathname: to, search: filterSearch }} end={!!end} onClick={onNavigate}
            className={({ isActive }) => cn(linkBase, isActive && linkActive)}>
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round" className="shrink-0">
              <path d={d} />
            </svg>
            <span className="flex-1">{label}</span>
          </NavLink>
        ))}
      </nav>

      <div className="m-3.5 mt-0 rounded-[var(--radius-vela-lg)] border border-line bg-bg-3 px-4 py-3 text-[11px] text-t2">
        Egypt car registration analytics · cars only
      </div>
    </div>
  )
}

export function Sidebar() {
  return (
    <aside className="hidden w-[248px] shrink-0 border-r border-line bg-bg-1 lg:flex lg:flex-col">
      <SidebarContent />
    </aside>
  )
}
