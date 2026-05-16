# envoy-gateway Helm Chart

A Helm umbrella chart for deploying Envoy Gateway as a Kubernetes-native API gateway.

## Overview

This chart deploys Envoy Gateway, a Kubernetes-native implementation of the Gateway API specification built on top of Envoy Proxy. It includes:

- **gateway-crds-helm**: Gateway API and Envoy Gateway CRDs installed as Kubernetes resources
- **gateway-helm**: The Envoy Gateway controller

CRDs are managed via `gateway-crds-helm` as regular templated resources, enabling proper upgrades without the limitations of the Helm `crds/` directory.

## Prerequisites

- Kubernetes 1.26+
- Helm 3.8+ (OCI support required)

## Installation

```bash
helm install envoy-gateway oci://ghcr.io/nicklasfrahm-dev/charts/envoy-gateway \
  -n envoy-gateway-system \
  --create-namespace
```

## Configuration

| Parameter                    | Description                           | Default |
| ---------------------------- | ------------------------------------- | ------- |
| `gateway-crds-helm.enabled`  | Install Envoy Gateway CRDs            | `true`  |
| `gateway-helm.enabled`       | Install Envoy Gateway controller      | `true`  |

For all available configuration options, refer to the upstream chart documentation:

- [gateway-helm API reference](https://gateway.envoyproxy.io/docs/install/gateway-helm-api/)
- [gateway-crds-helm API reference](https://gateway.envoyproxy.io/docs/install/gateway-crds-helm-api/)
