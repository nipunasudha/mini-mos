variable "project_id" {
  description = "GCP project that owns the network."
  type        = string
}

variable "region" {
  description = "Region for the subnet, router and NAT."
  type        = string
}

variable "name" {
  description = "Prefix for every resource name. Fixtures pass a unique value so parallel runs never collide."
  type        = string
}
