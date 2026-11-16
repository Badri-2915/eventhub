import { formatPaise } from '../../lib/money'
import type { PublicCategory } from '../../lib/types'
import { Icon } from '../ui/Icon'

type Props = {
  categories: PublicCategory[]
  qty: Record<string, number>
  onChange: (categoryId: string, qty: number) => void
  maxTotal: number
  disabled?: boolean
}

/** General-admission picker: one row per category with a − n + stepper. */
export function GaPicker({ categories, qty, onChange, maxTotal, disabled }: Props) {
  const total = Object.values(qty).reduce((a, b) => a + b, 0)
  return (
    <div className="divide-y divide-line">
      {categories.map((c) => {
        const n = qty[c.id] ?? 0
        const soldOut = c.available <= 0
        const canAdd = !disabled && n < c.available && total < maxTotal
        return (
          <div key={c.id} className="flex items-center gap-4 py-4 first:pt-0 last:pb-0">
            <span className="h-10 w-1 rounded-full" style={{ background: c.color }} />
            <div className="min-w-0 flex-1">
              <div className="font-medium">{c.name}</div>
              <div className="mt-0.5 text-sm text-ink-muted">
                {formatPaise(c.price_paise)}
                <span className="mx-1.5 text-line">|</span>
                {soldOut ? (
                  <span className="text-clay-500">Sold out</span>
                ) : (
                  <span className={c.available <= 10 ? 'text-sand-700' : ''}>{c.available} available</span>
                )}
              </div>
            </div>
            <div className="flex items-center rounded-full border border-line bg-white p-1">
              <button
                type="button"
                aria-label={`Remove one ${c.name}`}
                className="grid h-8 w-8 cursor-pointer place-items-center rounded-full text-ink-muted transition hover:bg-mist disabled:cursor-not-allowed disabled:opacity-30"
                disabled={disabled || n === 0}
                onClick={() => onChange(c.id, n - 1)}
              >
                <Icon name="minus" />
              </button>
              <span className="w-8 text-center text-sm font-semibold tabular-nums">{n}</span>
              <button
                type="button"
                aria-label={`Add one ${c.name}`}
                className="grid h-8 w-8 cursor-pointer place-items-center rounded-full text-pine-700 transition hover:bg-pine-50 disabled:cursor-not-allowed disabled:opacity-30"
                disabled={!canAdd}
                onClick={() => onChange(c.id, n + 1)}
              >
                <Icon name="plus" />
              </button>
            </div>
          </div>
        )
      })}
    </div>
  )
}
