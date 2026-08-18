# A dedicated runtime identity per service, rather than falling back to the
# project's default Compute Engine service account, so each service's
# permissions (e.g. the Secret Manager grants below) stay scoped to what
# that service actually needs.
#
# account_id includes environment and location, not just the service name:
# the same tenant project can host the same service name across multiple
# clusters (e.g. more than one dev instance), and google_service_account IDs
# must be unique per project.
resource "google_service_account" "this" {
  for_each = local.services

  project      = each.value.project
  account_id   = "svc-${each.value.name}-${each.value.values.platform.environment}-${each.value.values.platform.location}"
  display_name = "Cloud Run runtime identity for ${each.value.name} (${each.value.values.platform.environment}-${each.value.values.platform.location})"
}

# Cloud Run V2 requires the deploying principal to be able to act as the
# identity it assigns to the service, which isn't implied by any role
# var.DEPLOYER_SERVICE_ACCOUNT holds on the project itself.
resource "google_service_account_iam_member" "deployer_can_act_as" {
  for_each = local.services

  service_account_id = google_service_account.this[each.key].name
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:${var.DEPLOYER_SERVICE_ACCOUNT}"
}

# Grants each service's runtime identity access to read the Secret Manager
# secrets it references via env.fromSecrets. The secrets themselves are
# created out of band (e.g. via the GCP console) - this only grants access
# to ones that already exist.
resource "google_secret_manager_secret_iam_member" "runtime_can_access" {
  for_each = local.secret_grants

  project   = each.value.project
  secret_id = each.value.secret
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.this[each.value.service].email}"
}

resource "google_cloud_run_v2_service" "this" {
  for_each = local.services

  name     = each.value.name
  project  = each.value.project
  location = each.value.region
  ingress  = "INGRESS_TRAFFIC_ALL"

  # google_service_account.this[each.key].email is referenced below, but
  # the grants that make that identity usable aren't - without these, the
  # deploy can race ahead of the IAM propagation it depends on.
  depends_on = [
    google_service_account_iam_member.deployer_can_act_as,
    google_secret_manager_secret_iam_member.runtime_can_access,
  ]

  template {
    service_account = google_service_account.this[each.key].email

    # local.enabled_volumes' type follows Kubernetes' EmptyDirVolumeSource
    # medium enum ("Memory" is the only value Cloud Run's empty_dir volumes
    # currently support, e.g. for a writable /tmp under an otherwise
    # read-only root filesystem); upper() translates it to Cloud Run's own
    # MEMORY medium enum.
    dynamic "volumes" {
      for_each = local.enabled_volumes[each.key]
      content {
        name = volumes.key
        empty_dir {
          medium = upper(volumes.value.type)
        }
      }
    }

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

      dynamic "volume_mounts" {
        for_each = local.enabled_volumes[each.key]
        content {
          name       = volume_mounts.key
          mount_path = volume_mounts.value.mountPath
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
