package config

import (
	"strings"
	"testing"
)

// base sets a valid production environment; each case then breaks one thing.
func base(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgresql://user:pw@db.example.com/eventhub?sslmode=require")
	t.Setenv("JWT_SECRET", strings.Repeat("s", 32))
	t.Setenv("GOOGLE_CLIENT_ID", "client.apps.googleusercontent.com")
	t.Setenv("HOLD_MINUTES", "5")
	t.Setenv("HOLD_RATE_PER_MIN", "")
	t.Setenv("ADMIN_EMAILS", "")
	t.Setenv("FRONTEND_ORIGIN", "")
}

func TestLoadValidProductionConfig(t *testing.T) {
	base(t)
	t.Setenv("ADMIN_EMAILS", " Admin@Example.com , ops@example.com ,")
	t.Setenv("FRONTEND_ORIGIN", "https://eventhub.example.com/")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.HoldRatePerMin != 20 || c.HoldDuration.Minutes() != 5 {
		t.Fatalf("defaults: rate %d, hold %v", c.HoldRatePerMin, c.HoldDuration)
	}
	if c.FrontendOrigin != "https://eventhub.example.com" {
		t.Fatalf("trailing slash must be trimmed (QR links), got %q", c.FrontendOrigin)
	}
	if !c.IsAdmin("admin@example.com") || !c.IsAdmin("OPS@example.com") || c.IsAdmin("someone@example.com") {
		t.Fatal("admin emails are comma-separated, trimmed and case-insensitive")
	}
	if c.IsDev() {
		t.Fatal("production is not dev")
	}
}

func TestLoadRejectsBadConfig(t *testing.T) {
	cases := []struct {
		name, key, value, wantMsg string
	}{
		{"missing database", "DATABASE_URL", "", "DATABASE_URL is required"},
		{"pasted psql command", "DATABASE_URL", "psql 'postgresql://user:SECRETPW@host/db'", "DATABASE_URL must look like"},
		{"wrong scheme", "DATABASE_URL", "mysql://user:SECRETPW@host/db", "DATABASE_URL must look like"},
		{"short JWT secret", "JWT_SECRET", "too-short", "JWT_SECRET must be at least 32 bytes"},
		{"no Google client in production", "GOOGLE_CLIENT_ID", "", "GOOGLE_CLIENT_ID is required"},
		{"bad hold minutes", "HOLD_MINUTES", "0", "HOLD_MINUTES must be a positive integer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base(t)
			t.Setenv(tc.key, tc.value)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("want error containing %q, got %v", tc.wantMsg, err)
			}
			// A config error must never print the password (it ends up in deploy logs).
			if strings.Contains(err.Error(), "SECRETPW") {
				t.Fatalf("error leaks the password: %v", err)
			}
		})
	}
}

func TestDevNeedsNoGoogleClient(t *testing.T) {
	base(t)
	t.Setenv("APP_ENV", "development")
	t.Setenv("GOOGLE_CLIENT_ID", "")
	if _, err := Load(); err != nil {
		t.Fatalf("development runs without Google sign-in: %v", err)
	}
}

func TestBadRateLimitFallsBackToDefault(t *testing.T) {
	base(t)
	for _, v := range []string{"abc", "-5", "0"} {
		t.Setenv("HOLD_RATE_PER_MIN", v)
		c, err := Load()
		if err != nil || c.HoldRatePerMin != 20 {
			t.Fatalf("HOLD_RATE_PER_MIN=%q: got %d, %v", v, c.HoldRatePerMin, err)
		}
	}
}
