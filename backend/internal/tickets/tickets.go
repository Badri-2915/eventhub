// Package tickets renders ticket QR codes and handles door check-in.
//
// The QR encodes a link to <site>/check-in/<ticket code>. The host scans it
// with any phone camera, which opens the check-in page; only the event's host
// can actually check a ticket in, and only once.
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
}

func NewService(store *db.Store, siteURL string) *Service {
	return &Service{store: store, siteURL: siteURL}
}

type CheckInResult struct {
	Result        string     `json:"result"` // CHECKED_IN | ALREADY_CHECKED_IN
	BookingID     uuid.UUID  `json:"booking_id"`
	EventID       uuid.UUID  `json:"event_id"`
	EventTitle    string     `json:"event_title"`
	AttendeeName  string     `json:"attendee_name"`
	AttendeeEmail string     `json:"attendee_email"`
	Tickets       int32      `json:"tickets"`
	Items         string     `json:"items"`
	CheckedInAt   *time.Time `json:"checked_in_at"`
}

// CheckIn validates a ticket for the host's own event and marks it used.
// Scanning the same ticket again reports when it was first used.
func (s *Service) CheckIn(ctx context.Context, hostID uuid.UUID, code string) (CheckInResult, error) {
	if !codePattern.MatchString(code) {
		return CheckInResult{}, ErrUnknownTicket
	}
	t, err := s.store.GetTicketByCode(ctx, pgtype.Text{String: code, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && t.HostID != hostID) {
		return CheckInResult{}, ErrUnknownTicket // don't reveal other hosts' tickets
	}
	if err != nil {
		return CheckInResult{}, err
	}
	if t.Status != sqlc.BookingStatusCONFIRMED {
		return CheckInResult{}, ErrTicketVoid
	}

	res := CheckInResult{
		Result: "ALREADY_CHECKED_IN", BookingID: t.ID, EventID: t.EventID, EventTitle: t.EventTitle,
		AttendeeName: t.AttendeeName, AttendeeEmail: t.AttendeeEmail, Tickets: t.Tickets, Items: t.Items,
		CheckedInAt: t.CheckedInAt,
	}
	at, err := s.store.MarkCheckedIn(ctx, t.ID)
	switch {
	case err == nil:
		res.Result, res.CheckedInAt = "CHECKED_IN", at
	case errors.Is(err, pgx.ErrNoRows):
		// Lost the race or already used: re-read the original time.
		if again, err := s.store.GetTicketByCode(ctx, pgtype.Text{String: code, Valid: true}); err == nil {
			res.CheckedInAt = again.CheckedInAt
		}
	default:
		return CheckInResult{}, err
	}
	return res, nil
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
