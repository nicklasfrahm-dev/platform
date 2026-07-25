variable "DEPLOYER_SERVICE_ACCOUNT" {
  description = "Email of the principal running `tofu apply` (see modules/cloudrun-service's variable of the same name). Set via TF_VAR_DEPLOYER_SERVICE_ACCOUNT in CI."
  type        = string
}
