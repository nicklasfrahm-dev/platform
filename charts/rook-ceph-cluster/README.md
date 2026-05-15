# rook-ceph-cluster

Umbrella chart deploying a Rook-managed Ceph cluster with an optional oauth2-proxy OIDC/PKCE gateway in front of the Ceph Dashboard.

## Dependencies

| Chart | Version | Purpose |
|---|---|---|
| [rook-ceph-cluster](https://charts.rook.io/release) | 1.18.7 | Ceph cluster CRDs and daemons |
| [oauth2-proxy](https://oauth2-proxy.github.io/manifests) | 10.4.3 | OIDC/PKCE authentication in front of the Ceph Dashboard (disabled by default) |

## Prerequisites

The Rook operator must be installed in the cluster before deploying this chart.

## oauth2-proxy

The proxy is disabled by default. Enable it per cluster with:

```yaml
oauth2-proxy:
  enabled: true
```

When enabled, two additional sub-features can be opted into under the same key:

| Key | Default | Purpose |
|---|---|---|
| `oauth2-proxy.httpRoute.enabled` | `false` | Expose the proxy via a Gateway API HTTPRoute |
| `oauth2-proxy.ssoSetup.enabled` | `false` | Run a post-install/post-upgrade Job to configure Ceph Dashboard SSO |

### Cookie secret

The oauth2-proxy requires a cookie secret that must be created manually before enabling:

```sh
kubectl create secret generic ceph-dashboard-proxy \
  --from-literal=cookie-secret=$(openssl rand -base64 32 | tr -d '\n') \
  -n <namespace>
```

### SSO setup job

When `oauth2-proxy.ssoSetup.enabled` is `true`, a Helm post-install/post-upgrade Job runs after each sync to:

1. Enable OAuth2 SSO on the Ceph Dashboard (`ceph dashboard sso enable oauth2`)
2. Create or update each user listed in `oauth2-proxy.ssoSetup.adminUsers` with the `administrator` role
3. Remove any dashboard users no longer present in `oauth2-proxy.ssoSetup.adminUsers`

The job image must match the running Ceph cluster version (controlled by `oauth2-proxy.ssoSetup.image`).
