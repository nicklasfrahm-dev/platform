FROM python:3.12-slim AS downloader

# Inject build arguments.
ARG MODEL
ARG ALIAS

# Avoid interactive prompts during package installation.
ENV DEBIAN_FRONTEND=noninteractive

# Install Hugging Face CLI.
RUN pip install --no-cache-dir "huggingface_hub[cli]>=0.24.0"

WORKDIR /models

# Download model securely with token (if provided)
# Use BuildKit secret for HF_TOKEN when possible
RUN --mount=type=secret,id=HF_TOKEN \
  mkdir -p /models/${ALIAS} && \
  hf download "$MODEL" --local-dir /models/${ALIAS} --token "$(cat /run/secrets/HF_TOKEN)" && \
  find ./${ALIAS} ! -name '*.safetensors' ! -name '*.json' ! -name '*.txt' -type f -delete -o -type d -empty -delete

# v3.22.2
FROM alpine@sha256:4b7ce07002c69e8f3d704a9c5d6fd3053be500b7f1c69fc0d80990c2ad8dd412 AS models
ARG ALIAS

# Set working directory.
WORKDIR /models/${ALIAS}

# Copy model from the build stage.
COPY --from=downloader /models/${ALIAS} /models/${ALIAS}
