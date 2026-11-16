export type User = {
  id: string
  email: string
  name: string
  avatar_url: string
  is_host: boolean
  is_admin: boolean
}

export const EVENT_CATEGORIES = ['MOVIE', 'COMEDY', 'CONCERT', 'THEATRE', 'WORKSHOP', 'OTHER'] as const
export type EventCategory = (typeof EVENT_CATEGORIES)[number]
export type SeatingMode = 'NUMBERED' | 'GA'
export type EventStatus = 'DRAFT' | 'PUBLISHED' | 'CANCELLED' | 'COMPLETED'

export type TicketCategory = {
  id: string
  name: string
  color: string
  sort_order: number
  price_paise: number
  capacity: number | null
  sold: number
  held: number
}

export type HostEvent = {
  id: string
  host_id: string
  title: string
  description: string
  category: EventCategory
  poster_url: string
  map_url: string
  venue_name: string
  city: string
  address: string
  starts_at: string | null
  ends_at: string | null
  seating_mode: SeatingMode
  status: EventStatus
  layout: unknown
  max_per_booking: number
  cancel_cutoff_hours: number
  created_at: string
  published_at: string | null
  categories: TicketCategory[]
  summary?: { capacity: number; sold: number; revenue_paise: number }
}

export type CategoryInput = {
  id?: string
  name: string
  color: string
  price_paise: number
  capacity: number | null
}

export type EventInput = {
  title: string
  description: string
  category: EventCategory
  poster_url: string
  map_url: string
  venue_name: string
  city: string
  address: string
  starts_at: string | null
  ends_at: string | null
  seating_mode: SeatingMode
  layout: unknown
  max_per_booking: number
  cancel_cutoff_hours: number
  categories: CategoryInput[]
}

// ---- public browsing -------------------------------------------------------

export type EventCardData = {
  id: string
  title: string
  category: EventCategory
  poster_url: string
  venue_name: string
  city: string
  starts_at: string
  seating_mode: SeatingMode
  min_price_paise: number
  max_price_paise: number
  total_tickets: number
  available_tickets: number
}

export type PublicCategory = {
  id: string
  name: string
  color: string
  price_paise: number
  total: number
  available: number
}

export type PublicEvent = {
  id: string
  title: string
  description: string
  category: EventCategory
  poster_url: string
  map_url: string
  venue_name: string
  city: string
  address: string
  starts_at: string
  ends_at: string | null
  seating_mode: SeatingMode
  status: EventStatus
  max_per_booking: number
  cancel_cutoff_hours: number
  on_sale: boolean
  grid: { rows: number; cols: number } | null
  categories: PublicCategory[]
}

export type SeatStatus = 'AVAILABLE' | 'HELD' | 'BOOKED' | 'BLOCKED'

export type Seat = {
  id: string
  category_id: string
  row_label: string
  seat_number: number
  label: string
  grid_row: number
  grid_col: number
  status: SeatStatus
}

// ---- bookings --------------------------------------------------------------

export type BookingStatus = 'PENDING' | 'CONFIRMED' | 'CANCELLED' | 'EXPIRED' | 'FAILED'

export type HoldResult = { booking_id: string; expires_at: string; total_paise: number }

export type Booking = {
  id: string
  status: BookingStatus
  total_paise: number
  expires_at: string
  ticket_code: string | null
  created_at: string
  confirmed_at: string | null
  cancelled_at: string | null
  checked_in_at: string | null
  can_cancel: boolean
  event: {
    id: string
    title: string
    category: EventCategory
    venue_name: string
    city: string
    address: string
    poster_url: string
    map_url: string
    starts_at: string
    seating_mode: SeatingMode
    cancel_cutoff_hours: number
  }
  items: {
    category_id: string
    category_name: string
    category_color: string
    seat_id: string | null
    seat_label: string
    quantity: number
    unit_price_paise: number
  }[]
}

export type BookingSummary = {
  id: string
  status: BookingStatus
  total_paise: number
  ticket_code: string | null
  ticket_count: number
  expires_at: string
  event_id: string
  event_title: string
  event_venue: string
  event_city: string
  event_category: EventCategory
  event_poster_url: string
  event_starts_at: string
}

// ---- layout designer -------------------------------------------------------

/** Cell: "" unassigned seat, "a" aisle, "x" blocked seat, otherwise a category id. */
export type Layout = { rows: number; cols: number; cells: string[][] }

// ---- host analytics --------------------------------------------------------

export type CategoryStats = {
  id: string
  name: string
  color: string
  price_paise: number
  total: number
  sold: number
  held: number
  available: number
  revenue_paise: number
}

export type EventStats = {
  event: HostEvent
  total: number
  sold: number
  held: number
  available: number
  bookings: number
  revenue_paise: number
  categories: CategoryStats[]
  sales_by_day: { day: string; bookings: number; tickets: number; revenue_paise: number }[]
}

export type Attendee = {
  booking_id: string
  name: string
  email: string
  ticket_code: string
  tickets: number
  items: string
  total_paise: number
  confirmed_at: string | null
  checked_in_at: string | null
}

export type CheckInResult = {
  result: 'CHECKED_IN' | 'ALREADY_CHECKED_IN'
  booking_id: string
  event_id: string
  event_title: string
  attendee_name: string
  attendee_email: string
  tickets: number
  items: string
  checked_in_at: string | null
}

// ---- admin -----------------------------------------------------------------

export type AdminOverview = {
  users: number
  hosts: number
  events: number
  live_events: number
  bookings: number
  revenue_paise: number
}

export type AdminEvent = {
  id: string
  title: string
  category: EventCategory
  status: EventStatus
  city: string
  starts_at: string | null
  seating_mode: SeatingMode
  host_name: string
  host_email: string
  capacity: number
  sold: number
  revenue_paise: number
}
