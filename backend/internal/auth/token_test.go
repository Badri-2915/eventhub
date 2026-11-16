package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var secret = []byte("unit-test-secret-unit-test-secret!!")

func TestTokenRoundTrip(t *testing.T) {
	id := uuid.New()
	raw, err := NewTokens(secret).Issue(id)
	if err != nil {
		t.Fatal(err)
	}
	got, err := NewTokens(secret).Verify(raw)
	if err != nil || got != id {
		t.Fatalf("Verify = %v, %v; want %v", got, err, id)
	}
}

func TestTokenRejectsTampering(t *testing.T) {
	raw, _ := NewTokens(secret).Issue(uuid.New())
	parts := strings.Split(raw, ".")

	// Someone swaps in another user's id but cannot re-sign it.
	other, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject: uuid.NewString(), Issuer: tokenIssuer, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}).SignedString([]byte("attacker-secret-attacker-secret!!!"))
	forgedPayload := strings.Split(other, ".")[1]

	cases := map[string]string{
		"payload swapped":       parts[0] + "." + forgedPayload + "." + parts[2],
		"signed with wrong key": other,
		"signature removed":     parts[0] + "." + parts[1] + ".",
		"garbage":               "not-a-token",
		"empty":                 "",
	}
	for name, tok := range cases {
		if _, err := NewTokens(secret).Verify(tok); err == nil {
			t.Errorf("%s: token accepted", name)
		}
	}
}

func TestTokenRejectsExpiredWrongIssuerAndNoneAlg(t *testing.T) {
	sign := func(c jwt.RegisteredClaims, m jwt.SigningMethod, key any) string {
		s, err := jwt.NewWithClaims(m, c).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	id := uuid.NewString()
	expired := sign(jwt.RegisteredClaims{Subject: id, Issuer: tokenIssuer, ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute))}, jwt.SigningMethodHS256, secret)
	wrongIssuer := sign(jwt.RegisteredClaims{Subject: id, Issuer: "someone-else", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}, jwt.SigningMethodHS256, secret)
	noExpiry := sign(jwt.RegisteredClaims{Subject: id, Issuer: tokenIssuer}, jwt.SigningMethodHS256, secret)
	noneAlg := sign(jwt.RegisteredClaims{Subject: id, Issuer: tokenIssuer, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType)
	badSubject := sign(jwt.RegisteredClaims{Subject: "not-a-uuid", Issuer: tokenIssuer, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}, jwt.SigningMethodHS256, secret)

	for name, tok := range map[string]string{"expired": expired, "wrong issuer": wrongIssuer, "no expiry": noExpiry, `alg "none"`: noneAlg, "bad subject": badSubject} {
		if _, err := NewTokens(secret).Verify(tok); err == nil {
			t.Errorf("%s: token accepted", name)
		}
	}
}

func TestSessionLastsSevenDays(t *testing.T) {
	raw, _ := NewTokens(secret).Issue(uuid.New())
	var c jwt.RegisteredClaims
	if _, _, err := jwt.NewParser().ParseUnverified(raw, &c); err != nil {
		t.Fatal(err)
	}
	if d := c.ExpiresAt.Sub(c.IssuedAt.Time); d != 7*24*time.Hour {
		t.Fatalf("session length %v, want 7 days", d)
	}
}
