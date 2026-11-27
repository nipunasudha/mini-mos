variable "project_id" {
  type = string
}

variable "name" {
  description = "Cluster name; also the prefix of the node pool and node service account."
  type        = string
}

variable "location" {
  description = "Zone, e.g. us-east1-b. A zone (not a region) makes a zonal cluster: one control plane, no regional surcharge."
  type        = string
}

variable "network_id" {
  type = string
}

variable "subnet_id" {
  type = string
}

variable "pods_range_name" {
  type = string
}

variable "services_range_name" {
  type = string
}

variable "master_authorized_cidrs" {
  description = "CIDRs allowed to reach the public control-plane endpoint (the operator's IP, e.g. 203.0.113.7/32)."
  type        = list(string)
}

variable "node_count" {
  description = "Initial size of the spot pool. The autoscaler then keeps it between 2 and 4."
  type        = number
  default     = 3
}

variable "machine_type" {
  type    = string
  default = "e2-standard-4"
}

variable "master_ipv4_cidr_block" {
  description = "Private range for the control plane's side of the peering. Must not overlap the VPC ranges."
  type        = string
  default     = "172.16.0.0/28"
}
