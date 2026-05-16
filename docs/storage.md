# Storage

[rook-ceph](https://rook.io) is the storage operator for the platform. It provides a single control plane for all Ceph storage backends — block (RBD), filesystem (CephFS), and object (RadosGW) — so the same operator manages any future storage type without introducing additional components.

Currently deployed backends:

- **Ceph RBD** — block storage for `ReadWriteOnce` volumes.
- **Rados Gateway (RGW)** — S3-compatible object storage.

CephFS is intentionally not deployed; the platform avoids workloads that require `ReadWriteMany` (RWX) access.

This document covers the cluster configuration and the SSO setup for the Ceph Dashboard.

## Cluster configuration

The cluster is deployed using the `rook-ceph-cluster` Helm chart (`charts/rook-ceph-cluster/`), which wraps the upstream [rook-ceph-cluster](https://rook.io) chart and adds the Dashboard SSO integration described below.

Notable settings in `values.yaml`:

| Setting | Value | Reason |
|---|---|---|
| `cephClusterSpec.network.connections.encryption` | `true` | Encrypts all inter-daemon traffic (msgr2) |
| `cephClusterSpec.network.connections.requireMsgr2` | `true` | Drops plain-text connections |
| `cephClusterSpec.dashboard.ssl` | `false` | TLS is terminated at the gateway; the dashboard listens plain HTTP internally |
| `cephClusterSpec.storage.config.encryptedDevice` | `true` | All OSDs use dm-crypt |
| `cephClusterSpec.removeOSDsIfOutAndSafeToRemove` | `true` | Automates OSD removal after failure |

## Dashboard SSO

### Architecture

```
Browser
  → Gateway API (Cilium/Envoy)
    → oauth2-proxy  ──────────────────────────────────────────┐
        (sidecar: jti-injector on localhost:8080)              │
          → Ceph Dashboard (rook-ceph-mgr-dashboard:7000)      │
                                                               │
        ← redirects to Dex for OIDC login ────────────────────┘
```

All components run in the `platform` namespace. The Ceph Dashboard listens without TLS on port 7000; the gateway handles HTTPS termination.

### Components

**oauth2-proxy** handles OIDC authentication against Dex. It runs in `--reverse-proxy=true` mode, which trusts the gateway's forwarded headers and adds `X-Access-Token` to every upstream request.

**jti-injector** is an [OpenResty](https://openresty.org) (nginx + LuaJIT) sidecar that sits between oauth2-proxy and the Ceph Dashboard. It modifies the JWT before Ceph sees it (see [Why the jti-injector is necessary](#why-the-jti-injector-is-necessary) below).

**SSO setup job** is a Helm post-install/post-upgrade hook that runs `ceph dashboard sso enable oauth2` with a `roles_path` JMESPath expression generated from `oauth2-proxy.ssoSetup.groupRoles`.

### Token flow

1. Browser navigates to the dashboard hostname.
2. oauth2-proxy redirects to Dex for OIDC login.
3. After authentication Dex issues an access token and ID token (both RS256 JWTs).
4. oauth2-proxy completes the flow, stores the session, and forwards requests with the access token as `X-Access-Token`.
5. The jti-injector receives each request, stamps a synthetic `jti` claim and rewrites `sub` to `preferred_username`, then proxies to the Ceph Dashboard.
6. The Ceph Dashboard auth middleware reads `X-Access-Token`, decodes the JWT, and looks up or creates the user.

### Access control

Access is restricted at two levels:

1. **oauth2-proxy `allowedGroups`** — users not in the listed groups are rejected at the proxy with 403 before reaching Ceph.
2. **Ceph `roles_path`** — a JMESPath expression evaluated against the JWT payload assigns Ceph roles. Users who pass the proxy but match no group mapping receive an empty role set and are denied by Ceph on every permission-gated endpoint.

The `groupRoles` values interface generates the `roles_path` expression at deploy time:

```yaml
oauth2-proxy:
  alphaConfig:
    configData:
      providers:
        - allowedGroups:
            - org:platform        # proxy-level gate

  ssoSetup:
    groupRoles:
      - group: org:platform       # full administrators
        roles:
          - administrator
      - group: org:observers      # read-only access
        roles:
          - read-only
```

Available Ceph Dashboard roles: `administrator`, `read-only`, `block-manager`, `rgw-manager`, `cluster-manager`, `pool-manager`, `cephfs-manager`.

---

## Why the jti-injector is necessary

This section documents a compatibility issue between Dex v2.44.0 and Ceph Dashboard v20 that requires the jti-injector sidecar.

### Ceph Dashboard OAuth2 auth middleware

When `ceph dashboard sso enable oauth2` is active, the `AuthManagerTool` in
[`src/pybind/mgr/dashboard/services/auth/auth.py`](https://github.com/ceph/ceph/blob/v20.2.1/src/pybind/mgr/dashboard/services/auth/auth.py)
runs on every protected request:

```python
def _check_authentication(self):
    token = JwtManager.get_token(cherrypy.request)  # reads X-Access-Token header
    if token:
        user = JwtManager.get_user(token)
        if user:
            self._check_authorization(user.username)
            return
    raise cherrypy.HTTPError(401, ...)
```

`JwtManager.get_token()` delegates to `OAuth2.get_token()` (defined in
[`src/pybind/mgr/dashboard/services/auth/oauth2.py`](https://github.com/ceph/ceph/blob/v20.2.1/src/pybind/mgr/dashboard/services/auth/oauth2.py)),
which reads the `token` cookie or the `X-Access-Token` header.

### Signature verification is skipped in OAuth2 mode

`JwtManager.decode()` behaves differently when OAuth2 SSO is active:

```python
def decode(cls, message, secret):
    oauth2_sso_protocol = mgr.SSO_DB.protocol == AuthType.OAUTH2

    if decoded_header['alg'] != cls.JWT_ALGORITHM and not oauth2_sso_protocol:
        raise InvalidAlgorithmError()   # skipped in OAuth2 mode

    if base64_secret != incoming_secret and not oauth2_sso_protocol:
        raise InvalidTokenError()       # skipped in OAuth2 mode

    decoded_message = decode_jwt_segment(base64_message)
    if oauth2_sso_protocol:
        decoded_message['username'] = decoded_message['sub']  # sub → username
```

This means **Ceph accepts any JWT in OAuth2 mode without verifying the signature or algorithm**, which is intentional — it trusts the proxy layer to have already authenticated the token.

### The `jti` requirement breaks Dex tokens

`JwtManager.get_user()` looks up the user only if the `jti` claim is present:

```python
def get_user(cls, token):
    try:
        dtoken = cls.decode_token(token)
        if 'jti' in dtoken and not cls.is_blocklisted(dtoken['jti']):
            user = AuthManager.get_user(dtoken['username'])
            if 'iat' in dtoken and user.last_update <= dtoken['iat']:
                return user
        else:
            cls.logger.debug('Token is block-listed')  # also fires when jti is missing
    except ...
    return None
```

The `jti` (JWT ID) claim exists for token revocation. The `else` branch fires for **both** blocklisted tokens and tokens that simply lack the claim, silently returning `None` and causing a 401 on every request.

Dex v2.44.0 does **not** emit `jti` in either its ID tokens or access tokens. Confirmed by inspecting the Dex source — `jti` only appears in the introspection handler response, never in token generation:

```
$ grep -rn "jti" repos/dex/ --include="*.go" | grep -v "_test.go|vendor"
server/introspectionhandler.go:70: JwtTokenID string `json:"jti,omitempty"`
```

There is no configuration option in Dex to add `jti` to issued tokens.

### Why not SAML?

Ceph Dashboard supports SAML2 (`ceph dashboard sso enable saml2`). SAML would bypass the `jti` problem entirely because Ceph handles the SAML assertion itself and then issues its own Ceph-signed JWT (via `JwtManager.gen_token()`) which does carry `jti`. However:

- Dex cannot act as a SAML IdP — it is an OIDC provider only.
- GitHub does not support SAML for regular accounts (only GitHub Enterprise).

### The jti-injector workaround

Since Ceph skips signature verification in OAuth2 mode, we can modify the JWT payload before it reaches Ceph. The jti-injector sidecar (OpenResty/Lua) intercepts each request from oauth2-proxy and:

1. Decodes the JWT payload (base64url decode, no signature verification needed).
2. Stamps a `jti` claim as `md5(token)` — deterministic, unique per token.
3. Rewrites `sub` to `preferred_username` so Ceph creates human-readable usernames instead of Dex's opaque subject identifiers (e.g. `nicklasfrahm` instead of `CggyMDM4MjMyNhIGZ2l0aHVi`).
4. Re-encodes with the original (now invalid) signature, which Ceph ignores.

The `jti` value does not need to be cryptographically random — its only function here is satisfying the `if 'jti' in dtoken` check. Token revocation (the feature `jti` was designed for) is not used in this setup.

### `api/auth/check` vs regular endpoints

The `api/auth/check` endpoint (used by the Angular frontend on login) is declared on a `secure=False` controller:

```python
@APIRouter('/auth', secure=False)
class Auth(RESTController, ControllerAuthMixin):
    ...
    def check(self, token):  # token comes from POST body
        if mgr.SSO_DB.protocol == AuthType.OAUTH2:
            user = OAuth2.get_user(token)  # calls _create_user() with full JWT claims
```

This endpoint bypasses `AuthManagerTool` entirely and processes the token from the request body. It is how Ceph creates the user on first login (using `sub`, `name`, and `email` from the JWT payload). Regular API endpoints go through `AuthManagerTool` and thus require the `jti` claim.

The jti-injector modifies `X-Access-Token` on **all** requests, so the token that reaches `auth/oauth2/login` (and is subsequently returned to the Angular frontend as `access_token` in the redirect URL) is already the modified version. The frontend then passes this modified token as the body to `api/auth/check`, ensuring user creation also uses `preferred_username` as `sub`.
