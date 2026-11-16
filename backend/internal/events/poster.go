package events

import (
	"crypto/sha1" //nolint:gosec // Cloudinary's signing scheme requires SHA-1
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/eventhub/backend/internal/auth"
	"github.com/eventhub/backend/internal/db/sqlc"
	"github.com/eventhub/backend/internal/httpx"
)

// Cloudinary holds credentials parsed from CLOUDINARY_URL
// (cloudinary://<api_key>:<api_secret>@<cloud_name>). Zero value = disabled.
type Cloudinary struct {
	CloudName, APIKey, APISecret string
}

func ParseCloudinaryURL(raw string) (Cloudinary, error) {
	if raw == "" {
		return Cloudinary{}, nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "cloudinary" || u.User == nil {
		return Cloudinary{}, fmt.Errorf("CLOUDINARY_URL must look like cloudinary://key:secret@cloud")
	}
	secret, _ := u.User.Password()
	return Cloudinary{CloudName: u.Host, APIKey: u.User.Username(), APISecret: secret}, nil
}

func (c Cloudinary) Enabled() bool { return c.CloudName != "" && c.APIKey != "" && c.APISecret != "" }

// posterSignature returns parameters for a signed direct browser upload, so
// the API secret never leaves the server and the file never passes through it.
func (h *Handler) posterSignature(c *gin.Context) {
	if !h.cloud.Enabled() {
		httpx.Abort(c, httpx.New(http.StatusServiceUnavailable, "UPLOADS_DISABLED", "Image uploads are not configured. Paste an image URL instead."))
		return
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	if _, err := h.svc.GetForHost(c, auth.UserID(c), id); err != nil {
		httpx.Abort(c, err)
		return
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	folder := "eventhub/posters"
	// Params must be sorted alphabetically: folder, timestamp.
	sum := sha1.Sum([]byte("folder=" + folder + "&timestamp=" + ts + h.cloud.APISecret)) //nolint:gosec
	c.JSON(http.StatusOK, gin.H{
		"upload_url": fmt.Sprintf("https://api.cloudinary.com/v1_1/%s/image/upload", h.cloud.CloudName),
		"api_key":    h.cloud.APIKey,
		"timestamp":  ts,
		"folder":     folder,
		"signature":  hex.EncodeToString(sum[:]),
	})
}

// setPoster updates the poster; allowed in any status (it doesn't affect inventory).
func (h *Handler) setPoster(c *gin.Context) {
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var body struct {
		PosterURL string `json:"poster_url"`
	}
	if !httpx.BindJSON(c, &body) {
		return
	}
	if body.PosterURL != "" {
		if u, err := url.Parse(body.PosterURL); err != nil || u.Scheme != "https" {
			httpx.Abort(c, httpx.BadRequest("Poster must be an https URL"))
			return
		}
	}
	ev, err := h.svc.store.SetEventPoster(c, sqlc.SetEventPosterParams{ID: id, HostID: auth.UserID(c), PosterUrl: body.PosterURL})
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Abort(c, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"poster_url": ev.PosterUrl})
}
