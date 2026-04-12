#!/usr/bin/env bash
set -euo pipefail

CONTEXT="admin@cph02"
S3_ENDPOINT="https://s3.cph02.nicklasfrahm.dev"
MC="/home/nicklasfrahm/.arkade/bin/mc"
K="kubectl --context ${CONTEXT}"

# 1. Apply the OBC
echo "==> Applying ObjectBucketClaim..."
${K} apply -f "$(dirname "$0")/../deploy/manifests/llm/bucket.yaml"

# 2. Wait for the OBC to be Bound
echo "==> Waiting for OBC to be Bound..."
for i in $(seq 1 30); do
  PHASE=$(${K} get objectbucketclaim models -n llm -o jsonpath='{.status.phase}' 2>/dev/null || true)
  if [[ "${PHASE}" == "Bound" ]]; then
    echo "    OBC is Bound."
    break
  fi
  if [[ "${i}" -eq 30 ]]; then
    echo "ERROR: OBC did not become Bound after 150s" >&2
    exit 1
  fi
  echo "    Phase: ${PHASE:-pending} (attempt ${i}/30)..."
  sleep 5
done

# 3. Extract credentials
echo "==> Extracting S3 credentials..."
BUCKET_NAME=$(${K} get cm models -n llm -o jsonpath='{.data.BUCKET_NAME}')
ACCESS_KEY=$(${K} get secret models -n llm -o jsonpath='{.data.AWS_ACCESS_KEY_ID}' | base64 -d)
SECRET_KEY=$(${K} get secret models -n llm -o jsonpath='{.data.AWS_SECRET_ACCESS_KEY}' | base64 -d)
HF_TOKEN=$(${K} get secret hf-secret -n llm -o jsonpath='{.data.HF_TOKEN}' | base64 -d)
echo "    Bucket: ${BUCKET_NAME}"

# 4. Configure mc alias
echo "==> Configuring mc alias 'ceph'..."
${MC} alias set ceph "${S3_ENDPOINT}" "${ACCESS_KEY}" "${SECRET_KEY}"

# 5. Stream each model file from HuggingFace directly into the bucket
declare -A MODELS=(
  ["cyankiwi/gemma-4-26B-A4B-it-AWQ-4bit"]="gemma-4-26b-a4b-awq-4bit"
  ["Qwen/Qwen2.5-Coder-7B-Instruct-AWQ"]="qwen25-coder-7b-instruct-awq"
)

for HF_REPO in "${!MODELS[@]}"; do
  TARGET_NAME="${MODELS[$HF_REPO]}"
  DEST="ceph/${BUCKET_NAME}/${TARGET_NAME}"

  echo "==> Fetching file list for ${HF_REPO}..."
  FILES=$(curl -sf "https://huggingface.co/api/models/${HF_REPO}" \
    -H "Authorization: Bearer ${HF_TOKEN}" \
    | jq -r '.siblings[].rfilename')

  echo "    Found $(echo "${FILES}" | wc -l) files."

  while IFS= read -r FILE; do
    URL="https://huggingface.co/${HF_REPO}/resolve/main/${FILE}"
    echo "  -> ${FILE}"
    curl -sfL "${URL}" \
      -H "Authorization: Bearer ${HF_TOKEN}" \
      | ${MC} pipe "${DEST}/${FILE}"
  done <<< "${FILES}"

  echo "    Done: ${HF_REPO} -> s3://${BUCKET_NAME}/${TARGET_NAME}/"
done

echo "==> All models uploaded successfully."
