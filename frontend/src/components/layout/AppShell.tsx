import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, NavLink, Outlet, useNavigate } from 'react-router-dom'
import { authApi } from '../../api/endpoints'
import { ME_KEY, useMe } from '../../hooks/useMe'
import { Icon } from '../ui/Icon'

export function Logo() {
  return (
    <Link to="/" className="flex items-center gap-2.5">
      <img src="/favicon.svg" alt="" className="h-7 w-7" />
      <span className="font-display text-xl font-medium tracking-tight">EventHub</span>
    </Link>
  )
}

export function AppShell() {
  const { data: me } = useMe()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const logout = useMutation({
    mutationFn: authApi.logout,
    onSuccess: () => {
      qc.clear()
      qc.setQueryData(ME_KEY, null)
      navigate('/')
    },
  })

  const link = ({ isActive }: { isActive: boolean }) =>
    `rounded-lg px-3 py-1.5 text-sm transition ${isActive ? 'bg-white text-ink shadow-soft' : 'text-ink-muted hover:text-ink'}`

  return (
    <div className="flex min-h-screen flex-col">
      <header className="sticky top-0 z-20 border-b border-line/80 bg-paper/85 backdrop-blur-md">
        <div className="mx-auto flex h-16 max-w-6xl items-center gap-6 px-5">
          <Logo />
          <nav className="hidden items-center gap-1 sm:flex">
            <NavLink to="/" end className={link}>
              Discover
            </NavLink>
            {me && (
              <NavLink to="/bookings" className={link}>
                My tickets
              </NavLink>
            )}
            <NavLink to="/host" className={link}>
              {me?.is_host ? 'Host dashboard' : 'Host an event'}
            </NavLink>
            {me?.is_admin && (
              <NavLink to="/admin" className={link}>
                Admin
              </NavLink>
            )}
          </nav>
          <div className="ml-auto flex items-center gap-2">
            {me ? (
              <>
                <div className="flex items-center gap-2.5 rounded-full py-1 pl-1 pr-3">
                  {me.avatar_url ? (
                    <img src={me.avatar_url} alt="" className="h-8 w-8 rounded-full" referrerPolicy="no-referrer" />
                  ) : (
                    <span className="grid h-8 w-8 place-items-center rounded-full bg-pine-100 text-sm font-semibold text-pine-800">
                      {me.name.slice(0, 1).toUpperCase()}
                    </span>
                  )}
                  <span className="hidden text-sm font-medium md:inline">{me.name}</span>
                </div>
                <button className="btn-quiet" onClick={() => logout.mutate()} title="Sign out">
                  <Icon name="logout" />
                  <span className="sr-only">Sign out</span>
                </button>
              </>
            ) : (
              <Link to="/login" className="btn-primary py-2">
                Sign in
              </Link>
            )}
          </div>
        </div>
        {/* compact nav for phones */}
        <nav className="flex gap-1 border-t border-line/70 px-4 py-2 sm:hidden">
          <NavLink to="/" end className={link}>
            Discover
          </NavLink>
          {me && (
            <NavLink to="/bookings" className={link}>
              Tickets
            </NavLink>
          )}
          <NavLink to="/host" className={link}>
            Host
          </NavLink>
          {me?.is_admin && (
            <NavLink to="/admin" className={link}>
              Admin
            </NavLink>
          )}
        </nav>
      </header>

      <main className="mx-auto w-full max-w-6xl flex-1 px-5 py-10">
        <Outlet />
      </main>

      <footer className="border-t border-line">
        <div className="mx-auto flex max-w-6xl flex-col gap-2 px-5 py-8 text-sm text-ink-faint sm:flex-row sm:items-center sm:justify-between">
          <span>© {new Date().getFullYear()} EventHub</span>
          <span>Fair, first-come seating. Every seat is sold exactly once.</span>
        </div>
      </footer>
    </div>
  )
}
