import { categoryLabel, dateParts } from '../../lib/format'
import type { EventCategory } from '../../lib/types'

const VARIANTS = 3

/** Picks one of the genre's built-in covers, stable per event (same event → same image). */
export function coverFor(category: EventCategory, eventId: string): string {
  let h = 0
  for (let i = 0; i < eventId.length; i++) h = (h * 31 + eventId.charCodeAt(i)) >>> 0
  return `/covers/${category.toLowerCase()}-${(h % VARIANTS) + 1}.svg`
}

export function EventCover({
  eventId,
  category,
  posterUrl,
  startsAt,
  className = '',
  showDate = true,
  showLabel = true,
}: {
  eventId: string
  category: EventCategory
  posterUrl?: string
  startsAt?: string | null
  className?: string
  showDate?: boolean
  showLabel?: boolean
}) {
  const d = startsAt ? dateParts(startsAt) : null
  return (
    <div className={`relative overflow-hidden bg-mist ${className}`}>
      <img src={posterUrl || coverFor(category, eventId)} alt="" className="absolute inset-0 h-full w-full object-cover" loading="lazy" />
      {showLabel && (
        <span className="absolute bottom-3 left-3 rounded-full bg-black/30 px-2.5 py-0.5 text-xs font-medium text-white backdrop-blur-sm">
          {categoryLabel[category]}
        </span>
      )}
      {showDate && d && (
        <div className="absolute right-3 top-3 rounded-lg bg-white/95 px-2.5 py-1.5 text-center shadow-sm backdrop-blur">
          <div className="text-[10px] font-semibold tracking-wider text-clay-500">{d.month}</div>
          <div className="font-display text-lg leading-none text-ink">{d.day}</div>
        </div>
      )}
    </div>
  )
}
