import { describe, expect, it } from 'vitest'
import { categoryLabel, directionsUrl, formatWhen, plural } from './format'

describe('format', () => {
  it('uses the host’s own map link when there is one', () => {
    const ev = { map_url: 'https://maps.app.goo.gl/abc', venue_name: 'Hall', address: 'Main Rd', city: 'Chennai' }
    expect(directionsUrl(ev)).toBe('https://maps.app.goo.gl/abc')
  })

  it('falls back to a safely encoded Maps search', () => {
    const url = directionsUrl({ venue_name: 'Rock & Roll Club', address: '', city: 'Pune' })
    expect(url).toBe('https://www.google.com/maps/search/?api=1&query=Rock%20%26%20Roll%20Club%2C%20Pune')
  })

  it('pluralises', () => {
    expect(plural(1, 'ticket')).toBe('1 ticket')
    expect(plural(3, 'ticket')).toBe('3 tickets')
    expect(plural(2, 'person', 'people')).toBe('2 people')
  })

  it('handles a missing date', () => {
    expect(formatWhen(null)).toBe('Date to be announced')
    expect(formatWhen('2026-11-22T14:00:00Z')).toContain('22 Nov')
  })

  it('has a label for every event category', () => {
    for (const c of ['MOVIE', 'COMEDY', 'CONCERT', 'THEATRE', 'WORKSHOP', 'OTHER']) expect(categoryLabel[c]).toBeTruthy()
  })
})
