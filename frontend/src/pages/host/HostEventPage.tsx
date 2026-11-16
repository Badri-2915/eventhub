import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState, type ReactNode } from 'react'
import { Link, useParams } from 'react-router-dom'
import { eventsApi, hostApi } from '../../api/endpoints'
import { EventCover } from '../../components/events/EventCover'
import { SeatLegend, SeatMap } from '../../components/seating/SeatMap'
import { Badge, EmptyState, ErrorNote, PageLoader } from '../../components/ui/Feedback'
import { Icon } from '../../components/ui/Icon'
import { seatsKey, useEventStream } from '../../hooks/useEventStream'
import { formatTime, formatWhen, plural } from '../../lib/format'
import { formatPaise } from '../../lib/money'
import type { EventStats } from '../../lib/types'

/** Live view of one event for its host: sales, seat map, attendees, cancel. */
export function HostEventPage() {
  const { id = '' } = useParams()
  const qc = useQueryClient()
  const live = useEventStream(id)
  const [confirmCancel, setConfirmCancel] = useState(false)

  const stats = useQuery({ queryKey: ['host', 'stats', id], queryFn: () => hostApi.stats(id), refetchInterval: 10_000 })
  const ev = stats.data?.event
  const numbered = ev?.seating_mode === 'NUMBERED' && ev.status !== 'DRAFT'
  const seats = useQuery({ queryKey: seatsKey(id), queryFn: () => eventsApi.seats(id), enabled: numbered, refetchInterval: live ? false : 20_000 })
  const grid = useQuery({ queryKey: ['event', id], queryFn: () => eventsApi.get(id), enabled: numbered })
  const attendees = useQuery({ queryKey: ['host', 'attendees', id], queryFn: () => hostApi.attendees(id), refetchInterval: 15_000 })

  const cancel = useMutation({
    mutationFn: () => hostApi.cancel(id),
    onSuccess: () => {
      setConfirmCancel(false)
      qc.invalidateQueries({ queryKey: ['host'] })
      qc.invalidateQueries({ queryKey: ['events'] })
    },
  })

  if (stats.isLoading) return <PageLoader />
  if (stats.error || !stats.data || !ev) return <ErrorNote>{stats.error?.message ?? 'Event not found'}</ErrorNote>
  const s = stats.data
  const pct = s.total ? Math.round((s.sold / s.total) * 100) : 0

  return (
    <div className="space-y-8">
      <Link to="/host" className="inline-flex items-center gap-1.5 text-sm text-ink-muted hover:text-ink">
        <Icon name="arrowLeft" /> Dashboard
      </Link>

      <header className="flex flex-wrap items-start gap-6">
        <EventCover eventId={ev.id} showLabel={false} category={ev.category} posterUrl={ev.poster_url} showDate={false} className="hidden h-24 w-36 shrink-0 rounded-2xl sm:block" />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            {ev.status === 'PUBLISHED' && <Badge tone="pine">Live</Badge>}
            {ev.status === 'CANCELLED' && <Badge tone="clay">Cancelled</Badge>}
            {ev.status === 'DRAFT' && <Badge>Draft</Badge>}
            {live && ev.status === 'PUBLISHED' && <span className="text-xs text-ink-faint">· updating live</span>}
          </div>
          <h1 className="mt-2 font-display text-3xl sm:text-4xl">{ev.title}</h1>
          <p className="mt-1 text-sm text-ink-muted">
            {formatWhen(ev.starts_at)} · {ev.venue_name}, {ev.city}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {ev.status !== 'DRAFT' && (
            <Link to={`/events/${id}`} className="btn-secondary">
              <Icon name="eye" /> Public page
            </Link>
          )}
          {ev.status === 'PUBLISHED' && (
            <Link to="/host/check-in" className="btn-primary">
              <Icon name="check" /> Check in guests
            </Link>
          )}
          <Link to={`/host/events/${id}/edit`} className="btn-secondary">
            <Icon name="edit" /> {ev.status === 'DRAFT' ? 'Edit draft' : ev.status === 'PUBLISHED' ? 'Edit event' : 'Details'}
          </Link>
        </div>
      </header>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Tile label="Tickets sold" value={`${s.sold} / ${s.total}`}>
          <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-mist">
            <div className="h-full rounded-full bg-pine-500 transition-all" style={{ width: `${pct}%` }} />
          </div>
        </Tile>
        <Tile label="Revenue" value={formatPaise(s.revenue_paise)} />
        <Tile label="On hold right now" value={String(s.held)} hint="Released automatically after 5 min" />
        <Tile
          label="Bookings"
          value={String(s.bookings)}
          extra={attendees.data ? `${attendees.data.filter((a) => a.checked_in_at).length} checked in at the door` : undefined} hint={ev.status === 'CANCELLED' ? 'Sales stopped · all refunded' : `${s.available} tickets still available`} />
      </div>

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <section className="card p-6">
          <h2 className="font-display text-xl">By ticket type</h2>
          <ul className="mt-5 space-y-4">
            {s.categories.map((c) => (
              <li key={c.id}>
                <div className="flex items-center gap-2 text-sm">
                  <span className="h-2.5 w-2.5 rounded-full" style={{ background: c.color }} />
                  <span className="flex-1 font-medium">{c.name}</span>
                  <span className="tabular-nums text-ink-muted">
                    {c.sold}/{c.total} · {formatPaise(c.revenue_paise)}
                  </span>
                </div>
                <div className="mt-2 flex h-2 overflow-hidden rounded-full bg-mist">
                  <div style={{ width: `${(c.sold / Math.max(c.total, 1)) * 100}%`, background: c.color }} />
                  <div className="bg-sand-300" style={{ width: `${(c.held / Math.max(c.total, 1)) * 100}%` }} />
                </div>
              </li>
            ))}
          </ul>
          <p className="mt-4 flex gap-4 text-xs text-ink-faint">
            <span className="text-pine-600">■ sold</span>
            <span className="text-sand-700">■ on hold</span>
          </p>
        </section>

        <SalesChart days={s.sales_by_day} />
      </div>

      {numbered && grid.data?.grid && (
        <section className="card min-w-0 p-6">
          <h2 className="mb-6 font-display text-xl">Seat map</h2>
          <SeatMap grid={grid.data.grid} seats={seats.data ?? []} categories={grid.data.categories} stageLabel={ev.category === 'MOVIE' ? 'Screen' : 'Stage'} readOnly />
          <div className="mt-6 border-t border-line pt-5">
            <SeatLegend categories={grid.data.categories} />
          </div>
        </section>
      )}

      <section className="card overflow-hidden">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-6 py-4">
          <h2 className="font-display text-xl">Attendees</h2>
          {(attendees.data?.length ?? 0) > 0 && (
            <a href={hostApi.attendeesCsvUrl(id)} className="btn-secondary py-2" download>
              Download CSV
            </a>
          )}
        </div>
        {attendees.data?.length === 0 ? (
          <div className="p-6">
            <EmptyState icon="users" title="No bookings yet">
              Confirmed bookings will appear here as they come in.
            </EmptyState>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="bg-mist/50 text-xs uppercase tracking-wider text-ink-faint">
                <tr>
                  <th className="px-6 py-3 font-medium">Name</th>
                  <th className="px-6 py-3 font-medium">Tickets</th>
                  <th className="px-6 py-3 font-medium">Code</th>
                  <th className="px-6 py-3 font-medium">Entry</th>
                  <th className="px-6 py-3 text-right font-medium">Paid</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line">
                {attendees.data?.map((a) => (
                  <tr key={a.booking_id}>
                    <td className="px-6 py-3">
                      <div className="font-medium">{a.name}</div>
                      <div className="text-ink-faint">{a.email}</div>
                    </td>
                    <td className="px-6 py-3 text-ink-muted">
                      {plural(a.tickets, 'ticket')}
                      <div className="text-xs text-ink-faint">{a.items}</div>
                    </td>
                    <td className="px-6 py-3 font-mono text-xs">{a.ticket_code}</td>
                    <td className="px-6 py-3">
                      {a.checked_in_at ? <Badge tone="pine">In · {formatTime(a.checked_in_at)}</Badge> : <span className="text-ink-faint">Not yet</span>}
                    </td>
                    <td className="px-6 py-3 text-right tabular-nums">{formatPaise(a.total_paise)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {ev.status === 'PUBLISHED' && (
        <section className="rounded-2xl border border-clay-500/25 bg-clay-50/50 p-6">
          <h2 className="font-medium text-clay-700">Cancel this event</h2>
          <p className="mt-1 text-sm text-ink-muted">
            Every booking is refunded in full and all seats are released. Ticket sales stop immediately. This cannot be undone.
          </p>
          {cancel.error && (
            <div className="mt-3">
              <ErrorNote>{cancel.error.message}</ErrorNote>
            </div>
          )}
          <div className="mt-4 flex gap-2">
            {confirmCancel ? (
              <>
                <button className="btn bg-clay-500 text-white hover:bg-clay-700" disabled={cancel.isPending} onClick={() => cancel.mutate()}>
                  {cancel.isPending ? 'Cancelling…' : `Yes, cancel and refund ${plural(s.bookings, 'booking')}`}
                </button>
                <button className="btn-quiet" onClick={() => setConfirmCancel(false)}>
                  Keep event
                </button>
              </>
            ) : (
              <button className="btn-secondary" onClick={() => setConfirmCancel(true)}>
                Cancel event
              </button>
            )}
          </div>
        </section>
      )}
      {cancel.data && (
        <p className="rounded-xl bg-pine-50 px-4 py-3 text-sm text-pine-800">
          Event cancelled. {plural(cancel.data.refunded_bookings, 'booking')} refunded and every seat released.
        </p>
      )}
    </div>
  )
}

function Tile({ label, value, hint, extra, children }: { label: string; value: string; hint?: string; extra?: string; children?: ReactNode }) {
  return (
    <div className="card px-5 py-4">
      <div className="text-sm text-ink-muted">{label}</div>
      <div className="mt-1 font-display text-3xl tabular-nums">{value}</div>
      {hint && <div className="mt-1 text-xs text-ink-faint">{hint}</div>}
      {extra && <div className="mt-0.5 text-xs font-medium text-pine-700">{extra}</div>}
      {children}
    </div>
  )
}

function SalesChart({ days }: { days: EventStats['sales_by_day'] }) {
  const max = Math.max(1, ...days.map((d) => d.tickets))
  return (
    <section className="card p-6">
      <h2 className="font-display text-xl">Sales over time</h2>
      {days.length === 0 ? (
        <p className="mt-5 text-sm text-ink-muted">No sales yet.</p>
      ) : (
        <div className="mt-6 flex h-40 items-end gap-2">
          {days.slice(-14).map((d) => (
            <div key={d.day} className="group flex h-full flex-1 flex-col items-center gap-2" title={`${d.tickets} tickets · ${formatPaise(d.revenue_paise)}`}>
              <span className="text-[11px] tabular-nums text-ink-muted">{d.tickets}</span>
              {/* the bar's % height needs a parent with a definite height */}
              <div className="flex w-full flex-1 items-end justify-center">
                <div className="w-full max-w-10 rounded-t-md bg-pine-500/80 transition group-hover:bg-pine-700" style={{ height: `${(d.tickets / max) * 100}%`, minHeight: 4 }} />
              </div>
              <span className="text-[11px] text-ink-faint">
                {new Date(d.day).toLocaleDateString('en-IN', { day: 'numeric', month: 'short' })}
              </span>
            </div>
          ))}
        </div>
      )}
    </section>
  )
}
