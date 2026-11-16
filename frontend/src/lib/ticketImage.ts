import { ticketQrUrl } from '../api/endpoints'
import { categoryLabel, formatLongDate, formatTime } from './format'
import { formatPrice } from './money'
import type { Booking } from './types'

// Ticket drawn on a canvas and saved as a PNG: easy to keep in a phone's
// gallery and show at the door, even offline. Colours match the site.
const W = 1080
const PAD = 80
const C = {
  paper: '#f7f6f2',
  card: '#ffffff',
  pine: '#1c4236',
  pineLight: '#b3d3c2',
  ink: '#1f2421',
  muted: '#5c635e',
  faint: '#8b918c',
  line: '#e4e1d9',
}

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.onload = () => resolve(img)
    img.onerror = () => reject(new Error('Could not load the QR code'))
    img.src = src
  })
}

/** Draws text wrapped to maxWidth; returns the y after the last line. */
function wrap(ctx: CanvasRenderingContext2D, text: string, x: number, y: number, maxWidth: number, lineHeight: number, maxLines = 3): number {
  const words = text.split(/\s+/)
  let line = ''
  let lines = 0
  for (let i = 0; i < words.length; i++) {
    const test = line ? `${line} ${words[i]}` : words[i]
    if (ctx.measureText(test).width > maxWidth && line) {
      if (lines === maxLines - 1) {
        ctx.fillText(line + '…', x, y)
        return y + lineHeight
      }
      ctx.fillText(line, x, y)
      y += lineHeight
      lines++
      line = words[i]
    } else {
      line = test
    }
  }
  if (line) ctx.fillText(line, x, y)
  return y + lineHeight
}

function label(ctx: CanvasRenderingContext2D, text: string, x: number, y: number) {
  ctx.font = '600 24px Inter, sans-serif'
  ctx.fillStyle = C.faint
  ctx.fillText(text.toUpperCase().split('').join(String.fromCharCode(8202)), x, y)
}

export async function downloadTicket(b: Booking, holderName: string) {
  if (!b.ticket_code) throw new Error('This booking has no ticket yet')
  await document.fonts.ready
  const qr = await loadImage(ticketQrUrl(b.ticket_code))

  // Draw the content on a tall transparent layer first, then size the card to fit it.
  const layer = document.createElement('canvas')
  layer.width = W
  layer.height = 2600
  const ctx = layer.getContext('2d')!
  const cardX = 40
  const cardW = W - 80

  ctx.fillStyle = C.pineLight
  ctx.font = '600 26px Inter, sans-serif'
  ctx.fillText(`EVENTHUB  ·  ${categoryLabel[b.event.category].toUpperCase()}`, PAD, 130)
  ctx.fillStyle = '#ffffff'
  ctx.font = '500 64px Fraunces, Georgia, serif'
  const bandBottom = wrap(ctx, b.event.title, PAD, 215, W - PAD * 2, 76, 2) + 10

  // details
  let y = bandBottom + 110
  label(ctx, 'When', PAD, y)
  ctx.fillStyle = C.ink
  ctx.font = '500 36px Inter, sans-serif'
  ctx.fillText(`${formatLongDate(b.event.starts_at)}  ·  ${formatTime(b.event.starts_at)}`, PAD, y + 50)

  y += 130
  label(ctx, 'Venue', PAD, y)
  ctx.fillStyle = C.ink
  ctx.font = '500 36px Inter, sans-serif'
  ctx.fillText(b.event.venue_name, PAD, y + 50)
  ctx.fillStyle = C.muted
  ctx.font = '400 30px Inter, sans-serif'
  y = wrap(ctx, [b.event.address, b.event.city].filter(Boolean).join(', '), PAD, y + 95, W - PAD * 2, 40, 2)

  y += 40
  const seats = b.items.some((i) => i.seat_label)
  label(ctx, seats ? 'Seats' : 'Tickets', PAD, y)
  ctx.fillStyle = C.ink
  ctx.font = '500 34px Inter, sans-serif'
  const items = b.items.map((i) => (i.seat_label ? `${i.seat_label} (${i.category_name})` : `${i.quantity} × ${i.category_name}`)).join(',  ')
  y = wrap(ctx, items, PAD, y + 50, W - PAD * 2, 46, 3)

  y += 30
  label(ctx, 'Name', PAD, y)
  label(ctx, b.total_paise === 0 ? 'Price' : 'Paid', W / 2 + 40, y)
  ctx.fillStyle = C.ink
  ctx.font = '500 34px Inter, sans-serif'
  ctx.fillText(holderName, PAD, y + 50)
  ctx.fillText(formatPrice(b.total_paise), W / 2 + 40, y + 50)

  // perforation
  y += 110
  ctx.strokeStyle = C.line
  ctx.lineWidth = 4
  ctx.setLineDash([16, 14])
  ctx.beginPath()
  ctx.moveTo(PAD, y)
  ctx.lineTo(W - PAD, y)
  ctx.stroke()
  ctx.setLineDash([])
  for (const cx of [cardX, cardX + cardW]) {
    ctx.fillStyle = C.paper
    ctx.beginPath()
    ctx.arc(cx, y, 30, 0, Math.PI * 2)
    ctx.fill()
  }

  // QR + code
  const qrSize = 420
  const qrY = y + 60
  ctx.drawImage(qr, (W - qrSize) / 2, qrY, qrSize, qrSize)
  ctx.textAlign = 'center'
  ctx.fillStyle = C.ink
  ctx.font = '600 48px Menlo, ui-monospace, monospace'
  ctx.fillText(b.ticket_code, W / 2, qrY + qrSize + 70)
  ctx.fillStyle = C.faint
  ctx.font = '400 28px Inter, sans-serif'
  ctx.fillText('Show this QR code at the entrance', W / 2, qrY + qrSize + 120)

  const bottom = qrY + qrSize + 190
  const canvas = document.createElement('canvas')
  canvas.width = W
  canvas.height = bottom + 40
  const out = canvas.getContext('2d')!
  out.fillStyle = C.paper
  out.fillRect(0, 0, W, canvas.height)
  out.fillStyle = C.card
  out.beginPath()
  out.roundRect(cardX, 40, cardW, bottom - 40, 40)
  out.fill()
  out.save()
  out.beginPath()
  out.roundRect(cardX, 40, cardW, bandBottom - 40, [40, 40, 0, 0])
  out.clip()
  out.fillStyle = C.pine
  out.fillRect(cardX, 40, cardW, bandBottom - 40)
  out.restore()
  out.drawImage(layer, 0, 0)

  const blob = await new Promise<Blob | null>((r) => canvas.toBlob(r, 'image/png'))
  if (!blob) throw new Error('Could not create the ticket image')
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `EventHub-ticket-${b.ticket_code}.png`
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
