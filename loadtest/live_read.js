// Read-only browsing load (list → event → seat map) against any deployment.
// API_BASE=https://<your-api>.onrender.com/api k6 run live_read.js
import http from 'k6/http'
import { check, sleep } from 'k6'
const B = __ENV.API_BASE || 'http://localhost:8080/api'
export const options = {
  stages: [{ duration: '10s', target: 20 }, { duration: '20s', target: 50 }, { duration: '10s', target: 0 }],
  summaryTrendStats: ['avg', 'p(50)', 'p(95)', 'max'],
}
export function setup() {
  const r = http.get(`${B}/events`).json()
  const list = Array.isArray(r) ? r : r.events || r.items
  return { ids: list.map((e) => e.id) }
}
export default function (d) {
  const id = d.ids[Math.floor(Math.random() * d.ids.length)]
  check(http.get(`${B}/events`), { list: (r) => r.status === 200 })
  check(http.get(`${B}/events/${id}`), { event: (r) => r.status === 200 })
  check(http.get(`${B}/events/${id}/seats`), { seats: (r) => r.status === 200 || r.status === 404 })
  sleep(1)
}
