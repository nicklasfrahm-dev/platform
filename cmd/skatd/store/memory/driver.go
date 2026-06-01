// Package memory provides an in-memory store backend for skatd.
// Import it for its side effect: registering the "memory" URI scheme.
package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nicklasfrahm-dev/platform/cmd/skatd/store"
)

func init() {
	store.Register("memory", func(_ context.Context, _ string) (store.Store, error) {
		return &memStore{items: make(map[string]*store.Resource)}, nil
	})
}

type memStore struct {
	mu    sync.RWMutex
	items map[string]*store.Resource
}

func key(kind store.ResourceKind, namespace, name string) string {
	return fmt.Sprintf("%s/%s/%s", kind, namespace, name)
}

func (s *memStore) Create(_ context.Context, r *store.Resource) (*store.Resource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(r.Kind, r.Namespace, r.Name)
	if _, exists := s.items[k]; exists {
		return nil, store.ErrAlreadyExists
	}
	cp := copyResource(r)
	cp.UID = uuid.New().String()
	cp.ResourceVersion = uuid.New().String()
	cp.CreatedAt = time.Now().UTC()
	s.items[k] = cp
	return copyResource(cp), nil
}

func (s *memStore) Get(_ context.Context, kind store.ResourceKind, namespace, name string) (*store.Resource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.items[key(kind, namespace, name)]
	if !ok {
		return nil, store.ErrNotFound
	}
	return copyResource(r), nil
}

func (s *memStore) List(_ context.Context, kind store.ResourceKind, namespace string) ([]*store.Resource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*store.Resource
	for _, r := range s.items {
		if r.Kind != kind {
			continue
		}
		if namespace != "" && r.Namespace != namespace {
			continue
		}
		out = append(out, copyResource(r))
	}
	return out, nil
}

func (s *memStore) Update(_ context.Context, r *store.Resource) (*store.Resource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(r.Kind, r.Namespace, r.Name)
	existing, ok := s.items[k]
	if !ok {
		return nil, store.ErrNotFound
	}
	if r.ResourceVersion != "" && r.ResourceVersion != existing.ResourceVersion {
		return nil, store.ErrConflict
	}
	cp := copyResource(r)
	cp.UID = existing.UID
	cp.CreatedAt = existing.CreatedAt
	cp.ResourceVersion = uuid.New().String()
	s.items[k] = cp
	return copyResource(cp), nil
}

func (s *memStore) Delete(_ context.Context, kind store.ResourceKind, namespace, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(kind, namespace, name)
	if _, ok := s.items[k]; !ok {
		return store.ErrNotFound
	}
	delete(s.items, k)
	return nil
}

func copyResource(r *store.Resource) *store.Resource {
	cp := *r
	if r.Labels != nil {
		cp.Labels = make(map[string]string, len(r.Labels))
		for k, v := range r.Labels {
			cp.Labels[k] = v
		}
	}
	if r.Annotations != nil {
		cp.Annotations = make(map[string]string, len(r.Annotations))
		for k, v := range r.Annotations {
			cp.Annotations[k] = v
		}
	}
	if r.Raw != nil {
		cp.Raw = make([]byte, len(r.Raw))
		copy(cp.Raw, r.Raw)
	}
	return &cp
}
