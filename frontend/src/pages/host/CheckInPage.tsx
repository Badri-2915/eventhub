import { useMutation } from '@tanstack/react-query'
import { useEffect, useRef, useState, type FormEvent } from 'react'
import { Link, useLocation, useParams } from 'react-router-dom'
import { ApiError } from '../../api/client'
import { hostApi } from '../../api/endpoints'
import { PageLoader, Spinner } from '../../components/ui/Feedback'
import { Icon } from '../../components/ui/Icon'
import { useMe } from '../../hooks/useMe'
import { formatLongDate, formatTime } from '../../lib/format'
import type { CheckInResult } from '../../lib/types'

const ERROR_TITLES: Record<string, string> = {
  TICKET_NOT_FOUND: 'Not a ticket for your events',
  TICKET_NOT_VALID: 'Ticket cancelled',
  CHECK_IN_CLOSED: 'Event is over',
}

/**
 * Door check-in. Scanning a ticket's QR code with any phone camera opens
 * /check-in/<code>. For the event's host it checks the ticket in straight away;
 * anyone else sees what the code is for. Staff can also type a code.
 */
export function CheckInPage() {
  const { code: fromQr } = useParams()
  const { data: me, isLoading } = useMe()
  if (isLoading) return <PageLoader />
  if (fromQr && !me?.is_host) return <NotHost code={fromQr.toUpperCase()} signedIn={!!me} />
  return <DoorDesk fromQr={fromQr?.toUpperCase()} />
}

/** What a guest (or a signed-out phone) sees after scanning a ticket. */
function NotHost({ code, signedIn }: { code: string; signedIn: boolean }) {
  const location = useLocation()
  return (
    <div className="mx-auto max-w-md space-y-6 text-center">
      <span className="mx-auto grid h-14 w-14 place-items-center rounded-full bg-pine-50 text-pine-700">
        <Icon name="ticket" className="h-7 w-7" />
      </span>
      <div>
        <p className="eyebrow">EventHub ticket</p>
        <h1 className="mt-2 font-display text-3xl">{code}</h1>
        <p className="mt-3 text-ink-muted">
          This ticket is checked in at the entrance by the event's host. If it is yours, just show this QR code at the door.
        </p>
      </div>
      <div className="flex justify-center gap-2">
        {signedIn ? (
          <Link to="/bookings" className="btn-primary">
            My tickets
          </Link>
        ) : (
          <Link to="/login" state={{ from: location.pathname }} className="btn-primary">
            Host? Sign in to check in
          </Link>
        )}
      </div>
    </div>
  )
}

function DoorDesk({ fromQr }: { fromQr?: string }) {
  const [code, setCode] = useState('')
  const [current, setCurrent] = useState<string | null>(null)
  const input = useRef<HTMLInputElement>(null)
  const check = useMutation({ mutationFn: hostApi.checkIn })
  const undo = useMutation({ mutationFn: hostApi.undoCheckIn })
  const started = useRef<string | null>(null)

  const run = (c: string) => {
    setCurrent(c)
    undo.reset()
    check.mutate(c)
  }

  useEffect(() => {
    if (fromQr && started.current !== fromQr) {
      started.current = fromQr // StrictMode runs effects twice in dev
      setCurrent(fromQr)
      check.mutate(fromQr)
    }
  }, [fromQr, check])

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const c = code.trim().toUpperCase()
    if (!c) return
    // Leave the scanned URL without remounting (the scan route is public, the desk is host-only).
    if (fromQr) window.history.replaceState(window.history.state, '', '/host/check-in')
    run(c)
    setCode('')
    input.current?.focus()
  }

  const pending = check.isPending || undo.isPending
  const r: CheckInResult | undefined = undo.data ?? check.data
  const err = undo.error ?? check.error
  const tone = err ? 'bad' : r?.result === 'CHECKED_IN' ? 'good' : r?.result === 'UNDONE' ? 'neutral' : r ? 'warn' : null
  const styles = {
    good: 'bg-pine-700 text-white',
    warn: 'bg-sand-100 text-sand-700 ring-1 ring-sand-300',
    bad: 'bg-clay-50 text-clay-700 ring-1 ring-clay-500/30',
    neutral: 'bg-white text-ink ring-1 ring-line',
  }
  const titles = { CHECKED_IN: 'Welcome in!', ALREADY_CHECKED_IN: 'Already checked in', UNDONE: 'Check-in undone' }

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

      {pending && (
        <div className="card flex items-center justify-center gap-3 p-10 text-ink-muted">
          <Spinner /> {undo.isPending ? 'Undoing…' : 'Checking ticket…'}
        </div>
      )}

      {tone && !pending && (
        <div className={`rounded-3xl p-8 text-center ${styles[tone]}`}>
          <span className={`mx-auto grid h-14 w-14 place-items-center rounded-full ${tone === 'neutral' ? 'bg-mist' : 'bg-white/20'}`}>
            <Icon name={tone === 'good' ? 'check' : tone === 'bad' ? 'x' : 'info'} className="h-7 w-7" />
          </span>
          {err ? (
            <>
              <h2 className="mt-4 font-display text-2xl">{ERROR_TITLES[err instanceof ApiError ? err.code : ''] ?? 'Not valid'}</h2>
              <p className="mt-1 text-sm">{err.message}</p>
              {current && <p className="mt-2 font-mono text-xs opacity-70">{current}</p>}
            </>
          ) : (
            r && (
              <>
                <h2 className={`mt-4 font-display text-2xl ${tone === 'good' ? 'text-white' : ''}`}>{titles[r.result]}</h2>
                <p className="mt-3 text-lg font-medium">{r.attendee_name}</p>
                <p className="text-sm opacity-80">
                  {r.tickets} ticket{r.tickets === 1 ? '' : 's'} · {r.items}
                </p>
                <p className="mt-1 text-sm opacity-80">{r.event_title}</p>
                {r.result === 'ALREADY_CHECKED_IN' && r.checked_in_at && (
                  <p className="mt-3 text-sm font-medium">First scanned at {formatTime(r.checked_in_at)}. Check the guest's name before letting them in.</p>
                )}
                {r.early && r.result === 'CHECKED_IN' && r.starts_at && (
                  <p className="mt-3 rounded-xl bg-white/15 px-3 py-2 text-sm">
                    Heads up: this event starts on {formatLongDate(r.starts_at)}. Undo if you scanned it by mistake.
                  </p>
                )}
                {r.result !== 'UNDONE' && current && (
                  <button
                    type="button"
                    className={`mt-4 cursor-pointer text-sm underline underline-offset-4 ${tone === 'good' ? 'text-white/80 hover:text-white' : 'opacity-80 hover:opacity-100'}`}
                    onClick={() => {
                      check.reset()
                      undo.mutate(current)
                    }}
                  >
                    Undo check-in
                  </button>
                )}
              </>
            )
          )}
        </div>
      )}

      {r && !pending && (
        <div className="card flex items-center justify-between px-5 py-4 text-sm">
          <span className="text-ink-muted">Guests arrived</span>
          <span className="font-display text-xl tabular-nums">
            {r.arrived} <span className="text-ink-faint">/ {r.expected}</span>
          </span>
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
        <button className="btn-primary shrink-0" disabled={pending}>
          Check in
        </button>
      </form>
    </div>
  )
}
