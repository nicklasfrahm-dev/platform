// Package authn provides OIDC Bearer token authentication for skatd.
package authn

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/nicklasfrahm-dev/platform/cmd/skatd/api"
)

type contextKey struct{}

// Claims holds the validated JWT payload surfaced to the authz layer.
type Claims struct {
	Issuer  string
	Subject string
	Extra   map[string]interface{}
}

type providerCache struct {
	mu        sync.Mutex
	issuerURL string
	clientID  string
	verifier  *oidc.IDTokenVerifier
}

func (c *providerCache) verifier_(ctx context.Context) (*oidc.IDTokenVerifier, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.verifier != nil {
		return c.verifier, nil
	}
	provider, err := oidc.NewProvider(ctx, c.issuerURL)
	if err != nil {
		return nil, fmt.Errorf("fetch OIDC provider %s: %w", c.issuerURL, err)
	}
	c.verifier = provider.Verifier(&oidc.Config{ClientID: c.clientID})
	return c.verifier, nil
}

// Middleware returns an HTTP middleware that validates Bearer JWTs and
// injects *Claims into the request context under a private key.
func Middleware(issuerURL, clientID string) func(http.Handler) http.Handler {
	cache := &providerCache{issuerURL: issuerURL, clientID: clientID}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := r.Header.Get("Authorization")
			if raw == "" || !strings.HasPrefix(raw, "Bearer ") {
				api.WriteStatus(w, http.StatusUnauthorized, "Unauthorized", "missing Bearer token")
				return
			}
			token := strings.TrimPrefix(raw, "Bearer ")
			v, err := cache.verifier_(r.Context())
			if err != nil {
				api.WriteStatus(w, http.StatusServiceUnavailable, "ServiceUnavailable", "OIDC provider unavailable")
				return
			}
			idToken, err := v.Verify(r.Context(), token)
			if err != nil {
				api.WriteStatus(w, http.StatusUnauthorized, "Unauthorized", "invalid token: "+err.Error())
				return
			}
			var extra map[string]interface{}
			_ = idToken.Claims(&extra)
			claims := &Claims{
				Issuer:  idToken.Issuer,
				Subject: idToken.Subject,
				Extra:   extra,
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, claims)))
		})
	}
}

// FromContext extracts validated Claims from a request context.
// Returns (nil, false) if no claims are present.
func FromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(contextKey{}).(*Claims)
	return c, ok
}

// InjectClaims stores Claims in a context, used by the UI after session verification.
func InjectClaims(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, contextKey{}, c)
}
