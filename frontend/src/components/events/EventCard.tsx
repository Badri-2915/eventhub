import { Link } from 'react-router-dom'
import { formatPaise } from '../../lib/money'
import { formatWhen } from '../../lib/format'
import type { EventCardData } from '../../lib/types'
import { AvailabilityBadge } from '../ui/Feedback'
import { Icon } from '../ui/Icon'
import { EventCover } from './EventCover'

export function EventCard({ ev }: { ev: EventCardData }) {
  return (
    <Link to={`/events/${ev.id}`} className="group card overflow-hidden transition duration-300 hover:-translate-y-0.5 hover:shadow-lift">
      <EventCover eventId={ev.id} category={ev.category} posterUrl={ev.poster_url} startsAt={ev.starts_at} className="aspect-[16/10]" />
      <div className="space-y-3 p-5">
        <div>
          <h3 className="font-display text-lg leading-snug transition group-hover:text-pine-700">{ev.title}</h3>
          <p className="mt-1.5 flex items-center gap-1.5 text-sm text-ink-muted">
            <Icon name="calendar" className="h-3.5 w-3.5" />
            {formatWhen(ev.starts_at)}
          </p>
          <p className="mt-0.5 flex items-center gap-1.5 text-sm text-ink-muted">
            <Icon name="pin" className="h-3.5 w-3.5" />
            {ev.venue_name}, {ev.city}
          </p>
        </div>
        <div className="flex items-center justify-between border-t border-line pt-3">
          <span className="text-sm">
            {ev.max_price_paise === 0 ? (
              <span className="font-semibold text-pine-700">Free</span>
            ) : ev.min_price_paise === 0 ? (
              <span className="font-semibold">Free – {formatPaise(ev.max_price_paise)}</span>
            ) : (
              <>
                <span className="text-ink-faint">from </span>
                <span className="font-semibold">{formatPaise(ev.min_price_paise)}</span>
              </>
            )}
            {ev.max_discount_percent > 0 && <span className="ml-2 text-xs font-medium text-pine-700">up to {ev.max_discount_percent}% off</span>}
          </span>
          <AvailabilityBadge available={ev.available_tickets} total={ev.total_tickets} />
        </div>
      </div>
    </Link>
  )
}
