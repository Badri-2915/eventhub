// Shared helpers. Uses the API's development-only /auth/dev login, so run
// these against a local API started with APP_ENV=development.
import http from 'k6/http'
import { fail } from 'k6'

export const BASE = __ENV.BASE_URL || 'http://localhost:8080/api'
const JSON_HEADERS = { 'Content-Type': 'application/json' }

export function headers(cookie, extra = {}) {
  return { headers: { ...JSON_HEADERS, Cookie: `eh_session=${cookie}`, ...extra } }
}

/** Signs in (creating the user if needed) and returns the session cookie value. */
export function login(email) {
  const res = http.post(`${BASE}/auth/dev`, JSON.stringify({ email }), { headers: JSON_HEADERS })
  if (res.status !== 200) fail(`login ${email}: ${res.status} ${res.body}`)
  return res.cookies.eh_session[0].value
}

export function loginMany(prefix, n) {
  const run = Date.now()
  return Array.from({ length: n }, (_, i) => login(`${prefix}-${run}-${i}@k6.test`))
}

function uuid() {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0
    return (c === 'x' ? r : (r & 0x3) | 0x8).toString(16)
  })
}
export { uuid }

/** Creates and publishes an event as a fresh host. Returns { host, event }. */
export function publishEvent(input) {
  const host = login(`k6-host-${Date.now()}@k6.test`)
  http.post(`${BASE}/me/become-host`, null, headers(host))
  const start = new Date(Date.now() + 30 * 24 * 3600 * 1000).toISOString()
  const body = {
    category: 'OTHER', venue_name: 'Load Test Hall', city: 'Loadtown',
    starts_at: start, max_per_booking: 10, ...input,
  }
  const created = http.post(`${BASE}/host/events`, JSON.stringify(body), headers(host))
  if (created.status !== 201) fail(`create event: ${created.status} ${created.body}`)
  const id = created.json('event.id')
  const pub = http.post(`${BASE}/host/events/${id}/publish`, null, headers(host))
  if (pub.status !== 200) fail(`publish: ${pub.status} ${pub.body}`)
  return { host, event: pub.json('event') }
}

/** Cancels the test event so it never shows up on the Discover page. */
export function cleanup(host, eventId) {
  http.post(`${BASE}/host/events/${eventId}/cancel`, null, headers(host))
}
