import { useNavigate } from 'react-router-dom'
import { useTheme } from '@/theme/ThemeProvider'
import { useAuth } from '@/auth'
import { Avatar, Dropdown } from '@/components/ui'

export function Topbar({ onOpenNav }: { onOpenNav: () => void }) {
  const { theme, toggleTheme } = useTheme()
  const { user, logout } = useAuth()
  const nav = useNavigate()

  return (
    <header className="sticky top-0 z-30 flex flex-none items-center gap-2.5 border-b border-line bg-bg-1/80 px-3.5 py-3 backdrop-blur-md sm:px-6">
      <button onClick={onOpenNav} aria-label="Open menu"
        className="flex h-9 w-9 shrink-0 items-center justify-center rounded-[10px] border border-line text-t1 hover:bg-bg-3 lg:hidden">
        <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M3 6h18M3 12h18M3 18h18" /></svg>
      </button>

      <div className="ml-auto flex shrink-0 items-center gap-1.5 sm:gap-2.5">
        <button onClick={toggleTheme} aria-label="Toggle theme"
          className="flex h-9 w-9 items-center justify-center rounded-[10px] border border-line text-t1 hover:bg-bg-3">
          {theme === 'dark' ? (
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z" /></svg>
          ) : (
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" /></svg>
          )}
        </button>

        <Dropdown
          align="right"
          trigger={
            <button className="flex items-center gap-2 rounded-[10px] pl-0.5 pr-1 hover:bg-bg-3">
              <Avatar name={user?.email || 'User'} size="sm" />
              <span className="hidden text-left leading-tight md:block">
                <span className="block text-[12.5px] font-bold text-t0">{user?.email}</span>
                <span className="block text-[10.5px] text-t2">Signed in</span>
              </span>
            </button>
          }
          items={[
            { label: 'Sign out', danger: true, onClick: async () => { await logout(); nav('/login') } },
          ]}
        />
      </div>
    </header>
  )
}
