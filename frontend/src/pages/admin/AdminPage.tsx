import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { Link, Navigate } from 'react-router-dom'
import { adminApi } from '../../api/endpoints'
import { Badge, ErrorNote, PageLoader } from '../../components/ui/Feedback'
import { useMe } from '../../hooks/useMe'
import { categoryLabel, formatWhen } from '../../lib/format'
import { formatPaise } from '../../lib/money'
import type { AdminEvent } from '../../lib/types'

/** Platform admin: totals, every event, and cancel-with-refund for moderation. */
export function AdminPage() {
  const { data: me, isLoading } = useMe()
  const overview = useQuery({ queryKey: ['admin', 'overview'], queryFn: adminApi.overview, enabled: !!me?.is_admin })
  const events = useQuery({ queryKey: ['admin', 'events'], queryFn: adminApi.events, enabled: !!me?.is_admin })

  if (isLoading) return <PageLoader />
  if (!me?.is_admin) return <Navigate to="/" replace />
  const o = overview.data

  return (
    <div className="space-y-8">
      <div>
        <p className="eyebrow">Admin</p>
        <h1 className="mt-2 font-display text-4xl">Platform overview</h1>
      </div>

      <div className="grid gap-4 sm:grid-cols-3 lg:grid-cols-6">
        {[
          ['Users', o?.users],
          ['Hosts', o?.hosts],
          ['Events', o?.events],
          ['Live now', o?.live_events],
          ['Bookings', o?.bookings],
          ['Revenue', o ? formatPaise(o.revenue_paise) : undefined],
        ].map(([label, value]) => (
          <div key={label} className="card px-5 py-4">
            <div className="text-sm text-ink-muted">{label}</div>
            <div className="mt-1 font-display text-2xl tabular-nums">{value ?? '—'}</div>
          </div>
        ))}
      </div>

      {events.error && <ErrorNote>{events.error.message}</ErrorNote>}
      <section className="card overflow-hidden">
        <h2 className="border-b border-line px-6 py-4 font-display text-xl">All events</h2>
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="bg-mist/50 text-xs uppercase tracking-wider text-ink-faint">
              <tr>
                <th className="px-6 py-3 font-medium">Event</th>
                <th className="px-6 py-3 font-medium">Host</th>
                <th className="px-6 py-3 font-medium">Sold</th>
                <th className="px-6 py-3 text-right font-medium">Revenue</th>
                <th className="px-6 py-3" />
              </tr>
            </thead>
            <tbody className="divide-y divide-line">
              {events.data?.map((ev) => <Row key={ev.id} ev={ev} />)}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  )
}

function Row({ ev }: { ev: AdminEvent }) {
  const qc = useQueryClient()
  const [confirming, setConfirming] = useState(false)
  const cancel = useMutation({
    mutationFn: () => adminApi.cancel(ev.id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['admin'] }),
  })
  return (
    <tr>
      <td className="px-6 py-3">
        <div className="flex items-center gap-2">
          <Link to={`/events/${ev.id}`} className="font-medium hover:text-pine-700">
            {ev.title}
          </Link>
          {ev.status === 'PUBLISHED' ? <Badge tone="pine">Live</Badge> : <Badge tone={ev.status === 'CANCELLED' ? 'clay' : 'neutral'}>{ev.status.toLowerCase()}</Badge>}
        </div>
        <div className="text-ink-faint">
          {categoryLabel[ev.category]} · {formatWhen(ev.starts_at)} · {ev.city}
        </div>
      </td>
      <td className="px-6 py-3">
        <div>{ev.host_name}</div>
        <div className="text-ink-faint">{ev.host_email}</div>
      </td>
      <td className="px-6 py-3 tabular-nums">
        {ev.sold} / {ev.capacity}
      </td>
      <td className="px-6 py-3 text-right tabular-nums">{formatPaise(ev.revenue_paise)}</td>
      <td className="whitespace-nowrap px-6 py-3 text-right">
        {ev.status === 'PUBLISHED' &&
          (confirming ? (
            <span className="inline-flex gap-1">
              <button className="btn bg-clay-500 py-1.5 text-white hover:bg-clay-700" disabled={cancel.isPending} onClick={() => cancel.mutate()}>
                Cancel & refund
              </button>
              <button className="btn-quiet py-1.5" onClick={() => setConfirming(false)}>
                Keep
              </button>
            </span>
          ) : (
            <button className="btn-quiet py-1.5" onClick={() => setConfirming(true)}>
              Cancel event
            </button>
          ))}
        {cancel.error && <div className="mt-1 text-xs text-clay-500">{cancel.error.message}</div>}
      </td>
    </tr>
  )
}
