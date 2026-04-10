#!/usr/bin/env bash

set -euo pipefail

cluster_dir=""
cluster_name=""
reboot=false

preflight_check() {
  if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
    echo "ℹ️  Usage: $0 <cluster-directory> [--reboot]"
    exit 1
  fi

  if [ ! -d "$1" ]; then
      echo "❌ Error: failed to find cluster directory: $1"
      exit 1
  fi

  dependencies=(talosctl yq kubectl)
  for cmd in "${dependencies[@]}"; do
    if ! command -v "$cmd" &> /dev/null; then
      echo "❌ Error: failed to find required command: $cmd"
      exit 1
    fi
  done
}

check_required_files() {
  local required_files=("meta.yaml" "controlplane.yaml" "worker.yaml" "talosconfig")

  for file in "${required_files[@]}"; do
    if [ ! -f "$cluster_dir/$file" ]; then
      echo "❌ Error: failed to find required file: $cluster_dir/$file"
      echo "ℹ️  Hint: run talosgen.sh first to generate the configuration files"
      exit 1
    fi
  done
}

wait_for_node_ready() {
  local name="$1"
  local host="$2"
  local talosconfig="$cluster_dir/talosconfig"
  local timeout=300
  local interval=5
  local elapsed=0

  echo "⏳ Info: Waiting for node to become ready (${host}/${name:-unknown})"

  # Wait for Talos to come back up (machined responding).
  while ! timeout 5s talosctl --talosconfig "$talosconfig" --nodes "$host" --endpoints "$host" version &>/dev/null; do
    sleep "$interval"
    elapsed=$((elapsed + interval))
    if [ "$elapsed" -ge "$timeout" ]; then
      echo "❌ Error: Timed out waiting for Talos to come back up (${host}/${name:-unknown})"
      return 1
    fi
  done

  # Wait for the Kubernetes node to be Ready.
  if [ -n "$name" ]; then
    while ! kubectl wait node "$name" --for=condition=Ready --timeout="${interval}s" &>/dev/null; do
      elapsed=$((elapsed + interval))
      if [ "$elapsed" -ge "$timeout" ]; then
        echo "❌ Error: Timed out waiting for Kubernetes node to be Ready (${host}/${name:-unknown})"
        return 1
      fi
    done
  fi

  echo "✅ Info: Node is ready (${host}/${name:-unknown})"
}

apply_config_to_node() {
  local host="$1"
  local name="$2"
  local config_file="$3"
  local talosconfig="$cluster_dir/talosconfig"

  echo "✅ Info: Applying configuration (${host}/${name:-unknown})"

  # Build a per-node config: strip HostnameConfig from the generated file and
  # append a clean one with the static hostname. This avoids conflicts between
  # the generated "auto: stable" HostnameConfig and the per-node hostname patch,
  # which Talos v1.12+ rejects when both fields are present.
  tmp_config=$(mktemp --suffix=.yaml)
  cleanup() { rm -f "$tmp_config"; }
  trap cleanup RETURN

  if [ -n "$name" ]; then
    yq 'select(.kind != "HostnameConfig")' "$config_file" > "$tmp_config"
    printf -- "---\napiVersion: v1alpha1\nkind: HostnameConfig\nhostname: %s\n" "$name" >> "$tmp_config"
  else
    cp "$config_file" "$tmp_config"
  fi

  # Use timeout to handle unreachable nodes.
  if timeout 10s talosctl apply \
    --talosconfig "$talosconfig" \
    --endpoints "$host" \
    --nodes "$host" \
    --file "$tmp_config"; then
    echo "✅ Info: Successfully applied configuration (${host}/${name:-unknown})"
  else
    # Try to apply the config in maintenance mode if the normal apply failed.
    echo "⚠️  Warn: Normal apply failed, trying maintenance mode (${host}/${name:-unknown})"
    if timeout 10s talosctl apply \
      --talosconfig "$talosconfig" \
      --endpoints "$host" \
      --nodes "$host" \
      --file "$tmp_config" \
      --insecure; then
      echo "✅ Info: Successfully applied configuration in maintenance mode (${host}/${name:-unknown})"
    else
      echo "❌ Error: Failed to apply configuration (${host}/${name:-unknown})"
      return 1
    fi
  fi

  if [ "$reboot" = true ]; then
    echo "🔄 Info: Rebooting node (${host}/${name:-unknown})"
    talosctl --talosconfig "$talosconfig" --nodes "$host" --endpoints "$host" reboot
    wait_for_node_ready "$name" "$host"
  fi
}

apply_talos_configs() {
  local failed_nodes=0

  cluster_name=$(yq eval '.name' "$cluster_dir/meta.yaml" || echo "INVALID")
  if [ "$cluster_name" == "INVALID" ]; then
    echo "❌ Error: failed to read cluster name from meta.yaml"
    exit 1
  fi

  echo "Applying Talos configurations for cluster: $cluster_name"
  echo "=================================================="

  # Get the number of control plane and worker nodes.
  controlplane_count=$(yq '.nodes.controlplanes | length' "$cluster_dir/meta.yaml" 2>/dev/null || echo "0")
  for ((i=0; i<controlplane_count; i++)); do
    host=$(yq -r ".nodes.controlplanes[$i].host" "$cluster_dir/meta.yaml" 2>/dev/null || echo "")
    if [ -z "$host" ]; then
      echo "⚠️  Warn: Skipping node: failed to find host for control plane node: $i"
      failed_nodes=$((failed_nodes + 1))
      continue
    fi

    name=$(yq -r ".nodes.controlplanes[$i].name" "$cluster_dir/meta.yaml" 2>/dev/null || echo "")
    if [ -z "$name" ]; then
      echo "⚠️  Warn: Using auto-generated name: failed to find name for control plane node: $i"
    fi

    if ! apply_config_to_node "$host" "$name" "$cluster_dir/controlplane.yaml"; then
      failed_nodes=$((failed_nodes + 1))
    fi
  done

  worker_count=$(yq '.nodes.workers | length' "$cluster_dir/meta.yaml" 2>/dev/null || echo "0")

  for ((i=0; i<worker_count; i++)); do
    host=$(yq -r ".nodes.workers[$i].host" "$cluster_dir/meta.yaml" 2>/dev/null || echo "")
    if [ -z "$host" ]; then
      echo "⚠️  Warn: Skipping node: failed to find host for worker node: $i"
      failed_nodes=$((failed_nodes + 1))
      continue
    fi

    name=$(yq -r ".nodes.workers[$i].name" "$cluster_dir/meta.yaml" 2>/dev/null || echo "")
    if [ -z "$name" ]; then
      echo "⚠️  Warn: Using auto-generated name: failed to find name for worker node: $i"
    fi

    if ! apply_config_to_node "$host" "$name" "$cluster_dir/worker.yaml"; then
      failed_nodes=$((failed_nodes + 1))
    fi
  done

  total_nodes=$((controlplane_count + worker_count))
  echo "=================================================="
  echo "Total nodes:       $total_nodes"
  echo "Failed nodes:      $failed_nodes"
  echo "Successful nodes:  $((total_nodes - failed_nodes))"

  if [ $failed_nodes -gt 0 ]; then
    echo ""
    echo "⚠️  Some nodes failed to receive configuration updates."
    echo "   This is likely due to nodes being offline or unreachable."
    echo "   You can retry this script later for the failed nodes."
    exit 1
  else
    echo ""
    echo "✅ All nodes successfully configured!"
  fi
}

main() {
  preflight_check "$@"

  # Strip trailing slash if any.
  cluster_dir="${1%/}"

  if [ "${2:-}" = "--reboot" ]; then
    reboot=true
  fi

  check_required_files

  # Set the cluster name.
  cluster_name=$(yq '.name' "$cluster_dir/meta.yaml")

  apply_talos_configs
}

main "$@"
