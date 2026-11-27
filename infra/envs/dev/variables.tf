variable "project_id" {
  description = "GCP project to build in."
  type        = string
}

variable "region" {
  type    = string
  default = "asia-south1"
}

variable "zone" {
  type    = string
  default = "asia-south1-a"
}

variable "github_repo" {
  description = "owner/name of the GitHub repo that CI runs in and Argo CD syncs from."
  type        = string
  default     = "nipunasudha/mini-mos"
}

variable "master_authorized_cidrs" {
  description = "CIDRs allowed to reach the cluster API, e.g. [\"203.0.113.7/32\"]. Your public IP, because Terraform itself installs Argo CD through this endpoint."
  type        = list(string)
}

variable "state_bucket" {
  description = "The Terraform state bucket (same one passed to init). gha-tf-plan gets read access to it."
  type        = string
}
