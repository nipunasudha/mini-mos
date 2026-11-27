variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "github_repo" {
  description = "GitHub repository allowed to authenticate, as owner/name."
  type        = string
}

variable "name_suffix" {
  description = "Appended to every name, e.g. -ab12cd. Empty for the real environment. Test fixtures set it: deleted pools and service accounts keep their IDs reserved for 30 days, and a test must not collide with the real registry."
  type        = string
  default     = ""
}

variable "state_bucket" {
  description = "Terraform state bucket that gha-tf-plan may read. Empty skips that grant (used by test fixtures)."
  type        = string
  default     = ""
}
