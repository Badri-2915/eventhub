import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { authApi } from '../../api/endpoints'
import { ME_KEY, useMe } from '../../hooks/useMe'
import { PageLoader } from '../ui/Feedback'
import { Icon, type IconName } from '../ui/Icon'

const perks: [IconName, string, string][] = [
  ['grid', 'Design your hall', 'Paint rows, aisles and price tiers on a simple grid.'],
  ['users', 'Sell fairly', 'First come, first served. No seat is ever sold twice.'],
  ['eye', 'Watch it live', 'See seats fill up in real time as people book.'],
]

/** Gate for /host/*: signed out → login, attendee → "Become a host". */
export function RequireHost() {
  const { data: me, isLoading } = useMe()
  const location = useLocation()
  const qc = useQueryClient()
  const become = useMutation({
    mutationFn: authApi.becomeHost,
    onSuccess: (user) => qc.setQueryData(ME_KEY, user),
  })

  if (isLoading) return <PageLoader />
  if (!me) return <Navigate to="/login" state={{ from: location.pathname }} replace />
  if (!me.is_host) {
    return (
      <div className="mx-auto max-w-2xl py-6 text-center">
        <p className="eyebrow">For organisers</p>
        <h1 className="mt-3 font-display text-4xl">Host your own events</h1>
        <p className="mx-auto mt-3 max-w-md text-ink-muted">
          Create an event in minutes, design the seating your way and sell tickets with live availability.
        </p>
        <div className="mt-10 grid gap-4 text-left sm:grid-cols-3">
          {perks.map(([icon, title, body]) => (
            <div key={title} className="card p-5">
              <span className="grid h-9 w-9 place-items-center rounded-lg bg-pine-50 text-pine-700">
                <Icon name={icon} />
              </span>
              <h3 className="mt-3 font-medium">{title}</h3>
              <p className="mt-1 text-sm text-ink-muted">{body}</p>
            </div>
          ))}
        </div>
        <button className="btn-primary mt-10 px-6" onClick={() => become.mutate()} disabled={become.isPending}>
          Become a host
        </button>
      </div>
    )
  }
  return <Outlet />
}
