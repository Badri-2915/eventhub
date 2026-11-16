package auth

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/eventhub/backend/internal/db"
	"github.com/eventhub/backend/internal/httpx"
)

const ctxUserID = "auth.user_id"

// RequireAuth rejects requests without a valid session cookie.
func RequireAuth(tokens *Tokens) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, err := c.Cookie(CookieName)
		if err != nil || raw == "" {
			httpx.Abort(c, httpx.ErrUnauthorized)
			return
		}
		id, err := tokens.Verify(raw)
		if err != nil {
			httpx.Abort(c, httpx.ErrUnauthorized)
			return
		}
		c.Set(ctxUserID, id)
		c.Next()
	}
}

// RequireHost must run after RequireAuth. It reads is_host from the DB so a
// freshly promoted user doesn't need a new token.
func RequireHost(store *db.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, err := store.GetUser(c, UserID(c))
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Abort(c, httpx.ErrUnauthorized)
			return
		}
		if err != nil {
			httpx.Abort(c, err)
			return
		}
		if !u.IsHost {
			httpx.Abort(c, httpx.ErrNotHost)
			return
		}
		c.Next()
	}
}

// UserID returns the authenticated user's id. Only valid behind RequireAuth.
func UserID(c *gin.Context) uuid.UUID {
	return c.MustGet(ctxUserID).(uuid.UUID)
}

// RequireAdmin must run after RequireAuth. Admins are configured by email.
func RequireAdmin(store *db.Store, isAdmin func(email string) bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, err := store.GetUser(c, UserID(c))
		if err != nil {
			httpx.Abort(c, httpx.ErrUnauthorized)
			return
		}
		if !isAdmin(u.Email) {
			httpx.Abort(c, httpx.ErrForbidden)
			return
		}
		c.Next()
	}
}
