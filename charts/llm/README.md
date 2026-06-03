# llm

Helm chart for deploying a large language model via vLLM on KServe.

## Authentication

The chart deploys an Envoy Gateway `SecurityPolicy` that requires a valid JWT issued by the Dex OIDC provider at `https://auth.nicklasfrahm.dev`. All requests to the LLM API must include a Bearer token in the `Authorization` header.

### Generating a token

Tokens are obtained via `kubectl oidc-login`, which is part of the [`kubelogin`](https://github.com/int128/kubelogin) plugin. Install it with:

```sh
kubectl krew install oidc-login
```

Then generate a token:

```sh
kubectl oidc-login get-token \
  --oidc-issuer-url=https://auth.nicklasfrahm.dev \
  --oidc-client-id=kubernetes \
  --oidc-extra-scope=email \
  --oidc-extra-scope=groups
```

This opens a browser window for authentication. On success, the token is printed to stdout under the `status.token` field. Extract it with:

```sh
TOKEN=$(kubectl oidc-login get-token \
  --oidc-issuer-url=https://auth.nicklasfrahm.dev \
  --oidc-client-id=kubernetes \
  --oidc-extra-scope=email \
  --oidc-extra-scope=groups \
  | jq -r .status.token)
```

### Using the token

Pass the token as a Bearer token when calling the API:

```sh
curl https://llm.cph02.nicklasfrahm.dev/v1/models \
  -H "Authorization: Bearer $TOKEN"
```

For OpenAI-compatible clients, set the base URL to `https://llm.cph02.nicklasfrahm.dev/v1` and the API key to the token value.
