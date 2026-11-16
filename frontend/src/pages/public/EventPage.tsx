import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../../api/client'
import { bookingsApi, eventsApi } from '../../api/endpoints'
import { EventCover } from '../../components/events/EventCover'
import { GaPicker } from '../../components/seating/GaPicker'
import { SeatLegend, SeatMap } from '../../components/seating/SeatMap'
import { AvailabilityBadge, ErrorNote, PageLoader } from '../../components/ui/Feedback'
import { Icon } from '../../components/ui/Icon'
import { eventKey, seatsKey, useEventStream } from '../../hooks/useEventStream'
import { useMe } from '../../hooks/useMe'
import { categoryLabel, directionsUrl, formatLongDate, formatTime, plural } from '../../lib/format'
import { formatPaise } from '../../lib/money'
import type { Seat } from '../../lib/types'

export function EventPage() {
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const { data: me } = useMe()
  const live = useEventStream(id)

  const event = useQuery({ queryKey: eventKey(id), queryFn: () => eventsApi.get(id) })
  const numbered = event.data?.seating_mode === 'NUMBERED'
  const seats = useQuery({
    queryKey: seatsKey(id),
    queryFn: () => eventsApi.seats(id),
    enabled: numbered,
    refetchInterval: live ? false : 20_000, // fallback when the live stream is down
  })

  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [qty, setQty] = useState<Record<string, number>>({})
  const [notice, setNotice] = useState<string | null>(null)

  // If someone else grabs a seat we had selected, drop it and say so.
  useEffect(() => {
    if (!seats.data || selected.size === 0) return
    const lost = seats.data.filter((s) => selected.has(s.id) && s.status !== 'AVAILABLE')
    if (lost.length) {
      setSelected((prev) => new Set([...prev].filter((sid) => !lost.some((l) => l.id === sid))))
      setNotice(`${lost.map((s) => s.label).join(', ')} was just taken by someone else.`)
    }
  }, [seats.data, selected])

  const ev = event.data
  const max = ev?.max_per_booking ?? 10
  const catById = useMemo(() => new Map(ev?.categories.map((c) => [c.id, c]) ?? []), [ev])
  const seatById = useMemo(() => new Map(seats.data?.map((s) => [s.id, s]) ?? []), [seats.data])

  const lines = useMemo(() => {
    if (!ev) return []
    if (numbered) {
      const groups = new Map<string, Seat[]>()
      selected.forEach((sid) => {
        const s = seatById.get(sid)
        if (s) groups.set(s.category_id, [...(groups.get(s.category_id) ?? []), s])
      })
      return [...groups].map(([cid, list]) => {
        const c = catById.get(cid)!
        list.sort((a, b) => a.grid_row - b.grid_row || a.grid_col - b.grid_col)
        return { key: cid, name: c?.name ?? '', color: c?.color, count: list.length, detail: list.map((s) => s.label).join(', '), amount: (c?.price_paise ?? 0) * list.length }
      })
    }
    return ev.categories
      .filter((c) => (qty[c.id] ?? 0) > 0)
      .map((c) => ({ key: c.id, name: c.name, color: c.color, count: qty[c.id], detail: `${formatPaise(c.price_paise)} each`, amount: c.price_paise * qty[c.id] }))
  }, [ev, numbered, selected, seatById, catById, qty])

  const count = lines.reduce((a, l) => a + l.count, 0)
  const total = lines.reduce((a, l) => a + l.amount, 0)

  const hold = useMutation({
    mutationFn: () =>
      bookingsApi.hold(
        numbered
          ? { event_id: id, seat_ids: [...selected] }
          : { event_id: id, items: Object.entries(qty).filter(([, n]) => n > 0).map(([category_id, n]) => ({ category_id, qty: n })) },
      ),
    onSuccess: (res) => navigate(`/checkout/${res.booking_id}`),
    onError: (err) => {
      if (err instanceof ApiError && err.code === 'SEATS_TAKEN') {
        const taken = (err.details?.seat_ids as string[] | undefined) ?? []
        setSelected((prev) => new Set([...prev].filter((sid) => !taken.includes(sid))))
      }
      qc.invalidateQueries({ queryKey: eventKey(id) })
    },
  })

  if (event.isLoading) return <PageLoader />
  if (event.error || !ev) return <ErrorNote>{event.error?.message ?? 'Event not found'}</ErrorNote>

  const totalAvail = ev.categories.reduce((a, c) => a + c.available, 0)
  const totalCap = ev.categories.reduce((a, c) => a + c.total, 0)

  const toggle = (s: Seat) => {
    setNotice(null)
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(s.id)) next.delete(s.id)
      else if (next.size < max) next.add(s.id)
      else setNotice(`You can choose up to ${max} seats in one booking.`)
      return next
    })
  }

  const proceed = () => {
    if (!me) {
      navigate('/login', { state: { from: `/events/${id}` } })
      return
    }
    hold.mutate()
  }

  return (
    <div className="space-y-10">
      <Link to="/" className="inline-flex items-center gap-1.5 text-sm text-ink-muted hover:text-ink">
        <Icon name="arrowLeft" /> All events
      </Link>

      <section className="grid gap-8 md:grid-cols-[1.1fr_1fr] md:items-center">
        <EventCover eventId={ev.id} category={ev.category} posterUrl={ev.poster_url} startsAt={ev.starts_at} className="aspect-[16/10] rounded-3xl" />
        <div>
          <p className="eyebrow">{categoryLabel[ev.category]}</p>
          <h1 className="mt-3 font-display text-4xl leading-tight">{ev.title}</h1>
          <dl className="mt-6 space-y-3 text-sm">
            <div className="flex gap-3">
              <Icon name="calendar" className="mt-0.5 h-4 w-4 text-pine-600" />
              <div>
                <dt className="font-medium">{formatLongDate(ev.starts_at)}</dt>
                <dd className="text-ink-muted">
                  {formatTime(ev.starts_at)}
                  {ev.ends_at && ` – ${formatTime(ev.ends_at)}`}
                </dd>
              </div>
            </div>
            <div className="flex gap-3">
              <Icon name="pin" className="mt-0.5 h-4 w-4 text-pine-600" />
              <div>
                <dt className="font-medium">{ev.venue_name}</dt>
                <dd className="text-ink-muted">{[ev.address, ev.city].filter(Boolean).join(', ')}</dd>
                <dd className="mt-1.5">
                  <a
                    href={directionsUrl(ev)}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-1 text-sm font-medium text-pine-700 hover:text-pine-800 hover:underline"
                  >
                    Get directions <Icon name="external" className="h-3.5 w-3.5" />
                  </a>
                </dd>
              </div>
            </div>
          </dl>
          {ev.description && <p className="mt-6 leading-relaxed text-ink-muted">{ev.description}</p>}
          <div className="mt-6 flex items-center gap-3">
            <AvailabilityBadge available={totalAvail} total={totalCap} />
            {live && (
              <span className="flex items-center gap-1.5 text-xs text-ink-faint">
                <span className="relative flex h-2 w-2">
                  <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-pine-300 opacity-75" />
                  <span className="relative inline-flex h-2 w-2 rounded-full bg-pine-500" />
                </span>
                Live availability
              </span>
            )}
          </div>
        </div>
      </section>

      {!ev.on_sale ? (
        <ErrorNote>
          {ev.status === 'CANCELLED'
            ? 'This event has been cancelled by the organiser. Anyone who booked has been refunded in full.'
            : 'Tickets for this event are no longer on sale.'}
        </ErrorNote>
      ) : (
        <section className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_340px] lg:items-start">
          <div className="card min-w-0 p-6 sm:p-8">
            <div className="mb-6 flex flex-wrap items-baseline justify-between gap-2">
              <h2 className="font-display text-2xl">{numbered ? 'Choose your seats' : 'Choose tickets'}</h2>
              <span className="text-sm text-ink-faint">Up to {max} per booking</span>
            </div>
            {numbered ? (
              seats.isLoading || !ev.grid ? (
                <PageLoader />
              ) : (
                <div className="space-y-6">
                  <SeatMap
                    grid={ev.grid}
                    seats={seats.data ?? []}
                    categories={ev.categories}
                    selected={selected}
                    onToggle={toggle}
                    stageLabel={ev.category === 'MOVIE' ? 'Screen' : 'Stage'}
                  />
                  <div className="border-t border-line pt-5">
                    <SeatLegend categories={ev.categories} />
                  </div>
                </div>
              )
            ) : (
              <GaPicker
                categories={ev.categories}
                qty={qty}
                maxTotal={max}
                onChange={(cid, n) => {
                  setNotice(null)
                  setQty((q) => ({ ...q, [cid]: n }))
                }}
              />
            )}
          </div>

          <aside className="card sticky top-24 p-6">
            <h3 className="font-display text-xl">Your selection</h3>
            {lines.length === 0 ? (
              <p className="mt-3 text-sm text-ink-muted">{numbered ? 'Tap seats on the map to add them.' : 'Pick how many tickets you need.'}</p>
            ) : (
              <ul className="mt-4 space-y-3">
                {lines.map((l) => (
                  <li key={l.key} className="flex items-start gap-3 text-sm">
                    <span className="mt-1.5 h-2 w-2 shrink-0 rounded-full" style={{ background: l.color }} />
                    <div className="min-w-0 flex-1">
                      <div className="font-medium">
                        {l.name} × {l.count}
                      </div>
                      <div className="truncate text-ink-faint">{l.detail}</div>
                    </div>
                    <span className="tabular-nums">{formatPaise(l.amount)}</span>
                  </li>
                ))}
              </ul>
            )}
            <div className="mt-5 flex items-baseline justify-between border-t border-line pt-4">
              <span className="text-sm text-ink-muted">{plural(count, 'ticket')}</span>
              <span className="font-display text-2xl tabular-nums">{formatPaise(total)}</span>
            </div>
            {(notice || hold.error) && (
              <div className="mt-4">
                <ErrorNote>{notice ?? hold.error?.message}</ErrorNote>
              </div>
            )}
            <button className="btn-primary mt-5 w-full py-3" disabled={count === 0 || hold.isPending} onClick={proceed}>
              {hold.isPending ? 'Holding…' : me ? 'Continue to checkout' : 'Sign in to continue'}
              <Icon name="arrowRight" />
            </button>
            <p className="mt-3 flex items-center justify-center gap-1.5 text-xs text-ink-faint">
              <Icon name="lock" className="h-3.5 w-3.5" /> We hold your tickets for 5 minutes
            </p>
          </aside>
        </section>
      )}
    </div>
  )
}
