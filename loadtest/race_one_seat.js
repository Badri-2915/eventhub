// 200 different users try to hold the SAME seat at the same moment.
// Pass condition: exactly one hold succeeds, everyone else gets 409 SEATS_TAKEN.
import http from 'k6/http'
import { check } from 'k6'
import { Counter } from 'k6/metrics'
import { BASE, cleanup, headers, loginMany, publishEvent, uuid } from './lib.js'

const N = Number(__ENV.VUS || 200)
const won = new Counter('holds_won')
const rejected = new Counter('holds_rejected')

export const options = {
  setupTimeout: '2m',
  scenarios: { race: { executor: 'per-vu-iterations', vus: N, iterations: 1, maxDuration: '1m' } },
  thresholds: { holds_won: ['count==1'], holds_rejected: [`count==${N - 1}`] },
}

export function setup() {
  const cat = uuid()
  const layout = { rows: 1, cols: 10, cells: [Array(10).fill(cat)] }
  const { host, event } = publishEvent({
    title: '[k6] seat race', seating_mode: 'NUMBERED', layout,
    categories: [{ id: cat, name: 'Standard', color: '#3b7d66', price_paise: 50000 }],
  })
  const seats = http.get(`${BASE}/events/${event.id}/seats`).json('seats')
  return { host, eventId: event.id, seatId: seats.find((s) => s.label === 'A5').id, users: loginMany('race', N) }
}

export default function (d) {
  const res = http.post(`${BASE}/holds`, JSON.stringify({ event_id: d.eventId, seat_ids: [d.seatId] }), {
    ...headers(d.users[__VU - 1]),
    responseCallback: http.expectedStatuses(201, 409),
  })
  if (res.status === 201) won.add(1)
  else if (res.status === 409) rejected.add(1)
  check(res, { 'held or cleanly rejected': (r) => r.status === 201 || r.status === 409 })
}

export function teardown(d) {
  cleanup(d.host, d.eventId)
}
