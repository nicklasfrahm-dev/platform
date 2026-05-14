package store

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"
)

// ResourceKind identifies the type of resource stored.
type ResourceKind string

const (
	KindSecret ResourceKind = "secrets"
	KindPolicy ResourceKind = "policies"
)

// Resource is the generic envelope persisted by backends.
// Raw holds the JSON-serialised spec; metadata fields are stored separately.
type Resource struct {
	UID             string
	Name            string
	Namespace       string
	Kind            ResourceKind
	ResourceVersion string
	Labels          map[string]string
	Annotations     map[string]string
	CreatedAt       time.Time
	Raw             []byte
}

// Store is the storage backend abstraction.
type Store interface {
	// Create persists a new resource. Returns ErrAlreadyExists if a resource
	// with the same kind/namespace/name already exists.
	Create(ctx context.Context, r *Resource) (*Resource, error)
	// Get retrieves a single resource by kind/namespace/name.
	// Returns ErrNotFound if absent.
	Get(ctx context.Context, kind ResourceKind, namespace, name string) (*Resource, error)
	// List retrieves all resources of the given kind in a namespace.
	// An empty namespace lists across all namespaces.
	List(ctx context.Context, kind ResourceKind, namespace string) ([]*Resource, error)
	// Update replaces a resource. If ResourceVersion is non-empty it must match
	// the stored value; otherwise ErrConflict is returned.
	Update(ctx context.Context, r *Resource) (*Resource, error)
	// Delete removes a resource. Returns ErrNotFound if absent.
	Delete(ctx context.Context, kind ResourceKind, namespace, name string) error
}

// DriverFactory creates a Store from a URI string.
type DriverFactory func(ctx context.Context, uri string) (Store, error)

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
	ErrConflict      = errors.New("resource version conflict")
)

var drivers = map[string]DriverFactory{}

// Register associates a URI scheme with a factory. Called from driver init().
func Register(scheme string, f DriverFactory) {
	drivers[scheme] = f
}

// Open selects a backend based on the URI scheme and delegates to its factory.
func Open(ctx context.Context, uri string) (Store, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("invalid database URI: %w", err)
	}
	f, ok := drivers[u.Scheme]
	if !ok {
		return nil, fmt.Errorf("unknown database scheme %q; did you import the driver?", u.Scheme)
	}
	return f(ctx, uri)
}
