-- Door check-in: set once when the host scans the ticket's QR code.
ALTER TABLE bookings ADD COLUMN checked_in_at timestamptz;
