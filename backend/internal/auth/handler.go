package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"google.golang.org/api/idtoken"

	"github.com/eventhub/backend/internal/config"
	"github.com/eventhub/backend/internal/db"
	"github.com/eventhub/backend/internal/db/sqlc"
	"github.com/eventhub/backend/internal/httpx"
)

type Handler struct {
	cfg    config.Config
	store  *db.Store
	tokens *Tokens
}

func NewHandler(cfg config.Config, store *db.Store, tokens *Tokens) *Handler {
	return &Handler{cfg: cfg, store: store, tokens: tokens}
}

func (h *Handler) Register(api *gin.RouterGroup) {
	api.POST("/auth/google", h.googleLogin)
	api.POST("/auth/logout", h.logout)
	if h.cfg.IsDev() {
		// Lets you sign in locally (and from load tests) without a Google client.
		api.POST("/auth/dev", h.devLogin)
	}

	me := api.Group("/me", RequireAuth(h.tokens))
	me.GET("", h.me)
	me.POST("/become-host", h.becomeHost)
}

type userDTO struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	IsHost    bool   `json:"is_host"`
	IsAdmin   bool   `json:"is_admin"`
}

func (h *Handler) toUserDTO(u sqlc.User) userDTO {
	return userDTO{
		ID: u.ID.String(), Email: u.Email, Name: u.Name, AvatarURL: u.AvatarUrl,
		IsHost: u.IsHost, IsAdmin: h.cfg.IsAdmin(u.Email),
	}
}

// googleLogin verifies the Google ID token server-side; nothing about the
// user's identity is taken from the request body except the signed token.
func (h *Handler) googleLogin(c *gin.Context) {
	var body struct {
		IDToken string `json:"id_token" binding:"required"`
	}
	if !httpx.BindJSON(c, &body) {
		return
	}
	payload, err := idtoken.Validate(c, body.IDToken, h.cfg.GoogleClientID)
	if err != nil {
		httpx.Abort(c, httpx.New(http.StatusUnauthorized, "INVALID_GOOGLE_TOKEN", "Google sign-in could not be verified"))
		return
	}
	email, _ := payload.Claims["email"].(string)
	verified, _ := payload.Claims["email_verified"].(bool)
	if email == "" || !verified {
		httpx.Abort(c, httpx.New(http.StatusUnauthorized, "EMAIL_NOT_VERIFIED", "Google account email is not verified"))
		return
	}
	name, _ := payload.Claims["name"].(string)
	picture, _ := payload.Claims["picture"].(string)

	h.loginAs(c, sqlc.UpsertUserByGoogleSubParams{
		GoogleSub: payload.Subject, Email: email, Name: name, AvatarUrl: picture,
	})
}

func (h *Handler) devLogin(c *gin.Context) {
	var body struct {
		Email string `json:"email" binding:"required,email"`
		Name  string `json:"name"`
	}
	if !httpx.BindJSON(c, &body) {
		return
	}
	if body.Name == "" {
		body.Name = strings.Split(body.Email, "@")[0]
	}
	h.loginAs(c, sqlc.UpsertUserByGoogleSubParams{
		GoogleSub: "dev:" + body.Email, Email: body.Email, Name: body.Name,
	})
}

func (h *Handler) loginAs(c *gin.Context, p sqlc.UpsertUserByGoogleSubParams) {
	u, err := h.store.UpsertUserByGoogleSub(c, p)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	tok, err := h.tokens.Issue(u.ID)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	h.setCookie(c, tok, int(sessionTTL.Seconds()))
	c.JSON(http.StatusOK, gin.H{"user": h.toUserDTO(u)})
}

func (h *Handler) logout(c *gin.Context) {
	h.setCookie(c, "", -1)
	c.Status(http.StatusNoContent)
}

func (h *Handler) setCookie(c *gin.Context, value string, maxAge int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(CookieName, value, maxAge, "/", "", !h.cfg.IsDev(), true)
}

func (h *Handler) me(c *gin.Context) {
	u, err := h.store.GetUser(c, UserID(c))
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Abort(c, httpx.ErrUnauthorized)
		return
	}
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": h.toUserDTO(u)})
}

func (h *Handler) becomeHost(c *gin.Context) {
	u, err := h.store.MakeHost(c, UserID(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": h.toUserDTO(u)})
}
