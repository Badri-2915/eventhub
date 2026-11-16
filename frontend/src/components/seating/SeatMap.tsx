import { useMemo, type CSSProperties } from 'react'
import { formatPaise } from '../../lib/money'
import type { Seat } from '../../lib/types'

export type SeatCategory = { id: string; name: string; color: string; price_paise: number }

type Props = {
  grid: { rows: number; cols: number }
  seats: Seat[]
  categories: SeatCategory[]
  selected?: Set<string>
  onToggle?: (seat: Seat) => void
  stageLabel?: string
  /** Host stats view: colour every seat by status, no interaction. */
  readOnly?: boolean
}

const statusText: Record<Seat['status'], string> = {
  AVAILABLE: 'Available',
  HELD: 'Being booked by someone',
  BOOKED: 'Booked',
  BLOCKED: 'Not for sale',
}

/** Shared by the customer event page, the layout designer preview and host views. */
export function SeatMap({ grid, seats, categories, selected, onToggle, stageLabel = 'Stage', readOnly }: Props) {
  const cats = useMemo(() => new Map(categories.map((c) => [c.id, c])), [categories])
  const byCell = useMemo(() => new Map(seats.map((s) => [`${s.grid_row}:${s.grid_col}`, s])), [seats])
  const rowLabel = useMemo(() => {
    const m = new Map<number, string>()
    seats.forEach((s) => m.set(s.grid_row, s.row_label))
    return m
  }, [seats])

  const size = grid.cols > 32 ? 18 : grid.cols > 20 ? 22 : grid.cols > 14 ? 24 : 26
  const gap = size > 20 ? 5 : 3

  return (
    <div className="overflow-x-auto pb-2">
      <div className="mx-auto w-max px-2">
        {/* Stage / screen, always at the top: row A is nearest to it. */}
        <div className="mx-auto mb-8" style={{ width: Math.max(grid.cols * (size + gap) * 0.8, 160) }}>
          <svg viewBox="0 0 200 14" className="w-full" preserveAspectRatio="none" aria-hidden>
            <path d="M2 12 Q100 -4 198 12" fill="none" stroke="#b3d3c2" strokeWidth="3" strokeLinecap="round" />
          </svg>
          <p className="mt-1 text-center text-[11px] font-medium uppercase tracking-[0.3em] text-ink-faint">{stageLabel}</p>
        </div>

        <div
          className="grid items-center"
          style={{ gridTemplateColumns: `1.75rem repeat(${grid.cols}, ${size}px) 1.75rem`, columnGap: gap, rowGap: gap }}
        >
          {Array.from({ length: grid.rows }, (_, r) => {
            const label = rowLabel.get(r)
            if (!label) return <div key={r} style={{ gridColumn: `1 / span ${grid.cols + 2}`, height: size / 2 }} />
            return [
              <span key={`l${r}`} className="text-center text-[11px] font-medium text-ink-faint">
                {label}
              </span>,
              ...Array.from({ length: grid.cols }, (_, c) => {
                const seat = byCell.get(`${r}:${c}`)
                if (!seat) return <span key={c} style={{ width: size, height: size }} />
                return (
                  <SeatButton
                    key={c}
                    seat={seat}
                    size={size}
                    color={cats.get(seat.category_id)?.color ?? '#8b918c'}
                    category={cats.get(seat.category_id)}
                    selected={selected?.has(seat.id) ?? false}
                    onToggle={readOnly ? undefined : onToggle}
                  />
                )
              }),
              <span key={`r${r}`} className="text-center text-[11px] font-medium text-ink-faint">
                {label}
              </span>,
            ]
          })}
        </div>
      </div>
    </div>
  )
}

function SeatButton({
  seat,
  size,
  color,
  category,
  selected,
  onToggle,
}: {
  seat: Seat
  size: number
  color: string
  category?: SeatCategory
  selected: boolean
  onToggle?: (s: Seat) => void
}) {
  const free = seat.status === 'AVAILABLE'
  const title = `${seat.label}${category ? ` · ${category.name} · ${formatPaise(category.price_paise)}` : ''} — ${selected ? 'Selected' : statusText[seat.status]}`
  const base = 'grid place-items-center rounded-t-[7px] rounded-b-[4px] text-[9px] font-semibold transition-all duration-150'

  if (seat.status === 'BLOCKED') {
    return (
      <span title={title} className={`${base} text-ink-faint/60`} style={{ width: size, height: size }}>
        ✕
      </span>
    )
  }

  let cls = ''
  let style: CSSProperties = { width: size, height: size }
  if (selected) {
    cls = 'bg-pine-700 text-white shadow-sm scale-105'
  } else if (seat.status === 'HELD') {
    cls = 'bg-sand-100 ring-1 ring-inset ring-sand-300'
  } else if (seat.status === 'BOOKED') {
    cls = 'bg-line/70'
  } else {
    // Category colour drives the ring and a soft hover tint, all in CSS.
    cls = 'bg-white ring-[1.5px] ring-inset text-transparent hover:-translate-y-px hover:bg-(--seat-tint) hover:text-ink-muted'
    style = { ...style, ['--tw-ring-color' as string]: color, ['--seat-tint' as string]: color + '26' }
  }

  const interactive = !!onToggle && (free || selected)
  return (
    <button
      type="button"
      title={title}
      aria-label={title}
      aria-pressed={selected}
      disabled={!interactive}
      onClick={() => onToggle?.(seat)}
      className={`${base} ${cls} ${interactive ? 'cursor-pointer' : 'cursor-default'}`}
      style={style}
    >
      {selected ? seat.seat_number : free ? seat.seat_number : ''}
    </button>
  )
}

export function SeatLegend({ categories }: { categories: SeatCategory[] }) {
  const swatch = 'h-4 w-4 rounded-t-[5px] rounded-b-[3px]'
  return (
    <div className="flex flex-wrap items-center gap-x-5 gap-y-2 text-xs text-ink-muted">
      {categories.map((c) => (
        <span key={c.id} className="flex items-center gap-1.5">
          <span className={`${swatch} bg-white ring-[1.5px] ring-inset`} style={{ ['--tw-ring-color' as string]: c.color }} />
          {c.name} · {formatPaise(c.price_paise)}
        </span>
      ))}
      <span className="h-4 w-px bg-line" />
      <span className="flex items-center gap-1.5">
        <span className={`${swatch} bg-pine-700`} /> Selected
      </span>
      <span className="flex items-center gap-1.5">
        <span className={`${swatch} bg-sand-100 ring-1 ring-inset ring-sand-300`} /> On hold
      </span>
      <span className="flex items-center gap-1.5">
        <span className={`${swatch} bg-line/70`} /> Booked
      </span>
    </div>
  )
}
