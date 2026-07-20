# Discover candidate services the same way ArgoCD's ApplicationSet does for
# Kubernetes (see charts/argocd-apps/templates/applicationset.yaml): every
# deploy/clusters/<cluster>/<tenant>/<service>.yml file identifies one
# service. Here, only the ones whose base values opt in via
# platform.target = "GoogleCloudRunService" are kept, and merged the same
# way Argo merges Helm values: 00-base -> 10-env -> the file that identifies
# the service -> injected platform values that always win.
locals {
  candidate_files = fileset(var.clusters_dir, "${var.cluster_glob}/*/*.y*ml")

  candidates = {
    for f in local.candidate_files :
    f => {
      cluster  = split("/", f)[0]
      tenant   = split("/", f)[1]
      service  = trimsuffix(trimsuffix(split("/", f)[2], ".yaml"), ".yml")
      override = yamldecode(file("${var.clusters_dir}/${f}"))
    }
  }

  # 00-base.yml may be absent or empty for services that rely solely on
  # chart defaults, mirroring the ApplicationSet's ignoreMissingValueFiles.
  base_values = {
    for f, candidate in local.candidates :
    f => try(yamldecode(file("${var.values_dir}/${candidate.service}/00-base.yml")), {})
  }

  # Everything not opting in via platform.target is assumed to be
  # Kubernetes-bound and is left for ArgoCD to deploy.
  cloud_run_candidates = {
    for f, candidate in local.candidates :
    f => candidate if try(local.base_values[f].platform.target, "") == "GoogleCloudRunService"
  }

  # Each candidate's cluster follows "<environment>-<location>" (e.g.
  # "prd-cbf01"), the same convention charts/argocd-apps' ApplicationSet
  # derives environment/location from for Kubernetes clusters.
  cluster_parts = {
    for f, candidate in local.cloud_run_candidates :
    f => split("-", candidate.cluster)
  }
  environments = {
    for f, parts in local.cluster_parts :
    f => parts[0]
  }
  locations = {
    for f, parts in local.cluster_parts :
    f => parts[length(parts) - 1]
  }

  # Translates a location code to its GCP region. Extend as new locations
  # come into use.
  region_by_location = {
    cbf01 = "us-central1"
  }

  env_values = {
    for f, candidate in local.cloud_run_candidates :
    f => try(yamldecode(file("${var.values_dir}/${candidate.service}/10-env-${local.environments[f]}.yml")), {})
  }

  platform_values = {
    for f, candidate in local.cloud_run_candidates :
    f => {
      platform = {
        environment = local.environments[f]
        location    = local.locations[f]
        domain      = var.domain
      }
    }
  }

  # merge() only merges top level keys, so every nested map that a service
  # may override (image, service, resources, env, autoscaling, probes,
  # expose) is merged one level deeper on top of the shallow merge to match
  # the schema in charts/service/values.yaml.
  merged_values = {
    for f, candidate in local.cloud_run_candidates :
    f => merge(
      local.base_values[f],
      local.env_values[f],
      candidate.override,
      local.platform_values[f],
      {
        image = merge(
          try(local.base_values[f].image, {}),
          try(local.env_values[f].image, {}),
          try(candidate.override.image, {}),
        )
        service = merge(
          try(local.base_values[f].service, {}),
          try(local.env_values[f].service, {}),
          try(candidate.override.service, {}),
        )
        resources = merge(
          try(local.base_values[f].resources, {}),
          try(local.env_values[f].resources, {}),
          try(candidate.override.resources, {}),
        )
        autoscaling = merge(
          try(local.base_values[f].autoscaling, {}),
          try(local.env_values[f].autoscaling, {}),
          try(candidate.override.autoscaling, {}),
        )
        env = {
          fromLiterals = merge(
            try(local.base_values[f].env.fromLiterals, {}),
            try(local.env_values[f].env.fromLiterals, {}),
            try(candidate.override.env.fromLiterals, {}),
          )
          fromSecrets = merge(
            try(local.base_values[f].env.fromSecrets, {}),
            try(local.env_values[f].env.fromSecrets, {}),
            try(candidate.override.env.fromSecrets, {}),
          )
        }
        probes = {
          startup = merge(
            try(local.base_values[f].probes.startup, {}),
            try(local.env_values[f].probes.startup, {}),
            try(candidate.override.probes.startup, {}),
          )
          liveness = merge(
            try(local.base_values[f].probes.liveness, {}),
            try(local.env_values[f].probes.liveness, {}),
            try(candidate.override.probes.liveness, {}),
          )
        }
        expose = merge(
          try(local.base_values[f].expose, {}),
          try(local.env_values[f].expose, {}),
          try(candidate.override.expose, {}),
          {
            path = merge(
              try(local.base_values[f].expose.path, {}),
              try(local.env_values[f].expose.path, {}),
              try(candidate.override.expose.path, {}),
            )
          }
        )
      }
    )
  }

  # One entry per Cloud Run service to deploy, keyed by the
  # <cluster>/<tenant>/<service>.yml path so that the same service name used
  # by different tenants or clusters can never collide.
  services = {
    for f, candidate in local.cloud_run_candidates :
    f => {
      name    = candidate.service
      project = candidate.tenant
      region  = local.region_by_location[local.locations[f]]
      values  = local.merged_values[f]
    }
  }

  # Reproduces charts/service/templates/httproute.yml's hostname precedence:
  # explicit host, then hostPrefix.domain, then name.location.domain.
  hostnames = {
    for f, service in local.services :
    f => (
      try(service.values.expose.host, "") != "" ? service.values.expose.host :
      try(service.values.expose.hostPrefix, "") != "" ? "${service.values.expose.hostPrefix}.${service.values.platform.domain}" :
      "${service.name}.${service.values.platform.location}.${service.values.platform.domain}"
    )
  }

  exposed_services = {
    for f, service in local.services :
    f => service if try(service.values.expose.enabled, false)
  }
}
