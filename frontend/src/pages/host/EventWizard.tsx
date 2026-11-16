import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { hostApi } from '../../api/endpoints'
import { LayoutDesigner } from '../../components/host/LayoutDesigner'
import { Badge, ErrorNote, PageLoader } from '../../components/ui/Feedback'
import { Icon } from '../../components/ui/Icon'
import { categoryLabel } from '../../lib/format'
import { AISLE, BLOCKED, isLayout, sanitizeLayout } from '../../lib/layout'
import { formatPaise, rupeesToPaise } from '../../lib/money'
import { EVENT_CATEGORIES, type EventInput, type HostEvent, type Layout, type SeatingMode } from '../../lib/types'

type StepId = 'details' | 'seating' | 'tickets' | 'layout' | 'review'
const STEP_TITLES: Record<StepId, string> = {
  details: 'Details',
  seating: 'Seating',
  tickets: 'Tickets',
  layout: 'Layout',
  review: 'Review',
}
const PALETTE = ['#B98B2F', '#5B7FA6', '#3B7D66', '#A65D57', '#6E8B74', '#7C7466']

// Form state keeps user-typed strings; conversion to API types happens on save.
type CatForm = { id: string; name: string; color: string; price: string; capacity: string }
type Form = Omit<EventInput, 'categories' | 'starts_at' | 'ends_at' | 'layout'> & {
  starts_at: string // datetime-local value
  ends_at: string
  layout: Layout | null
  categories: CatForm[]
}

// A factory, not a constant: category ids are primary keys, so every new event needs fresh ones.
const emptyForm = (): Form => ({
  title: '',
  description: '',
  category: 'COMEDY',
  poster_url: '',
  map_url: '',
  venue_name: '',
  city: '',
  address: '',
  starts_at: '',
  ends_at: '',
  seating_mode: 'GA',
  layout: null,
  max_per_booking: 10,
  cancel_cutoff_hours: 24,
  categories: [
    { id: crypto.randomUUID(), name: 'Premium', color: PALETTE[0], price: '999', capacity: '30' },
    { id: crypto.randomUUID(), name: 'Regular', color: PALETTE[2], price: '499', capacity: '70' },
  ],
})

function toLocalInput(iso: string | null): string {
  if (!iso) return ''
  const d = new Date(iso)
  return new Date(d.getTime() - d.getTimezoneOffset() * 60_000).toISOString().slice(0, 16)
}

function fromEvent(ev: HostEvent): Form {
  return {
    ...ev,
    starts_at: toLocalInput(ev.starts_at),
    ends_at: toLocalInput(ev.ends_at),
    layout: isLayout(ev.layout) ? ev.layout : null,
    categories: ev.categories.map((c) => ({
      id: c.id,
      name: c.name,
      color: c.color,
      price: String(c.price_paise / 100),
      capacity: c.capacity == null ? '' : String(c.capacity),
    })),
  }
}

function toInput(f: Form): EventInput {
  const ids = new Set(f.categories.map((c) => c.id))
  return {
    ...f,
    starts_at: f.starts_at ? new Date(f.starts_at).toISOString() : null,
    ends_at: f.ends_at ? new Date(f.ends_at).toISOString() : null,
    layout: f.seating_mode === 'NUMBERED' && f.layout ? sanitizeLayout(f.layout, ids) : null,
    categories: f.categories.map((c) => ({
      id: c.id,
      name: c.name,
      color: c.color,
      price_paise: rupeesToPaise(c.price),
      capacity: f.seating_mode === 'GA' && c.capacity !== '' ? Number(c.capacity) : null,
    })),
  }
}

/** Client-side mirror of the server's publish checks, for friendly hints. */
function publishIssues(f: Form): string[] {
  const issues: string[] = []
  if (!f.title.trim()) issues.push('Add a title')
  if (!f.starts_at) issues.push('Set a start time')
  else if (new Date(f.starts_at) <= new Date()) issues.push('Start time must be in the future')
  if (!f.venue_name.trim() || !f.city.trim()) issues.push('Add the venue name and city')
  if (f.categories.length === 0) issues.push('Add at least one ticket category')
  if (f.categories.some((c) => !c.name.trim())) issues.push('Every ticket category needs a name')
  if (f.seating_mode === 'GA' && f.categories.some((c) => !(Number(c.capacity) > 0))) issues.push('Every category needs a capacity')
  if (f.seating_mode === 'NUMBERED') {
    if (!f.layout) issues.push('Design the seating layout')
    else {
      const ids = new Set(f.categories.map((c) => c.id))
      const cells = f.layout.cells.flat()
      if (cells.some((c) => c !== AISLE && c !== BLOCKED && !ids.has(c))) issues.push('Some seats have no ticket category')
      if (!cells.some((c) => ids.has(c))) issues.push('The layout needs at least one sellable seat')
    }
  }
  return issues
}

export function EventWizard() {
  const { id } = useParams()
  const navigate = useNavigate()
  const qc = useQueryClient()
  // Step lives in the URL so it survives the /new → /:id/edit remount and reloads.
  const [params, setParams] = useSearchParams()
  const step = (params.get('step') as StepId | null) ?? 'details'
  const setStep = (s: StepId) => setParams({ step: s }, { replace: true })
  const [form, setForm] = useState<Form>(emptyForm)
  const [savedAt, setSavedAt] = useState<Date | null>(null)

  const existing = useQuery({ queryKey: ['host', 'event', id], queryFn: () => hostApi.get(id!), enabled: !!id })
  useEffect(() => {
    if (existing.data) setForm(fromEvent(existing.data))
  }, [existing.data])

  const steps: StepId[] =
    form.seating_mode === 'NUMBERED' ? ['details', 'seating', 'tickets', 'layout', 'review'] : ['details', 'seating', 'tickets', 'review']
  const idx = Math.max(0, steps.indexOf(step))
  const readOnly = existing.data != null && existing.data.status !== 'DRAFT'
  const issues = useMemo(() => publishIssues(form), [form])

  const save = useMutation({
    mutationFn: (f: Form) => (id ? hostApi.update(id, toInput(f)) : hostApi.create(toInput(f))),
    onSuccess: (ev) => {
      setSavedAt(new Date())
      qc.setQueryData(['host', 'event', ev.id], ev)
      qc.invalidateQueries({ queryKey: ['host', 'events'] })
      if (!id) navigate(`/host/events/${ev.id}/edit${window.location.search}`, { replace: true })
    },
  })

  const publish = useMutation({
    mutationFn: async () => {
      const saved = id ? await hostApi.update(id, toInput(form)) : await hostApi.create(toInput(form))
      return hostApi.publish(saved.id)
    },
    onSuccess: (ev) => {
      qc.invalidateQueries({ queryKey: ['host'] })
      qc.invalidateQueries({ queryKey: ['events'] })
      navigate(`/events/${ev.id}`)
    },
  })

  const set = <K extends keyof Form>(key: K, value: Form[K]) => setForm((f) => ({ ...f, [key]: value }))
  const go = (next: StepId) => {
    if (!readOnly && form.title.trim()) save.mutate(form)
    setStep(next)
  }

  if (id && existing.isLoading) return <PageLoader />
  if (existing.error) return <ErrorNote>{existing.error.message}</ErrorNote>

  const current = steps[idx]
  const designerCats = form.categories.map((c) => ({ id: c.id, name: c.name, color: c.color, price_paise: rupeesToPaise(c.price) }))

  return (
    <div className={`mx-auto space-y-8 ${current === 'layout' ? 'max-w-6xl' : 'max-w-3xl'}`}>
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <Link to="/host" className="inline-flex items-center gap-1.5 text-sm text-ink-muted hover:text-ink">
            <Icon name="arrowLeft" /> Dashboard
          </Link>
          <h1 className="mt-3 font-display text-4xl">{form.title || (id ? 'Untitled event' : 'New event')}</h1>
        </div>
        <div className="text-sm text-ink-faint">
          {readOnly ? (
            <Badge tone="pine">Published · read-only</Badge>
          ) : save.isPending ? (
            'Saving…'
          ) : savedAt ? (
            `Draft saved ${savedAt.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })}`
          ) : (
            'Draft'
          )}
        </div>
      </div>

      <ol className="flex items-center gap-2">
        {steps.map((s, i) => (
          <li key={s} className="flex flex-1 items-center gap-2 last:flex-none">
            <button type="button" onClick={() => go(s)} className="flex cursor-pointer items-center gap-2 text-sm">
              <span
                className={`grid h-7 w-7 shrink-0 place-items-center rounded-full text-xs font-semibold transition ${
                  i < idx ? 'bg-pine-100 text-pine-800' : i === idx ? 'bg-pine-700 text-white' : 'bg-white text-ink-faint ring-1 ring-line'
                }`}
              >
                {i < idx ? <Icon name="check" className="h-3.5 w-3.5" /> : i + 1}
              </span>
              <span className={`hidden sm:inline ${i === idx ? 'font-medium text-ink' : 'text-ink-muted'}`}>{STEP_TITLES[s]}</span>
            </button>
            {i < steps.length - 1 && <span className="h-px flex-1 bg-line" />}
          </li>
        ))}
      </ol>

      <fieldset disabled={readOnly && current !== 'layout'} className="card space-y-8 p-6 sm:p-8">
        {current === 'details' && <DetailsStep form={form} set={set} />}
        {current === 'seating' && <SeatingStep mode={form.seating_mode} onChange={(m) => set('seating_mode', m)} />}
        {current === 'tickets' && <TicketsStep form={form} set={set} />}
        {current === 'layout' && (
          <LayoutDesigner
            layout={form.layout}
            categories={designerCats}
            onChange={(l) => set('layout', l)}
            stageLabel={form.category === 'MOVIE' ? 'Screen' : 'Stage'}
            readOnly={readOnly}
          />
        )}
        {current === 'review' && <ReviewStep form={form} issues={readOnly ? [] : issues} />}
      </fieldset>

      {(save.error || publish.error) && <ErrorNote>{(publish.error ?? save.error)!.message}</ErrorNote>}

      <div className="flex items-center justify-between">
        <button className="btn-quiet" disabled={idx === 0} onClick={() => go(steps[idx - 1])}>
          <Icon name="arrowLeft" /> Back
        </button>
        <div className="flex gap-2">
          {!readOnly && (
            <button className="btn-secondary" disabled={save.isPending || !form.title.trim()} onClick={() => save.mutate(form)}>
              Save draft
            </button>
          )}
          {idx < steps.length - 1 ? (
            <button className="btn-primary" onClick={() => go(steps[idx + 1])}>
              Continue <Icon name="arrowRight" />
            </button>
          ) : readOnly ? (
            <Link to={`/events/${id}`} className="btn-primary">
              View public page <Icon name="arrowRight" />
            </Link>
          ) : (
            <button className="btn-primary" disabled={issues.length > 0 || publish.isPending} onClick={() => publish.mutate()}>
              {publish.isPending ? 'Publishing…' : 'Publish event'}
            </button>
          )}
        </div>
      </div>
    </div>
  )
}

type StepProps = { form: Form; set: <K extends keyof Form>(key: K, value: Form[K]) => void }

function Section({ title, hint, children }: { title: string; hint?: string; children: ReactNode }) {
  return (
    <div className="space-y-4">
      <div>
        <h2 className="font-display text-xl">{title}</h2>
        {hint && <p className="mt-0.5 text-sm text-ink-muted">{hint}</p>}
      </div>
      {children}
    </div>
  )
}

function DetailsStep({ form, set }: StepProps) {
  return (
    <>
      <Section title="About the event">
        <div>
          <label className="label">Title</label>
          <input className="input" value={form.title} onChange={(e) => set('title', e.target.value)} placeholder="Friday Stand-up Night" />
        </div>
        <div>
          <label className="label">Category</label>
          <select className="input" value={form.category} onChange={(e) => set('category', e.target.value as Form['category'])}>
            {EVENT_CATEGORIES.map((c) => (
              <option key={c} value={c}>
                {categoryLabel[c]}
              </option>
            ))}
          </select>
          <p className="mt-1.5 text-xs text-ink-faint">A matching cover image is added automatically.</p>
        </div>
        <div>
          <label className="label">Description</label>
          <textarea className="input min-h-28" value={form.description} onChange={(e) => set('description', e.target.value)} placeholder="What should people expect?" />
        </div>
      </Section>

      <Section title="Where and when">
        <div className="grid gap-4 sm:grid-cols-3">
          <div>
            <label className="label">Venue</label>
            <input className="input" value={form.venue_name} onChange={(e) => set('venue_name', e.target.value)} />
          </div>
          <div>
            <label className="label">City</label>
            <input className="input" value={form.city} onChange={(e) => set('city', e.target.value)} />
          </div>
          <div>
            <label className="label">Address</label>
            <input className="input" value={form.address} onChange={(e) => set('address', e.target.value)} />
          </div>
        </div>
        <div>
          <label className="label">
            Location link <span className="text-ink-faint">(optional)</span>
          </label>
          <input
            className="input"
            value={form.map_url}
            onChange={(e) => set('map_url', e.target.value)}
            placeholder="https://maps.app.goo.gl/…"
          />
          <p className="mt-1.5 text-xs text-ink-faint">
            In Google Maps, open your venue → Share → Copy link. If left empty, guests get directions from the address.
          </p>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <label className="label">Starts</label>
            <input type="datetime-local" className="input" value={form.starts_at} onChange={(e) => set('starts_at', e.target.value)} />
          </div>
          <div>
            <label className="label">Ends</label>
            <input type="datetime-local" className="input" value={form.ends_at} onChange={(e) => set('ends_at', e.target.value)} />
          </div>
        </div>
      </Section>

      <Section title="Booking rules">
        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <label className="label">Max tickets per booking</label>
            <input type="number" min={1} max={50} className="input" value={form.max_per_booking} onChange={(e) => set('max_per_booking', Number(e.target.value))} />
          </div>
          <div>
            <label className="label">Free cancellation until (hours before start)</label>
            <input type="number" min={0} className="input" value={form.cancel_cutoff_hours} onChange={(e) => set('cancel_cutoff_hours', Number(e.target.value))} />
          </div>
        </div>
      </Section>
    </>
  )
}

function SeatingStep({ mode, onChange }: { mode: SeatingMode; onChange: (m: SeatingMode) => void }) {
  const options: { value: SeatingMode; title: string; body: string; icon: 'grid' | 'users' }[] = [
    { value: 'NUMBERED', title: 'Numbered seats', body: 'Films, theatre and stage shows. Guests pick exact seats on a layout you design.', icon: 'grid' },
    { value: 'GA', title: 'General admission', body: 'Comedy clubs, gigs and workshops. Guests choose a ticket type and quantity.', icon: 'users' },
  ]
  return (
    <Section title="How will people sit?" hint="This can't be changed after you publish.">
      <div className="grid gap-4 sm:grid-cols-2">
        {options.map((o) => {
          const active = mode === o.value
          return (
            <button
              key={o.value}
              type="button"
              onClick={() => onChange(o.value)}
              className={`cursor-pointer rounded-2xl p-6 text-left transition ${active ? 'bg-pine-50 ring-2 ring-pine-500' : 'bg-white ring-1 ring-line hover:ring-ink-faint/40'}`}
            >
              <span className={`grid h-10 w-10 place-items-center rounded-xl ${active ? 'bg-pine-700 text-white' : 'bg-mist text-ink-muted'}`}>
                <Icon name={o.icon} className="h-5 w-5" />
              </span>
              <div className="mt-4 font-medium">{o.title}</div>
              <p className="mt-1 text-sm text-ink-muted">{o.body}</p>
            </button>
          )
        })}
      </div>
    </Section>
  )
}

function TicketsStep({ form, set }: StepProps) {
  const ga = form.seating_mode === 'GA'
  const cats = form.categories
  const update = (i: number, patch: Partial<CatForm>) => set('categories', cats.map((c, j) => (j === i ? { ...c, ...patch } : c)))
  const add = () =>
    set('categories', [...cats, { id: crypto.randomUUID(), name: '', color: PALETTE[cats.length % PALETTE.length], price: '', capacity: ga ? '0' : '' }])

  const totalCap = cats.reduce((s, c) => s + (Number(c.capacity) || 0), 0)
  const maxRevenue = cats.reduce((s, c) => s + rupeesToPaise(c.price) * (Number(c.capacity) || 0), 0)
  const cols = ga ? 'grid-cols-[2rem_1fr_6.5rem_6.5rem_2rem]' : 'grid-cols-[2rem_1fr_7rem_2rem]'

  return (
    <Section
      title="Ticket types"
      hint={ga ? 'Set a price and how many of each you will sell.' : 'List them from most to least valuable. You will paint them onto seats next.'}
    >
      <div className="space-y-2">
        <div className={`grid gap-3 text-xs font-medium text-ink-faint ${cols}`}>
          <span />
          <span>Name</span>
          <span>Price (₹)</span>
          {ga && <span>Capacity</span>}
          <span />
        </div>
        {cats.map((c, i) => (
          <div key={c.id} className={`grid items-center gap-3 ${cols}`}>
            <label className="relative h-8 w-8 cursor-pointer overflow-hidden rounded-lg ring-1 ring-line" style={{ background: c.color }} title="Colour">
              <input type="color" value={c.color} onChange={(e) => update(i, { color: e.target.value })} className="absolute inset-0 cursor-pointer opacity-0" />
            </label>
            <input className="input" value={c.name} onChange={(e) => update(i, { name: e.target.value })} placeholder="e.g. Premium" />
            <input className="input" type="number" min={0} value={c.price} onChange={(e) => update(i, { price: e.target.value })} />
            {ga && <input className="input" type="number" min={0} value={c.capacity} onChange={(e) => update(i, { capacity: e.target.value })} />}
            <button
              type="button"
              className="cursor-pointer text-ink-faint transition hover:text-clay-500"
              onClick={() => set('categories', cats.filter((_, j) => j !== i))}
              aria-label="Remove"
            >
              <Icon name="x" />
            </button>
          </div>
        ))}
      </div>
      <button type="button" className="btn-quiet -ml-3 text-pine-700" onClick={add}>
        <Icon name="plus" /> Add ticket type
      </button>
      {ga && (
        <div className="flex justify-between rounded-xl bg-mist/60 px-4 py-3 text-sm">
          <span className="text-ink-muted">{totalCap} tickets in total</span>
          <span>
            Max revenue <strong className="font-medium">{formatPaise(maxRevenue)}</strong>
          </span>
        </div>
      )}
    </Section>
  )
}

function ReviewStep({ form, issues }: { form: Form; issues: string[] }) {
  const seatCount = form.layout?.cells.flat().filter((c) => c !== AISLE && c !== BLOCKED && c !== '').length ?? 0
  const rows: [string, string][] = [
    ['Title', form.title || '—'],
    ['Category', categoryLabel[form.category]],
    ['Venue', [form.venue_name, form.city].filter(Boolean).join(', ') || '—'],
    ['Location link', form.map_url || 'From the address'],
    ['Starts', form.starts_at ? new Date(form.starts_at).toLocaleString('en-IN', { dateStyle: 'full', timeStyle: 'short' }) : '—'],
    ['Seating', form.seating_mode === 'GA' ? 'General admission' : `Numbered · ${seatCount} seats`],
    ['Per booking', `Up to ${form.max_per_booking} tickets`],
  ]
  return (
    <Section title="Ready to publish?" hint="After publishing, seating and capacities are locked so sold tickets stay valid.">
      <dl className="divide-y divide-line rounded-xl ring-1 ring-line">
        {rows.map(([k, v]) => (
          <div key={k} className="grid grid-cols-[8rem_1fr] gap-4 px-4 py-3 text-sm">
            <dt className="text-ink-muted">{k}</dt>
            <dd className="font-medium">{v}</dd>
          </div>
        ))}
      </dl>
      <ul className="flex flex-wrap gap-2">
        {form.categories.map((c) => (
          <li key={c.id} className="flex items-center gap-2 rounded-full bg-mist px-3 py-1 text-sm">
            <span className="h-2 w-2 rounded-full" style={{ background: c.color }} />
            {c.name || 'Untitled'} · {formatPaise(rupeesToPaise(c.price))}
            {form.seating_mode === 'GA' && <span className="text-ink-faint">× {c.capacity || 0}</span>}
          </li>
        ))}
      </ul>
      {issues.length > 0 && (
        <div className="rounded-xl bg-sand-50 px-4 py-3 text-sm text-sand-700">
          <p className="font-medium">Before you can publish:</p>
          <ul className="mt-1 list-inside list-disc space-y-0.5">
            {issues.map((i) => (
              <li key={i}>{i}</li>
            ))}
          </ul>
        </div>
      )}
    </Section>
  )
}
