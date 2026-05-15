# skatd

A storage-agnostic secret management server. The name comes from the Danish word *skat* (treasure) and *d* for daemon.

The API follows Kubernetes conventions — resources have `apiVersion`, `kind`, `metadata`, `spec`, and `status` fields — so `kubectl` can be pointed at it directly.

## Concepts

### Secrets

Key-value pairs stored under a namespace and name. Values are optionally encrypted at rest with AES-256-GCM when `SKATD_ENCRYPTION_KEY` is set.

```json
{
  "apiVersion": "skatd.io/v1",
  "kind": "Secret",
  "metadata": { "name": "my-secret", "namespace": "default" },
  "spec": {
    "data": {
      "DB_PASSWORD": "hunter2",
      "API_KEY": "abc123"
    }
  }
}
```

### Policies

> **TODO:** Replace the claims-map matching model with [CEL](https://cel.dev) expressions, giving full programmatic control over subject matching and access rules (e.g. `claims.groups.exists(g, g == 'admins')`). `SKATD_DEFAULT_POLICIES` should accept newline-separated JSON objects (one policy per line) rather than a JSON array, to make it easier to compose policies in multi-line env vars and config maps.

Policies grant access by binding OIDC subjects (identified by issuer + JWT claims) to a set of verbs and resources.

```json
{
  "apiVersion": "skatd.io/v1",
  "kind": "Policy",
  "metadata": { "name": "admin", "namespace": "default" },
  "spec": {
    "subjects": [
      {
        "issuer": "https://auth.example.com",
        "claims": { "email": "alice@example.com" }
      }
    ],
    "rules": [
      { "verbs": ["*"], "resources": ["*"] }
    ]
  }
}
```

Supported verbs: `get`, `list`, `create`, `update`, `delete`, `*`.  
Supported resources: `secrets`, `policies`, `*`.  
Omitting `namespaces` in a rule matches all namespaces.

## Configuration

All configuration is via environment variables.

| Variable | Required | Default | Description |
|---|---|---|---|
| `SKATD_OIDC_ISSUER_URL` | Yes | — | OIDC provider issuer URL |
| `SKATD_OIDC_CLIENT_ID` | Yes | — | OAuth2 client ID (used for Bearer token `aud` validation and PKCE UI flow) |
| `SKATD_SESSION_SECRET` | Yes | — | HMAC signing key for UI session cookies |
| `SKATD_DATABASE_URI` | No | `memory://` | Storage backend URI (see [Storage](#storage)) |
| `SKATD_DEFAULT_POLICIES` | No | `[]` | JSON array of `Policy` objects reconciled on every startup |
| `SKATD_ENCRYPTION_KEY` | No | — | Passphrase for AES-256-GCM encryption of secret values at rest. **TODO:** support key slots for rotation: comma-separated `<slot>:<base64-key>` pairs (e.g. `2:dGhlIG5ld...,1:dGhlIG9sZ...`); the highest slot number is used for new encryptions, all slots are tried for decryption. |
| `SKATD_EXTERNAL_URL` | No | `http://localhost:8080` | Public base URL, used to construct the PKCE redirect URI |
| `SKATD_PORT` | No | `8080` | HTTP listen port |

## Storage

The backend is selected from the scheme of `SKATD_DATABASE_URI`.

| URI | Backend |
|---|---|
| `memory://` | In-memory (default, data lost on restart) |
| `postgres://user:pass@host/db` | PostgreSQL (table created automatically on first start) |
| `firestore://project-id` | Cloud Firestore (`firestore://project-id/database-id` for non-default databases) |

## Bootstrap policies

`SKATD_DEFAULT_POLICIES` accepts a JSON array of `Policy` objects. On every startup skatd reconciles the store against this list:

- Policies in the list that are absent in the store are **created**.
- Policies in the list whose spec has changed are **updated**.
- Stored policies that are no longer in the list are **deleted**.

Reconciled policies are labeled `skatd.io/default: "true"`. Manually created policies are never touched.

Example — grant a single user full access:

```bash
export SKATD_DEFAULT_POLICIES='[{
  "apiVersion": "skatd.io/v1",
  "kind": "Policy",
  "metadata": {"name": "admin", "namespace": "default"},
  "spec": {
    "subjects": [{"issuer": "https://auth.example.com", "claims": {"email": "you@example.com"}}],
    "rules": [{"verbs": ["*"], "resources": ["*"]}]
  }
}]'
```

## API

Resources live under the `skatd.io/v1` API group and are namespace-scoped.

```
GET    /apis/skatd.io/v1/namespaces/{ns}/secrets
POST   /apis/skatd.io/v1/namespaces/{ns}/secrets
GET    /apis/skatd.io/v1/namespaces/{ns}/secrets/{name}
PUT    /apis/skatd.io/v1/namespaces/{ns}/secrets/{name}
DELETE /apis/skatd.io/v1/namespaces/{ns}/secrets/{name}

GET    /apis/skatd.io/v1/namespaces/{ns}/policies
POST   /apis/skatd.io/v1/namespaces/{ns}/policies
GET    /apis/skatd.io/v1/namespaces/{ns}/policies/{name}
PUT    /apis/skatd.io/v1/namespaces/{ns}/policies/{name}
DELETE /apis/skatd.io/v1/namespaces/{ns}/policies/{name}
```

All API requests require a Bearer OIDC token:

```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/apis/skatd.io/v1/namespaces/default/secrets
```

### kubectl

API discovery endpoints (`/api`, `/apis`, `/apis/skatd.io/v1`) are implemented, so kubectl works with a kubeconfig pointing at skatd:

```bash
kubectl --server=http://localhost:8080 --token="$TOKEN" \
  get secrets.skatd.io -n default
```

## Web UI

The UI is served at `/ui` and authenticates via OIDC PKCE (no client secret required). It provides a browser interface for listing, creating, and deleting secrets and viewing policies.

The PKCE state is stored in a short-lived signed cookie so no server-side session storage is needed — compatible with stateless deployments such as Cloud Run.

## Local development

Copy the example env vars, fill in your OIDC provider details, then start the hot-reload server:

```bash
cat > .env <<'EOF'
SKATD_OIDC_ISSUER_URL=https://auth.nicklasfrahm.dev
SKATD_OIDC_CLIENT_ID=skatd
SKATD_SESSION_SECRET=change-me-to-something-long-and-random
SKATD_EXTERNAL_URL=http://localhost:8080
EOF

make dev
```

Air watches all `.go` and `.html` files and rebuilds on change. The server is available at `http://localhost:8080`.

## Container image

The `Containerfile` produces a minimal image using a two-stage build:

```
builder  →  golang:1.26.3-trixie
runtime  →  gcr.io/distroless/static-debian13:nonroot
```

Build manually:

```bash
docker build -f cmd/skatd/Containerfile -t skatd:dev .
docker run --env-file .env -p 8080:8080 skatd:dev
```
