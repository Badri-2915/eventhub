// Package httpx holds the shared JSON error format and request helpers.
package httpx

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Error is an API error rendered as {"error": {"code", "message", "details"}}.
type Error struct {
	Status  int            `json:"-"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func New(status int, code, msg string) *Error {
	return &Error{Status: status, Code: code, Message: msg}
}

func (e *Error) With(details map[string]any) *Error {
	cp := *e
	cp.Details = details
	return &cp
}

var (
	ErrUnauthorized = New(http.StatusUnauthorized, "UNAUTHORIZED", "Sign in required")
	ErrForbidden    = New(http.StatusForbidden, "FORBIDDEN", "You are not allowed to do this")
	ErrNotHost      = New(http.StatusForbidden, "NOT_HOST", "Become a host to access this")
	ErrNotFound     = New(http.StatusNotFound, "NOT_FOUND", "Not found")
)

func BadRequest(msg string) *Error {
	return New(http.StatusBadRequest, "VALIDATION_FAILED", msg)
}

// Abort writes err as JSON. Unknown errors become a 500 and are logged.
func Abort(c *gin.Context, err error) {
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		slog.Error("internal error", "path", c.FullPath(), "err", err)
		apiErr = New(http.StatusInternalServerError, "INTERNAL", "Something went wrong")
	}
	c.AbortWithStatusJSON(apiErr.Status, gin.H{"error": apiErr})
}

// BindJSON decodes the body into dst, aborting with 400 on failure.
func BindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		Abort(c, BadRequest("Invalid request body: "+err.Error()))
		return false
	}
	return true
}
