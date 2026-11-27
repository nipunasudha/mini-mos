variable "project_id" {
  type = string
}

variable "suffix" {
  description = "Unique per test run (random.UniqueId, lower-cased). Keeps names apart from envs/dev and from other runs."
  type        = string
}

variable "master_authorized_cidrs" {
  description = "CIDRs allowed to reach the API. The live test passes the runner's own IP."
  type        = list(string)
}

variable "github_repo" {
  type    = string
  default = "nipunasudha/mini-mos"
}

variable "region" {
  type    = string
  default = "asia-south1"
}

variable "zone" {
  type    = string
  default = "asia-south1-a"
}
