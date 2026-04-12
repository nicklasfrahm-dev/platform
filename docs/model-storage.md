# Model Storage

Object storage for LLM model weights, backed by Rook-Ceph and exposed via the S3-compatible Ceph Object Gateway.

## Resources

### ObjectBucketClaim (`bucket.yaml`)

A Rook `ObjectBucketClaim` in the `llm` namespace. Rook provisions the bucket and injects access credentials into a `ConfigMap` and `Secret` with the same name (`models`) in the same namespace.

| Field | Value |
|---|---|
| API version | `objectbucket.io/v1alpha1` |
| Kind | `ObjectBucketClaim` |
| Name | `models` |
| Namespace | `llm` |
| Storage class | `ceph-bucket` |

### Generated ConfigMap — `models` (namespace: `llm`)

| Key | Description |
|---|---|
| `BUCKET_NAME` | Actual bucket name (e.g. `models-3bd7d589-48a0-44fb-a04a-1505c9ff5c8b`) |
| `BUCKET_HOST` | Internal RGW host |
| `BUCKET_PORT` | Internal RGW port |

### Generated Secret — `models` (namespace: `llm`)

| Key | Description |
|---|---|
| `AWS_ACCESS_KEY_ID` | S3 access key (base64-encoded) |
| `AWS_SECRET_ACCESS_KEY` | S3 secret key (base64-encoded) |

### Uploaded Models

Both models are stored under their renamed slugs inside the bucket.

| HuggingFace repo | Bucket path |
|---|---|
| `cyankiwi/gemma-4-26B-A4B-it-AWQ-4bit` | `<bucket>/gemma-4-26b-a4b-awq-4bit/` |
| `Qwen/Qwen2.5-Coder-7B-Instruct-AWQ` | `<bucket>/qwen25-coder-7b-instruct-awq/` |

## Connecting

### Public S3 endpoint

The Ceph Object Gateway is exposed externally via the Gateway API at:

```
https://s3.cph02.nicklasfrahm.dev
```

### Retrieve credentials

```bash
BUCKET_NAME=$(kubectl --context admin@cph02 get cm models -n llm -o jsonpath='{.data.BUCKET_NAME}')
ACCESS_KEY=$(kubectl --context admin@cph02 get secret models -n llm -o jsonpath='{.data.AWS_ACCESS_KEY_ID}' | base64 -d)
SECRET_KEY=$(kubectl --context admin@cph02 get secret models -n llm -o jsonpath='{.data.AWS_SECRET_ACCESS_KEY}' | base64 -d)
```

### `mc` (MinIO Client)

```bash
mc alias set ceph https://s3.cph02.nicklasfrahm.dev "${ACCESS_KEY}" "${SECRET_KEY}"

# List models
mc ls ceph/${BUCKET_NAME}

# List files in a specific model
mc ls ceph/${BUCKET_NAME}/gemma-4-26b-a4b-awq-4bit/
```

### AWS CLI

```bash
aws s3 ls s3://${BUCKET_NAME}/ \
  --endpoint-url https://s3.cph02.nicklasfrahm.dev \
  --no-verify-ssl
```

Configure credentials in `~/.aws/credentials`:

```ini
[ceph]
aws_access_key_id     = <ACCESS_KEY>
aws_secret_access_key = <SECRET_KEY>
```

### Python (`boto3`)

```python
import boto3

s3 = boto3.client(
    "s3",
    endpoint_url="https://s3.cph02.nicklasfrahm.dev",
    aws_access_key_id="<ACCESS_KEY>",
    aws_secret_access_key="<SECRET_KEY>",
)

# List objects in the models bucket
for obj in s3.list_objects_v2(Bucket="<BUCKET_NAME>")["Contents"]:
    print(obj["Key"])
```

### From a pod in the `llm` namespace

Workloads in the `llm` namespace can consume the credentials directly via `envFrom`:

```yaml
envFrom:
  - configMapRef:
      name: models
  - secretRef:
      name: models
```

This injects `BUCKET_NAME`, `BUCKET_HOST`, `BUCKET_PORT`, `AWS_ACCESS_KEY_ID`, and `AWS_SECRET_ACCESS_KEY` as environment variables, which most S3 clients pick up automatically.

## Using with KServe

KServe's storage initializer can pull model weights from S3 before the serving container starts. This eliminates the HuggingFace dependency at runtime and makes cold-start times independent of HuggingFace availability.

### How it works

When `storageUri` is set on an `InferenceService`, KServe injects an init container (the storage initializer) that downloads the model to `/mnt/models`. The serving container (vLLM) then reads from that local path via `MODEL_ID=/mnt/models` instead of pulling from HuggingFace Hub.

### 1. Create the S3 credentials secret

KServe's storage initializer looks for S3 configuration in a secret annotated with `serving.kserve.io/s3-*` keys. Create it by projecting the OBC-generated credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: kserve-s3-credentials
  namespace: llm
  annotations:
    serving.kserve.io/s3-endpoint: s3.cph02.nicklasfrahm.dev
    serving.kserve.io/s3-usehttps: "1"
    serving.kserve.io/s3-verifyssl: "1"
    serving.kserve.io/s3-region: us-east-1
type: Opaque
stringData:
  AWS_ACCESS_KEY_ID: <value from models secret>
  AWS_SECRET_ACCESS_KEY: <value from models secret>
```

Or apply it imperatively from the existing OBC credentials:

```bash
kubectl --context admin@cph02 create secret generic kserve-s3-credentials \
  -n llm \
  --from-literal=AWS_ACCESS_KEY_ID="$(kubectl --context admin@cph02 get secret models -n llm -o jsonpath='{.data.AWS_ACCESS_KEY_ID}' | base64 -d)" \
  --from-literal=AWS_SECRET_ACCESS_KEY="$(kubectl --context admin@cph02 get secret models -n llm -o jsonpath='{.data.AWS_SECRET_ACCESS_KEY}' | base64 -d)"

kubectl --context admin@cph02 annotate secret kserve-s3-credentials -n llm \
  serving.kserve.io/s3-endpoint=s3.cph02.nicklasfrahm.dev \
  serving.kserve.io/s3-usehttps=1 \
  serving.kserve.io/s3-verifyssl=1 \
  serving.kserve.io/s3-region=us-east-1
```

### 2. Create a ServiceAccount

KServe resolves S3 credentials through the `ServiceAccount` referenced by the `InferenceService`:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kserve-s3
  namespace: llm
secrets:
  - name: kserve-s3-credentials
```

### 3. Update the InferenceService

Replace `MODEL_ID` with `storageUri` pointing to the bucket path, and set `MODEL_ID` to the local mount path:

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: coder-qwen25
spec:
  predictor:
    serviceAccountName: kserve-s3
    deploymentStrategy:
      type: Recreate
    runtimeClassName: nvidia
    nodeSelector:
      feature.node.kubernetes.io/pci-10de.present: "true"
    tolerations:
      - key: "nvidia.com/gpu"
        operator: "Exists"
        effect: "NoSchedule"
    model:
      modelFormat:
        name: huggingface
      runtime: vllm-coder-runtime
      storageUri: s3://models-3bd7d589-48a0-44fb-a04a-1505c9ff5c8b/qwen25-coder-7b-instruct-awq
      env:
        - name: MODEL_ID
          value: /mnt/models
      resources:
        limits:
          nvidia.com/gpu: 1
          memory: "10Gi"
        requests:
          nvidia.com/gpu: 1
          memory: "10Gi"
```

For the Gemma model, replace `storageUri` with:

```
s3://models-3bd7d589-48a0-44fb-a04a-1505c9ff5c8b/gemma-4-26b-a4b-awq-4bit
```

### Comparison

| | HuggingFace Hub (current) | S3 / Ceph (this setup) |
|---|---|---|
| Cold-start model source | HuggingFace CDN | In-cluster Ceph RGW |
| Requires HF token at runtime | Yes | No |
| Model version control | HF repo tags | Bucket path |
| Bandwidth | External | Internal cluster network |
