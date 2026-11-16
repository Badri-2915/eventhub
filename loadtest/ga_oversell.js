// 300 users each try to buy 1 ticket from a general-admission pool of 100.
// Pass condition: exactly 100 holds and 100 confirmed payments; never oversold.
import http from 'k6/http'
import { check } from 'k6'
import { Counter } from 'k6/metrics'
import { BASE, cleanup, headers, loginMany, publishEvent, uuid } from './lib.js'

const N = Number(__ENV.VUS || 300)
const CAPACITY = 100
const held = new Counter('holds_won')
const paid = new Counter('payments_confirmed')

export const options = {
  setupTimeout: '2m',
  scenarios: { rush: { executor: 'per-vu-iterations', vus: N, iterations: 1, maxDuration: '1m' } },
  thresholds: { holds_won: [`count==${CAPACITY}`], payments_confirmed: [`count==${CAPACITY}`] },
}

export function setup() {
  const { host, event } = publishEvent({
    title: '[k6] GA oversell', seating_mode: 'GA',
    categories: [{ name: 'Entry', color: '#3b7d66', price_paise: 49900, capacity: CAPACITY }],
  })
  return { host, eventId: event.id, categoryId: event.categories[0].id, users: loginMany('ga', N) }
}

export default function (d) {
  const opts = { ...headers(d.users[__VU - 1]), responseCallback: http.expectedStatuses(200, 201, 409) }
  const hold = http.post(`${BASE}/holds`, JSON.stringify({ event_id: d.eventId, items: [{ category_id: d.categoryId, qty: 1 }] }), opts)
  if (hold.status !== 201) return
  held.add(1)
  const bookingId = hold.json('booking_id')
  const confirm = http.post(`${BASE}/bookings/${bookingId}/confirm`, '{}', headers(d.users[__VU - 1], { 'Idempotency-Key': uuid() }))
  if (confirm.status === 200 && confirm.json('booking.status') === 'CONFIRMED') paid.add(1)
}

export function teardown(d) {
  const ev = http.get(`${BASE}/events/${d.eventId}`).json('event')
  check(ev, { 'sold out, never oversold': (e) => e.categories[0].available === 0 })
  cleanup(d.host, d.eventId)
}
