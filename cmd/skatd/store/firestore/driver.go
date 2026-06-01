// Package firestore provides a Cloud Firestore store backend for skatd.
// Import it for its side effect: registering the "firestore" URI scheme.
//
// URI format: firestore://project-id  or  firestore://project-id/database-id
package firestore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/google/uuid"
	"github.com/nicklasfrahm-dev/platform/cmd/skatd/store"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func init() {
	store.Register("firestore", open)
}

func open(ctx context.Context, uri string) (store.Store, error) {
	// Strip scheme: "firestore://project-id/database" → "project-id/database"
	path := strings.TrimPrefix(uri, "firestore://")
	parts := strings.SplitN(path, "/", 2)
	projectID := parts[0]
	if projectID == "" {
		return nil, fmt.Errorf("firestore URI must include a project ID: firestore://project-id")
	}
	databaseID := "(default)"
	if len(parts) == 2 && parts[1] != "" {
		databaseID = parts[1]
	}
	client, err := firestore.NewClientWithDatabase(ctx, projectID, databaseID, []option.ClientOption{}...)
	if err != nil {
		return nil, fmt.Errorf("create firestore client: %w", err)
	}
	return &fsStore{client: client}, nil
}

const collection = "skatd_resources"

// docID produces a stable document ID from the resource triple.
func docID(kind store.ResourceKind, namespace, name string) string {
	return fmt.Sprintf("%s__%s__%s", kind, namespace, name)
}

type fsDoc struct {
	UID             string            `firestore:"uid"`
	Kind            string            `firestore:"kind"`
	Namespace       string            `firestore:"namespace"`
	Name            string            `firestore:"name"`
	ResourceVersion string            `firestore:"resource_version"`
	Labels          map[string]string `firestore:"labels"`
	Annotations     map[string]string `firestore:"annotations"`
	CreatedAt       time.Time         `firestore:"created_at"`
	Raw             []byte            `firestore:"raw"`
}

func docToResource(d *fsDoc) *store.Resource {
	labels := d.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	annotations := d.Annotations
	if annotations == nil {
		annotations = map[string]string{}
	}
	return &store.Resource{
		UID:             d.UID,
		Kind:            store.ResourceKind(d.Kind),
		Namespace:       d.Namespace,
		Name:            d.Name,
		ResourceVersion: d.ResourceVersion,
		Labels:          labels,
		Annotations:     annotations,
		CreatedAt:       d.CreatedAt,
		Raw:             d.Raw,
	}
}

type fsStore struct {
	client *firestore.Client
}

func (s *fsStore) Create(ctx context.Context, r *store.Resource) (*store.Resource, error) {
	id := docID(r.Kind, r.Namespace, r.Name)
	ref := s.client.Collection(collection).Doc(id)

	doc := fsDoc{
		UID:             uuid.New().String(),
		Kind:            string(r.Kind),
		Namespace:       r.Namespace,
		Name:            r.Name,
		ResourceVersion: uuid.New().String(),
		Labels:          r.Labels,
		Annotations:     r.Annotations,
		CreatedAt:       time.Now().UTC(),
		Raw:             r.Raw,
	}

	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if err != nil && status.Code(err) != codes.NotFound {
			return err
		}
		if snap != nil && snap.Exists() {
			return store.ErrAlreadyExists
		}
		return tx.Set(ref, doc)
	})
	if err != nil {
		if errors.Is(err, store.ErrAlreadyExists) {
			return nil, store.ErrAlreadyExists
		}
		return nil, fmt.Errorf("create: %w", err)
	}
	return docToResource(&doc), nil
}

func (s *fsStore) Get(ctx context.Context, kind store.ResourceKind, namespace, name string) (*store.Resource, error) {
	snap, err := s.client.Collection(collection).Doc(docID(kind, namespace, name)).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, store.ErrNotFound
		}
		return nil, fmt.Errorf("get: %w", err)
	}
	var d fsDoc
	if err = snap.DataTo(&d); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return docToResource(&d), nil
}

func (s *fsStore) List(ctx context.Context, kind store.ResourceKind, namespace string) ([]*store.Resource, error) {
	q := s.client.Collection(collection).Where("kind", "==", string(kind))
	if namespace != "" {
		q = q.Where("namespace", "==", namespace)
	}
	iter := q.Documents(ctx)
	defer iter.Stop()

	var out []*store.Resource
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list: %w", err)
		}
		var d fsDoc
		if err = snap.DataTo(&d); err != nil {
			return nil, fmt.Errorf("decode: %w", err)
		}
		out = append(out, docToResource(&d))
	}
	return out, nil
}

func (s *fsStore) Update(ctx context.Context, r *store.Resource) (*store.Resource, error) {
	id := docID(r.Kind, r.Namespace, r.Name)
	ref := s.client.Collection(collection).Doc(id)
	newRV := uuid.New().String()

	var updated fsDoc
	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return store.ErrNotFound
			}
			return err
		}
		var existing fsDoc
		if err = snap.DataTo(&existing); err != nil {
			return err
		}
		if r.ResourceVersion != "" && r.ResourceVersion != existing.ResourceVersion {
			return store.ErrConflict
		}
		updated = fsDoc{
			UID:             existing.UID,
			Kind:            string(r.Kind),
			Namespace:       r.Namespace,
			Name:            r.Name,
			ResourceVersion: newRV,
			Labels:          r.Labels,
			Annotations:     r.Annotations,
			CreatedAt:       existing.CreatedAt,
			Raw:             r.Raw,
		}
		return tx.Set(ref, updated)
	})
	if err != nil {
		return nil, err
	}
	return docToResource(&updated), nil
}

func (s *fsStore) Delete(ctx context.Context, kind store.ResourceKind, namespace, name string) error {
	ref := s.client.Collection(collection).Doc(docID(kind, namespace, name))
	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return store.ErrNotFound
			}
			return err
		}
		if !snap.Exists() {
			return store.ErrNotFound
		}
		return tx.Delete(ref)
	})
	return err
}

// ensure json is used (suppress unused import if Raw is always []byte)
var _ = json.Marshal
