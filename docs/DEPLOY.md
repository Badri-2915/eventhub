# Deploying EventHub for free

```
Browser ──► Vercel (React site) ──/api/* rewrite──► Render (Go API, Docker)
                                                     ├──► Neon (PostgreSQL)
                                                     └──► Upstash (Redis pub/sub + rate limit)
```

The site and API share one origin through Vercel's `/api` rewrite, so the httpOnly
session cookie works without CORS. Expect about 30 minutes. Do the steps in order.

## What you need (all free, sign up with GitHub)

| Account | Used for | You copy |
|---|---|---|
| GitHub | hosts the code; Render/Vercel deploy from it | — |
| Google Cloud Console | "Sign in with Google" | OAuth **Client ID** |
| Neon (neon.tech) | PostgreSQL | **direct** connection string |
| Upstash (upstash.com) | Redis | `rediss://…` URL |
| Render (render.com) | runs the Go API | the API URL |
| Vercel (vercel.com) | hosts the website | the site URL |

Keep secrets in the dashboards only. Never commit them or paste them in chat.

## 1. Push the code to GitHub

```bash
cd eventhub
git add -A && git commit -m "EventHub"
gh repo create eventhub --public --source=. --push     # or create the repo on github.com and `git push`
```

## 2. Neon (database)

1. Create a project (region: Singapore / Mumbai is closest to India).
2. **Connection Details** → turn **off** "Pooled connection" → copy the string
   (`postgres://…@ep-xxx.ap-southeast-1.aws.neon.tech/neondb?sslmode=require`).
   The direct (non-pooler) host is needed for migrations and prepared statements.

## 3. Upstash (Redis)

Create a Redis database (same region) → copy the **TLS** URL: `rediss://default:…@….upstash.io:6379`.

## 4. Google sign-in

1. console.cloud.google.com → create a project → **APIs & Services → OAuth consent screen** → External → fill the app name and your email → add yourself as a test user.
2. **Credentials → Create credentials → OAuth client ID → Web application**.
3. **Authorised JavaScript origins:** `http://localhost:5173` now; add your Vercel URL after step 6.
4. Copy the **Client ID** (`….apps.googleusercontent.com`). No client secret is needed.

## 5. Render (API)

1. **New → Blueprint** → choose the repo. It reads [`render.yaml`](../render.yaml).
2. Fill the prompted values: `DATABASE_URL` (Neon), `REDIS_URL` (Upstash), `GOOGLE_CLIENT_ID`,
   `FRONTEND_ORIGIN` (put `https://example.com` for now; fix it after step 6),
   `ADMIN_EMAILS` (your Google email, which makes you admin).
   `JWT_SECRET` is generated for you.
3. Deploy, then open `https://<your-service>.onrender.com/api/healthz` → `{"db":"ok","redis":"ok"}`.
   If the deploy fails, open the service → **Logs**: the last lines name the missing or wrong setting.

## 6. Vercel (website)

1. Edit [`frontend/vercel.json`](../frontend/vercel.json): replace `eventhub-api.onrender.com` with your Render host. Commit and push.
2. **Add New → Project** → import the repo → **Root Directory: `frontend`** (Vite is auto-detected).
3. Environment variables:
   - `VITE_GOOGLE_CLIENT_ID` = the same Client ID
   - `VITE_STREAM_BASE` = `https://<your-service>.onrender.com/api` (live seat updates go straight to the API)
4. Deploy and copy the site URL, e.g. `https://eventhub-yourname.vercel.app`.

## 7. Connect the pieces

1. Render → environment → set `FRONTEND_ORIGIN` to the Vercel URL → it redeploys.
2. Google Console → OAuth client → add the Vercel URL to **Authorised JavaScript origins**.
3. Seed the demo events into Neon from your laptop:
   ```bash
   cd backend && DATABASE_URL='<neon direct url>' go run ./cmd/seed
   ```

Open the Vercel URL, sign in with Google, book a seat. Done.

## Keep the API awake (UptimeRobot)

uptimerobot.com → **New monitor → HTTP(s)** → URL `https://<your-service>.onrender.com/api/ping`
→ interval **5 minutes**. You also get an email if the site goes down.
Use `/api/ping`, not `/api/healthz`: ping doesn't touch the database, so Neon's free
compute can still sleep while nobody is booking.

## Optional later: emails

Emails (ticket with QR, host booking alerts, cancellation notices) are built but off.
To turn them on: sign up at brevo.com (free, 300/day) → verify a sender email →
create an API key → in Render add `BREVO_API_KEY` and `MAIL_FROM` = `EventHub <your-verified-email>`.
(Brevo uses HTTPS, which works even where free hosts block SMTP ports.)

## Free-tier notes

- Render's free API **sleeps after ~15 min idle** unless UptimeRobot pings it (above); a cold start takes ~30–60 s.
- If the live stream drops, the seat map falls back to refreshing every 20 s automatically.
- The developer sign-in is disabled in production (`APP_ENV=production`).
- To deploy updates, push to `main`: Render and Vercel redeploy automatically, and GitHub Actions runs the tests ([`.github/workflows/ci.yml`](../.github/workflows/ci.yml)).
