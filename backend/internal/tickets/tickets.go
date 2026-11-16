// Package tickets renders ticket QR codes and handles door check-in.
//
// The QR encodes a link to <site>/check-in/<ticket code>. The host scans it
// with any phone camera, which opens the check-in page; only the event's host
// can actually check a ticket in, and only once. Check-in closes once the event
// is over; scanning well before the start still works but is flagged as early.
package tickets

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	qrcode "github.com/skip2/go-qrcode"

	"github.com/eventhub/backend/internal/auth"
	"github.com/eventhub/backend/internal/db"
	"github.com/eventhub/backend/internal/db/sqlc"
	"github.com/eventhub/backend/internal/httpx"
)

var codePattern = regexp.MustCompile(`^EH-[A-Z0-9]{8}$`)

var (
	ErrUnknownTicket = httpx.New(http.StatusNotFound, "TICKET_NOT_FOUND", "No ticket with this code for your events")
	ErrTicketVoid    = httpx.New(http.StatusGone, "TICKET_NOT_VALID", "This ticket was cancelled or refunded")
	ErrEventOver     = httpx.New(http.StatusConflict, "CHECK_IN_CLOSED", "This event is already over, so its tickets can't be used")
	ErrNotCheckedIn  = httpx.New(http.StatusConflict, "NOT_CHECKED_IN", "This ticket hasn't been checked in")
)

const (
	earlyWindow = 12 * time.Hour // scans before start − 12h are flagged as early
	lateWindow  = 6 * time.Hour  // check-in closes 6h after the end (or 12h after the start)
)

// CheckInURL is what the QR code points to.
func CheckInURL(siteURL, code string) string { return siteURL + "/check-in/" + code }

// QRPNG renders the check-in link for a ticket code as a PNG.
func QRPNG(siteURL, code string) ([]byte, error) {
	return qrcode.Encode(CheckInURL(siteURL, code), qrcode.Medium, 512)
}

type Service struct {
	store   *db.Store
	siteURL string
	Now     func() time.Time // replaceable in tests
}

func NewService(store *db.Store, siteURL string) *Service {
	return &Service{store: store, siteURL: siteURL, Now: time.Now}
}

type CheckInResult struct {
	Result        string     `json:"result"` // CHECKED_IN | ALREADY_CHECKED_IN | UNDONE
	BookingID     uuid.UUID  `json:"booking_id"`
	EventID       uuid.UUID  `json:"event_id"`
	EventTitle    string     `json:"event_title"`
	AttendeeName  string     `json:"attendee_name"`
	AttendeeEmail string     `json:"attendee_email"`
	Tickets       int32      `json:"tickets"`
	Items         string     `json:"items"`
	CheckedInAt   *time.Time `json:"checked_in_at"`
	StartsAt      *time.Time `json:"starts_at"`
	Early         bool       `json:"early"`    // scanned more than 12h before the start
	Arrived       int32      `json:"arrived"`  // guests in so far, for the door counter
	Expected      int32      `json:"expected"` // all confirmed tickets for the event
}

// lookup finds a confirmed ticket for one of the host's events.
func (s *Service) lookup(ctx context.Context, hostID uuid.UUID, code string) (sqlc.GetTicketByCodeRow, error) {
	if !codePattern.MatchString(code) {
		return sqlc.GetTicketByCodeRow{}, ErrUnknownTicket
	}
	t, err := s.store.GetTicketByCode(ctx, pgtype.Text{String: code, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && t.HostID != hostID) {
		return t, ErrUnknownTicket // don't reveal other hosts' tickets
	}
	if err != nil {
		return t, err
	}
	if t.Status != sqlc.BookingStatusCONFIRMED {
		return t, ErrTicketVoid
	}
	return t, nil
}

func (s *Service) result(ctx context.Context, t sqlc.GetTicketByCodeRow, outcome string) (CheckInResult, error) {
	counts, err := s.store.CheckInCounts(ctx, t.EventID)
	if err != nil {
		return CheckInResult{}, err
	}
	return CheckInResult{
		Result: outcome, BookingID: t.ID, EventID: t.EventID, EventTitle: t.EventTitle,
		AttendeeName: t.AttendeeName, AttendeeEmail: t.AttendeeEmail, Tickets: t.Tickets, Items: t.Items,
		CheckedInAt: t.CheckedInAt, StartsAt: t.StartsAt,
		Early:   t.StartsAt != nil && s.Now().Before(t.StartsAt.Add(-earlyWindow)),
		Arrived: counts.Arrived, Expected: counts.Expected,
	}, nil
}

// closesAt is when the door shuts for an event.
func closesAt(t sqlc.GetTicketByCodeRow) (time.Time, bool) {
	switch {
	case t.EndsAt != nil:
		return t.EndsAt.Add(lateWindow), true
	case t.StartsAt != nil:
		return t.StartsAt.Add(2 * lateWindow), true
	}
	return time.Time{}, false
}

// CheckIn validates a ticket for the host's own event and marks it used.
// Scanning the same ticket again reports when it was first used.
func (s *Service) CheckIn(ctx context.Context, hostID uuid.UUID, code string) (CheckInResult, error) {
	t, err := s.lookup(ctx, hostID, code)
	if err != nil {
		return CheckInResult{}, err
	}
	if end, ok := closesAt(t); ok && s.Now().After(end) {
		return CheckInResult{}, ErrEventOver
	}

	outcome := "ALREADY_CHECKED_IN"
	at, err := s.store.MarkCheckedIn(ctx, t.ID)
	switch {
	case err == nil:
		outcome, t.CheckedInAt = "CHECKED_IN", at
	case errors.Is(err, pgx.ErrNoRows):
		// Lost the race or already used: re-read the original time.
		if again, err := s.store.GetTicketByCode(ctx, pgtype.Text{String: code, Valid: true}); err == nil {
			t.CheckedInAt = again.CheckedInAt
		}
	default:
		return CheckInResult{}, err
	}
	return s.result(ctx, t, outcome)
}

// UndoCheckIn reverses a mistaken scan so the ticket can be scanned again.
func (s *Service) UndoCheckIn(ctx context.Context, hostID uuid.UUID, code string) (CheckInResult, error) {
	t, err := s.lookup(ctx, hostID, code)
	if err != nil {
		return CheckInResult{}, err
	}
	if _, err := s.store.UndoCheckIn(ctx, t.ID); errors.Is(err, pgx.ErrNoRows) {
		return CheckInResult{}, ErrNotCheckedIn
	} else if err != nil {
		return CheckInResult{}, err
	}
	t.CheckedInAt = nil
	return s.result(ctx, t, "UNDONE")
}

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterPublic mounts the QR image (public so email clients can load it;
// the ticket code itself is the secret and is already in that email).
func (h *Handler) RegisterPublic(api *gin.RouterGroup) {
	api.GET("/tickets/:code/qr.png", h.qr)
}

// RegisterHost mounts check-in; the group must enforce auth + host.
func (h *Handler) RegisterHost(host *gin.RouterGroup) {
	host.POST("/check-in/:code", h.checkIn)
	host.DELETE("/check-in/:code", h.undo)
}

func (h *Handler) qr(c *gin.Context) {
	code := c.Param("code")
	if !codePattern.MatchString(code) {
		c.Status(http.StatusNotFound)
		return
	}
	png, err := QRPNG(h.svc.siteURL, code)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Data(http.StatusOK, "image/png", png)
}

func (h *Handler) checkIn(c *gin.Context) {
	res, err := h.svc.CheckIn(c, auth.UserID(c), c.Param("code"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) undo(c *gin.Context) {
	res, err := h.svc.UndoCheckIn(c, auth.UserID(c), c.Param("code"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}
