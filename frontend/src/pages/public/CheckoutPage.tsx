import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo } from 'react'
import { Link, Navigate, useNavigate, useParams } from 'react-router-dom'
import { bookingsApi } from '../../api/endpoints'
import { EventCover } from '../../components/events/EventCover'
import { ErrorNote, PageLoader } from '../../components/ui/Feedback'
import { Icon } from '../../components/ui/Icon'
import { mmss, useCountdown } from '../../hooks/useCountdown'
import { formatWhen } from '../../lib/format'
import { formatPrice } from '../../lib/money'

/** One key per booking, kept across reloads, so a retried payment is never charged twice. */
function paymentKey(bookingId: string) {
  const k = `pay:${bookingId}`
  try {
    const existing = sessionStorage.getItem(k)
    if (existing) return existing
    const fresh = crypto.randomUUID()
    sessionStorage.setItem(k, fresh)
    return fresh
  } catch {
    return `${bookingId}:${crypto.randomUUID()}`
  }
}

export function CheckoutPage() {
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const booking = useQuery({ queryKey: ['booking', id], queryFn: () => bookingsApi.get(id) })
  const left = useCountdown(booking.data?.expires_at)
  const key = useMemo(() => paymentKey(id), [id])

  const pay = useMutation({
    mutationFn: (simulate: 'success' | 'failure') => bookingsApi.confirm(id, key, simulate),
    onSuccess: (b) => {
      qc.setQueryData(['booking', id], b)
      qc.invalidateQueries({ queryKey: ['bookings'] })
      if (b.status === 'CONFIRMED') navigate(`/bookings/${id}`, { replace: true })
    },
  })
  const release = useMutation({
    mutationFn: () => bookingsApi.release(id),
    onSuccess: () => navigate(`/events/${booking.data?.event.id}`, { replace: true }),
  })

  if (booking.isLoading) return <PageLoader />
  const b = booking.data
  if (!b) return <ErrorNote>{booking.error?.message ?? 'Booking not found'}</ErrorNote>
  if (b.status === 'CONFIRMED') return <Navigate to={`/bookings/${b.id}`} replace />

  const expired = b.status === 'EXPIRED' || (b.status === 'PENDING' && left === 0)
  const free = b.total_paise === 0
  const failed = b.status === 'FAILED'
  const urgent = left < 60

  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <Link to={`/events/${b.event.id}`} className="inline-flex items-center gap-1.5 text-sm text-ink-muted hover:text-ink">
        <Icon name="arrowLeft" /> Back to event
      </Link>

      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="eyebrow">Checkout</p>
          <h1 className="mt-2 font-display text-4xl">{free ? 'Confirm your tickets' : 'Review & pay'}</h1>
        </div>
        {b.status === 'PENDING' && !expired && (
          <div className={`flex items-center gap-2 rounded-full px-4 py-2 text-sm font-medium ${urgent ? 'bg-clay-50 text-clay-700' : 'bg-pine-50 text-pine-800'}`}>
            <Icon name="clock" />
            Held for <span className="tabular-nums">{mmss(left)}</span>
          </div>
        )}
      </div>

      {(expired || failed) && (
        <div className="card flex flex-col items-start gap-4 p-6 sm:flex-row sm:items-center">
          <span className="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-clay-50 text-clay-500">
            <Icon name={failed ? 'x' : 'clock'} />
          </span>
          <div className="flex-1">
            <h2 className="font-medium">{failed ? 'Payment did not go through' : 'Your hold has expired'}</h2>
            <p className="text-sm text-ink-muted">
              {failed ? 'You were not charged and your seats have been released.' : 'The seats went back on sale so others could book them.'}
            </p>
          </div>
          <Link to={`/events/${b.event.id}`} className="btn-primary">
            Choose again
          </Link>
        </div>
      )}

      <div className="card overflow-hidden">
        <div className="flex gap-5 border-b border-line p-6">
          <EventCover eventId={b.event.id} showLabel={false} category={b.event.category} posterUrl={b.event.poster_url} showDate={false} className="hidden h-20 w-28 shrink-0 rounded-xl sm:block" />
          <div>
            <h2 className="font-display text-xl">{b.event.title}</h2>
            <p className="mt-1 text-sm text-ink-muted">{formatWhen(b.event.starts_at)}</p>
            <p className="text-sm text-ink-muted">
              {b.event.venue_name}, {b.event.city}
            </p>
          </div>
        </div>
        <ul className="divide-y divide-line px-6">
          {b.items.map((it, i) => (
            <li key={i} className="flex items-center gap-3 py-3 text-sm">
              <span className="h-2 w-2 rounded-full" style={{ background: it.category_color }} />
              <span className="flex-1">
                {it.category_name}
                {it.seat_label ? <span className="text-ink-faint"> · Seat {it.seat_label}</span> : <span className="text-ink-faint"> × {it.quantity}</span>}
              </span>
              <span className="tabular-nums">{formatPrice(it.unit_price_paise * it.quantity)}</span>
            </li>
          ))}
        </ul>
        <div className="flex items-baseline justify-between bg-mist/50 px-6 py-4">
          <span className="text-sm text-ink-muted">Total</span>
          <span className="font-display text-2xl tabular-nums">{formatPrice(b.total_paise)}</span>
        </div>
      </div>

      {b.status === 'PENDING' && !expired && (
        <div className="card space-y-4 p-6">
          {free ? (
            <p className="text-sm text-ink-muted">These tickets are free. Confirm to get your QR ticket.</p>
          ) : (
            <div className="flex items-start gap-3 rounded-xl bg-sand-50 px-4 py-3 text-sm text-sand-700">
              <Icon name="info" className="mt-0.5 h-4 w-4 shrink-0" />
              This is a demo checkout. No real payment is taken.
            </div>
          )}
          {pay.error && <ErrorNote>{pay.error.message}</ErrorNote>}
          <button className="btn-primary w-full py-3 text-base" disabled={pay.isPending} onClick={() => pay.mutate('success')}>
            <Icon name={free ? 'check' : 'lock'} />
            {pay.isPending ? 'Processing…' : free ? 'Get free tickets' : `Pay ${formatPrice(b.total_paise)}`}
          </button>
          <div className="flex justify-between text-sm">
            <button className="cursor-pointer text-ink-faint hover:text-ink" disabled={release.isPending} onClick={() => release.mutate()}>
              Release my seats
            </button>
            {!free && (
              <button className="cursor-pointer text-ink-faint hover:text-clay-500" disabled={pay.isPending} onClick={() => pay.mutate('failure')}>
                Simulate a failed payment
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
