import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import type { PublicEvent, Seat } from '../lib/types'

type StreamMessage = {
  seats?: { id: string; status: Seat['status'] }[]
  categories?: { id: string; available: number }[]
}

export const eventKey = (id: string) => ['event', id] as const
export const seatsKey = (id: string) => ['event', id, 'seats'] as const

/**
 * Subscribes to live availability (SSE) and patches the TanStack Query cache
 * so seats recolour instantly. Returns whether the stream is connected; callers
 * fall back to polling when it isn't.
 */
export function useEventStream(eventId: string | undefined) {
  const qc = useQueryClient()
  const [live, setLive] = useState(false)

  useEffect(() => {
    if (!eventId) return
    // VITE_STREAM_BASE lets production open the stream straight on the API host,
    // since site-level proxies (e.g. Vercel rewrites) may buffer long-lived responses.
    const base = import.meta.env.VITE_STREAM_BASE || import.meta.env.VITE_API_BASE || '/api'
    const es = new EventSource(`${base}/events/${eventId}/stream`)
    es.onopen = () => setLive(true)
    es.onerror = () => setLive(false)
    es.onmessage = (e) => {
      const msg: StreamMessage = JSON.parse(e.data)
      if (msg.seats?.length) {
        const next = new Map(msg.seats.map((s) => [s.id, s.status]))
        qc.setQueryData<Seat[]>(seatsKey(eventId), (old) =>
          old?.map((s) => (next.has(s.id) ? { ...s, status: next.get(s.id)! } : s)),
        )
        // Per-category counts are derived server-side; refresh them quietly.
        qc.invalidateQueries({ queryKey: eventKey(eventId), exact: true })
      }
      if (msg.categories?.length) {
        const next = new Map(msg.categories.map((c) => [c.id, c.available]))
        qc.setQueryData<PublicEvent>(eventKey(eventId), (old) =>
          old && {
            ...old,
            categories: old.categories.map((c) => (next.has(c.id) ? { ...c, available: Math.max(0, next.get(c.id)!) } : c)),
          },
        )
      }
    }
    return () => es.close()
  }, [eventId, qc])

  return live
}
