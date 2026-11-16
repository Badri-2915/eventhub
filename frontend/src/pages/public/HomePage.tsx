import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { eventsApi } from '../../api/endpoints'
import { EventCard } from '../../components/events/EventCard'
import { EmptyState, ErrorNote } from '../../components/ui/Feedback'
import { Icon } from '../../components/ui/Icon'
import { categoryLabel } from '../../lib/format'
import { EVENT_CATEGORIES } from '../../lib/types'

export function HomePage() {
  const [params, setParams] = useSearchParams()
  const q = params.get('q') ?? ''
  const city = params.get('city') ?? ''
  const category = params.get('category') ?? ''
  const [draft, setDraft] = useState(q)

  const setParam = (key: string, value: string) => {
    const next = new URLSearchParams(params)
    if (value) next.set(key, value)
    else next.delete(key)
    setParams(next, { replace: true })
  }

  const cities = useQuery({ queryKey: ['cities'], queryFn: eventsApi.cities, staleTime: 5 * 60_000 })
  const events = useQuery({
    queryKey: ['events', { q, city, category }],
    queryFn: () => eventsApi.list({ q, city, category }),
    placeholderData: (prev) => prev,
  })

  return (
    <div className="space-y-12">
      <section className="relative overflow-hidden rounded-3xl bg-pine-800 px-8 py-14 text-white sm:px-14 sm:py-16">
        <svg viewBox="0 0 600 300" className="absolute -right-24 -top-10 h-[130%] opacity-[0.12]" aria-hidden>
          {Array.from({ length: 7 }, (_, r) =>
            Array.from({ length: 12 }, (_, c) => (
              <rect key={`${r}-${c}`} x={c * 46} y={r * 40 + Math.abs(c - 5.5) * 3} width="32" height="28" rx="8" fill="none" stroke="white" strokeWidth="1.5" />
            )),
          )}
        </svg>
        <div className="relative max-w-xl">
          <p className="text-sm font-medium tracking-wide text-pine-200">Films · Comedy · Music · Theatre</p>
          <h1 className="mt-3 font-display text-4xl leading-[1.1] text-white sm:text-5xl">Find a good evening out.</h1>
          <p className="mt-4 text-pine-100/90">Choose your exact seat, hold it for five minutes, and get your ticket instantly.</p>

          <form
            className="mt-8 flex flex-col gap-2 rounded-2xl bg-white p-2 shadow-lift sm:flex-row"
            onSubmit={(e) => {
              e.preventDefault()
              setParam('q', draft.trim())
            }}
          >
            <label className="flex flex-1 items-center gap-2 px-3 text-ink">
              <Icon name="search" className="h-4 w-4 text-ink-faint" />
              <input
                className="w-full bg-transparent py-2 text-sm outline-none placeholder:text-ink-faint"
                placeholder="Search events or venues"
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
              />
            </label>
            <select
              className="rounded-xl bg-mist px-3 py-2 text-sm text-ink outline-none"
              value={city}
              onChange={(e) => setParam('city', e.target.value)}
              aria-label="City"
            >
              <option value="">All cities</option>
              {cities.data?.map((c) => (
                <option key={c}>{c}</option>
              ))}
            </select>
            <button className="btn-primary">Search</button>
          </form>
        </div>
      </section>

      <section className="space-y-6">
        <div className="flex flex-wrap items-end justify-between gap-4">
          <div>
            <h2 className="font-display text-3xl">Upcoming</h2>
            <p className="mt-1 text-sm text-ink-muted">
              {events.data ? `${events.data.length} event${events.data.length === 1 ? '' : 's'}` : ' '}
              {city && ` in ${city}`}
              {q && ` matching “${q}”`}
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            {['', ...EVENT_CATEGORIES].map((c) => (
              <button
                key={c || 'all'}
                onClick={() => setParam('category', c)}
                className={`cursor-pointer rounded-full px-3.5 py-1.5 text-sm transition ${
                  category === c ? 'bg-ink text-white' : 'bg-white text-ink-muted ring-1 ring-line hover:text-ink'
                }`}
              >
                {c ? categoryLabel[c] : 'All'}
              </button>
            ))}
          </div>
        </div>

        {events.error && <ErrorNote>{events.error.message}</ErrorNote>}
        {events.isLoading && (
          <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
            {Array.from({ length: 6 }, (_, i) => (
              <div key={i} className="card h-80 animate-pulse bg-mist/50" />
            ))}
          </div>
        )}
        {events.data?.length === 0 && (
          <EmptyState icon="search" title="Nothing here yet">
            Try a different city or category, or clear your search.
          </EmptyState>
        )}
        <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
          {events.data?.map((ev) => <EventCard key={ev.id} ev={ev} />)}
        </div>
      </section>
    </div>
  )
}
