variable "deployer" {
  description = "Email of the principal running `tofu apply` (see modules/cloudrun-service's variable of the same name). Set via TF_VAR_deployer in CI."
  type        = string
}
