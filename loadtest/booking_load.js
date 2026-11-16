// Sustained load: 200 users browse an event, hold tickets and pay, for 60 s.
// Records throughput and latency. Start the API with HOLD_RATE_PER_MIN=1000000
// so the per-user rate limiter doesn't cap the test.
import http from 'k6/http'
import { check, sleep } from 'k6'
import { Counter } from 'k6/metrics'
import { BASE, cleanup, headers, loginMany, publishEvent, uuid } from './lib.js'

const VUS = Number(__ENV.VUS || 200)
const bookings = new Counter('bookings_confirmed')

export const options = {
  setupTimeout: '3m',
  scenarios: { load: { executor: 'constant-vus', vus: VUS, duration: __ENV.DURATION || '60s' } },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    'http_req_duration{name:hold}': ['p(95)<500'],
    'http_req_duration{name:confirm}': ['p(95)<500'],
  },
}

export function setup() {
  const { host, event } = publishEvent({
    title: '[k6] booking load', seating_mode: 'GA',
    categories: [{ name: 'Entry', color: '#3b7d66', price_paise: 49900, capacity: 1000000 }],
  })
  return { host, eventId: event.id, categoryId: event.categories[0].id, users: loginMany('load', VUS) }
}

export default function (d) {
  const user = d.users[__VU - 1]
  http.get(`${BASE}/events/${d.eventId}`, { tags: { name: 'event' } })

  const hold = http.post(
    `${BASE}/holds`,
    JSON.stringify({ event_id: d.eventId, items: [{ category_id: d.categoryId, qty: 1 + (__ITER % 2) }] }),
    { ...headers(user), tags: { name: 'hold' } },
  )
  if (!check(hold, { 'hold 201': (r) => r.status === 201 })) return

  const confirm = http.post(`${BASE}/bookings/${hold.json('booking_id')}/confirm`, '{}', {
    ...headers(user, { 'Idempotency-Key': uuid() }),
    tags: { name: 'confirm' },
  })
  if (check(confirm, { 'confirmed': (r) => r.status === 200 })) bookings.add(1)
  sleep(0.1)
}

export function teardown(d) {
  cleanup(d.host, d.eventId)
}
