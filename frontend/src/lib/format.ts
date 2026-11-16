const dayFmt = new Intl.DateTimeFormat('en-IN', { weekday: 'short', day: 'numeric', month: 'short' })
const timeFmt = new Intl.DateTimeFormat('en-IN', { hour: 'numeric', minute: '2-digit' })
const longFmt = new Intl.DateTimeFormat('en-IN', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' })

/** "Sat, 22 Nov · 7:30 pm" */
export function formatWhen(iso: string | null | undefined): string {
  if (!iso) return 'Date to be announced'
  const d = new Date(iso)
  return `${dayFmt.format(d)} · ${timeFmt.format(d)}`
}

export function formatLongDate(iso: string): string {
  return longFmt.format(new Date(iso))
}

export function formatTime(iso: string): string {
  return timeFmt.format(new Date(iso))
}

export function dateParts(iso: string) {
  const d = new Date(iso)
  return {
    day: d.getDate(),
    month: d.toLocaleString('en-IN', { month: 'short' }).toUpperCase(),
    weekday: d.toLocaleString('en-IN', { weekday: 'short' }),
  }
}

export const categoryLabel: Record<string, string> = {
  MOVIE: 'Film',
  COMEDY: 'Comedy',
  CONCERT: 'Music',
  THEATRE: 'Theatre',
  WORKSHOP: 'Workshop',
  OTHER: 'Event',
}

export function plural(n: number, one: string, many = one + 's') {
  return `${n} ${n === 1 ? one : many}`
}

/** The host's shared location link, or a Maps search for the venue address. */
export function directionsUrl(ev: { map_url?: string; venue_name: string; address: string; city: string }): string {
  if (ev.map_url) return ev.map_url
  const q = [ev.venue_name, ev.address, ev.city].filter(Boolean).join(', ')
  return `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(q)}`
}
