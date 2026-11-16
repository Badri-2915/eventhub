import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { PublicCategory } from '../../lib/types'
import { GaPicker } from './GaPicker'

const cat = (over: Partial<PublicCategory>): PublicCategory => ({
  id: 'c1', name: 'Silver', color: '#999', price_paise: 50000, list_price_paise: 50000,
  discount_percent: 0, total: 100, available: 100, ...over,
})

describe('GaPicker (open-seating ticket stepper)', () => {
  it('adds and removes tickets', async () => {
    const onChange = vi.fn()
    render(<GaPicker categories={[cat({})]} qty={{ c1: 2 }} onChange={onChange} maxTotal={10} />)
    await userEvent.click(screen.getByRole('button', { name: 'Add one Silver' }))
    expect(onChange).toHaveBeenLastCalledWith('c1', 3)
    await userEvent.click(screen.getByRole('button', { name: 'Remove one Silver' }))
    expect(onChange).toHaveBeenLastCalledWith('c1', 1)
  })

  it('cannot go below zero', () => {
    render(<GaPicker categories={[cat({})]} qty={{}} onChange={() => {}} maxTotal={10} />)
    expect(screen.getByRole('button', { name: 'Remove one Silver' })).toBeDisabled()
  })

  it('stops at the per-booking limit across categories', () => {
    const cats = [cat({ id: 'a', name: 'Gold' }), cat({ id: 'b', name: 'Silver' })]
    render(<GaPicker categories={cats} qty={{ a: 3, b: 1 }} onChange={() => {}} maxTotal={4} />)
    expect(screen.getByRole('button', { name: 'Add one Gold' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Add one Silver' })).toBeDisabled()
  })

  it('stops at what is left and shows sold out', () => {
    const cats = [cat({ id: 'a', name: 'Gold', available: 2 }), cat({ id: 'b', name: 'Balcony', available: 0 })]
    render(<GaPicker categories={cats} qty={{ a: 2 }} onChange={() => {}} maxTotal={10} />)
    expect(screen.getByRole('button', { name: 'Add one Gold' })).toBeDisabled()
    expect(screen.getByText('Sold out')).toBeInTheDocument()
    expect(screen.getByText('2 available')).toBeInTheDocument()
  })

  it('shows the discounted price with the original struck through', () => {
    render(<GaPicker categories={[cat({ price_paise: 40000, list_price_paise: 50000, discount_percent: 20 })]} qty={{}} onChange={() => {}} maxTotal={4} />)
    expect(screen.getByText('₹400')).toBeInTheDocument()
    expect(screen.getByText('₹500').tagName).toBe('S')
    expect(screen.getByText('20% off')).toBeInTheDocument()
  })

  it('freezes while a hold is in progress', () => {
    render(<GaPicker categories={[cat({})]} qty={{ c1: 1 }} onChange={() => {}} maxTotal={4} disabled />)
    expect(screen.getByRole('button', { name: 'Add one Silver' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Remove one Silver' })).toBeDisabled()
  })
})
