# Gateway Helm Chart

A Helm chart for deploying a Kubernetes Gateway API Gateway with automatic TLS certificate management using cert-manager.

## Overview

This chart deploys a Kubernetes Gateway resource that manages ingress traffic using the Gateway API. It includes:

- **Gateway**: A Kubernetes Gateway resource with HTTP and HTTPS listeners
- **Certificate Issuers**: cert-manager Issuer/ClusterIssuer resources for automatic TLS certificate provisioning
- **Certificates**: cert-manager Certificate resources for TLS termination

## Prerequisites

- Kubernetes cluster with Gateway API CRDs installed
- A Gateway API-compatible ingress controller (e.g., Cilium, Istio, Envoy Gateway)
- cert-manager installed in the cluster
- Helm 3.0+

## Installation

### Install the chart

```bash
helm install my-gateway oci://ghcr.io/nicklasfrahm-dev/charts/gateway -n kube-system
```

### Install with custom values

```bash
helm install my-gateway oci://ghcr.io/nicklasfrahm-dev/charts/gateway -n kube-system -f my-values.yaml
```

## Configuration

### Gateway Configuration

> The `Gateway` (and its supporting `EnvoyProxy`/`ClientTrafficPolicy` resources) take their name from the Helm release name, e.g. `helm install my-gateway ...` creates a `Gateway` named `my-gateway`. Reference that name in `parentRefs` of `HTTPRoute` resources.

| Parameter                 | Description                                     | Default                  |
| ------------------------- | ----------------------------------------------- | ------------------------ |
| `gateway.className`       | Gateway class name                              | `cilium`                 |
| `gateway.tls.enabled`     | Enable HTTPS listener and TLS certificates      | `true`                   |
| `gateway.tls.issuer.kind` | cert-manager issuer kind (Issuer/ClusterIssuer) | `Issuer`                 |
| `gateway.tls.issuer.name` | cert-manager issuer name                        | `letsencrypt-production` |
| `gateway.hostnames`       | List of hostnames for the gateway               | `["example.com"]`        |

### Certificate Issuers Configuration

| Parameter                               | Description                            | Default                                                  |
| --------------------------------------- | -------------------------------------- | -------------------------------------------------------- |
| `issuers.letsEncryptProduction.enabled` | Enable Let's Encrypt production issuer | `true`                                                   |
| `issuers.letsEncryptProduction.kind`    | Issuer type (Issuer/ClusterIssuer)     | `Issuer`                                                 |
| `issuers.letsEncryptProduction.name`    | Issuer name                            | `letsencrypt-production`                                 |
| `issuers.letsEncryptProduction.server`  | ACME server URL                        | `https://acme-v02.api.letsencrypt.org/directory`         |
| `issuers.letsEncryptStaging.enabled`    | Enable Let's Encrypt staging issuer    | `false`                                                  |
| `issuers.letsEncryptStaging.kind`       | Issuer type (Issuer/ClusterIssuer)     | `Issuer`                                                 |
| `issuers.letsEncryptStaging.name`       | Issuer name                            | `letsencrypt-staging`                                    |
| `issuers.letsEncryptStaging.server`     | ACME server URL                        | `https://acme-staging-v02.api.letsencrypt.org/directory` |

## Example Values

### Basic Configuration

```yaml
gateway:
  className: "cilium"
  hostnames:
    - "api.example.com"
    - "app.example.com"
```

### Using Staging Environment

```yaml
gateway:
  className: "cilium"
  tls:
    enabled: true
    issuer:
      kind: "Issuer"
      name: "letsencrypt-staging"
  hostnames:
    - "staging.example.com"

issuers:
  letsEncryptProduction:
    enabled: false
  letsEncryptStaging:
    enabled: true
```

### Multiple Hostnames with ClusterIssuer

```yaml
gateway:
  className: "istio"
  tls:
    enabled: true
    issuer:
      kind: "ClusterIssuer"
      name: "cluster-letsencrypt"
  hostnames:
    - "api.example.com"
    - "www.example.com"
    - "admin.example.com"

issuers:
  letsEncryptProduction:
    enabled: true
    kind: "ClusterIssuer"
    name: "cluster-letsencrypt"
```

## Usage

After installing the chart, you can create HTTPRoute resources to route traffic through the gateway:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-app-route
  namespace: my-app
spec:
  parentRefs:
    - name: my-gateway
      namespace: kube-system
  hostnames:
    - "api.example.com"
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: my-app-service
          port: 80
```

## TLS Certificate Management

When TLS is enabled, the chart automatically:

1. Creates cert-manager Certificate resources for each hostname
2. Configures the Gateway to use these certificates for HTTPS termination
3. Sets up HTTP-01 challenge solving through the Gateway

Certificate secrets are named using the pattern: `{hostname-with-dots-replaced-by-dashes}-tls`

For example:

- `api.example.com` → `api-example-com-tls`
- `www.example.com` → `www-example-com-tls`

## Troubleshooting

### Check Gateway Status

```bash
kubectl get gateway -A
kubectl describe gateway <gateway-name>
```

### Check Certificate Status

```bash
kubectl get certificates
kubectl describe certificate <cert-name>
```

### Check Certificate Issuer Status

```bash
kubectl get issuer
kubectl describe issuer <issuer-name>
```

### Common Issues

1. **Gateway not ready**: Check if the Gateway class controller is running
2. **Certificate not issued**: Verify the issuer is properly configured and the HTTP-01 challenge can be completed
3. **DNS issues**: Ensure hostnames resolve to the Gateway's external IP

## Uninstalling

```bash
helm uninstall my-gateway -n kube-system
```
