import type { ReactNode } from 'react'
import { Icon, type IconName } from './Icon'

export function Spinner({ className = 'h-5 w-5' }: { className?: string }) {
  return <span className={`inline-block animate-spin rounded-full border-2 border-pine-200 border-t-pine-700 ${className}`} />
}

export function PageLoader() {
  return (
    <div className="grid min-h-[40vh] place-items-center">
      <Spinner className="h-7 w-7" />
    </div>
  )
}

export function EmptyState({ icon = 'ticket', title, children, action }: { icon?: IconName; title: string; children?: ReactNode; action?: ReactNode }) {
  return (
    <div className="card flex flex-col items-center px-6 py-14 text-center">
      <span className="grid h-12 w-12 place-items-center rounded-full bg-pine-50 text-pine-700">
        <Icon name={icon} className="h-5 w-5" />
      </span>
      <h3 className="mt-4 font-display text-xl">{title}</h3>
      {children && <p className="mt-1.5 max-w-sm text-sm text-ink-muted">{children}</p>}
      {action && <div className="mt-6">{action}</div>}
    </div>
  )
}

export function ErrorNote({ children }: { children: ReactNode }) {
  return (
    <div className="flex items-start gap-2.5 rounded-xl border border-clay-500/20 bg-clay-50 px-4 py-3 text-sm text-clay-700">
      <Icon name="info" className="mt-0.5 h-4 w-4 shrink-0" />
      <div>{children}</div>
    </div>
  )
}

type Tone = 'pine' | 'sand' | 'clay' | 'neutral'
const tones: Record<Tone, string> = {
  pine: 'bg-pine-50 text-pine-700 ring-pine-200/60',
  sand: 'bg-sand-50 text-sand-700 ring-sand-300/60',
  clay: 'bg-clay-50 text-clay-700 ring-clay-500/20',
  neutral: 'bg-mist text-ink-muted ring-line',
}

export function Badge({ tone = 'neutral', children }: { tone?: Tone; children: ReactNode }) {
  return <span className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium ring-1 ring-inset ${tones[tone]}`}>{children}</span>
}

/** Availability as a calm badge: plenty / filling fast / few left / sold out. */
export function AvailabilityBadge({ available, total }: { available: number; total: number }) {
  if (total === 0) return null
  if (available <= 0) return <Badge tone="clay">Sold out</Badge>
  const ratio = available / total
  if (available <= 10 || ratio < 0.1) return <Badge tone="sand">Only {available} left</Badge>
  if (ratio < 0.4) return <Badge tone="sand">Filling fast</Badge>
  return <Badge tone="pine">Available</Badge>
}
