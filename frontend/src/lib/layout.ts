import type { Layout, Seat } from './types'

export const AISLE = 'a'
export const BLOCKED = 'x'
export const UNASSIGNED = ''

export function blankLayout(rows: number, cols: number): Layout {
  return { rows, cols, cells: Array.from({ length: rows }, () => Array<string>(cols).fill(UNASSIGNED)) }
}

export function rowLabel(i: number): string {
  return i < 26 ? String.fromCharCode(65 + i) : rowLabel(Math.floor(i / 26) - 1) + String.fromCharCode(65 + (i % 26))
}

/**
 * Same labelling rules as the backend: rows lettered from the screen side,
 * all-aisle rows skipped; seats numbered left→right, aisles skipped.
 * Returns pseudo-seats so the designer can preview exactly what customers see.
 */
export function previewSeats(layout: Layout, fallbackCategory: string): Seat[] {
  const seats: Seat[] = []
  let letter = 0
  layout.cells.forEach((row, r) => {
    if (row.every((c) => c === AISLE)) return
    const label = rowLabel(letter++)
    let n = 0
    row.forEach((cell, c) => {
      if (cell === AISLE) return
      n++
      seats.push({
        id: `${r}:${c}`,
        category_id: cell === BLOCKED || cell === UNASSIGNED ? fallbackCategory : cell,
        row_label: label,
        seat_number: n,
        label: `${label}${n}`,
        grid_row: r,
        grid_col: c,
        status: cell === BLOCKED ? 'BLOCKED' : 'AVAILABLE',
      })
    })
  })
  return seats
}

/** Rows label for the designer's left gutter (null for all-aisle rows). */
export function rowLabels(layout: Layout): (string | null)[] {
  let letter = 0
  return layout.cells.map((row) => (row.every((c) => c === AISLE) ? null : rowLabel(letter++)))
}

/** Presets paint seat rows by tier; categories are ordered most → least valuable. */
export function applyPreset(layout: Layout, preset: 'stage' | 'cinema', categoryIds: string[]): Layout {
  if (categoryIds.length === 0) return layout
  const n = layout.rows
  const tierFor = (r: number) => {
    if (preset === 'stage') {
      if (r < Math.max(1, Math.round(n * 0.2))) return 0
      if (r < Math.max(2, Math.round(n * 0.5))) return 1
      return 2
    }
    if (r >= n - 2) return 0
    if (r < 3) return 2
    return 1
  }
  return {
    ...layout,
    cells: layout.cells.map((row, r) =>
      row.map((cell) => (cell === AISLE ? cell : categoryIds[Math.min(tierFor(r), categoryIds.length - 1)])),
    ),
  }
}

/** Drop references to categories that no longer exist. */
export function sanitizeLayout(layout: Layout, categoryIds: Set<string>): Layout {
  return {
    ...layout,
    cells: layout.cells.map((row) =>
      row.map((c) => (c === AISLE || c === BLOCKED || c === UNASSIGNED || categoryIds.has(c) ? c : UNASSIGNED)),
    ),
  }
}

export function isLayout(v: unknown): v is Layout {
  return !!v && typeof v === 'object' && Array.isArray((v as Layout).cells)
}
