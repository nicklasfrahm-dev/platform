resource "google_cloud_run_v2_service" "this" {
  for_each = local.services

  name     = each.value.name
  project  = each.value.project
  location = each.value.region
  ingress  = "INGRESS_TRAFFIC_ALL"

  template {
    containers {
      image = "${each.value.values.image.repository}:${each.value.values.image.tag}"

      ports {
        container_port = each.value.values.service.containerPort
      }

      resources {
        limits = {
          cpu    = each.value.values.resources.cpu
          memory = each.value.values.resources.memory
        }
      }

      dynamic "env" {
        for_each = each.value.values.env.fromLiterals
        content {
          name  = env.key
          value = env.value
        }
      }

      # env.fromSecrets follows charts/service/values.yaml's shape of
      # { VARIABLE: { secretName: keyName } }, mirroring a Kubernetes
      # secretKeyRef. Secret Manager secrets have no equivalent to a
      # Kubernetes Secret's multiple keys, so keyName is reinterpreted here
      # as the Secret Manager version (defaults to "latest" upstream).
      dynamic "env" {
        for_each = each.value.values.env.fromSecrets
        content {
          name = env.key
          value_source {
            secret_key_ref {
              secret  = keys(env.value)[0]
              version = values(env.value)[0]
            }
          }
        }
      }

      dynamic "startup_probe" {
        for_each = try(each.value.values.probes.startup.enabled, false) ? [each.value.values.probes.startup] : []
        content {
          http_get {
            path = startup_probe.value.path
          }
          period_seconds    = startup_probe.value.periodSeconds
          failure_threshold = startup_probe.value.failureThreshold
        }
      }

      dynamic "liveness_probe" {
        for_each = try(each.value.values.probes.liveness.enabled, false) ? [each.value.values.probes.liveness] : []
        content {
          http_get {
            path = liveness_probe.value.path
          }
          initial_delay_seconds = liveness_probe.value.initialDelaySeconds
          period_seconds        = liveness_probe.value.periodSeconds
          failure_threshold     = liveness_probe.value.failureThreshold
        }
      }
    }

    scaling {
      min_instance_count = each.value.values.autoscaling.minReplicas
      max_instance_count = each.value.values.autoscaling.maxReplicas
    }
  }
}

# expose.enabled makes the service publicly reachable, mirroring the
# HTTPRoute that charts/service creates behind the shared gateway.
resource "google_cloud_run_v2_service_iam_member" "public" {
  for_each = local.exposed_services

  project  = each.value.project
  location = each.value.region
  name     = google_cloud_run_v2_service.this[each.key].name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

resource "google_cloud_run_domain_mapping" "this" {
  for_each = local.exposed_services

  name     = local.hostnames[each.key]
  project  = each.value.project
  location = each.value.region

  metadata {
    namespace = each.value.project
  }

  spec {
    route_name = google_cloud_run_v2_service.this[each.key].name
  }
}
