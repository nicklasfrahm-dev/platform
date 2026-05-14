// Package pkce implements a stateless OIDC PKCE flow using signed cookies.
// The code_verifier is stored in a short-lived JWT cookie so no server-side
// state is required — Cloud Run compatible.
package pkce

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const cookieName = "_skatd_pkce"

// StateClaims holds the PKCE state stored in the cookie JWT.
type StateClaims struct {
	Verifier    string `json:"verifier"`
	RedirectURI string `json:"redirect_uri"`
	jwt.RegisteredClaims
}

// NewChallenge generates a random code_verifier and derives its S256 challenge.
func NewChallenge() (verifier, challenge string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate verifier: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

// EncodeState signs a StateClaims into a JWT string.
func EncodeState(verifier, redirectURI, nonce string, signingKey []byte) (string, error) {
	claims := StateClaims{
		Verifier:    verifier,
		RedirectURI: redirectURI,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        nonce,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(signingKey)
}

// DecodeState parses and verifies a state JWT, returning its claims.
func DecodeState(tokenStr string, signingKey []byte) (*StateClaims, error) {
	claims := &StateClaims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return signingKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("verify state JWT: %w", err)
	}
	return claims, nil
}

// CookieName returns the name of the PKCE state cookie.
func CookieName() string { return cookieName }
