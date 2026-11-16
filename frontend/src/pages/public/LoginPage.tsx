import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import { authApi } from '../../api/endpoints'
import { GoogleSignIn, googleConfigured } from '../../components/layout/GoogleSignIn'
import { ErrorNote } from '../../components/ui/Feedback'
import { ME_KEY, useMe } from '../../hooks/useMe'

export function LoginPage() {
  const { data: me } = useMe()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const from = (useLocation().state as { from?: string } | null)?.from ?? '/'
  const [email, setEmail] = useState('guest@eventhub.dev')

  const onUser = ({ user }: Awaited<ReturnType<typeof authApi.google>>) => {
    qc.setQueryData(ME_KEY, user)
    navigate(from, { replace: true })
  }
  const google = useMutation({ mutationFn: authApi.google, onSuccess: onUser })
  const dev = useMutation({ mutationFn: () => authApi.dev(email, ''), onSuccess: onUser })
  const error = google.error ?? dev.error

  if (me) return <Navigate to={from} replace />

  return (
    <div className="mx-auto grid max-w-4xl overflow-hidden rounded-3xl border border-line bg-white shadow-soft md:grid-cols-2">
      <div className="relative hidden bg-pine-800 p-10 text-pine-50 md:block">
        <svg viewBox="0 0 400 400" className="absolute inset-0 h-full w-full opacity-15" aria-hidden>
          {Array.from({ length: 9 }, (_, r) =>
            Array.from({ length: 9 }, (_, c) => (
              <rect key={`${r}-${c}`} x={30 + c * 40} y={60 + r * 36} width="26" height="24" rx="6" fill="none" stroke="currentColor" />
            )),
          )}
        </svg>
        <div className="relative flex h-full flex-col justify-end">
          <p className="font-display text-3xl leading-snug">“The best seat in the house should go to whoever gets there first.”</p>
          <p className="mt-4 text-sm text-pine-200">Live availability · 5-minute holds · Instant tickets</p>
        </div>
      </div>

      <div className="p-8 sm:p-12">
        <h1 className="font-display text-3xl">Welcome back</h1>
        <p className="mt-2 text-sm text-ink-muted">Sign in to book tickets or host your own events.</p>

        <div className="mt-8 space-y-6">
          {googleConfigured ? (
            <GoogleSignIn onToken={(t) => google.mutate(t)} />
          ) : (
            <p className="rounded-xl bg-mist px-4 py-3 text-xs text-ink-muted">
              Google sign-in turns on once <code className="font-mono">VITE_GOOGLE_CLIENT_ID</code> is set.
            </p>
          )}

          {import.meta.env.DEV && (
            <form
              className="space-y-3 border-t border-line pt-6"
              onSubmit={(e) => {
                e.preventDefault()
                dev.mutate()
              }}
            >
              <label className="label" htmlFor="dev-email">
                Developer sign-in <span className="text-ink-faint">(local only)</span>
              </label>
              <input id="dev-email" className="input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
              <button className="btn-secondary w-full" disabled={dev.isPending}>
                Continue with email
              </button>
            </form>
          )}

          {error && <ErrorNote>{error.message}</ErrorNote>}
        </div>
      </div>
    </div>
  )
}
