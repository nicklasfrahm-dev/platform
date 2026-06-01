// Package ui implements the HTMX web UI for skatd.
package ui

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/nicklasfrahm-dev/platform/cmd/skatd/api"
	"github.com/nicklasfrahm-dev/platform/cmd/skatd/authn"
	"github.com/nicklasfrahm-dev/platform/cmd/skatd/authz"
	"github.com/nicklasfrahm-dev/platform/cmd/skatd/crypto"
	"github.com/nicklasfrahm-dev/platform/cmd/skatd/store"
	"github.com/nicklasfrahm-dev/platform/cmd/skatd/ui/pkce"
	"github.com/nicklasfrahm-dev/platform/cmd/skatd/ui/session"
	"golang.org/x/oauth2"
)

// Handler serves the skatd web UI.
type Handler struct {
	Store       store.Store
	Enc         crypto.Encryptor
	Engine      authz.Engine
	OIDCIssuer  string
	ClientID    string
	RedirectURL string
	SigningKey   []byte
	SessionTTL  time.Duration

	provider     *oidc.Provider
	oauth2Config *oauth2.Config

	tmplSecretsList   *template.Template
	tmplSecretDetail  *template.Template
	tmplSecretCreate  *template.Template
	tmplPoliciesList  *template.Template
	tmplError         *template.Template
}

// Init fetches the OIDC provider metadata and pre-parses templates.
func (h *Handler) Init(ctx context.Context) error {
	p, err := oidc.NewProvider(ctx, h.OIDCIssuer)
	if err != nil {
		return fmt.Errorf("fetch OIDC provider: %w", err)
	}
	h.provider = p
	h.oauth2Config = &oauth2.Config{
		ClientID:    h.ClientID,
		Endpoint:    p.Endpoint(),
		RedirectURL: h.RedirectURL,
		Scopes:      []string{oidc.ScopeOpenID, "email", "profile"},
	}
	h.tmplSecretsList = mustParse("secrets_list.html")
	h.tmplSecretDetail = mustParse("secret_detail.html")
	h.tmplSecretCreate = mustParse("secret_create.html")
	h.tmplPoliciesList = mustParse("policies_list.html")
	h.tmplError = mustParse("error.html")
	if h.SessionTTL == 0 {
		h.SessionTTL = 8 * time.Hour
	}
	return nil
}

// ServeHTTP implements http.Handler and routes all /ui/* requests.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ui/login", h.login)
	mux.HandleFunc("GET /ui/callback", h.callback)
	mux.HandleFunc("GET /ui/secrets/new", h.sessionGuard(h.secretNew))
	mux.HandleFunc("POST /ui/secrets", h.sessionGuard(h.secretCreate))
	mux.HandleFunc("GET /ui/secrets/{ns}/{name}", h.sessionGuard(h.secretDetail))
	mux.HandleFunc("DELETE /ui/secrets/{ns}/{name}", h.sessionGuard(h.secretDelete))
	mux.HandleFunc("GET /ui/secrets", h.sessionGuard(h.secretsList))
	mux.HandleFunc("GET /ui/policies", h.sessionGuard(h.policiesList))
	mux.HandleFunc("GET /ui", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/secrets", http.StatusFound)
	})
	mux.ServeHTTP(w, r)
}

// --- PKCE auth flow ---

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	verifier, challenge, err := pkce.NewChallenge()
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "Failed to generate PKCE challenge")
		return
	}
	nonce := randomString(16)
	stateToken, err := pkce.EncodeState(verifier, h.RedirectURL, nonce, h.SigningKey)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "Failed to encode state")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     pkce.CookieName(),
		Value:    stateToken,
		Path:     "/ui",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})
	authURL := h.oauth2Config.AuthCodeURL(nonce,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (h *Handler) callback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie(pkce.CookieName())
	if err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Missing PKCE state cookie")
		return
	}
	// Clear the PKCE cookie immediately.
	http.SetCookie(w, &http.Cookie{Name: pkce.CookieName(), Path: "/ui", MaxAge: -1})

	stateClaims, err := pkce.DecodeState(stateCookie.Value, h.SigningKey)
	if err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid PKCE state: "+err.Error())
		return
	}
	if r.URL.Query().Get("state") != stateClaims.ID {
		h.renderError(w, r, http.StatusBadRequest, "State mismatch")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		errParam := r.URL.Query().Get("error")
		h.renderError(w, r, http.StatusBadRequest, "No authorization code: "+errParam)
		return
	}
	token, err := h.oauth2Config.Exchange(r.Context(), code,
		oauth2.SetAuthURLParam("code_verifier", stateClaims.Verifier),
	)
	if err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Token exchange failed: "+err.Error())
		return
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		h.renderError(w, r, http.StatusBadRequest, "No id_token in response")
		return
	}
	verifier := h.provider.Verifier(&oidc.Config{ClientID: h.ClientID})
	idToken, err := verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		h.renderError(w, r, http.StatusUnauthorized, "ID token verification failed: "+err.Error())
		return
	}
	var extra map[string]interface{}
	_ = idToken.Claims(&extra)
	email, _ := extra["email"].(string)

	sessionVal, err := session.Encode(&session.Data{
		Subject: idToken.Subject,
		Email:   email,
		Issuer:  idToken.Issuer,
		Extra:   extra,
	}, h.SigningKey, h.SessionTTL)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "Failed to create session")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     session.CookieName(),
		Value:    sessionVal,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(h.SessionTTL.Seconds()),
	})
	http.Redirect(w, r, "/ui/secrets", http.StatusFound)
}

// --- Session guard ---

func (h *Handler) sessionGuard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(session.CookieName())
		if err != nil {
			http.Redirect(w, r, "/ui/login", http.StatusFound)
			return
		}
		data, err := session.Decode(c.Value, h.SigningKey)
		if err != nil {
			http.SetCookie(w, &http.Cookie{Name: session.CookieName(), Path: "/", MaxAge: -1})
			http.Redirect(w, r, "/ui/login", http.StatusFound)
			return
		}
		claims := sessionToClaims(data)
		next(w, r.WithContext(authn.InjectClaims(r.Context(), claims)))
	}
}

// --- Secrets pages ---

func (h *Handler) secretsList(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("ns")
	if ns == "" {
		ns = "default"
	}
	claims, _ := authn.FromContext(r.Context())
	if !h.Engine.Allowed(claims, "list", "secrets", ns) {
		h.renderError(w, r, http.StatusForbidden, "Access denied")
		return
	}
	resources, err := h.Store.List(r.Context(), store.KindSecret, ns)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]api.Secret, 0, len(resources))
	for _, res := range resources {
		s, convErr := resourceToSecret(res, h.Enc)
		if convErr != nil {
			h.renderError(w, r, http.StatusInternalServerError, convErr.Error())
			return
		}
		items = append(items, *s)
	}
	h.render(w, r, h.tmplSecretsList, map[string]any{
		"Page":      "secrets",
		"User":      userEmail(r, h.SigningKey),
		"Namespace": ns,
		"Items":     items,
	})
}

func (h *Handler) secretDetail(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("ns"), r.PathValue("name")
	claims, _ := authn.FromContext(r.Context())
	if !h.Engine.Allowed(claims, "get", "secrets", ns) {
		h.renderError(w, r, http.StatusForbidden, "Access denied")
		return
	}
	res, err := h.Store.Get(r.Context(), store.KindSecret, ns, name)
	if err != nil {
		h.renderError(w, r, http.StatusNotFound, "Secret not found")
		return
	}
	s, err := resourceToSecret(res, h.Enc)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	h.render(w, r, h.tmplSecretDetail, map[string]any{
		"Page":   "secrets",
		"User":   userEmail(r, h.SigningKey),
		"Secret": s,
	})
}

func (h *Handler) secretNew(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("ns")
	if ns == "" {
		ns = "default"
	}
	h.render(w, r, h.tmplSecretCreate, map[string]any{
		"Page":      "secrets",
		"User":      userEmail(r, h.SigningKey),
		"Namespace": ns,
	})
}

func (h *Handler) secretCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid form data")
		return
	}
	ns := r.FormValue("namespace")
	if ns == "" {
		ns = "default"
	}
	name := r.FormValue("name")
	rawData := r.FormValue("data")

	claims, _ := authn.FromContext(r.Context())
	if !h.Engine.Allowed(claims, "create", "secrets", ns) {
		h.renderError(w, r, http.StatusForbidden, "Access denied")
		return
	}

	data := parseKV(rawData)
	secret := &api.Secret{
		TypeMeta: api.TypeMeta{APIVersion: api.APIVersion, Kind: "Secret"},
		Metadata: api.ObjectMeta{Name: name, Namespace: ns},
		Spec:     api.SecretSpec{Data: data},
	}
	res, err := secretToResource(secret, h.Enc)
	if err != nil {
		h.renderWithError(w, r, h.tmplSecretCreate, ns, err.Error())
		return
	}
	if _, err = h.Store.Create(r.Context(), res); err != nil {
		h.renderWithError(w, r, h.tmplSecretCreate, ns, err.Error())
		return
	}
	http.Redirect(w, r, "/ui/secrets?ns="+ns, http.StatusFound)
}

func (h *Handler) secretDelete(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("ns"), r.PathValue("name")
	claims, _ := authn.FromContext(r.Context())
	if !h.Engine.Allowed(claims, "delete", "secrets", ns) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := h.Store.Delete(r.Context(), store.KindSecret, ns, name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// HTMX expects empty 200 to swap out the row.
	w.WriteHeader(http.StatusOK)
}

// --- Policies page ---

func (h *Handler) policiesList(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("ns")
	if ns == "" {
		ns = "default"
	}
	claims, _ := authn.FromContext(r.Context())
	if !h.Engine.Allowed(claims, "list", "policies", ns) {
		h.renderError(w, r, http.StatusForbidden, "Access denied")
		return
	}
	resources, err := h.Store.List(r.Context(), store.KindPolicy, ns)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]api.Policy, 0, len(resources))
	for _, res := range resources {
		p, convErr := resourceToPolicy(res)
		if convErr != nil {
			h.renderError(w, r, http.StatusInternalServerError, convErr.Error())
			return
		}
		items = append(items, *p)
	}
	h.render(w, r, h.tmplPoliciesList, map[string]any{
		"Page":      "policies",
		"User":      userEmail(r, h.SigningKey),
		"Namespace": ns,
		"Items":     items,
	})
}

// --- Helpers ---

func (h *Handler) render(w http.ResponseWriter, _ *http.Request, t *template.Template, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) renderError(w http.ResponseWriter, r *http.Request, code int, msg string) {
	w.WriteHeader(code)
	h.render(w, r, h.tmplError, map[string]any{
		"Page":    "",
		"User":    userEmail(r, h.SigningKey),
		"Code":    code,
		"Message": msg,
	})
}

func (h *Handler) renderWithError(w http.ResponseWriter, r *http.Request, t *template.Template, ns, errMsg string) {
	h.render(w, r, t, map[string]any{
		"Page":      "secrets",
		"User":      userEmail(r, h.SigningKey),
		"Namespace": ns,
		"Error":     errMsg,
	})
}

func userEmail(r *http.Request, signingKey []byte) string {
	c, err := r.Cookie(session.CookieName())
	if err != nil {
		return ""
	}
	data, err := session.Decode(c.Value, signingKey)
	if err != nil {
		return ""
	}
	return data.Email
}

func sessionToClaims(d *session.Data) *authn.Claims {
	extra := make(map[string]interface{}, len(d.Extra)+1)
	for k, v := range d.Extra {
		extra[k] = v
	}
	return &authn.Claims{
		Issuer:  d.Issuer,
		Subject: d.Subject,
		Extra:   extra,
	}
}

func randomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// parseKV parses "KEY=VALUE\nKEY2=VALUE2" into a map.
func parseKV(raw string) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		out[strings.TrimSpace(line[:idx])] = strings.TrimSpace(line[idx+1:])
	}
	return out
}

// resourceToSecret and secretToResource mirror handler/secrets.go for UI-direct store access.

func resourceToSecret(res *store.Resource, enc crypto.Encryptor) (*api.Secret, error) {
	var spec api.SecretSpec
	if err := json.Unmarshal(res.Raw, &spec); err != nil {
		return nil, err
	}
	decrypted := make(map[string]string, len(spec.Data))
	for k, v := range spec.Data {
		pt, err := crypto.DecryptString(enc, v)
		if err != nil {
			return nil, err
		}
		decrypted[k] = pt
	}
	return &api.Secret{
		TypeMeta: api.TypeMeta{APIVersion: api.APIVersion, Kind: "Secret"},
		Metadata: api.ObjectMeta{
			Name:              res.Name,
			Namespace:         res.Namespace,
			UID:               res.UID,
			ResourceVersion:   res.ResourceVersion,
			Labels:            res.Labels,
			Annotations:       res.Annotations,
			CreationTimestamp: res.CreatedAt,
		},
		Spec: api.SecretSpec{Data: decrypted},
	}, nil
}

func secretToResource(s *api.Secret, enc crypto.Encryptor) (*store.Resource, error) {
	encrypted := make(map[string]string, len(s.Spec.Data))
	for k, v := range s.Spec.Data {
		ct, err := crypto.EncryptString(enc, v)
		if err != nil {
			return nil, err
		}
		encrypted[k] = ct
	}
	raw, err := json.Marshal(api.SecretSpec{Data: encrypted})
	if err != nil {
		return nil, err
	}
	return &store.Resource{
		Name:      s.Metadata.Name,
		Namespace: s.Metadata.Namespace,
		Kind:      store.KindSecret,
		Labels:    s.Metadata.Labels,
		Raw:       raw,
	}, nil
}

func resourceToPolicy(res *store.Resource) (*api.Policy, error) {
	var spec api.PolicySpec
	if err := json.Unmarshal(res.Raw, &spec); err != nil {
		return nil, err
	}
	return &api.Policy{
		TypeMeta: api.TypeMeta{APIVersion: api.APIVersion, Kind: "Policy"},
		Metadata: api.ObjectMeta{
			Name:              res.Name,
			Namespace:         res.Namespace,
			UID:               res.UID,
			ResourceVersion:   res.ResourceVersion,
			Labels:            res.Labels,
			Annotations:       res.Annotations,
			CreationTimestamp: res.CreatedAt,
		},
		Spec: spec,
	}, nil
}
