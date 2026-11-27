# Same three modules as envs/dev, but with unique names, a local backend (no
# backend block) and a 2-node pool, so a test run can never touch real state.

provider "google" {
  project = var.project_id
  region  = var.region
}

locals {
  name = "mini-mos-${var.suffix}"
}

module "network" {
  source = "../../../modules/network"

  project_id = var.project_id
  region     = var.region
  name       = local.name
}

module "gke" {
  source = "../../../modules/gke"

  project_id              = var.project_id
  name                    = local.name
  location                = var.zone
  network_id              = module.network.network_id
  subnet_id               = module.network.subnet_id
  pods_range_name         = module.network.pods_range_name
  services_range_name     = module.network.services_range_name
  master_authorized_cidrs = var.master_authorized_cidrs
  node_count              = 2
}

module "registry" {
  source = "../../../modules/registry"

  project_id  = var.project_id
  region      = var.region
  github_repo = var.github_repo
  name_suffix = "-${var.suffix}"
}
