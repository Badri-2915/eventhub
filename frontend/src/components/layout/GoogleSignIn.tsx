import { useEffect, useRef } from 'react'

type CredentialResponse = { credential: string }

declare global {
  interface Window {
    google?: {
      accounts: {
        id: {
          initialize: (opts: { client_id: string; callback: (r: CredentialResponse) => void }) => void
          renderButton: (el: HTMLElement, opts: Record<string, unknown>) => void
        }
      }
    }
  }
}

export const GOOGLE_CLIENT_ID: string = import.meta.env.VITE_GOOGLE_CLIENT_ID ?? ''
export const googleConfigured = GOOGLE_CLIENT_ID !== '' && !GOOGLE_CLIENT_ID.startsWith('xxxx')

/** Renders Google's official button; hands the signed ID token to onToken. */
export function GoogleSignIn({ onToken }: { onToken: (idToken: string) => void }) {
  const ref = useRef<HTMLDivElement>(null)
  const cb = useRef(onToken)
  cb.current = onToken

  useEffect(() => {
    let cancelled = false
    const tryInit = () => {
      if (cancelled) return
      if (!window.google || !ref.current) {
        setTimeout(tryInit, 100) // GIS script loads async
        return
      }
      window.google.accounts.id.initialize({
        client_id: GOOGLE_CLIENT_ID,
        callback: (r) => cb.current(r.credential),
      })
      window.google.accounts.id.renderButton(ref.current, { theme: 'outline', size: 'large', width: 300 })
    }
    tryInit()
    return () => {
      cancelled = true
    }
  }, [])

  return <div ref={ref} className="flex justify-center" />
}
