import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { hostApi } from '../../api/endpoints'
import { EventCover } from '../../components/events/EventCover'
import { Badge, EmptyState, ErrorNote, PageLoader } from '../../components/ui/Feedback'
import { Icon } from '../../components/ui/Icon'
import { formatWhen } from '../../lib/format'
import { formatPaise } from '../../lib/money'
import type { EventStatus, HostEvent } from '../../lib/types'

const statusTone: Record<EventStatus, 'pine' | 'neutral' | 'clay' | 'sand'> = {
  DRAFT: 'neutral',
  PUBLISHED: 'pine',
  CANCELLED: 'clay',
  COMPLETED: 'sand',
}

export function HostDashboard() {
  const { data: events, isLoading, error } = useQuery({ queryKey: ['host', 'events'], queryFn: hostApi.list })

  if (isLoading) return <PageLoader />
  if (error) return <ErrorNote>{error.message}</ErrorNote>
  const published = events?.filter((e) => e.status === 'PUBLISHED').length ?? 0
  const drafts = events?.filter((e) => e.status === 'DRAFT').length ?? 0

  return (
    <div className="space-y-8">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="eyebrow">Host dashboard</p>
          <h1 className="mt-2 font-display text-4xl">Your events</h1>
        </div>
        <Link to="/host/events/new" className="btn-primary">
          <Icon name="plus" /> New event
        </Link>
      </div>

      <div className="grid gap-4 sm:grid-cols-3">
        <Stat label="All events" value={events?.length ?? 0} />
        <Stat label="Live" value={published} />
        <Stat label="Drafts" value={drafts} />
      </div>

      {events?.length === 0 ? (
        <EmptyState
          icon="sparkle"
          title="Create your first event"
          action={
            <Link to="/host/events/new" className="btn-primary">
              Get started
            </Link>
          }
        >
          It takes about two minutes. You can save a draft and come back any time.
        </EmptyState>
      ) : (
        <div className="space-y-3">
          {events?.map((ev) => <Row key={ev.id} ev={ev} />)}
        </div>
      )}
    </div>
  )
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div className="card px-5 py-4">
      <div className="text-sm text-ink-muted">{label}</div>
      <div className="mt-1 font-display text-3xl">{value}</div>
    </div>
  )
}

function Row({ ev }: { ev: HostEvent }) {
  const prices = ev.categories.map((c) => c.price_paise)
  const qc = useQueryClient()
  const [confirming, setConfirming] = useState(false)
  const remove = useMutation({
    mutationFn: () => hostApi.deleteDraft(ev.id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['host', 'events'] }),
  })
  return (
    <div className="card flex items-center gap-5 p-4">
      <EventCover eventId={ev.id} showLabel={false} category={ev.category} posterUrl={ev.poster_url} showDate={false} className="hidden h-16 w-24 shrink-0 rounded-xl sm:block" />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <h3 className="truncate font-medium">{ev.title}</h3>
          <Badge tone={statusTone[ev.status]}>{ev.status === 'PUBLISHED' ? 'Live' : ev.status.toLowerCase()}</Badge>
        </div>
        <p className="mt-0.5 text-sm text-ink-muted">
          {formatWhen(ev.starts_at)}
          {ev.city && ` · ${ev.city}`} · {ev.seating_mode === 'GA' ? 'General admission' : 'Numbered seats'}
        </p>
      </div>
      <div className="hidden w-44 text-right text-sm md:block">
        {ev.status === 'DRAFT' || !ev.summary ? (
          prices.length ? `${formatPaise(Math.min(...prices))} – ${formatPaise(Math.max(...prices))}` : <span className="text-ink-faint">No tickets</span>
        ) : (
          <>
            <div className="tabular-nums">
              <span className="font-medium">{ev.summary.sold}</span>
              <span className="text-ink-faint"> / {ev.summary.capacity} sold</span>
            </div>
            <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-mist">
              <div className="h-full rounded-full bg-pine-500" style={{ width: `${(ev.summary.sold / Math.max(ev.summary.capacity, 1)) * 100}%` }} />
            </div>
            <div className="mt-1 text-xs text-ink-faint">{formatPaise(ev.summary.revenue_paise)} revenue</div>
          </>
        )}
      </div>
      <div className="flex items-center gap-1">
        {ev.status === 'DRAFT' &&
          (confirming ? (
            <>
              <span className="mr-1 text-sm text-ink-muted">Delete draft?</span>
              <button className="btn bg-clay-500 py-2 text-white hover:bg-clay-700" disabled={remove.isPending} onClick={() => remove.mutate()}>
                Delete
              </button>
              <button className="btn-quiet py-2" onClick={() => setConfirming(false)}>
                Keep
              </button>
            </>
          ) : (
            <button className="btn-quiet" title="Delete draft" aria-label="Delete draft" onClick={() => setConfirming(true)}>
              <Icon name="trash" />
            </button>
          ))}
        {ev.status === 'PUBLISHED' && (
          <Link to={`/events/${ev.id}`} className="btn-quiet" title="View public page">
            <Icon name="eye" />
          </Link>
        )}
        {!confirming && (
          <Link to={ev.status === 'DRAFT' ? `/host/events/${ev.id}/edit` : `/host/events/${ev.id}`} className="btn-secondary py-2">
            {ev.status === 'DRAFT' ? 'Edit' : 'Manage'}
          </Link>
        )}
      </div>
    </div>
  )
}
