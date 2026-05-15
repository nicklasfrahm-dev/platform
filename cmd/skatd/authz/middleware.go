package authz

import (
	"net/http"

	"github.com/nicklasfrahm-dev/platform/cmd/skatd/api"
	"github.com/nicklasfrahm-dev/platform/cmd/skatd/authn"
)

// Check returns an HTTP middleware that verifies the caller has permission to
// perform verb on resource in the namespace from the URL path value "ns".
func Check(eng Engine, verb, resource string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := authn.FromContext(r.Context())
			if !ok {
				api.WriteStatus(w, http.StatusUnauthorized, "Unauthorized", "authentication required")
				return
			}
			namespace := r.PathValue("ns")
			if !eng.Allowed(claims, verb, resource, namespace) {
				api.WriteStatus(w, http.StatusForbidden, "Forbidden", "access denied by policy")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
