terraform {
  required_version = ">= 1.9.0"

  required_providers {
    helm = {
      source  = "hashicorp/helm"
      version = "3.1.1"
    }
    talos = {
      source  = "siderolabs/talos"
      version = "0.10.0"
    }
  }

  backend "gcs" {
    bucket = "nicklasfrahm"
    prefix = "tofu/state/homelab"
  }
}
