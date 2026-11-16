import { formatPaise, formatPrice } from '../../lib/money'

type Props = { price: number; list?: number; discount?: number; className?: string }

/** "Free", "₹499", or "₹400 ₹500 20% off" for a discounted ticket. */
export function PriceTag({ price, list = price, discount = 0, className = '' }: Props) {
  return (
    <span className={`inline-flex flex-wrap items-baseline gap-x-1.5 ${className}`}>
      <span className={price === 0 ? 'font-medium text-pine-700' : ''}>{formatPrice(price)}</span>
      {discount > 0 && list > price && (
        <>
          <s className="text-ink-faint">{formatPaise(list)}</s>
          <span className="text-xs font-medium text-pine-700">{discount}% off</span>
        </>
      )}
    </span>
  )
}
