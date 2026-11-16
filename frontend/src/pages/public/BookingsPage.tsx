import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { bookingsApi } from '../../api/endpoints'
import { EventCover } from '../../components/events/EventCover'
import { Badge, EmptyState, ErrorNote, PageLoader } from '../../components/ui/Feedback'
import { Icon } from '../../components/ui/Icon'
import { formatWhen, plural } from '../../lib/format'
import { formatPrice } from '../../lib/money'
import type { BookingSummary } from '../../lib/types'

const tabs = ['Upcoming', 'Past', 'Cancelled'] as const
type Tab = (typeof tabs)[number]

function tabOf(b: BookingSummary): Tab {
  if (b.status === 'CANCELLED') return 'Cancelled'
  return new Date(b.event_starts_at) < new Date() ? 'Past' : 'Upcoming'
}

export function BookingsPage() {
  const [tab, setTab] = useState<Tab>('Upcoming')
  const { data, isLoading, error } = useQuery({ queryKey: ['bookings'], queryFn: bookingsApi.mine })

  if (isLoading) return <PageLoader />
  if (error) return <ErrorNote>{error.message}</ErrorNote>
  const list = (data ?? []).filter((b) => tabOf(b) === tab)

  return (
    <div className="space-y-8">
      <div>
        <h1 className="font-display text-4xl">My tickets</h1>
        <p className="mt-2 text-ink-muted">Everything you have booked, in one place.</p>
      </div>

      <div className="inline-flex rounded-xl bg-mist p-1">
        {tabs.map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`cursor-pointer rounded-lg px-4 py-1.5 text-sm transition ${tab === t ? 'bg-white font-medium text-ink shadow-soft' : 'text-ink-muted hover:text-ink'}`}
          >
            {t}
          </button>
        ))}
      </div>

      {list.length === 0 ? (
        <EmptyState
          title={tab === 'Upcoming' ? 'No upcoming tickets' : `No ${tab.toLowerCase()} bookings`}
          action={
            tab === 'Upcoming' && (
              <Link to="/" className="btn-primary">
                Discover events
              </Link>
            )
          }
        >
          {tab === 'Upcoming' ? 'When you book something, your tickets will appear here.' : undefined}
        </EmptyState>
      ) : (
        <div className="space-y-4">
          {list.map((b) => (
            <Link
              key={b.id}
              to={b.status === 'PENDING' ? `/checkout/${b.id}` : `/bookings/${b.id}`}
              className="card flex items-center gap-5 p-4 transition hover:shadow-lift"
            >
              <EventCover eventId={b.event_id} showLabel={false} category={b.event_category} posterUrl={b.event_poster_url} startsAt={b.event_starts_at} className="hidden h-24 w-36 shrink-0 rounded-xl sm:block" />
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <h3 className="truncate font-display text-lg">{b.event_title}</h3>
                  {b.status === 'PENDING' && <Badge tone="sand">Awaiting payment</Badge>}
                  {b.status === 'CANCELLED' && <Badge tone="clay">Cancelled · refunded</Badge>}
                </div>
                <p className="mt-1 text-sm text-ink-muted">{formatWhen(b.event_starts_at)}</p>
                <p className="text-sm text-ink-muted">
                  {b.event_venue}, {b.event_city}
                </p>
              </div>
              <div className="text-right">
                <div className="font-medium tabular-nums">{formatPrice(b.total_paise)}</div>
                <div className="text-sm text-ink-faint">{plural(b.ticket_count, 'ticket')}</div>
              </div>
              <Icon name="arrowRight" className="h-4 w-4 text-ink-faint" />
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}
