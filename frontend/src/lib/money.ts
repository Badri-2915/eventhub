const inr = new Intl.NumberFormat('en-IN', { style: 'currency', currency: 'INR', maximumFractionDigits: 2 })

/** Money is stored as integer paise; only format at the edges. */
export function formatPaise(paise: number): string {
  return inr.format(paise / 100).replace(/\.00$/, '')
}

export function rupeesToPaise(rupees: string): number {
  const n = Number(rupees)
  return Number.isFinite(n) ? Math.round(n * 100) : 0
}
