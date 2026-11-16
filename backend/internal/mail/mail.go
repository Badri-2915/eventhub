// Package mail sends transactional email through one of three backends:
// Brevo's HTTP API (free tier; works where outbound SMTP is blocked), any SMTP
// server (Gmail, or Mailpit locally), or the log when nothing is configured.
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

type Message struct {
	To      string
	Subject string
	HTML    string
	Text    string
}

type Sender interface {
	Send(ctx context.Context, m Message) error
}

type Config struct {
	From         string // "EventHub <tickets@example.com>"
	BrevoAPIKey  string
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
}

// New picks a backend from config: Brevo API key, else SMTP host, else log.
func New(c Config) (Sender, string, error) {
	from, err := mail.ParseAddress(c.From)
	if err != nil {
		return nil, "", fmt.Errorf("MAIL_FROM must look like \"EventHub <you@example.com>\": %w", err)
	}
	switch {
	case c.BrevoAPIKey != "":
		return &brevo{key: c.BrevoAPIKey, from: from, http: &http.Client{Timeout: 15 * time.Second}}, "brevo", nil
	case c.SMTPHost != "":
		return &smtpSender{cfg: c, from: from}, "smtp " + c.SMTPHost + ":" + c.SMTPPort, nil
	default:
		return logSender{}, "log (no email provider configured)", nil
	}
}

// ---- log -------------------------------------------------------------------

type logSender struct{}

func (logSender) Send(_ context.Context, m Message) error {
	slog.Info("email (not sent: no provider configured)", "to", m.To, "subject", m.Subject)
	return nil
}

// ---- SMTP ------------------------------------------------------------------

type smtpSender struct {
	cfg  Config
	from *mail.Address
}

func (s *smtpSender) Send(_ context.Context, m Message) error {
	boundary := fmt.Sprintf("eh-%d", time.Now().UnixNano())
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\n", s.from.String(), m.To, mime.QEncoding.Encode("utf-8", m.Subject))
	fmt.Fprintf(&b, "Date: %s\r\nContent-Type: multipart/alternative; boundary=%q\r\n\r\n", time.Now().Format(time.RFC1123Z), boundary)
	fmt.Fprintf(&b, "--%s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n", boundary, m.Text)
	fmt.Fprintf(&b, "--%s\r\nContent-Type: text/html; charset=utf-8\r\n\r\n%s\r\n--%s--\r\n", boundary, m.HTML, boundary)

	var auth smtp.Auth
	if s.cfg.SMTPUsername != "" {
		auth = smtp.PlainAuth("", s.cfg.SMTPUsername, s.cfg.SMTPPassword, s.cfg.SMTPHost)
	}
	// SendMail upgrades to TLS (STARTTLS) whenever the server offers it, e.g. Gmail on 587.
	return smtp.SendMail(s.cfg.SMTPHost+":"+s.cfg.SMTPPort, auth, s.from.Address, []string{m.To}, []byte(b.String()))
}

// ---- Brevo HTTP API ----------------------------------------------------------

type brevo struct {
	key  string
	from *mail.Address
	http *http.Client
}

func (b *brevo) Send(ctx context.Context, m Message) error {
	name := b.from.Name
	if name == "" {
		name = "EventHub"
	}
	body, _ := json.Marshal(map[string]any{
		"sender":      map[string]string{"name": name, "email": b.from.Address},
		"to":          []map[string]string{{"email": m.To}},
		"subject":     m.Subject,
		"htmlContent": m.HTML,
		"textContent": m.Text,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.brevo.com/v3/smtp/email", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("api-key", b.key)
	req.Header.Set("Content-Type", "application/json")
	res, err := b.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(res.Body).Decode(&e)
		return fmt.Errorf("brevo: %s %s", res.Status, e.Message)
	}
	return nil
}
