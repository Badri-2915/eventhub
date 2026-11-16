const inr = new Intl.NumberFormat('en-IN', { style: 'currency', currency: 'INR', maximumFractionDigits: 2 })

/** Money is stored as integer paise; only format at the edges. */
export function formatPaise(paise: number): string {
  return inr.format(paise / 100).replace(/\.00$/, '')
}

/** A ticket price as attendees see it: "Free" instead of ₹0. */
export function formatPrice(paise: number): string {
  return paise === 0 ? 'Free' : formatPaise(paise)
}

/** Mirrors the server: discounts round to whole rupees; no discount keeps paise. */
export function discountedPaise(listPaise: number, percent: number): number {
  if (!percent) return listPaise
  return Math.round((listPaise * (100 - percent)) / 10_000) * 100
}

export function rupeesToPaise(rupees: string): number {
  const n = Number(rupees)
  return Number.isFinite(n) ? Math.round(n * 100) : 0
}
