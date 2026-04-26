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

WIPE=false
HF_REPO=""

while [[ $# -gt 0 ]]; do
  case $1 in
    --wipe)
      WIPE=true
      shift
      ;;
    *)
      HF_REPO="$1"
      shift
      ;;
  esac
done

if [[ -z "${HF_REPO}" ]]; then
  echo "Usage: $0 [--wipe] <HF_REPO>" >&2
  exit 1
fi

# 5. Stream model file from HuggingFace directly into the bucket
# We will use a naming convention derived from the HF repo name
TARGET_NAME=$(echo "${HF_REPO}" | sed 's/.*\///; s/\//-/g' | tr '[:upper:]' '[:lower:]')
DEST="ceph/${BUCKET_NAME}/${TARGET_NAME}"

if [ "$WIPE" = true ]; then
  echo "==> Wiping existing data for ${TARGET_NAME}..."
  ${MC} rm --recursive --force "${DEST}/" 2>/dev/null || true
fi

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

echo "==> Model uploaded successfully."
