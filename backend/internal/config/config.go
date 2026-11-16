package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/eventhub/backend/internal/mail"
)

type Config struct {
	Env            string // "development" or "production"
	Port           string
	DatabaseURL    string
	RedisURL       string
	GoogleClientID string
	JWTSecret      []byte
	CloudinaryURL  string
	HoldDuration   time.Duration
	HoldRatePerMin int
	FrontendOrigin string
	AdminEmails    []string // ADMIN_EMAILS, comma-separated
	Mail           mail.Config
}

// IsAdmin reports whether email is in ADMIN_EMAILS (case-insensitive).
func (c Config) IsAdmin(email string) bool {
	for _, a := range c.AdminEmails {
		if strings.EqualFold(a, email) {
			return true
		}
	}
	return false
}

func (c Config) IsDev() bool { return c.Env == "development" }

func Load() (Config, error) {
	_ = godotenv.Load() // optional .env for local dev; real env vars win

	holdMin, err := strconv.Atoi(getenv("HOLD_MINUTES", "5"))
	if err != nil || holdMin <= 0 {
		return Config{}, fmt.Errorf("HOLD_MINUTES must be a positive integer")
	}

	c := Config{
		Env:            getenv("APP_ENV", "production"),
		Port:           getenv("PORT", "8080"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		RedisURL:       getenv("REDIS_URL", "redis://localhost:6379"),
		GoogleClientID: os.Getenv("GOOGLE_CLIENT_ID"),
		JWTSecret:      []byte(os.Getenv("JWT_SECRET")),
		CloudinaryURL:  os.Getenv("CLOUDINARY_URL"),
		HoldDuration:   time.Duration(holdMin) * time.Minute,
		HoldRatePerMin: atoiOr(os.Getenv("HOLD_RATE_PER_MIN"), 20),
		FrontendOrigin: strings.TrimRight(getenv("FRONTEND_ORIGIN", "http://localhost:5173"), "/"),
		AdminEmails:    splitList(os.Getenv("ADMIN_EMAILS")),
		Mail: mail.Config{
			From:         getenv("MAIL_FROM", "EventHub <tickets@eventhub.local>"),
			BrevoAPIKey:  os.Getenv("BREVO_API_KEY"),
			SMTPHost:     os.Getenv("SMTP_HOST"),
			SMTPPort:     getenv("SMTP_PORT", "587"),
			SMTPUsername: os.Getenv("SMTP_USERNAME"),
			SMTPPassword: os.Getenv("SMTP_PASSWORD"),
		},
	}

	if c.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	// Checked here so later parse errors (which echo the URL and its password) can't reach the logs.
	if u, err := url.Parse(c.DatabaseURL); err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" {
		return Config{}, fmt.Errorf("DATABASE_URL must look like postgresql://<user>:<password>@<host>/<db>?sslmode=require")
	}
	if len(c.JWTSecret) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET must be at least 32 bytes")
	}
	if !c.IsDev() && c.GoogleClientID == "" {
		return Config{}, fmt.Errorf("GOOGLE_CLIENT_ID is required outside development")
	}
	return c, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func atoiOr(s string, fallback int) int {
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n
	}
	return fallback
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
