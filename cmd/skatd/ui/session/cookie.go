// Package session manages stateless UI sessions via signed HTTP-only cookies.
package session

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const cookieName = "_skatd_session"

// Data holds the authenticated user's session state.
type Data struct {
	Subject string
	Email   string
	Issuer  string
	// Extra holds additional JWT claims from the ID token (e.g. "name", "picture").
	Extra map[string]interface{}
}

// SessionClaims is the JWT payload stored in the session cookie.
type SessionClaims struct {
	Email  string                 `json:"email"`
	Issuer string                 `json:"iss_url"`
	Extra  map[string]interface{} `json:"extra,omitempty"`
	jwt.RegisteredClaims
}

// Encode creates a signed session cookie value from session Data.
func Encode(d *Data, signingKey []byte, ttl time.Duration) (string, error) {
	claims := SessionClaims{
		Email:  d.Email,
		Issuer: d.Issuer,
		Extra:  d.Extra,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   d.Subject,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(signingKey)
}

// Decode parses and verifies a session cookie value, returning the session Data.
func Decode(tokenStr string, signingKey []byte) (*Data, error) {
	claims := &SessionClaims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return signingKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("verify session: %w", err)
	}
	return &Data{
		Subject: claims.Subject,
		Email:   claims.Email,
		Issuer:  claims.Issuer,
		Extra:   claims.Extra,
	}, nil
}

// CookieName returns the name of the session cookie.
func CookieName() string { return cookieName }
