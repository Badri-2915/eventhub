import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { Link, Navigate, useParams } from 'react-router-dom'
import { bookingsApi, ticketQrUrl } from '../../api/endpoints'
import { Badge, ErrorNote, PageLoader } from '../../components/ui/Feedback'
import { Icon } from '../../components/ui/Icon'
import { useMe } from '../../hooks/useMe'
import { categoryLabel, directionsUrl, formatLongDate, formatTime, formatWhen } from '../../lib/format'
import { formatPrice } from '../../lib/money'
import { downloadTicket } from '../../lib/ticketImage'

export function BookingPage() {
  const { id = '' } = useParams()
  const qc = useQueryClient()
  const [confirming, setConfirming] = useState(false)
  const booking = useQuery({ queryKey: ['booking', id], queryFn: () => bookingsApi.get(id) })
  const { data: me } = useMe()
  const download = useMutation({ mutationFn: () => downloadTicket(booking.data!, me?.name ?? '') })
  const cancel = useMutation({
    mutationFn: () => bookingsApi.cancel(id),
    onSuccess: (b) => {
      qc.setQueryData(['booking', id], b)
      qc.invalidateQueries({ queryKey: ['bookings'] })
      setConfirming(false)
    },
  })

  if (booking.isLoading) return <PageLoader />
  const b = booking.data
  if (!b) return <ErrorNote>{booking.error?.message ?? 'Booking not found'}</ErrorNote>
  if (b.status === 'PENDING') return <Navigate to={`/checkout/${b.id}`} replace />

  const cancelled = b.status === 'CANCELLED'
  const seats = b.items.filter((i) => i.seat_label).map((i) => i.seat_label)

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <Link to="/bookings" className="inline-flex items-center gap-1.5 text-sm text-ink-muted hover:text-ink">
        <Icon name="arrowLeft" /> My tickets
      </Link>

      {b.status === 'CONFIRMED' && (
        <div className="flex items-center gap-3 text-pine-700">
          <span className="grid h-9 w-9 place-items-center rounded-full bg-pine-100">
            <Icon name="check" />
          </span>
          <p className="font-medium">You're going! Your tickets are confirmed.</p>
        </div>
      )}

      {/* Ticket */}
      <div className={`overflow-hidden rounded-3xl border border-line bg-white shadow-lift ${cancelled ? 'opacity-70' : ''}`}>
        <div className="bg-pine-800 px-8 py-7 text-white">
          <p className="text-xs font-medium uppercase tracking-[0.2em] text-pine-200">{categoryLabel[b.event.category]}</p>
          <h1 className="mt-2 font-display text-3xl leading-tight text-white">{b.event.title}</h1>
        </div>
        <div className="grid grid-cols-2 gap-6 px-8 py-6 text-sm">
          <div>
            <p className="eyebrow">Date</p>
            <p className="mt-1 font-medium">{formatLongDate(b.event.starts_at)}</p>
          </div>
          <div>
            <p className="eyebrow">Time</p>
            <p className="mt-1 font-medium">{formatTime(b.event.starts_at)}</p>
          </div>
          <div className="col-span-2">
            <p className="eyebrow">Venue</p>
            <p className="mt-1 font-medium">{b.event.venue_name}</p>
            <p className="text-ink-muted">{[b.event.address, b.event.city].filter(Boolean).join(', ')}</p>
            <a
              href={directionsUrl(b.event)}
              target="_blank"
              rel="noopener noreferrer"
              className="mt-1.5 inline-flex items-center gap-1 font-medium text-pine-700 hover:text-pine-800 hover:underline"
            >
              Get directions <Icon name="external" className="h-3.5 w-3.5" />
            </a>
          </div>
          <div className="col-span-2">
            <p className="eyebrow">{seats.length ? 'Seats' : 'Tickets'}</p>
            <ul className="mt-2 flex flex-wrap gap-2">
              {b.items.map((it, i) => (
                <li key={i} className="flex items-center gap-1.5 rounded-lg bg-mist px-2.5 py-1">
                  <span className="h-2 w-2 rounded-full" style={{ background: it.category_color }} />
                  {it.seat_label ? `${it.seat_label} · ${it.category_name}` : `${it.quantity} × ${it.category_name}`}
                </li>
              ))}
            </ul>
          </div>
        </div>

        {/* perforation */}
        <div className="relative flex items-center">
          <span className="-ml-3 h-6 w-6 rounded-full border border-line bg-paper" />
          <span className="flex-1 border-t-2 border-dashed border-line" />
          <span className="-mr-3 h-6 w-6 rounded-full border border-line bg-paper" />
        </div>

        <div className="flex flex-col items-center gap-6 px-8 py-6 sm:flex-row">
          {b.ticket_code && !cancelled && (
            <img src={ticketQrUrl(b.ticket_code)} alt="Ticket QR code" className="h-36 w-36 rounded-xl ring-1 ring-line" />
          )}
          <div className="flex-1 text-center sm:text-left">
            <p className="eyebrow">Ticket code</p>
            <p className="mt-1 font-mono text-2xl font-semibold tracking-wider">{b.ticket_code ?? '—'}</p>
            {!cancelled && (
              <p className="mt-1 text-sm text-ink-muted">
                {b.checked_in_at ? `Checked in at ${formatTime(b.checked_in_at)}` : 'Show this QR code at the entrance.'}
              </p>
            )}
          </div>
          <div className="text-center sm:text-right">
            <p className="eyebrow">{b.total_paise === 0 ? 'Price' : 'Paid'}</p>
            <p className="mt-1 font-display text-2xl tabular-nums">{formatPrice(b.total_paise)}</p>
          </div>
        </div>
      </div>

      {b.status === 'CONFIRMED' && b.ticket_code && (
        <div className="flex flex-col gap-2 sm:flex-row">
          <button className="btn-primary flex-1 py-3" disabled={download.isPending} onClick={() => download.mutate()}>
            <Icon name="download" />
            {download.isPending ? 'Preparing…' : 'Download ticket'}
          </button>
          <a href={directionsUrl(b.event)} target="_blank" rel="noopener noreferrer" className="btn-secondary flex-1 py-3">
            <Icon name="pin" /> Directions
          </a>
        </div>
      )}
      {download.error && <ErrorNote>{download.error.message}</ErrorNote>}

      {cancelled && (
        <div className="flex items-center gap-2">
          <Badge tone="clay">Cancelled</Badge>
          <span className="text-sm text-ink-muted">Refunded in full{b.cancelled_at && ` on ${formatWhen(b.cancelled_at)}`}.</span>
        </div>
      )}

      {b.status === 'CONFIRMED' && (
        <div className="card p-5 text-sm">
          {b.can_cancel ? (
            confirming ? (
              <div className="flex flex-wrap items-center gap-3">
                <span className="flex-1 text-ink">Cancel and get a full refund? Your seats will be released.</span>
                <button className="btn-quiet" onClick={() => setConfirming(false)}>
                  Keep tickets
                </button>
                <button className="btn bg-clay-500 text-white hover:bg-clay-700" disabled={cancel.isPending} onClick={() => cancel.mutate()}>
                  {cancel.isPending ? 'Cancelling…' : 'Yes, cancel'}
                </button>
              </div>
            ) : (
              <div className="flex flex-wrap items-center justify-between gap-3">
                <span className="text-ink-muted">Free cancellation until {b.event.cancel_cutoff_hours} hours before the event.</span>
                <button className="btn-secondary" onClick={() => setConfirming(true)}>
                  Cancel booking
                </button>
              </div>
            )
          ) : (
            <span className="text-ink-muted">The cancellation window for this event has closed.</span>
          )}
          {cancel.error && (
            <div className="mt-3">
              <ErrorNote>{cancel.error.message}</ErrorNote>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
