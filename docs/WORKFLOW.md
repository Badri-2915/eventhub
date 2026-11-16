# EventHub workflows

Three kinds of people use EventHub. Everyone signs in the same way (Google; or the
developer sign-in locally). Roles are additive:

| Role | How you get it | What you can do |
|---|---|---|
| **Attendee** | sign in | browse, book, pay, view/download QR ticket, cancel |
| **Host** | click **Host an event → Become a host** (one click, self-serve) | create/publish events, live sales, attendees + CSV, door check-in, cancel events |
| **Admin** | your email is in the `ADMIN_EMAILS` setting | platform totals, see every event, cancel any event (with refunds) |

## 1. Host creates an event

```
Host dashboard → New event
  1 Details      title, genre (a cover image is picked automatically), description,
                 venue, city, address, Google Maps link, start/end, booking rules
  2 Seating      Numbered seats (films, theatre)  or  General admission (comedy, gigs)
  3 Tickets      ticket types + prices (+ capacity for general admission)
  4 Layout       numbered only: grid designer (paint tiers, aisles, blocked seats,
                 presets, "preview as customer")
  5 Review       checklist → Publish
```

Drafts autosave on every step and can be deleted. **Publish** turns the layout into
real seats in one transaction and locks seating/prices so sold tickets stay valid.

## 2. Attendee books

```
Discover → event page → pick seats (or quantities) → Continue
   └─ HOLD: seats reserved for 5 minutes (atomic; nobody else can take them)
Checkout (countdown) → Pay
   └─ CONFIRM: simulated payment with an Idempotency-Key (retries never double-charge)
Ticket page: QR code + details  →  "Download ticket" saves one image (all details + QR)
                                       to keep in the phone gallery, works offline
```

- Seats change colour live for everyone viewing the event (SSE + Redis).
- If the 5 minutes run out, the seats go back on sale automatically.
- **Cancel** from the ticket page until the host's cutoff (default 24 h before) → full refund.

## 3. At the door (QR check-in)

```
Guest shows QR  →  host scans it with the normal phone camera
                →  opens  <site>/check-in/EH-XXXXXXXX  (host must be signed in)
                →  ✅ "Welcome in! Asha Rao · A5, A6"      first scan
                   ⚠️ "Already checked in at 7:02 pm"     same ticket again
                   ❌ "Not valid"                          fake, cancelled, or another host's event
```

No scanner app needed. Staff can also type the code. Only the event's host can check
its tickets in, and each ticket checks in exactly once (even if two phones scan at
the same instant). The host dashboard shows who has arrived.

## 4. Cancelling

| Who | What | Effect |
|---|---|---|
| Attendee | **Cancel booking** on the ticket page (until the host's cutoff, default 24 h before) | full refund, seats back on sale |
| Host | **Manage → Cancel this event** | every booking refunded, all seats released, sales stop, tickets stop scanning |
| Admin | **Admin → Cancel event** on any event | same as the host cancelling |

Drafts that were never published can simply be **deleted** from the host dashboard.

## 4b. Emails: future extension (already built, off by default)

Tickets live on the website and are downloaded, so no email account is needed. The
email feature is already written and tested, just switched off:

| When | Who gets it | What |
|---|---|---|
| Payment confirmed | attendee | ticket with QR code, seats, venue, directions link |
| Payment confirmed | host | "New booking: Name × 2" + running totals |
| Event cancelled | every attendee | cancellation + full refund notice |

Turn it on by setting `BREVO_API_KEY` + `MAIL_FROM` (Brevo's free plan: 300 emails/day),
or any SMTP server. Emails are sent in the background *after* the booking is saved, so
a slow email provider can never block or undo a booking.

## 5. Admin

`/admin` (link appears in the top bar for admins): users, hosts, live events,
bookings and revenue across the platform, plus every event with its host and sales.
An admin can cancel any event, which refunds everyone. Admin access
is just an email allowlist (`ADMIN_EMAILS`), so there's no separate admin password to leak.

## 6. Payments: simulated on purpose

Checkout uses a simulated gateway ("Pay" / "Simulate failed payment"). Real gateways
(Razorpay, Stripe India) need a registered business and KYC, so for a portfolio
project the gateway is mocked while everything around it is built the real way:

- **Hold-then-pay:** inventory is reserved before money moves, released on failure/timeout.
- **Idempotency keys:** a retried "Pay" returns the first result instead of charging twice.
- **Payments table:** status SUCCESS / FAILED / REFUNDED per booking, refunds on cancel.

Swapping in Razorpay later: at hold time create a Razorpay *order* for the booking
total; the browser opens Razorpay Checkout; a **webhook** (signature-verified) calls
the existing confirm path with the payment id as the idempotency key. Nothing else changes.

## 7. Correctness in one paragraph (interview version)

Every seat or ticket is claimed by a single conditional `UPDATE … WHERE status =
'AVAILABLE'` (or `capacity - sold - held >= qty`) inside a transaction, so two buyers
can never both win; a `CHECK (sold + held <= capacity)` constraint backs it up. Holds
expire lazily (expired holds count as free in the same query) and a background sweeper
using `SKIP LOCKED` tidies up. Proven by tests: a 500-way race for one seat yields
exactly one hold; 300 buyers for 100 tickets sell exactly 100; 200 users booking for
60 s sustained ~350 bookings/s with zero errors.
