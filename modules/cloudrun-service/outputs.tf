output "uris" {
  description = "The URI of each deployed Cloud Run service, keyed by \"<project>/<service>\"."
  value = {
    for key, service in google_cloud_run_v2_service.this :
    "${local.services[key].project}/${local.services[key].name}" => service.uri
  }
}

output "values" {
  description = "The fully merged values used to deploy each service, keyed by \"<project>/<service>\"."
  value = {
    for key, service in local.services :
    "${service.project}/${service.name}" => service.values
  }
}
