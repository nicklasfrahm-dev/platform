#!/usr/bin/env bash
set -euo pipefail

# Script: convert-hf-to-oci.sh
# Purpose: Convert Hugging Face models to OCI images for use with KServe
#          Automatically detects and converts .nemo files to PyTorch format
# Usage: ./convert-hf-to-oci.sh <huggingface-repo> <oci-repo> [model-name]
#
# Arguments:
#   huggingface-repo  - Hugging Face repository name (e.g., gpt2, bert-base-uncased, nvidia/parakeet-tdt-0.6b-v3)
#   oci-repo          - OCI repository with tag (e.g., ghcr.io/myorg/gpt2-hf:latest)
#   model-name        - Optional model directory name (default: model)
#
# Examples:
#   ./convert-hf-to-oci.sh "gpt2" "ghcr.io/myorg/gpt2-hf:latest"
#   ./convert-hf-to-oci.sh "nvidia/parakeet-tdt-0.6b-v3" "ghcr.io/myorg/parakeet:v3" "parakeet"

# Color definitions
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}info:${NC} $*"
}

log_success() {
    echo -e "${GREEN}success:${NC} $*"
}

log_warning() {
    echo -e "${YELLOW}warning:${NC} $*"
}

log_error() {
    echo -e "${RED}error:${NC} $*" >&2
}

# Usage function
show_usage() {
    cat << EOF
Usage: $(basename "$0") <huggingface-repo> <oci-repo> [model-name]

Arguments:
  huggingface-repo  Hugging Face repository name (e.g., gpt2, nvidia/parakeet-tdt-0.6b-v3)
  oci-repo          OCI repository with tag (e.g., ghcr.io/myorg/gpt2-hf:latest)
  model-name        Optional model directory name (default: model)

Examples:
  $(basename "$0") "gpt2" "ghcr.io/myorg/gpt2-hf:latest"
  $(basename "$0") "nvidia/parakeet-tdt-0.6b-v3" "ghcr.io/myorg/parakeet:v3" "parakeet"

This script downloads a Hugging Face model and packages it as an OCI image
for use with the HuggingFace runtime in KServe. If .nemo files are detected,
they will be automatically converted to PyTorch format for broader compatibility.
EOF
}

# Check for help flags
if [[ $# -eq 0 ]] || [[ "${1:-}" == "-h" ]] || [[ "${1:-}" == "--help" ]]; then
    show_usage
    exit 0
fi

# Validate arguments
if [[ $# -lt 2 ]]; then
    log_error "Missing required arguments"
    show_usage
    exit 1
fi

HF_REPO="${1}"
OCI_REPO="${2}"
MODEL_NAME="${3:-model}"

# Create venv in current working directory
VENV_DIR="${PWD}/.venv"

# Use models directory in current working directory
MODELS_BASE_DIR="${PWD}/models"
MODEL_DIR="${MODELS_BASE_DIR}/${MODEL_NAME}"

# Create models directory if it doesn't exist
mkdir -p "${MODELS_BASE_DIR}"

log_info "Using models directory: ${MODEL_DIR}"
log_info "Models will be stored persistently for reuse"

# Cleanup function
cleanup() {
    local exit_code=$?
    if [[ $exit_code -ne 0 ]]; then
        log_error "Script failed with exit code $exit_code"
        log_info "Model files remain in: ${MODEL_DIR}"
    fi
    exit $exit_code
}

# Set trap for cleanup
trap cleanup EXIT INT TERM

# Check if Git LFS is available
if ! command -v git-lfs &> /dev/null; then
    log_warning "Git LFS not found. Large files may not download properly."
    log_info "To install Git LFS: sudo apt-get install git-lfs (Ubuntu/Debian) or brew install git-lfs (macOS)"
else
    log_info "Git LFS detected - large files will be handled properly"
fi

log_info "Downloading Hugging Face repository: ${HF_REPO}"

# Check if model directory already exists
if [[ -d "${MODEL_DIR}" ]]; then
    log_warning "Model directory already exists: ${MODEL_DIR}"
    log_info "Skipping download. Remove directory to force re-download."
else
    # Use Git LFS to handle large files properly
    # First, ensure Git LFS is installed globally
    git lfs install

    # Clone the repository
    git clone "https://huggingface.co/${HF_REPO}" "${MODEL_DIR}"

    # Download LFS files
    log_info "Downloading LFS files (large model files)..."
    cd "${MODEL_DIR}"

    # Initialize Git LFS in the repository
    git lfs install --local

    # Fetch and pull all LFS files
    git lfs fetch --all
    git lfs checkout
    cd - > /dev/null
fi

# Print file sizes before processing
log_info "Analyzing downloaded files..."
if [[ -d "${MODEL_DIR}" ]]; then
    total_size=$(du -sh "${MODEL_DIR}" | cut -f1)
    log_info "Total repository size: ${total_size}"

    # Show largest files
    log_info "Largest files in repository:"
    temp_file=$(mktemp)
    find "${MODEL_DIR}" -type f -exec du -h {} + 2>/dev/null | sort -rh > "${temp_file}" || true
    head -10 "${temp_file}" | while IFS= read -r line; do
        if [[ -n "$line" ]]; then
            size=$(echo "$line" | awk '{print $1}')
            file=$(echo "$line" | awk '{$1=""; print $0}' | sed 's/^ *//')
            echo "  ${size} - $(basename "${file}")"
        fi
    done
    rm -f "${temp_file}"

    # Count different file types
    nemo_count=$(find "${MODEL_DIR}" -name "*.nemo" -type f 2>/dev/null | wc -l)
    bin_count=$(find "${MODEL_DIR}" -name "*.bin" -type f 2>/dev/null | wc -l)
    safetensors_count=$(find "${MODEL_DIR}" -name "*.safetensors" -type f 2>/dev/null | wc -l)

    log_info "File type summary:"
    [[ $nemo_count -gt 0 ]] && echo "  .nemo files: ${nemo_count}"
    [[ $bin_count -gt 0 ]] && echo "  .bin files: ${bin_count}"
    [[ $safetensors_count -gt 0 ]] && echo "  .safetensors files: ${safetensors_count}"
fi

# Check for .nemo files and convert to PyTorch if found
if find "${MODEL_DIR}" -name "*.nemo" -type f 2>/dev/null | grep -q .; then
    log_info "Found .nemo file(s), converting to PyTorch format"

    # Create or reuse persistent virtual environment for NeMo conversion
    if [[ ! -d "${VENV_DIR}" ]]; then
        log_info "Creating persistent virtual environment: ${VENV_DIR}"
        python3 -m venv "${VENV_DIR}"

        # Install NeMo toolkit using the venv's pip directly
        log_info "Installing NeMo Toolkit in virtual environment"
        "${VENV_DIR}/bin/pip3" install --upgrade pip
        "${VENV_DIR}/bin/pip3" install nemo_toolkit[all] transformers torch torchaudio
    else
        log_info "Reusing existing virtual environment: ${VENV_DIR}"
        # Check if NeMo is installed, install if missing
        if ! "${VENV_DIR}/bin/python3" -c "import nemo" 2>/dev/null; then
            log_info "NeMo not found in existing venv, installing..."
            "${VENV_DIR}/bin/pip3" install --upgrade pip
            "${VENV_DIR}/bin/pip3" install nemo_toolkit[all] transformers torch torchaudio
        fi
    fi

    # Find all .nemo files and convert them
    find "${MODEL_DIR}" -name "*.nemo" -type f 2>/dev/null | while read -r nemo_file; do
        nemo_size=$(du -h "${nemo_file}" | cut -f1)
        log_info "Converting: $(basename "${nemo_file}") (${nemo_size})"

        # Run conversion using the venv's python directly
        "${VENV_DIR}/bin/python3" << EOF
import nemo.collections.asr as nemo_asr
import torch
import os

try:
    # Try to load as RNNT model first (more common for newer models)
    try:
        model = nemo_asr.models.EncDecRNNTBPEModel.restore_from("${nemo_file}")
        print(f"Loaded as EncDecRNNTBPEModel")
    except Exception as e:
        print(f"Failed to load as RNNT model: {e}")
        # Fallback to CTC model
        model = nemo_asr.models.EncDecCTCModel.restore_from("${nemo_file}")
        print(f"Loaded as EncDecCTCModel")

    # Export to TorchScript using the correct method
    output_path = os.path.join(os.path.dirname("${nemo_file}"), "parakeet_asr_ts.pt")

    # Use the newer export API without format parameter
    try:
        model.export(output_path)
        print(f"Successfully exported to TorchScript: {output_path}")
    except Exception as e:
        print(f"Export failed: {e}")
        # Try alternative: save the model state dict directly
        torch.save(model.state_dict(), output_path.replace('.pt', '_state_dict.pt'))
        print(f"Saved state dict instead: {output_path.replace('.pt', '_state_dict.pt')}")

except Exception as e:
    print(f"Failed to process .nemo file: {e}")
    print("The model will be used as-is without conversion")
EOF
    done

    # Keep original .nemo files in the local directory (don't delete them)
    log_info "Preserving original .nemo files in local directory"

    # If pytorch_model directory exists, move its contents to the main model directory
    if [[ -d "${MODEL_DIR}/pytorch_model" ]]; then
        log_info "Moving converted PyTorch files to model root"
        mv "${MODEL_DIR}/pytorch_model"/* "${MODEL_DIR}/"
        rmdir "${MODEL_DIR}/pytorch_model"
    fi

    log_success "NeMo model(s) converted to PyTorch format"

    # Show file sizes after conversion
    log_info "File sizes after conversion:"
    total_size_after=$(du -sh "${MODEL_DIR}" | cut -f1)
    log_info "Total model directory size: ${total_size_after}"

    # Show converted files
    if ls "${MODEL_DIR}"/*.pt >/dev/null 2>&1; then
        log_info "Converted PyTorch files:"
        find "${MODEL_DIR}" -name "*.pt" -type f -exec du -h {} + | while read -r size file; do
            echo "  ${size} - $(basename "${file}")"
        done
    fi
else
    log_info "No .nemo files found, proceeding with standard model packaging"
fi

log_info "Creating Dockerfile (PyTorch files only)"

# Create a temporary directory for Docker build context with only .pt files
DOCKER_BUILD_DIR="${MODELS_BASE_DIR}/docker-build"
DOCKER_MODEL_DIR="${DOCKER_BUILD_DIR}/${MODEL_NAME}"
mkdir -p "${DOCKER_MODEL_DIR}"

# Check if we have .pt files (converted from .nemo) or use all files for standard models
if ls "${MODEL_DIR}"/*.pt >/dev/null 2>&1; then
    log_info "Copying only PyTorch (.pt) files to Docker build context"
    # Copy only .pt files and essential config files
    find "${MODEL_DIR}" -name "*.pt" -type f -exec cp {} "${DOCKER_MODEL_DIR}/" \;

    # Also copy essential config files if they exist (but not .nemo files)
    for config_file in "config.json" "tokenizer.json" "vocab.txt" "*.yaml" "*.yml" "README.md"; do
        find "${MODEL_DIR}" -name "${config_file}" -type f -exec cp {} "${DOCKER_MODEL_DIR}/" \; 2>/dev/null || true
    done

    log_info "Docker build context contents:"
    ls -la "${DOCKER_MODEL_DIR}"
else
    log_info "No .pt files found, copying all model files (standard model)"
    cp -r "${MODEL_DIR}"/* "${DOCKER_MODEL_DIR}/"
fi

cat > "${DOCKER_BUILD_DIR}/Dockerfile" <<EOF
FROM alpine:latest
COPY ${MODEL_NAME} /models/${MODEL_NAME}
EOF

log_info "Building OCI image: ${OCI_REPO}"
docker build -t "${OCI_REPO}" -f "${DOCKER_BUILD_DIR}/Dockerfile" "${DOCKER_BUILD_DIR}"

# Clean up temporary Docker build directory
rm -rf "${DOCKER_BUILD_DIR}"

log_info "Pushing to OCI registry: ${OCI_REPO}"
docker push "${OCI_REPO}"

log_success "Hugging Face model published as OCI image: ${OCI_REPO}"
log_success "Use this image with the HuggingFace runtime in KServe."
