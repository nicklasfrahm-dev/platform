variable "cluster_glob" {
  description = <<-EOT
    Glob used to select which deploy/clusters/<cluster> directories to scan
    for services (e.g. "prd-*"). Every matched cluster name must follow
    "<environment>-<location>", the same convention used for Kubernetes
    clusters. A matched cluster directory does not need a real Kubernetes
    cluster behind it: it can exist purely to declare Cloud Run services.
  EOT
  type        = string
}

variable "clusters_dir" {
  description = <<-EOT
    Path to the deploy/clusters directory. Every
    "<clusters_dir>/<cluster>/<tenant>/<service>.yml" file found under a
    cluster matching var.cluster_glob is a deploy candidate: the tenant
    directory name is used as the GCP project ID, and the file's content is
    merged in as the highest-precedence values overlay.
  EOT
  type        = string
}

variable "values_dir" {
  description = <<-EOT
    Path to the deploy/services directory. A candidate service is only
    deployed here when its "<values_dir>/<service>/00-base.yml" sets
    platform.target to "GoogleCloudRunService" - everything else is assumed
    to be Kubernetes-bound and is left to ArgoCD. A
    "<values_dir>/<service>/10-env-<environment>.yml" overlay is merged on
    top when present. Values follow the schema of charts/service/values.yaml.
  EOT
  type        = string
}

variable "domain" {
  description = "The base domain used to build a hostname for services that set expose.enabled without an explicit expose.host or expose.hostPrefix."
  type        = string
  default     = ""
}
