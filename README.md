# Cloud

This repository contains the configuration of all my infrastructure. Some components use the [Operator SDK][operator-sdk] to manage resources.

## Documentation

- [API](./docs/api.md)

## Provisioning

To provision the Kubernetes clusters, [OpenTofu][opentofu] is used.

[operator-sdk]: https://sdk.operatorframework.io/
[opentofu]: https://opentofu.org/

## Helm charts

The `charts/` directory contains a set of umbrella charts, which may be used to deploy
a set of hand-picked services to Kubernetes.

### Helm quality assurance

For quality assurance, the charts are continuously tested using GitHub Actions
via three high-level steps:

- **Linting**: The charts are linted using the `helm lint` command.
- **Diffing**: The charts are diffed against the latest version.
- **Versioning**: The chart version is validated on every PR.

## LLM Inference Benchmarks

GPU inference benchmarks run on `deer` (RTX 3090 24 GB) via KServe / vLLM at `llm.cph02.nicklasfrahm.dev`. Full results in [`docs/benchmark.md`](./docs/benchmark.md).
