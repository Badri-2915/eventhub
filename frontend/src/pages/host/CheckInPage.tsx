import { useMutation } from '@tanstack/react-query'
import { useEffect, useRef, useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { hostApi } from '../../api/endpoints'
import { Spinner } from '../../components/ui/Feedback'
import { Icon } from '../../components/ui/Icon'
import { formatTime } from '../../lib/format'

/**
 * Door check-in. Scanning a ticket's QR code with any phone camera opens
 * /check-in/<code>, which checks the ticket in straight away. Staff can also
 * type a code. Only the event's host can check its tickets in, and only once.
 */
export function CheckInPage() {
  const { code: fromQr } = useParams()
  const navigate = useNavigate()
  const [code, setCode] = useState('')
  const input = useRef<HTMLInputElement>(null)
  const check = useMutation({ mutationFn: hostApi.checkIn })
  const started = useRef<string | null>(null)

  useEffect(() => {
    if (fromQr && started.current !== fromQr) {
      started.current = fromQr // StrictMode runs effects twice in dev
      check.mutate(fromQr.toUpperCase())
    }
  }, [fromQr, check])

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const c = code.trim().toUpperCase()
    if (!c) return
    if (fromQr) navigate('/host/check-in', { replace: true })
    check.mutate(c)
    setCode('')
    input.current?.focus()
  }

  const r = check.data
  const tone = check.isError ? 'bad' : r?.result === 'CHECKED_IN' ? 'good' : r ? 'warn' : null
  const styles = {
    good: 'bg-pine-700 text-white',
    warn: 'bg-sand-100 text-sand-700 ring-1 ring-sand-300',
    bad: 'bg-clay-50 text-clay-700 ring-1 ring-clay-500/30',
  }

  return (
    <div className="mx-auto max-w-md space-y-6">
      <Link to="/host" className="inline-flex items-center gap-1.5 text-sm text-ink-muted hover:text-ink">
        <Icon name="arrowLeft" /> Dashboard
      </Link>
      <div>
        <p className="eyebrow">At the door</p>
        <h1 className="mt-2 font-display text-4xl">Check in guests</h1>
        <p className="mt-2 text-sm text-ink-muted">Scan the QR code on a ticket with your phone camera, or type the ticket code.</p>
      </div>

      {check.isPending && (
        <div className="card flex items-center justify-center gap-3 p-10 text-ink-muted">
          <Spinner /> Checking ticket…
        </div>
      )}

      {tone && !check.isPending && (
        <div className={`rounded-3xl p-8 text-center ${styles[tone]}`}>
          <span className="mx-auto grid h-14 w-14 place-items-center rounded-full bg-white/20">
            <Icon name={tone === 'good' ? 'check' : tone === 'warn' ? 'info' : 'x'} className="h-7 w-7" />
          </span>
          {check.isError ? (
            <>
              <h2 className="mt-4 font-display text-2xl">Not valid</h2>
              <p className="mt-1 text-sm">{check.error.message}</p>
            </>
          ) : (
            r && (
              <>
                <h2 className={`mt-4 font-display text-2xl ${tone === 'good' ? 'text-white' : ''}`}>
                  {tone === 'good' ? 'Welcome in!' : 'Already checked in'}
                </h2>
                <p className="mt-3 text-lg font-medium">{r.attendee_name}</p>
                <p className="text-sm opacity-80">
                  {r.tickets} ticket{r.tickets === 1 ? '' : 's'} · {r.items}
                </p>
                <p className="mt-1 text-sm opacity-80">{r.event_title}</p>
                {tone === 'warn' && r.checked_in_at && <p className="mt-3 text-sm font-medium">First scanned at {formatTime(r.checked_in_at)}</p>}
              </>
            )
          )}
        </div>
      )}

      <form onSubmit={submit} className="card flex gap-2 p-4">
        <input
          ref={input}
          className="input font-mono uppercase tracking-wider"
          placeholder="EH-XXXXXXXX"
          value={code}
          onChange={(e) => setCode(e.target.value)}
          autoCapitalize="characters"
          autoComplete="off"
        />
        <button className="btn-primary shrink-0" disabled={check.isPending}>
          Check in
        </button>
      </form>
    </div>
  )
}
