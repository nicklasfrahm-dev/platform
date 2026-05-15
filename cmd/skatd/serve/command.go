// Package serve implements the "serve" Cobra subcommand for skatd.
package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/nicklasfrahm-dev/platform/cmd/skatd/api"
	"github.com/nicklasfrahm-dev/platform/cmd/skatd/authn"
	"github.com/nicklasfrahm-dev/platform/cmd/skatd/authz"
	"github.com/nicklasfrahm-dev/platform/cmd/skatd/crypto"
	"github.com/nicklasfrahm-dev/platform/cmd/skatd/handler"
	"github.com/nicklasfrahm-dev/platform/cmd/skatd/store"
	"github.com/nicklasfrahm-dev/platform/cmd/skatd/ui"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	// Register storage backends.
	_ "github.com/nicklasfrahm-dev/platform/cmd/skatd/store/firestore"
	_ "github.com/nicklasfrahm-dev/platform/cmd/skatd/store/memory"
	_ "github.com/nicklasfrahm-dev/platform/cmd/skatd/store/postgres"
)

type config struct {
	OIDCIssuerURL   string
	OIDCClientID    string
	SessionSecret   string
	DatabaseURI     string
	DefaultPolicies string
	EncryptionKey   string
	Port            string
	ExternalURL     string
}

func loadConfig() (*config, error) {
	cfg := &config{
		OIDCIssuerURL:   os.Getenv("SKATD_OIDC_ISSUER_URL"),
		OIDCClientID:    os.Getenv("SKATD_OIDC_CLIENT_ID"),
		SessionSecret:   os.Getenv("SKATD_SESSION_SECRET"),
		DatabaseURI:     os.Getenv("SKATD_DATABASE_URI"),
		DefaultPolicies: os.Getenv("SKATD_DEFAULT_POLICIES"),
		EncryptionKey:   os.Getenv("SKATD_ENCRYPTION_KEY"),
		Port:            os.Getenv("SKATD_PORT"),
		ExternalURL:     os.Getenv("SKATD_EXTERNAL_URL"),
	}
	if cfg.OIDCIssuerURL == "" {
		return nil, fmt.Errorf("SKATD_OIDC_ISSUER_URL is required")
	}
	if cfg.OIDCClientID == "" {
		return nil, fmt.Errorf("SKATD_OIDC_CLIENT_ID is required")
	}
	if cfg.SessionSecret == "" {
		return nil, fmt.Errorf("SKATD_SESSION_SECRET is required")
	}
	if cfg.DatabaseURI == "" {
		cfg.DatabaseURI = "memory://"
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	return cfg, nil
}

// RootCommand returns the "serve" Cobra command.
func RootCommand(logger *zap.Logger) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the skatd HTTP server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(cmd.Context(), logger)
		},
	}
}

func run(ctx context.Context, logger *zap.Logger) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	// Storage backend.
	str, err := store.Open(ctx, cfg.DatabaseURI)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	logger.Info("storage backend opened", zap.String("uri", cfg.DatabaseURI))

	// Encryption.
	var enc crypto.Encryptor
	if cfg.EncryptionKey != "" {
		enc, err = crypto.NewAES256GCM(cfg.EncryptionKey)
		if err != nil {
			return fmt.Errorf("create encryptor: %w", err)
		}
		logger.Info("encryption enabled")
	} else {
		enc = crypto.NewNoop()
		logger.Warn("ENCRYPTION_KEY not set; secret values stored in plaintext")
	}

	// Authz engine (starts empty; reconcile will populate it).
	eng := authz.New(nil)

	// Reconcile default policies before serving traffic.
	if err = reconcileDefaultPolicies(ctx, str, eng, cfg.DefaultPolicies, logger); err != nil {
		return fmt.Errorf("reconcile default policies: %w", err)
	}

	// Handlers.
	sh := &handler.SecretsHandler{Store: str, Enc: enc}
	ph := &handler.PoliciesHandler{Store: str, Engine: eng}

	authnMW := authn.Middleware(cfg.OIDCIssuerURL, cfg.OIDCClientID)

	// Determine callback URL for PKCE.
	externalURL := cfg.ExternalURL
	if externalURL == "" {
		externalURL = "http://localhost:" + cfg.Port
	}
	callbackURL := externalURL + "/ui/callback"

	// UI handler.
	uiH := &ui.Handler{
		Store:       str,
		Enc:         enc,
		Engine:      eng,
		OIDCIssuer:  cfg.OIDCIssuerURL,
		ClientID:    cfg.OIDCClientID,
		RedirectURL: callbackURL,
		SigningKey:  []byte(cfg.SessionSecret),
	}
	if err = uiH.Init(ctx); err != nil {
		return fmt.Errorf("init UI handler: %w", err)
	}

	mux := http.NewServeMux()

	// Discovery (no auth required).
	mux.HandleFunc("GET /api", handler.APIVersions)
	mux.HandleFunc("GET /api/v1", handler.CoreResourceList)
	mux.HandleFunc("GET /apis", handler.APIGroupList)
	mux.HandleFunc("GET /apis/skatd.io/v1", handler.APIResourceListV1)

	// Secrets (authn + authz per verb).
	mux.Handle("GET /apis/skatd.io/v1/namespaces/{ns}/secrets",
		chain(http.HandlerFunc(sh.List), authnMW, authz.Check(eng, "list", "secrets")))
	mux.Handle("POST /apis/skatd.io/v1/namespaces/{ns}/secrets",
		chain(http.HandlerFunc(sh.Create), authnMW, authz.Check(eng, "create", "secrets")))
	mux.Handle("GET /apis/skatd.io/v1/namespaces/{ns}/secrets/{name}",
		chain(http.HandlerFunc(sh.Get), authnMW, authz.Check(eng, "get", "secrets")))
	mux.Handle("PUT /apis/skatd.io/v1/namespaces/{ns}/secrets/{name}",
		chain(http.HandlerFunc(sh.Update), authnMW, authz.Check(eng, "update", "secrets")))
	mux.Handle("DELETE /apis/skatd.io/v1/namespaces/{ns}/secrets/{name}",
		chain(http.HandlerFunc(sh.Delete), authnMW, authz.Check(eng, "delete", "secrets")))

	// Policies (authn + authz per verb).
	mux.Handle("GET /apis/skatd.io/v1/namespaces/{ns}/policies",
		chain(http.HandlerFunc(ph.List), authnMW, authz.Check(eng, "list", "policies")))
	mux.Handle("POST /apis/skatd.io/v1/namespaces/{ns}/policies",
		chain(http.HandlerFunc(ph.Create), authnMW, authz.Check(eng, "create", "policies")))
	mux.Handle("GET /apis/skatd.io/v1/namespaces/{ns}/policies/{name}",
		chain(http.HandlerFunc(ph.Get), authnMW, authz.Check(eng, "get", "policies")))
	mux.Handle("PUT /apis/skatd.io/v1/namespaces/{ns}/policies/{name}",
		chain(http.HandlerFunc(ph.Update), authnMW, authz.Check(eng, "update", "policies")))
	mux.Handle("DELETE /apis/skatd.io/v1/namespaces/{ns}/policies/{name}",
		chain(http.HandlerFunc(ph.Delete), authnMW, authz.Check(eng, "delete", "policies")))

	// UI (session-cookie auth managed internally).
	mux.Handle("/ui", uiH)
	mux.Handle("/ui/", uiH)

	addr := ":" + cfg.Port
	logger.Info("skatd listening", zap.String("addr", addr))
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	return srv.ListenAndServe()
}

// chain applies middlewares in left-to-right order (outermost first).
func chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// reconcileDefaultPolicies applies the DEFAULT_POLICIES env var declaratively.
// Policies bearing the label "skatd.io/default: true" are owned by this reconciler:
// created when absent, updated when changed, deleted when removed from the env var.
// Manually created policies (no label) are never touched.
func reconcileDefaultPolicies(ctx context.Context, str store.Store, eng authz.Engine, rawJSON string, logger *zap.Logger) error {
	var desired []api.Policy
	if rawJSON != "" {
		if err := json.Unmarshal([]byte(rawJSON), &desired); err != nil {
			return fmt.Errorf("parse DEFAULT_POLICIES: %w", err)
		}
	}

	// Inject the managed label into every desired policy.
	for i := range desired {
		if desired[i].Metadata.Labels == nil {
			desired[i].Metadata.Labels = make(map[string]string)
		}
		desired[i].Metadata.Labels["skatd.io/default"] = "true"
	}

	// List all stored policies (all namespaces) and find existing default ones.
	allResources, err := str.List(ctx, store.KindPolicy, "")
	if err != nil {
		return err
	}
	existing := make(map[string]*store.Resource)
	for _, r := range allResources {
		if r.Labels["skatd.io/default"] == "true" {
			existing[r.Namespace+"/"+r.Name] = r
		}
	}

	// Create or update desired policies.
	desiredKeys := make(map[string]bool, len(desired))
	for _, p := range desired {
		key := p.Metadata.Namespace + "/" + p.Metadata.Name
		desiredKeys[key] = true

		desiredSpecRaw, marshalErr := json.Marshal(p.Spec)
		if marshalErr != nil {
			return marshalErr
		}

		if ex, ok := existing[key]; ok {
			// Update only if spec changed.
			var storedSpec api.PolicySpec
			if unmarshalErr := json.Unmarshal(ex.Raw, &storedSpec); unmarshalErr != nil {
				return unmarshalErr
			}
			storedRaw, _ := json.Marshal(storedSpec)
			if !bytes.Equal(storedRaw, desiredSpecRaw) {
				if _, updateErr := str.Update(ctx, &store.Resource{
					Name:            p.Metadata.Name,
					Namespace:       p.Metadata.Namespace,
					Kind:            store.KindPolicy,
					ResourceVersion: ex.ResourceVersion,
					Labels:          p.Metadata.Labels,
					Annotations:     p.Metadata.Annotations,
					Raw:             desiredSpecRaw,
				}); updateErr != nil {
					return updateErr
				}
				logger.Info("updated default policy", zap.String("name", p.Metadata.Name))
			}
		} else {
			if _, createErr := str.Create(ctx, &store.Resource{
				Name:        p.Metadata.Name,
				Namespace:   p.Metadata.Namespace,
				Kind:        store.KindPolicy,
				Labels:      p.Metadata.Labels,
				Annotations: p.Metadata.Annotations,
				Raw:         desiredSpecRaw,
			}); createErr != nil {
				return createErr
			}
			logger.Info("created default policy", zap.String("name", p.Metadata.Name))
		}
	}

	// Delete default policies no longer in the desired set.
	for key, r := range existing {
		if !desiredKeys[key] {
			if deleteErr := str.Delete(ctx, store.KindPolicy, r.Namespace, r.Name); deleteErr != nil {
				return deleteErr
			}
			logger.Info("deleted default policy", zap.String("name", r.Name))
		}
	}

	// Reload the engine with the full current policy set.
	return reloadEngine(ctx, str, eng)
}

// reloadEngine lists all policies and reloads the authz engine.
func reloadEngine(ctx context.Context, str store.Store, eng authz.Engine) error {
	resources, err := str.List(ctx, store.KindPolicy, "")
	if err != nil {
		return err
	}
	policies := make([]api.Policy, 0, len(resources))
	for _, res := range resources {
		var spec api.PolicySpec
		if err = json.Unmarshal(res.Raw, &spec); err != nil {
			return err
		}
		policies = append(policies, api.Policy{
			TypeMeta: api.TypeMeta{APIVersion: api.APIVersion, Kind: "Policy"},
			Metadata: api.ObjectMeta{
				Name:      res.Name,
				Namespace: res.Namespace,
				UID:       res.UID,
				Labels:    res.Labels,
			},
			Spec: spec,
		})
	}
	eng.Reload(policies)
	return nil
}
