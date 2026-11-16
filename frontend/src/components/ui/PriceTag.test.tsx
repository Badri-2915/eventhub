import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { PriceTag } from './PriceTag'

describe('PriceTag', () => {
  it('shows Free for zero', () => {
    render(<PriceTag price={0} />)
    expect(screen.getByText('Free')).toBeInTheDocument()
  })

  it('shows a plain price without a discount', () => {
    const { container } = render(<PriceTag price={49900} />)
    expect(container).toHaveTextContent('₹499')
    expect(container.querySelector('s')).toBeNull()
  })

  it('shows the old price struck through and the % off', () => {
    render(<PriceTag price={39900} list={49900} discount={20} />)
    expect(screen.getByText('₹399')).toBeInTheDocument()
    expect(screen.getByText('₹499').tagName).toBe('S')
    expect(screen.getByText('20% off')).toBeInTheDocument()
  })

  it('hides a discount that does not lower the price', () => {
    render(<PriceTag price={0} list={0} discount={50} />)
    expect(screen.queryByText('50% off')).toBeNull()
  })
})
