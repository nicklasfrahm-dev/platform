// Package postgres provides a PostgreSQL store backend for skatd.
// Import it for its side effect: registering the "postgres" and "postgresql" URI schemes.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nicklasfrahm-dev/platform/cmd/skatd/store"
)

func init() {
	store.Register("postgres", open)
	store.Register("postgresql", open)
}

func open(ctx context.Context, uri string) (store.Store, error) {
	pool, err := pgxpool.New(ctx, uri)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err = pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	s := &pgStore{pool: pool}
	if err = s.migrate(ctx); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

type pgStore struct {
	pool *pgxpool.Pool
}

const ddl = `
CREATE TABLE IF NOT EXISTS skatd_resources (
    uid              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    kind             TEXT NOT NULL,
    namespace        TEXT NOT NULL,
    name             TEXT NOT NULL,
    resource_version TEXT NOT NULL DEFAULT gen_random_uuid()::text,
    labels           JSONB NOT NULL DEFAULT '{}',
    annotations      JSONB NOT NULL DEFAULT '{}',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    raw              BYTEA NOT NULL,
    UNIQUE (kind, namespace, name)
);`

func (s *pgStore) migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, ddl)
	return err
}

func (s *pgStore) Create(ctx context.Context, r *store.Resource) (*store.Resource, error) {
	labelsJSON, _ := json.Marshal(r.Labels)
	annotationsJSON, _ := json.Marshal(r.Annotations)
	row := s.pool.QueryRow(ctx, `
		INSERT INTO skatd_resources (kind, namespace, name, labels, annotations, raw)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING uid, resource_version, created_at`,
		string(r.Kind), r.Namespace, r.Name,
		string(labelsJSON), string(annotationsJSON), r.Raw,
	)
	var uid, rv string
	var createdAt time.Time
	if err := row.Scan(&uid, &rv, &createdAt); err != nil {
		if isUniqueViolation(err) {
			return nil, store.ErrAlreadyExists
		}
		return nil, fmt.Errorf("create: %w", err)
	}
	cp := copyResource(r)
	cp.UID = uid
	cp.ResourceVersion = rv
	cp.CreatedAt = createdAt
	return cp, nil
}

func (s *pgStore) Get(ctx context.Context, kind store.ResourceKind, namespace, name string) (*store.Resource, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT uid, resource_version, labels, annotations, created_at, raw
		FROM skatd_resources WHERE kind=$1 AND namespace=$2 AND name=$3`,
		string(kind), namespace, name,
	)
	return scanRow(kind, namespace, name, row)
}

func (s *pgStore) List(ctx context.Context, kind store.ResourceKind, namespace string) ([]*store.Resource, error) {
	var rows pgx.Rows
	var err error
	if namespace == "" {
		rows, err = s.pool.Query(ctx, `
			SELECT uid, resource_version, labels, annotations, created_at, raw, namespace, name
			FROM skatd_resources WHERE kind=$1 ORDER BY namespace, name`, string(kind))
	} else {
		rows, err = s.pool.Query(ctx, `
			SELECT uid, resource_version, labels, annotations, created_at, raw, namespace, name
			FROM skatd_resources WHERE kind=$1 AND namespace=$2 ORDER BY name`,
			string(kind), namespace)
	}
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	defer rows.Close()

	var out []*store.Resource
	for rows.Next() {
		var (
			uid, rv, ns, n        string
			labelsJSON, annJSON   []byte
			createdAt             time.Time
			raw                   []byte
		)
		if err = rows.Scan(&uid, &rv, &labelsJSON, &annJSON, &createdAt, &raw, &ns, &n); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		res := &store.Resource{
			UID: uid, Kind: kind, Namespace: ns, Name: n,
			ResourceVersion: rv, CreatedAt: createdAt, Raw: raw,
		}
		_ = json.Unmarshal(labelsJSON, &res.Labels)
		_ = json.Unmarshal(annJSON, &res.Annotations)
		out = append(out, res)
	}
	return out, rows.Err()
}

func (s *pgStore) Update(ctx context.Context, r *store.Resource) (*store.Resource, error) {
	labelsJSON, _ := json.Marshal(r.Labels)
	annotationsJSON, _ := json.Marshal(r.Annotations)

	var query string
	var args []any
	if r.ResourceVersion != "" {
		query = `UPDATE skatd_resources
			SET resource_version=gen_random_uuid()::text, labels=$1, annotations=$2, raw=$3
			WHERE kind=$4 AND namespace=$5 AND name=$6 AND resource_version=$7
			RETURNING uid, resource_version, created_at`
		args = []any{string(labelsJSON), string(annotationsJSON), r.Raw,
			string(r.Kind), r.Namespace, r.Name, r.ResourceVersion}
	} else {
		query = `UPDATE skatd_resources
			SET resource_version=gen_random_uuid()::text, labels=$1, annotations=$2, raw=$3
			WHERE kind=$4 AND namespace=$5 AND name=$6
			RETURNING uid, resource_version, created_at`
		args = []any{string(labelsJSON), string(annotationsJSON), r.Raw,
			string(r.Kind), r.Namespace, r.Name}
	}

	row := s.pool.QueryRow(ctx, query, args...)
	var uid, rv string
	var createdAt time.Time
	if err := row.Scan(&uid, &rv, &createdAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Could be not-found or version conflict; check existence.
			var exists bool
			_ = s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM skatd_resources WHERE kind=$1 AND namespace=$2 AND name=$3)`,
				string(r.Kind), r.Namespace, r.Name).Scan(&exists)
			if !exists {
				return nil, store.ErrNotFound
			}
			return nil, store.ErrConflict
		}
		return nil, fmt.Errorf("update: %w", err)
	}
	cp := copyResource(r)
	cp.UID = uid
	cp.ResourceVersion = rv
	cp.CreatedAt = createdAt
	return cp, nil
}

func (s *pgStore) Delete(ctx context.Context, kind store.ResourceKind, namespace, name string) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM skatd_resources WHERE kind=$1 AND namespace=$2 AND name=$3`,
		string(kind), namespace, name,
	)
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func scanRow(kind store.ResourceKind, namespace, name string, row pgx.Row) (*store.Resource, error) {
	var uid, rv string
	var labelsJSON, annJSON, raw []byte
	var createdAt time.Time
	if err := row.Scan(&uid, &rv, &labelsJSON, &annJSON, &createdAt, &raw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, fmt.Errorf("scan: %w", err)
	}
	res := &store.Resource{
		UID: uid, Kind: kind, Namespace: namespace, Name: name,
		ResourceVersion: rv, CreatedAt: createdAt, Raw: raw,
	}
	_ = json.Unmarshal(labelsJSON, &res.Labels)
	_ = json.Unmarshal(annJSON, &res.Annotations)
	return res, nil
}

func isUniqueViolation(err error) bool {
	return err != nil && (contains(err.Error(), "unique") || contains(err.Error(), "23505"))
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
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
