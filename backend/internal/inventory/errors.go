package inventory

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/eventhub/backend/internal/httpx"
)

var (
	ErrEventNotFound    = httpx.New(http.StatusNotFound, "EVENT_NOT_FOUND", "Event not found")
	ErrNotOnSale        = httpx.New(http.StatusConflict, "EVENT_NOT_ON_SALE", "Tickets for this event are not on sale")
	ErrEmptySelection   = httpx.New(http.StatusBadRequest, "EMPTY_SELECTION", "Select at least one ticket")
	ErrWrongMode        = httpx.New(http.StatusBadRequest, "WRONG_SEATING_MODE", "This event does not use that kind of ticket selection")
	ErrSeatsTaken       = httpx.New(http.StatusConflict, "SEATS_TAKEN", "Some of those seats were just taken. Please pick again.")
	ErrNotEnough        = httpx.New(http.StatusConflict, "NOT_ENOUGH_TICKETS", "Not enough tickets left in that category")
	ErrHoldInProgress   = httpx.New(http.StatusConflict, "HOLD_IN_PROGRESS", "You already have a hold in progress for this event")
	ErrBusy             = httpx.New(http.StatusConflict, "TRY_AGAIN", "Lots of people are booking right now. Please try again.")
	ErrBookingNotFound  = httpx.New(http.StatusNotFound, "BOOKING_NOT_FOUND", "Booking not found")
	ErrHoldExpired      = httpx.New(http.StatusGone, "HOLD_EXPIRED", "Your hold has expired. Please select tickets again.")
	ErrAlreadyConfirmed = httpx.New(http.StatusConflict, "ALREADY_CONFIRMED", "This booking is already confirmed")
	ErrKeyReused        = httpx.New(http.StatusConflict, "IDEMPOTENCY_KEY_REUSED", "This payment key belongs to a different booking")
	ErrMissingKey       = httpx.New(http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required")
	ErrNotCancellable   = httpx.New(http.StatusConflict, "NOT_CANCELLABLE", "Only confirmed bookings can be cancelled")
	ErrCancelWindow     = httpx.New(http.StatusConflict, "CANCEL_WINDOW_CLOSED", "The cancellation window for this event has closed")
	ErrNotPending       = httpx.New(http.StatusConflict, "NOT_PENDING", "This hold is no longer active")
)

// mapPgError turns lock/constraint conflicts into clean 409s instead of 500s.
func mapPgError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case "23505": // unique_violation
		if pgErr.ConstraintName == "bookings_one_pending_idx" {
			return ErrHoldInProgress
		}
	case "23514": // check_violation: the sold + held <= capacity safety net fired
		return ErrNotEnough
	case "40P01", "40001": // deadlock / serialization failure
		return ErrBusy
	}
	return err
}
