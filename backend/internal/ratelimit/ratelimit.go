// Package ratelimit is a Redis fixed-window limiter keyed per user.
package ratelimit

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/eventhub/backend/internal/auth"
	"github.com/eventhub/backend/internal/httpx"
)

// PerUser allows `limit` requests per `window` for each signed-in user.
// Fails open if Redis is unavailable: booking correctness lives in Postgres.
func PerUser(rdb *redis.Client, name string, limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		bucket := time.Now().Unix() / int64(window.Seconds())
		key := fmt.Sprintf("rl:%s:%s:%d", name, auth.UserID(c), bucket)

		pipe := rdb.TxPipeline()
		incr := pipe.Incr(c, key)
		pipe.Expire(c, key, window)
		if _, err := pipe.Exec(c); err != nil {
			slog.Warn("rate limiter unavailable", "err", err)
			c.Next()
			return
		}
		if incr.Val() > int64(limit) {
			c.Header("Retry-After", strconv.Itoa(int(window.Seconds())))
			httpx.Abort(c, httpx.New(http.StatusTooManyRequests, "RATE_LIMITED", "Too many attempts. Please wait a moment."))
			return
		}
		c.Next()
	}
}
