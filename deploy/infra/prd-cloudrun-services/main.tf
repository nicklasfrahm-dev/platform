locals {
  root = "${path.module}/../../.."

  # Fetch global configuration options, such as the DNS zone.
  global_config = yamldecode(file("${local.root}/config.yaml"))
}

module "cloudrun_service" {
  source = "../../../modules/cloudrun-service"

  cluster_glob = "prd-*"
  clusters_dir = "${local.root}/deploy/clusters"
  values_dir   = "${local.root}/deploy/services"
  domain       = local.global_config.dns.zone
}

output "uris" {
  description = "The URI of each deployed Cloud Run service, keyed by \"<project>/<service>\"."
  value       = module.cloudrun_service.uris
}
