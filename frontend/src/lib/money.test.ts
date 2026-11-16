import { describe, expect, it } from 'vitest'
import { discountedPaise, formatPaise, formatPrice, rupeesToPaise } from './money'

describe('money', () => {
  it('formats paise as rupees and drops a trailing .00', () => {
    expect(formatPaise(50000)).toBe('₹500')
    expect(formatPaise(39920)).toBe('₹399.20')
    expect(formatPaise(12345600)).toBe('₹1,23,456') // Indian digit grouping
  })

  it('shows Free for a zero price', () => {
    expect(formatPrice(0)).toBe('Free')
    expect(formatPrice(29900)).toBe('₹299')
  })

  // Must match the server's rounding exactly (backend events.discounted),
  // or the checkout total would differ from the price shown on the page.
  it.each([
    [49900, 20, 39900], // ₹499 − 20% = ₹399.20 → ₹399
    [99900, 15, 84900], // ₹999 − 15% = ₹849.15 → ₹849
    [50000, 0, 50000], // no discount keeps the exact price
    [29950, 0, 29950], // …including paise
    [10000, 90, 1000], // the largest allowed discount
    [999, 50, 500], // ₹9.99 − 50% = ₹4.995 → ₹5
  ])('discountedPaise(%i, %i%%) = %i', (list, pct, want) => {
    expect(discountedPaise(list, pct)).toBe(want)
  })

  it('converts typed rupees to paise safely', () => {
    expect(rupeesToPaise('499')).toBe(49900)
    expect(rupeesToPaise('12.5')).toBe(1250)
    expect(rupeesToPaise('0.1')).toBe(10) // no 9.999… float drift
    expect(rupeesToPaise('abc')).toBe(0)
    expect(rupeesToPaise('')).toBe(0)
  })
})
