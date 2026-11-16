import { useEffect, useMemo, useRef, useState, type PointerEvent, type ReactNode } from 'react'
import { AISLE, BLOCKED, UNASSIGNED, applyPreset, blankLayout, previewSeats, rowLabels } from '../../lib/layout'
import { formatPaise } from '../../lib/money'
import type { Layout } from '../../lib/types'
import { SeatMap } from '../seating/SeatMap'
import { Icon } from '../ui/Icon'

export type DesignerCategory = { id: string; name: string; color: string; price_paise: number }

type Props = {
  layout: Layout | null
  categories: DesignerCategory[]
  onChange: (layout: Layout) => void
  stageLabel: string
  readOnly?: boolean
}

const MAX = 50

/**
 * Grid editor for numbered seating. The screen/stage is drawn at the top;
 * row A is nearest to it. Click or drag to paint, click a row letter or
 * column number to paint the whole line.
 */
export function LayoutDesigner({ layout, categories, onChange, stageLabel, readOnly }: Props) {
  const [rows, setRows] = useState(layout?.rows ?? 10)
  const [cols, setCols] = useState(layout?.cols ?? 16)
  const [tool, setTool] = useState<string>(categories[0]?.id ?? UNASSIGNED)
  const [preview, setPreview] = useState(false)
  const painting = useRef(false)

  useEffect(() => {
    const stop = () => (painting.current = false)
    window.addEventListener('pointerup', stop)
    return () => window.removeEventListener('pointerup', stop)
  }, [])

  // Keep the active tool valid if categories change.
  useEffect(() => {
    if (tool !== AISLE && tool !== BLOCKED && tool !== UNASSIGNED && !categories.some((c) => c.id === tool)) {
      setTool(categories[0]?.id ?? UNASSIGNED)
    }
  }, [categories, tool])

  const catMap = useMemo(() => new Map(categories.map((c) => [c.id, c])), [categories])

  const summary = useMemo(() => {
    const counts = new Map<string, number>()
    let blocked = 0
    let unassigned = 0
    layout?.cells.forEach((row) =>
      row.forEach((c) => {
        if (c === AISLE) return
        if (c === BLOCKED) blocked++
        else if (c === UNASSIGNED || !catMap.has(c)) unassigned++
        else counts.set(c, (counts.get(c) ?? 0) + 1)
      }),
    )
    const sellable = [...counts.values()].reduce((a, b) => a + b, 0)
    const revenue = [...counts].reduce((a, [id, n]) => a + n * (catMap.get(id)?.price_paise ?? 0), 0)
    return { counts, blocked, unassigned, sellable, revenue }
  }, [layout, catMap])

  const create = () => {
    const r = clamp(rows)
    const c = clamp(cols)
    const next = blankLayout(r, c)
    // Keep what was already painted where it still fits.
    layout?.cells.forEach((row, i) => row.forEach((cell, j) => i < r && j < c && (next.cells[i][j] = cell)))
    onChange(next)
  }

  const paint = (cells: [number, number][]) => {
    if (!layout || readOnly) return
    const next = layout.cells.map((row) => [...row])
    let changed = false
    for (const [r, c] of cells) {
      if (next[r][c] !== tool) {
        next[r][c] = tool
        changed = true
      }
    }
    if (changed) onChange({ ...layout, cells: next })
  }

  if (!layout) {
    return (
      <div className="rounded-2xl border border-dashed border-line bg-mist/40 px-6 py-12 text-center">
        <span className="mx-auto grid h-12 w-12 place-items-center rounded-full bg-white text-pine-700 shadow-soft">
          <Icon name="grid" className="h-5 w-5" />
        </span>
        <h3 className="mt-4 font-display text-xl">Design your hall</h3>
        <p className="mx-auto mt-1 max-w-sm text-sm text-ink-muted">Start with the number of rows and seats per row. You can resize later.</p>
        <SizeInputs rows={rows} cols={cols} setRows={setRows} setCols={setCols} />
        <button className="btn-primary mt-5" onClick={create} disabled={readOnly}>
          Create grid
        </button>
      </div>
    )
  }

  const labels = rowLabels(layout)
  const size = layout.cols > 32 ? 16 : layout.cols > 22 ? 20 : 24

  return (
    <div className="space-y-5">
      {!readOnly && (
        <div className="flex flex-wrap items-end justify-between gap-4">
          <div className="flex items-end gap-3">
            <SizeInputs rows={rows} cols={cols} setRows={setRows} setCols={setCols} compact />
            <button className="btn-secondary" onClick={create}>
              Resize
            </button>
          </div>
          <div className="flex flex-wrap gap-2">
            <span className="self-center text-xs text-ink-faint">Presets</span>
            <button className="btn-secondary py-2 text-xs" onClick={() => onChange(applyPreset(layout, 'stage', categories.map((c) => c.id)))}>
              Stage show
            </button>
            <button className="btn-secondary py-2 text-xs" onClick={() => onChange(applyPreset(layout, 'cinema', categories.map((c) => c.id)))}>
              Cinema
            </button>
            <button className="btn-quiet py-2 text-xs" onClick={() => onChange(blankLayout(layout.rows, layout.cols))}>
              Clear
            </button>
          </div>
        </div>
      )}

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_260px]">
        <div className="min-w-0 rounded-2xl border border-line bg-white">
          <div className="flex flex-wrap items-center gap-2 border-b border-line px-4 py-3">
            {!readOnly && !preview && (
              <>
                {categories.map((c) => (
                  <ToolButton key={c.id} active={tool === c.id} onClick={() => setTool(c.id)}>
                    <span className="h-3 w-3 rounded-[3px]" style={{ background: c.color }} />
                    {c.name || 'Untitled'}
                  </ToolButton>
                ))}
                <span className="mx-1 h-5 w-px bg-line" />
                <ToolButton active={tool === AISLE} onClick={() => setTool(AISLE)}>
                  <span className="h-3 w-3 rounded-[3px] border border-dashed border-ink-faint" /> Aisle
                </ToolButton>
                <ToolButton active={tool === BLOCKED} onClick={() => setTool(BLOCKED)}>
                  <span className="text-[10px] text-ink-faint">✕</span> Blocked
                </ToolButton>
                <ToolButton active={tool === UNASSIGNED} onClick={() => setTool(UNASSIGNED)}>
                  <Icon name="x" className="h-3 w-3" /> Eraser
                </ToolButton>
              </>
            )}
            <button className="btn-quiet ml-auto py-1.5 text-xs" onClick={() => setPreview((p) => !p)}>
              <Icon name={preview ? 'edit' : 'eye'} className="h-3.5 w-3.5" />
              {preview ? 'Back to editing' : 'Preview as customer'}
            </button>
          </div>

          <div className="overflow-x-auto p-5">
            {preview ? (
              <SeatMap
                grid={{ rows: layout.rows, cols: layout.cols }}
                seats={previewSeats(layout, categories[0]?.id ?? '')}
                categories={categories}
                stageLabel={stageLabel}
              />
            ) : (
              <div className="mx-auto w-max select-none touch-none">
                <div className="mb-5 rounded-full bg-pine-100 py-1 text-center text-[10px] font-medium uppercase tracking-[0.3em] text-pine-800">
                  {stageLabel}
                </div>
                <div className="grid gap-[3px]" style={{ gridTemplateColumns: `1.5rem repeat(${layout.cols}, ${size}px)` }}>
                  <span />
                  {Array.from({ length: layout.cols }, (_, c) => (
                    <button
                      key={c}
                      title="Paint this column"
                      className="cursor-pointer text-center text-[9px] text-ink-faint hover:text-pine-700"
                      onClick={() => paint(layout.cells.map((_, r) => [r, c] as [number, number]))}
                    >
                      {c + 1}
                    </button>
                  ))}
                  {layout.cells.map((row, r) => [
                    <button
                      key={`l${r}`}
                      title="Paint this row"
                      className="cursor-pointer text-[11px] font-medium text-ink-faint hover:text-pine-700"
                      onClick={() => paint(row.map((_, c) => [r, c] as [number, number]))}
                    >
                      {labels[r] ?? '·'}
                    </button>,
                    ...row.map((cell, c) => (
                      <Cell
                        key={c}
                        cell={cell}
                        size={size}
                        color={catMap.get(cell)?.color}
                        onPointerDown={() => {
                          painting.current = true
                          paint([[r, c]])
                        }}
                        onPointerEnter={() => painting.current && paint([[r, c]])}
                      />
                    )),
                  ])}
                </div>
              </div>
            )}
          </div>
        </div>

        <aside className="space-y-3 rounded-2xl border border-line bg-white p-5 text-sm">
          <h4 className="font-medium">Summary</h4>
          <ul className="space-y-2">
            {categories.map((c) => (
              <li key={c.id} className="flex items-center gap-2">
                <span className="h-2.5 w-2.5 rounded-full" style={{ background: c.color }} />
                <span className="flex-1 truncate">{c.name || 'Untitled'}</span>
                <span className="tabular-nums text-ink-muted">
                  {summary.counts.get(c.id) ?? 0} × {formatPaise(c.price_paise)}
                </span>
              </li>
            ))}
          </ul>
          <div className="space-y-1.5 border-t border-line pt-3 text-ink-muted">
            <Row label="Blocked" value={summary.blocked} />
            <Row label="Sellable seats" value={summary.sellable} strong />
            <Row label="Max revenue" value={formatPaise(summary.revenue)} strong />
          </div>
          {summary.unassigned > 0 && (
            <p className="rounded-lg bg-sand-50 px-3 py-2 text-xs text-sand-700">
              {summary.unassigned} seat{summary.unassigned === 1 ? ' has' : 's have'} no category yet. Paint them or mark them as aisle.
            </p>
          )}
          {!readOnly && <p className="text-xs text-ink-faint">Tip: drag across seats to paint quickly. Click a row letter or column number to paint the whole line.</p>}
        </aside>
      </div>
    </div>
  )
}

function Cell({ cell, size, color, onPointerDown, onPointerEnter }: { cell: string; size: number; color?: string; onPointerDown: () => void; onPointerEnter: () => void }) {
  const base = 'rounded-t-[6px] rounded-b-[3px] cursor-crosshair transition-colors'
  const style = { width: size, height: size }
  const handlers = {
    onPointerDown: (e: PointerEvent) => {
      e.preventDefault()
      onPointerDown()
    },
    onPointerEnter,
  }
  if (cell === AISLE) return <div {...handlers} className={`${base} hover:bg-mist`} style={style} />
  if (cell === BLOCKED)
    return (
      <div {...handlers} className={`${base} grid place-items-center bg-mist text-[9px] text-ink-faint`} style={style}>
        ✕
      </div>
    )
  if (!color) return <div {...handlers} className={`${base} border border-dashed border-ink-faint/50 bg-white`} style={style} />
  return <div {...handlers} className={base} style={{ ...style, background: color + '30', boxShadow: `inset 0 0 0 1.5px ${color}` }} />
}

function ToolButton({ active, onClick, children }: { active: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`flex cursor-pointer items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs transition ${
        active ? 'bg-ink text-white' : 'text-ink-muted ring-1 ring-line hover:text-ink'
      }`}
    >
      {children}
    </button>
  )
}

function Row({ label, value, strong }: { label: string; value: string | number; strong?: boolean }) {
  return (
    <div className="flex justify-between">
      <span>{label}</span>
      <span className={`tabular-nums ${strong ? 'font-medium text-ink' : ''}`}>{value}</span>
    </div>
  )
}

function SizeInputs({ rows, cols, setRows, setCols, compact }: { rows: number; cols: number; setRows: (n: number) => void; setCols: (n: number) => void; compact?: boolean }) {
  return (
    <div className={`flex items-end gap-2 ${compact ? '' : 'mx-auto mt-6 w-max'}`}>
      <div>
        <label className="label text-left">Rows</label>
        <input type="number" min={1} max={MAX} className="input w-20" value={rows} onChange={(e) => setRows(Number(e.target.value))} />
      </div>
      <span className="pb-3 text-ink-faint">×</span>
      <div>
        <label className="label text-left">Seats per row</label>
        <input type="number" min={1} max={MAX} className="input w-24" value={cols} onChange={(e) => setCols(Number(e.target.value))} />
      </div>
    </div>
  )
}

function clamp(n: number) {
  return Math.min(MAX, Math.max(1, Math.floor(n) || 1))
}
