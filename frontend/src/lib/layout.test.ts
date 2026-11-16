import { describe, expect, it } from 'vitest'
import { AISLE, BLOCKED, UNASSIGNED, applyPreset, blankLayout, previewSeats, rowLabel, rowLabels, sanitizeLayout } from './layout'
import type { Layout } from './types'

describe('seat layout (must match the backend seat generator)', () => {
  it('letters rows A…Z, then AA, AB…', () => {
    expect([0, 1, 25, 26, 27, 51, 52].map(rowLabel)).toEqual(['A', 'B', 'Z', 'AA', 'AB', 'AZ', 'BA'])
  })

  it('skips aisles when numbering and keeps blocked seats unsellable', () => {
    const layout: Layout = { rows: 1, cols: 6, cells: [['p', 'p', AISLE, 'p', BLOCKED, 'p']] }
    const seats = previewSeats(layout, 'fallback')
    expect(seats.map((s) => s.label)).toEqual(['A1', 'A2', 'A3', 'A4', 'A5'])
    expect(seats.find((s) => s.label === 'A4')?.status).toBe('BLOCKED')
    expect(seats.filter((s) => s.status === 'AVAILABLE')).toHaveLength(4)
  })

  it('skips all-aisle rows when lettering', () => {
    const layout: Layout = { rows: 3, cols: 2, cells: [['p', 'p'], [AISLE, AISLE], ['p', 'p']] }
    expect(rowLabels(layout)).toEqual(['A', null, 'B'])
    expect(previewSeats(layout, 'x').map((s) => s.label)).toEqual(['A1', 'A2', 'B1', 'B2'])
  })

  it('creates an empty grid of the requested size', () => {
    const l = blankLayout(2, 3)
    expect(l.cells).toEqual([
      [UNASSIGNED, UNASSIGNED, UNASSIGNED],
      [UNASSIGNED, UNASSIGNED, UNASSIGNED],
    ])
  })

  it('stage preset puts the best tier at the front and keeps aisles', () => {
    const base: Layout = { rows: 10, cols: 2, cells: Array.from({ length: 10 }, () => ['', AISLE]) }
    const out = applyPreset(base, 'stage', ['gold', 'silver', 'bronze'])
    expect(out.cells[0][0]).toBe('gold')
    expect(out.cells[9][0]).toBe('bronze')
    expect(out.cells.every((r) => r[1] === AISLE)).toBe(true)
  })

  it('cinema preset puts the best tier at the back', () => {
    const base = blankLayout(8, 1)
    const out = applyPreset(base, 'cinema', ['recliner', 'prime', 'classic'])
    expect(out.cells[7][0]).toBe('recliner')
    expect(out.cells[0][0]).toBe('classic')
  })

  it('removes deleted ticket categories from the grid', () => {
    const l: Layout = { rows: 1, cols: 4, cells: [['keep', 'gone', AISLE, BLOCKED]] }
    expect(sanitizeLayout(l, new Set(['keep'])).cells[0]).toEqual(['keep', UNASSIGNED, AISLE, BLOCKED])
  })
})
