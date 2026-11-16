import type {
  AdminEvent,
  AdminOverview,
  Attendee,
  Booking,
  BookingSummary,
  CheckInResult,
  EventCardData,
  EventInput,
  EventStats,
  HoldResult,
  HostEvent,
  PublicEvent,
  Seat,
  User,
} from '../lib/types'
import { api } from './client'

export const authApi = {
  me: () => api<{ user: User }>('/me').then((r) => r.user),
  google: (idToken: string) => api<{ user: User }>('/auth/google', { method: 'POST', json: { id_token: idToken } }),
  dev: (email: string, name: string) => api<{ user: User }>('/auth/dev', { method: 'POST', json: { email, name } }),
  logout: () => api<void>('/auth/logout', { method: 'POST' }),
  becomeHost: () => api<{ user: User }>('/me/become-host', { method: 'POST' }).then((r) => r.user),
}

export type EventFilters = { q?: string; city?: string; category?: string }

export const eventsApi = {
  list: (f: EventFilters) => {
    const params = new URLSearchParams()
    Object.entries(f).forEach(([k, v]) => v && params.set(k, v))
    return api<{ events: EventCardData[] }>(`/events?${params}`).then((r) => r.events)
  },
  cities: () => api<{ cities: string[] }>('/events/cities').then((r) => r.cities),
  get: (id: string) => api<{ event: PublicEvent }>(`/events/${id}`).then((r) => r.event),
  seats: (id: string) => api<{ seats: Seat[] }>(`/events/${id}/seats`).then((r) => r.seats),
}

export const bookingsApi = {
  hold: (body: { event_id: string; seat_ids?: string[]; items?: { category_id: string; qty: number }[] }) =>
    api<HoldResult>('/holds', { method: 'POST', json: body }),
  release: (id: string) => api<void>(`/holds/${id}`, { method: 'DELETE' }),
  get: (id: string) => api<{ booking: Booking }>(`/bookings/${id}`).then((r) => r.booking),
  confirm: (id: string, key: string, simulate: 'success' | 'failure') =>
    api<{ booking: Booking }>(`/bookings/${id}/confirm`, {
      method: 'POST',
      json: { simulate },
      headers: { 'Idempotency-Key': key },
    }).then((r) => r.booking),
  cancel: (id: string) => api<{ booking: Booking }>(`/bookings/${id}/cancel`, { method: 'POST' }).then((r) => r.booking),
  mine: () => api<{ bookings: BookingSummary[] }>('/me/bookings').then((r) => r.bookings),
}

export const hostApi = {
  list: () => api<{ events: HostEvent[] }>('/host/events').then((r) => r.events),
  get: (id: string) => api<{ event: HostEvent }>(`/host/events/${id}`).then((r) => r.event),
  create: (input: EventInput) =>
    api<{ event: HostEvent }>('/host/events', { method: 'POST', json: input }).then((r) => r.event),
  update: (id: string, input: EventInput) =>
    api<{ event: HostEvent }>(`/host/events/${id}`, { method: 'PUT', json: input }).then((r) => r.event),
  publish: (id: string) => api<{ event: HostEvent }>(`/host/events/${id}/publish`, { method: 'POST' }).then((r) => r.event),
  deleteDraft: (id: string) => api<void>(`/host/events/${id}`, { method: 'DELETE' }),
  cancel: (id: string) => api<{ refunded_bookings: number }>(`/host/events/${id}/cancel`, { method: 'POST' }),
  stats: (id: string) => api<EventStats>(`/host/events/${id}/stats`),
  attendees: (id: string) => api<{ attendees: Attendee[] }>(`/host/events/${id}/attendees`).then((r) => r.attendees),
  checkIn: (code: string) => api<CheckInResult>(`/host/check-in/${encodeURIComponent(code)}`, { method: 'POST' }),
  attendeesCsvUrl: (id: string) => `${import.meta.env.VITE_API_BASE ?? '/api'}/host/events/${id}/attendees.csv`,
}


export const ticketQrUrl = (code: string) => `${import.meta.env.VITE_API_BASE ?? '/api'}/tickets/${code}/qr.png`

export const adminApi = {
  overview: () => api<AdminOverview>('/admin/overview'),
  events: () => api<{ events: AdminEvent[] }>('/admin/events').then((r) => r.events),
  cancel: (id: string) => api<{ refunded_bookings: number }>(`/admin/events/${id}/cancel`, { method: 'POST' }),
}
